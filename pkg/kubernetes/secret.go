package kubernetes

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const createdBy = "nais.io/created-by"
const createdByValue = "hunter2"

const lastModifiedBy = "hunter2.nais.io/last-modified-by"
const lastModified = "hunter2.nais.io/last-modified"
const secretVersion = "hunter2.nais.io/secret-version"

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
	return labels != nil && labels[createdBy] == createdByValue
}

func OpaqueSecret(data SecretData) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      data.Name,
			Namespace: data.Namespace,
			Labels: map[string]string{
				createdBy: createdByValue,
			},
			Annotations: map[string]string{
				lastModified:   data.LastModified.Format(time.RFC3339),
				lastModifiedBy: data.LastModifiedBy,
				secretVersion:  data.SecretVersion,
			},
		},
		StringData: data.Payload,
		Type:       corev1.SecretTypeOpaque,
	}
}
