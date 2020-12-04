package synchronizer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/nais/hunter2/pkg/google"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/metrics"
	log "github.com/sirupsen/logrus"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
)

const (
	StaticSecretDataKey    = "secret"
	MatchingSecretLabelKey = "sync"
	SecretContainsEnvKey   = "env"
)

type Synchronizer struct {
	logger              *log.Entry
	namespace           string
	secretManagerClient google.SecretManagerClient
	clientset           kubernetes2.Interface
}

func NewSynchronizer(logger *log.Entry, namespace string, secretManagerClient google.SecretManagerClient, clientSet kubernetes2.Interface) *Synchronizer {
	return &Synchronizer{logger: logger, namespace: namespace, secretManagerClient: secretManagerClient, clientset: clientSet}
}

func (in *Synchronizer) Sync(ctx context.Context, msg google.PubSubMessage) error {
	in.logger = in.logger.WithFields(log.Fields{
		"secretName":     msg.GetSecretName(),
		"secretVersion":  msg.GetSecretVersion(),
		"principalEmail": msg.GetPrincipalEmail(),
	})

	if err := in.skipNonOwnedSecrets(ctx, msg); err != nil {
		return err
	}

	in.logger.Debugf("fetching secret metadata for secret: %s", msg.GetSecretName())
	metadata, err := in.secretManagerClient.GetSecretMetadata(ctx, msg.GetSecretName())
	if err == nil {
		if !secretContainsMatchingLabels(metadata) {
			in.logger.Debugf("secret does not contain matching labels, skipping...")
			msg.Ack()
			return nil
		}
	} else {
		if err = in.handleSecretManagerError(err); err != nil {
			return fmt.Errorf("while getting secret manager secret metadata: %w", err)
		}
	}

	in.logger.Debugf("fetching secret data for secret: %s", msg.GetSecretName())
	raw, err := in.secretManagerClient.GetSecretData(ctx, msg.GetSecretName())
	if err != nil {
		if err = in.handleSecretManagerError(err); err != nil {
			return fmt.Errorf("while accessing secret manager secret: %w", err)
		}
		// delete secret if not found in secret manager
		err = in.deleteKubernetesSecret(ctx, msg.GetSecretName())
	} else {
		payload, err := SecretPayload(metadata, raw)
		if err != nil {
			return fmt.Errorf("wrong secret format: %s", err)
		}
		err = in.createOrUpdateKubernetesSecret(ctx, msg, payload)
	}

	if err != nil {
		metrics.Errors.WithLabelValues(metrics.ErrorTypeKubernetesSecretOperation).Inc()
		return fmt.Errorf("while synchronizing k8s secret: %w", err)
	}

	in.logger.Info("successfully processed message, acking")
	metrics.Success.Inc()
	msg.Ack()

	return nil
}

func (in *Synchronizer) skipNonOwnedSecrets(ctx context.Context, msg google.PubSubMessage) error {
	secret, err := in.clientset.CoreV1().Secrets(in.namespace).Get(ctx, msg.GetSecretName(), metav1.GetOptions{})
	switch {
	case err == nil && !kubernetes.IsOwned(*secret):
		msg.Ack()
		metrics.Errors.WithLabelValues(metrics.ErrorTypeNotManaged).Inc()
		return fmt.Errorf("secret %s exists in cluster, but is not managed by hunter2", msg.GetSecretName())
	case err != nil && !errors.IsNotFound(err):
		return fmt.Errorf("error while getting Kubernetes secret %s: %w", msg.GetSecretName(), err)
	default:
		return nil
	}
}

func (in *Synchronizer) handleSecretManagerError(err error) error {
	grpcerr, ok := status.FromError(err)
	if ok && grpcerr.Code() == codes.NotFound {
		// continue if not found in secret manager
		return nil
	}
	// unhandled errors - return without acking; pubsub will retry message until acked
	metrics.Errors.WithLabelValues(metrics.ErrorTypeSecretManagerAccess).Inc()
	return fmt.Errorf("error while performing secret manager operation: %w", err)
}

func (in *Synchronizer) createOrUpdateKubernetesSecret(ctx context.Context, msg google.PubSubMessage, payload map[string]string) error {
	secret := kubernetes.OpaqueSecret(ToSecretData(in.namespace, msg, payload))
	in.logger.Debugf("creating/updating k8s secret '%s'", msg.GetSecretName())

	_, err := in.clientset.CoreV1().Secrets(in.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && errors.IsAlreadyExists(err) {
		_, err = in.clientset.CoreV1().Secrets(in.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}
	return err
}

func (in *Synchronizer) deleteKubernetesSecret(ctx context.Context, name string) error {
	in.logger.Debugf("deleting k8s secret '%s'", name)
	err := in.clientset.CoreV1().Secrets(in.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}
	return err
}

func ToSecretData(namespace string, msg google.PubSubMessage, payload map[string]string) kubernetes.SecretData {
	return kubernetes.SecretData{
		Name:           msg.GetSecretName(),
		Namespace:      namespace,
		LastModified:   msg.GetTimestamp(),
		LastModifiedBy: msg.GetPrincipalEmail(),
		SecretVersion:  msg.GetSecretVersion(),
		Payload:        payload,
	}
}

func parseSecretEnvironmentVariables(data string) (map[string]string, error) {
	env := make(map[string]string)
	lines := strings.Split(data, "\n")
	for n, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' { // remove empty lines and comments
			continue
		}
		keyval := strings.SplitN(line, "=", 2)
		if len(keyval) != 2 {
			return nil, fmt.Errorf("wrong environment variable format; expected KEY=VALUE")
		}
		key := keyval[0]
		val := keyval[1]
		if _, ok := env[key]; ok {
			return nil, fmt.Errorf("duplicate environment variable on line %d", n+1)
		}
		env[key] = val
	}
	return env, nil
}

func SecretPayload(metadata *secretmanagerpb.Secret, raw []byte) (map[string]string, error) {
	if secretContainsEnvironmentVariables(metadata) {
		return parseSecretEnvironmentVariables(string(raw))
	} else {
		return map[string]string{
			StaticSecretDataKey: string(raw),
		}, nil
	}
}

func secretLabelEnabled(metadata *secretmanagerpb.Secret, key string) bool {
	val, ok := metadata.Labels[key]
	enabled, _ := strconv.ParseBool(val)
	return ok && enabled
}

func secretContainsMatchingLabels(metadata *secretmanagerpb.Secret) bool {
	return secretLabelEnabled(metadata, MatchingSecretLabelKey)
}

func secretContainsEnvironmentVariables(metadata *secretmanagerpb.Secret) bool {
	return secretLabelEnabled(metadata, SecretContainsEnvKey)
}
