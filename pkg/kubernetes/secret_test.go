package kubernetes_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/nais/hunter2/pkg/kubernetes"
)

var secretData = kubernetes.SecretData{
	Name:      "some-secret",
	Namespace: "some-namespace",
	Payload: map[string][]byte{
		"some-key": []byte("some-value"),
	},
	LastModified:   time.Now(),
	LastModifiedBy: "person@some-domain.test",
	SecretVersion:  "1",
}

func TestOpaqueSecret(t *testing.T) {
	secret := kubernetes.OpaqueSecret(secretData)

	assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	assert.Equal(t, "Secret", secret.TypeMeta.Kind)
	assert.Equal(t, "v1", secret.TypeMeta.APIVersion)
	assert.Equal(t, secretData.Name, secret.GetName())
	assert.Equal(t, secretData.Namespace, secret.GetNamespace())
	assert.Equal(t, map[string]string{
		kubernetes.CreatedBy: kubernetes.CreatedByValue,
	}, secret.GetLabels())
	assert.Equal(t, map[string]string{
		kubernetes.LastModified:        secretData.LastModified.Format(time.RFC3339),
		kubernetes.LastModifiedBy:      secretData.LastModifiedBy,
		kubernetes.SecretVersion:       secretData.SecretVersion,
		kubernetes.StakaterReloaderKey: "true",
	}, secret.GetAnnotations())
	assert.Equal(t, secretData.Payload, secret.Data)

	secretDataUppercase := secretData
	secretDataUppercase.Name = "Some-Name-With-UpperCase"
	secret = kubernetes.OpaqueSecret(secretDataUppercase)
	assert.Equal(t, "some-name-with-uppercase", secret.Name)
}

func TestIsOwned(t *testing.T) {
	ownedSecret := kubernetes.OpaqueSecret(secretData)
	nonOwnedSecret := ownedSecret.DeepCopy()
	nonOwnedSecret.SetLabels(map[string]string{})

	assert.True(t, kubernetes.IsOwned(*ownedSecret))
	assert.False(t, kubernetes.IsOwned(*nonOwnedSecret))
}
