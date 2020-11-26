package synchronizer

import (
	"context"
	"fmt"

	"github.com/nais/hunter2/pkg/google"
	"github.com/nais/hunter2/pkg/kubernetes"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
)

func Sync(ctx context.Context, logger *log.Entry, msg google.PubSubMessage, namespace string, secretManagerClient *google.SecretManagerClient, clientSet kubernetes2.Interface) error {
	logger.Debugf("got message: %s", msg.Data)

	logger.Debugf("fetching secret data for secret: %s", msg.SecretName)
	payload, err := secretManagerClient.GetSecretData(ctx, msg.SecretName)
	if err != nil {
		return fmt.Errorf("error while accessing secret manager secret: %v", err)
	}

	logger.Debugf("creating k8s secret '%s'", msg.SecretName)
	secret := kubernetes.OpaqueSecret(msg.SecretName, namespace, map[string]string{
		msg.SecretName: string(payload),
	})
	_, err = clientSet.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = clientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("error while creating or updating k8s secret: %v", err)
	}

	logger.Debugf("processed message ok, acking")
	msg.Ack()

	return nil
}
