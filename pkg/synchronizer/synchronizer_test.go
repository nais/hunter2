package synchronizer_test

import (
	"context"
	"github.com/nais/hunter2/pkg/fake"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/synchronizer"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesFake "k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func TestToSecretData(t *testing.T) {
	principalEmail := "some-principal@domain.test"
	secretName := "some-secret"
	secretVersion := "1"
	timestamp := time.Now()

	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, timestamp)

	namespace := "some-namespace"
	payload := []byte("some-payload")

	secretData := synchronizer.ToSecretData(namespace, msg, payload)

	assert.Equal(t, secretData.Namespace, namespace)
	assert.Equal(t, secretData.Name, secretName)
	assert.Equal(t, secretData.Payload, map[string]string{
		synchronizer.StaticSecretDataKey: string(payload),
	})
	assert.Equal(t, secretData.SecretVersion, secretVersion)
	assert.Equal(t, secretData.LastModified, timestamp)
	assert.Equal(t, secretData.LastModifiedBy, principalEmail)
}

func TestSynchronizer(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	logger := log.NewEntry(log.StandardLogger())

	principalEmail := "some-principal@domain.test"
	secretName := "some-secret"
	secretVersion := "1"
	timestamp := time.Now()

	msg := fake.NewPubSubMessage(principalEmail, secretName, secretVersion, timestamp)

	namespace := "some-namespace"
	payload := []byte("some-payload")

	secretManagerClient := fake.NewSecretManagerClient(payload, nil)
	kubernetesClient := kubernetesFake.NewSimpleClientset()
	syncer := synchronizer.NewSynchronizer(logger, namespace, secretManagerClient, kubernetesClient)
	ctx := context.Background()

	err := syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	// assert that synchronizer has created new secret
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

	// assert non-owned secret is skipped
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
	_, err = kubernetesClient.CoreV1().Secrets(namespace).Create(ctx, nonOwnedSecret, metav1.CreateOptions{})
	assert.NoError(t, err)

	principalEmail = "some-principal@domain.test"
	secretVersion = "1"
	timestamp = time.Now()

	msg = fake.NewPubSubMessage(principalEmail, nonOwnedSecretName, secretVersion, timestamp)
	err = syncer.Sync(ctx, msg)
	assert.Error(t, err)

	secret, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, nonOwnedSecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, nonOwnedSecret, secret)

	// assert that secret is updated
	principalEmail = "some-principal@domain.test"
	secretVersion = "2"
	timestamp = time.Now()

	msg = fake.NewPubSubMessage(principalEmail, secretName, secretVersion, timestamp)
	err = syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	secret, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, secretVersion, secret.GetAnnotations()[kubernetes.SecretVersion])

	// assert that owned secret not found in secret manager is deleted from kubernetes
	secretManagerClient = fake.NewSecretManagerClient(payload, status.Error(codes.NotFound, "secret not found"))
	syncer = synchronizer.NewSynchronizer(logger, namespace, secretManagerClient, kubernetesClient)
	err = syncer.Sync(ctx, msg)
	assert.NoError(t, err)

	_, err = kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}
