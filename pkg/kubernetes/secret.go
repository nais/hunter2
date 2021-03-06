package kubernetes

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const CreatedBy = "nais.io/created-by"
const CreatedByValue = "hunter2"

const LastModifiedBy = "hunter2.nais.io/last-modified-by"
const LastModified = "hunter2.nais.io/last-modified"
const SecretVersion = "hunter2.nais.io/secret-version"

type SecretData struct {
	Name           string
	Namespace      string
	Payload        map[string]string
	LastModified   time.Time
	LastModifiedBy string
	SecretVersion  string
}

func IsOwned(secret corev1.Secret) bool {
	labels := secret.GetLabels()
	return labels != nil && labels[CreatedBy] == CreatedByValue
}

func OpaqueSecret(data SecretData) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(data.Name),
			Namespace: data.Namespace,
			Labels: map[string]string{
				CreatedBy: CreatedByValue,
			},
			Annotations: map[string]string{
				LastModified:   data.LastModified.Format(time.RFC3339),
				LastModifiedBy: data.LastModifiedBy,
				SecretVersion:  data.SecretVersion,
			},
		},
		StringData: data.Payload,
		Type:       corev1.SecretTypeOpaque,
	}
}
