package synchronizer

import (
	"context"
	"fmt"

	"github.com/nais/hunter2/pkg/google"
	"github.com/nais/hunter2/pkg/kubernetes"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
)

const staticSecretDataKey = "secret"

func createOrUpdateSecret(ctx context.Context, name, namespace string, payload []byte, clientSet kubernetes2.Interface) error {
	secret := kubernetes.OpaqueSecret(name, namespace, map[string]string{
		staticSecretDataKey: string(payload),
	})
	_, err := clientSet.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = clientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}

	return err
}

func deleteSecret(ctx context.Context, name, namespace string, clientset kubernetes2.Interface) error {
	err := clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func Sync(ctx context.Context, logger *log.Entry, msg google.PubSubMessage, namespace string, secretManagerClient *google.SecretManagerClient, clientSet kubernetes2.Interface) error {
	var notexist bool

	logger.Debugf("got message: %s", msg.Data)

	logger.Debugf("fetching secret data for secret: %s", msg.SecretName)
	payload, err := secretManagerClient.GetSecretData(ctx, msg.SecretName)
	if err != nil {
		grpcerr, ok := status.FromError(err)
		if !ok || grpcerr.Code() != codes.NotFound {
			return fmt.Errorf("error while accessing secret manager secret: %v", err)
		}
		notexist = true
	}

	logger.Debugf("synchronizing k8s secret '%s'", msg.SecretName)
	if notexist {
		err = deleteSecret(ctx, msg.SecretName, namespace, clientSet)
	} else {
		err = createOrUpdateSecret(ctx, msg.SecretName, namespace, payload, clientSet)
	}

	if err != nil {
		return fmt.Errorf("error while synchronizing k8s secret: %v", err)
	}

	logger.Debugf("processed message ok, acking")

	msg.Ack()

	return nil
}
