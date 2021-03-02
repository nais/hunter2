package synchronizer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesFake "k8s.io/client-go/kubernetes/fake"

	"github.com/nais/hunter2/pkg/fake"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/synchronizer"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var (
	logger              = log.NewEntry(log.StandardLogger())
	kubernetesClient    = kubernetesFake.NewSimpleClientset()
	projectID           = "12345678"
	namespace           = "some-namespace"
	principalEmail      = "some-principal@domain.test"
	secretName          = "some-secret"
	secretVersion       = "1"
	timestamp           = time.Now()
	ctx                 = context.Background()
	genericPayload      = []byte("some-payload")
	envPayload          = []byte("FOO=BAR\nBAR=BAZ\n  # comment\n\n\n")
	envMultilinePayload = []byte("FOO=BAR\nBAR\n\\BAR=BAZ\n  \\#comment\n\n\n")
	metadata            = &secretmanagerpb.Secret{
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
	metadataWithMultilineEnv = &secretmanagerpb.Secret{
		Name: secretName,
		Labels: map[string]string{
			"sync":      "true",
			"multiline": "true",
		},
	}
)

func TestToSecretData(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, namespace, projectID, timestamp)
	payload, err := synchronizer.SecretPayload(metadata, genericPayload)
	assert.NoError(t, err)

	secretData := synchronizer.ToSecretData(msg, payload)

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

func TestToSecretDataWithMultilineEnv(t *testing.T) {
	payload, err := synchronizer.SecretPayload(metadataWithMultilineEnv, envMultilinePayload)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR\nBAR",
		"BAR": "BAZ",
	}, payload)
}

func TestSynchronizer_Sync_CreateNewSecret(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, namespace, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient)

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
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient)
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, namespace, projectID, timestamp)

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

	msg := fake.NewPubSubMessage(principalEmail, nonOwnedSecretName, secretVersion, namespace, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient)

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

	msg := fake.NewPubSubMessage(principalEmail, nonMatchingSecretName, secretVersion, namespace, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, nil)
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	_, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, nonMatchingSecretName, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestSynchronizer_Sync_DeleteNotFoundSecret(t *testing.T) {
	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, namespace, projectID, timestamp)
	secretManagerClient := fake.NewSecretManagerClient(genericPayload, metadata, status.Error(codes.NotFound, "secret not found"))
	syncer := synchronizer.NewSynchronizer(logger, secretManagerClient, kubernetesClient)

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	_, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestParseSecretEnvironmentVariables(t *testing.T) {
	validMetadata := "KEY.VALUE=VALUE\nKEY-VALUE=VALUE\nKEY_VALUE=VALUE\nKEY0VALUE=VALUE\nkey_VALUE.s=VALUE"
	result, _ := synchronizer.ParseSecretEnvironmentVariables(validMetadata)
	lines := strings.Split(validMetadata, "\n")

	assert.True(t, len(lines) == len(result))
	assert.Equal(t, "VALUE", result["KEY.VALUE"])
	assert.Equal(t, "VALUE", result["KEY-VALUE"])
	assert.Equal(t, "VALUE", result["KEY_VALUE"])
	assert.Equal(t, "VALUE", result["KEY0VALUE"])
	assert.Equal(t, "VALUE", result["key_VALUE.s"])

	noneValidMetadata := "KEY$VALUE=VALUE"
	result, err := synchronizer.ParseSecretEnvironmentVariables(noneValidMetadata)
	expectedErrorMsg := "pattern: '^[a-zA-Z0-9-_.]+$' do not match for environment key: KEY$VALUE"
	assert.EqualErrorf(t, err, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
	assert.True(t, len(result) == 0)
}

func TestParsMultiLineEnvironmentVariables(t *testing.T) {
	validMetadata := "FIRST_KEY=-----BEGIN RSA PRIVATE KEY-----\nMIIEsomekey\n-----END RSA PRIVATE KEY-----\n\\\nOTHER_KEY=VALUE"
	result, _ := synchronizer.ParsMultiLineEnvironmentVariables(validMetadata)
	assert.True(t, len(result) == 2)
	assert.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nMIIEsomekey\n-----END RSA PRIVATE KEY-----", result["FIRST_KEY"])
	assert.Equal(t, "VALUE", result["OTHER_KEY"])

	validMetadataWithOnlyOneKey := "FIRST_KEY=-----BEGIN RSA PRIVATE KEY-----\nMIIEsomekey\n-----END RSA PRIVATE KEY-----\n"
	result, _ = synchronizer.ParsMultiLineEnvironmentVariables(validMetadataWithOnlyOneKey)
	assert.True(t, len(result) == 1)
	assert.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nMIIEsomekey\n-----END RSA PRIVATE KEY-----", result["FIRST_KEY"])
}
