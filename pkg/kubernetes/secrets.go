package kubernetes
import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO
func OpaqueSecret(payload map[string]string) corev1.Secret {
	return corev1.Secret{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Immutable:  nil,
		Data:       nil,
		StringData: nil,
		Type:       "",
	}
}
