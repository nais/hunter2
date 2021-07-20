package synchronizer_test

import (
	"context"
	"testing"
	"time"

	"github.com/nais/hunter2/pkg/fake"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/synchronizer"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesFake "k8s.io/client-go/kubernetes/fake"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var (
	logger           = log.NewEntry(log.StandardLogger())
	kubernetesClient = kubernetesFake.NewSimpleClientset()
	projectID        = "12345678"
	namespace        = "some-namespace"
	principalEmail   = "some-principal@domain.test"
	secretName       = "some-secret"
	secretVersion    = "1"
	timestamp        = time.Now()
	ctx              = context.Background()
	genericPayload   = []byte("some-payload")
	envPayload       = []byte("FOO=BAR\nBAR=BAZ\n  # comment\n\n\n")
	cache            = map[string]string{"12345678": namespace}
	metadata         = &secretmanagerpb.Secret{
		Name: secretName,
		Labels: map[string]string{
			"sync": "true",
		},
	}
	metadataWithEnv = &secretmanagerpb.Secret{
		Name: secretName,
		Labels: map[string]string{
			"sync": "true",
			"env":  "true",
		},
	}
)

func TestToSecretData(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, projectID, timestamp)
	payload, err := synchronizer.SecretPayload(metadata, genericPayload)
	assert.NoError(t, err)

	secretData := synchronizer.ToSecretData(msg, namespace, payload)

	assert.Equal(t, secretData.Namespace, namespace)
	assert.Equal(t, secretData.Name, secretName)
	assert.Equal(t, secretData.Payload, payload)
	assert.Equal(t, secretData.SecretVersion, secretVersion)
	assert.Equal(t, secretData.LastModified, timestamp)
	assert.Equal(t, secretData.LastModifiedBy, principalEmail)
}

func TestToSecretDataWithEnv(t *testing.T) {
	payload, err := synchronizer.SecretPayload(metadataWithEnv, envPayload)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR",
		"BAR": "BAZ",
	}, payload)
}

func TestToEnvMultilineEncoded(t *testing.T) {
	envPayload = []byte("FOO=\"one line\\ntwo line\\n\"\nBAR=BAZ")
	payload, err := synchronizer.SecretPayload(metadataWithEnv, envPayload)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "one line\ntwo line\n",
		"BAR": "BAZ",
	}, payload)

}

func TestSynchronizer_Sync_CreateNewSecret(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient, cache)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	secret, err := kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, secretName, secret.GetName())
	assert.Equal(t, namespace, secret.GetNamespace())
	assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	assert.Equal(t, map[string]string{
		kubernetes.CreatedBy: kubernetes.CreatedByValue,
	}, secret.GetLabels())
	assert.Equal(t, map[string]string{
		kubernetes.LastModified:   timestamp.Format(time.RFC3339),
		kubernetes.LastModifiedBy: principalEmail,
		kubernetes.SecretVersion:  secretVersion,
	}, secret.GetAnnotations())
}

func TestSynchronizer_Sync_UpdateExistingSecret(t *testing.T) {
	secretVersion = "2"

	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient, cache)
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, projectID, timestamp)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	secret, err := kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, secretVersion, secret.GetAnnotations()[kubernetes.SecretVersion])
}

func TestSynchronizer_Sync_SkipNonOwnedSecret(t *testing.T) {
	nonOwnedSecretName := "non-owned"
	nonOwnedSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonOwnedSecretName,
			Namespace: namespace,
		},
		StringData: nil,
		Type:       corev1.SecretTypeOpaque,
	}
	_, err := kubernetesClient.CoreV1().Secrets(namespace).Create(ctx, nonOwnedSecret, metav1.CreateOptions{})
	assert.NoError(t, err)

	msg := fake.NewPubSubMessage(principalEmail, nonOwnedSecretName, secretVersion, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient, cache)

	err = syncer.Sync(ctx, msg)
	assert.Error(t, err)

	secret, err := kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, nonOwnedSecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, nonOwnedSecret, secret)
}

func TestSynchronizer_Sync_SkipNonMatchingLabels(t *testing.T) {
	nonMatchingSecretName := "non-matching-secret"
	nonMatchingMetadata := metadata
	nonMatchingMetadata.Labels = map[string]string{"some-key": "some-value"}

	msg := fake.NewPubSubMessage(principalEmail, nonMatchingSecretName, secretVersion, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient, cache)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	_, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, nonMatchingSecretName, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestSynchronizer_Sync_DeleteNotFoundSecret(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, status.Error(codes.NotFound, "secret not found"))
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient, cache)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	_, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}
