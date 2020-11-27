package synchronizer

import (
	"context"
	"fmt"
	"github.com/nais/hunter2/pkg/metrics"

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

type Synchronizer struct {
	logger              *log.Entry
	namespace           string
	secretManagerClient *google.SecretManagerClient
	clientset           kubernetes2.Interface
}

func NewSynchronizer(logger *log.Entry, namespace string, secretManagerClient *google.SecretManagerClient, clientSet kubernetes2.Interface) *Synchronizer {
	return &Synchronizer{logger: logger, namespace: namespace, secretManagerClient: secretManagerClient, clientset: clientSet}
}

func (in *Synchronizer) Sync(ctx context.Context, msg google.PubSubMessage) error {
	var notexist bool

	in.logger.Debugf("fetching secret data for secret: %s", msg.SecretName)

	payload, err := in.secretManagerClient.GetSecretData(ctx, msg.SecretName)
	if err != nil {
		grpcerr, ok := status.FromError(err)
		if !ok || grpcerr.Code() != codes.NotFound {
			return fmt.Errorf("error while accessing secret manager secret: %v", err)
		}
		notexist = true
	}

	secret, err := in.clientset.CoreV1().Secrets(in.namespace).Get(ctx, msg.SecretName, metav1.GetOptions{})
	if err == nil && !kubernetes.IsOwned(*secret) {
		msg.Ack()
		return fmt.Errorf("secret exists in cluster, but is not managed by hunter2")
	}

	if notexist {
		err = in.deleteSecret(ctx, msg.SecretName)
	} else {
		err = in.createOrUpdateSecret(ctx, msg, payload)
	}

	if err != nil {
		return fmt.Errorf("error while synchronizing k8s secret: %v", err)
	}

	in.logger.Debugf("processed message ok, acking")
	metrics.Success.Inc()
	msg.Ack()

	return nil
}

func (in *Synchronizer) createOrUpdateSecret(ctx context.Context, msg google.PubSubMessage, payload []byte) error {
	data := kubernetes.SecretData{
		Name:           msg.SecretName,
		Namespace:      in.namespace,
		LastModified:   msg.LogMessage.Timestamp,
		LastModifiedBy: msg.LogMessage.ProtoPayload.AuthenticationInfo.PrincipalEmail,
		SecretVersion:  google.ParseSecretVersion(msg.LogMessage.ProtoPayload.ResourceName),
		Payload: map[string]string{
			staticSecretDataKey: string(payload),
		},
	}
	secret := kubernetes.OpaqueSecret(data)

	in.logger.Debugf("creating/updating k8s secret '%s'", msg.SecretName)

	_, err := in.clientset.CoreV1().Secrets(in.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = in.clientset.CoreV1().Secrets(in.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}

	return err
}

func (in *Synchronizer) deleteSecret(ctx context.Context, name string) error {
	in.logger.Debugf("deleting k8s secret '%s'", name)
	err := in.clientset.CoreV1().Secrets(in.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}
