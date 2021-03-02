package synchronizer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nais/hunter2/pkg/google"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/metrics"
	log "github.com/sirupsen/logrus"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
)

const (
	StaticSecretDataKey    = "secret"
	MatchingSecretLabelKey = "sync"
	SecretContainsEnvKey   = "env"
	MultilineSecretLabel   = "multiline"
	Pattern                = `^[a-zA-Z0-9-_.]+$`
)

type Synchronizer struct {
	logger              *log.Entry
	secretManagerClient google.SecretManagerClient
	clientset           kubernetes2.Interface
}

func NewSynchronizer(logger *log.Entry, secretManagerClient google.SecretManagerClient, clientSet kubernetes2.Interface) *Synchronizer {
	return &Synchronizer{logger: logger, secretManagerClient: secretManagerClient, clientset: clientSet}
}

func (in *Synchronizer) ManagedSecrets(ctx context.Context) ([]corev1.Secret, error) {
	labelSelector := fmt.Sprintf("%s=%s", kubernetes.CreatedBy, kubernetes.CreatedByValue)

	secrets, err := in.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("listing managed secrets: %v", err)
	}

	return secrets.Items, nil
}

func (in *Synchronizer) Sync(ctx context.Context, msg google.PubSubMessage) error {
	in.logger = in.logger.WithFields(log.Fields{
		"secretName":     msg.GetSecretName(),
		"secretVersion":  msg.GetSecretVersion(),
		"principalEmail": msg.GetPrincipalEmail(),
		"projectID":      msg.GetProjectID(),
		"namespace":      msg.GetNamespace(),
	})

	if err := in.skipNonOwnedSecrets(ctx, msg); err != nil {
		return err
	}

	in.logger.Debugf("fetching secret metadata for secret: %s", msg.GetSecretName())
	metadata, err := in.secretManagerClient.GetSecretMetadata(ctx, msg.GetProjectID(), msg.GetSecretName())
	if err == nil {
		if !secretContainsMatchingLabels(metadata) {
			metrics.LogRequest(metrics.SystemSecretManager, metrics.OperationRead, metrics.StatusNoSyncLabel)
			in.logger.Debugf("secret does not contain matching labels, skipping...")
			msg.Ack()
			return nil
		}
	} else {
		if err = in.ignoreNotFound(err); err != nil {
			metrics.LogRequest(metrics.SystemSecretManager, metrics.OperationRead, metrics.StatusError)
			return fmt.Errorf("while getting secret manager secret metadata: %w", err)
		}
	}

	in.logger.Debugf("fetching secret data for secret: %s", msg.GetSecretName())
	raw, err := in.secretManagerClient.GetSecretData(ctx, msg.GetProjectID(), msg.GetSecretName())
	if err != nil {
		if err = in.ignoreNotFound(err); err != nil {
			metrics.LogRequest(metrics.SystemSecretManager, metrics.OperationRead, metrics.StatusError)
			return fmt.Errorf("while accessing secret manager secret: %w", err)
		}
		// delete secret if not found in secret manager
		err = in.deleteKubernetesSecret(ctx, msg)
	} else {
		payload, err := SecretPayload(metadata, raw)
		metrics.LogRequest(metrics.SystemSecretManager, metrics.OperationRead, metrics.ErrorStatus(err, metrics.StatusInvalidData))
		if err != nil {
			return fmt.Errorf("wrong secret format: %s", err)
		}
		err = in.createOrUpdateKubernetesSecret(ctx, msg, payload)
	}

	if err != nil {
		return fmt.Errorf("while synchronizing k8s secret: %w", err)
	}

	in.logger.Info("successfully processed message, acking")
	msg.Ack()

	return nil
}

func (in *Synchronizer) skipNonOwnedSecrets(ctx context.Context, msg google.PubSubMessage) error {
	secret, err := in.clientset.CoreV1().Secrets(msg.GetNamespace()).Get(ctx, msg.GetSecretName(), metav1.GetOptions{})
	switch {
	case err == nil && !kubernetes.IsOwned(*secret):
		msg.Ack()
		metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationRead, metrics.StatusNotManaged)
		return fmt.Errorf("secret %s exists in cluster, but is not managed by hunter2", msg.GetSecretName())
	case err != nil && !errors.IsNotFound(err):
		metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationRead, metrics.StatusError)
		return fmt.Errorf("error while getting Kubernetes secret %s: %w", msg.GetSecretName(), err)
	default:
		return nil
	}
}

func (in *Synchronizer) ignoreNotFound(err error) error {
	grpcerr, ok := status.FromError(err)
	if ok && grpcerr.Code() == codes.NotFound {
		// continue if not found in secret manager
		return nil
	}
	// unhandled errors - return without acking; pubsub will retry message until acked
	return fmt.Errorf("error while performing secret manager operation: %w", err)
}

func (in *Synchronizer) createOrUpdateKubernetesSecret(ctx context.Context, msg google.PubSubMessage, payload map[string]string) error {
	secret := kubernetes.OpaqueSecret(ToSecretData(msg, payload))
	in.logger.Debugf("creating/updating k8s secret '%s'", msg.GetSecretName())

	_, err := in.clientset.CoreV1().Secrets(msg.GetNamespace()).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && errors.IsAlreadyExists(err) {
		_, err = in.clientset.CoreV1().Secrets(msg.GetNamespace()).Update(ctx, secret, metav1.UpdateOptions{})
		metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationUpdate, metrics.ErrorStatus(err, metrics.StatusError))
		return err
	}

	metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationCreate, metrics.ErrorStatus(err, metrics.StatusError))
	return err
}

func (in *Synchronizer) deleteKubernetesSecret(ctx context.Context, msg google.PubSubMessage) error {
	in.logger.Debugf("deleting k8s secret '%s'", msg.GetSecretName())
	err := in.clientset.CoreV1().Secrets(msg.GetNamespace()).Delete(ctx, msg.GetSecretName(), metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}

	metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationDelete, metrics.ErrorStatus(err, metrics.StatusError))

	return err
}

func ToSecretData(msg google.PubSubMessage, payload map[string]string) kubernetes.SecretData {
	return kubernetes.SecretData{
		Name:           msg.GetSecretName(),
		Namespace:      msg.GetNamespace(),
		LastModified:   msg.GetTimestamp(),
		LastModifiedBy: msg.GetPrincipalEmail(),
		SecretVersion:  msg.GetSecretVersion(),
		Payload:        payload,
	}
}

func ParseSecretEnvironmentVariables(data string) (map[string]string, error) {
	lines := strings.Split(data, "\n")
	return ParseSecrets(lines)
}

// multiline: true
func ParsMultiLineEnvironmentVariables(data string) (map[string]string, error) {
	lines := strings.Split(data, "&")
	return ParseSecrets(lines)
}

func ParseSecrets(lines []string) (map[string]string, error) {
	env := make(map[string]string)
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
		validKey, _ := regexp.MatchString(Pattern, key)
		if !validKey {
			return nil, fmt.Errorf("pattern: '%s' do not match for environment key: %s", Pattern, key)
		}
		if _, ok := env[key]; ok {
			return nil, fmt.Errorf("duplicate environment variable on line %d", n+1)
		}
		env[key] = val
	}
	return env, nil
}

func SecretPayload(metadata *secretmanagerpb.Secret, raw []byte) (map[string]string, error) {
	if secretContainsEnvironmentVariables(metadata) {
		return ParseSecretEnvironmentVariables(string(raw))
	} else if secretContainsMultiLineEnvironmentVariables(metadata) {
		return ParsMultiLineEnvironmentVariables(string(raw))
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

func secretContainsMultiLineEnvironmentVariables(metadata *secretmanagerpb.Secret) bool {
	return secretLabelEnabled(metadata, MultilineSecretLabel)
}
