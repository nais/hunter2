package synchronizer

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
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
	"strconv"
)

const (
	StaticSecretDataKey    = "secret"
	MatchingSecretLabelKey = "sync"
	SecretContainsEnvKey   = "env"
	ProjectIDAnnotation    = "cnrm.cloud.google.com/project-id"
)

type Synchronizer struct {
	logger                *log.Entry
	secretManagerClient   google.SecretManagerClient
	clientset             kubernetes2.Interface
	projectNamespaceCache map[string]string
}

func NewSynchronizer(logger *log.Entry, secretManagerClient google.SecretManagerClient, clientSet kubernetes2.Interface, projectNamespaceCache map[string]string) *Synchronizer {
	if projectNamespaceCache == nil {
		projectNamespaceCache = make(map[string]string)
	}

	return &Synchronizer{
		logger:                logger,
		secretManagerClient:   secretManagerClient,
		clientset:             clientSet,
		projectNamespaceCache: projectNamespaceCache,
	}
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
	namespace, err := in.getNamespaceFromProjectID(ctx, msg.GetProjectID())
	if err != nil {
		return fmt.Errorf("getting namespace: %+v", err)
	}
	secret, err := in.clientset.CoreV1().Secrets(namespace).Get(ctx, msg.GetSecretName(), metav1.GetOptions{})
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
	namespace, err := in.getNamespaceFromProjectID(ctx, msg.GetProjectID())
	if err != nil {
		return fmt.Errorf("getting namespace: %+v", err)
	}
	secret := kubernetes.OpaqueSecret(ToSecretData(msg, namespace, payload))
	in.logger.Debugf("creating/updating k8s secret '%s'", msg.GetSecretName())

	_, err = in.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && errors.IsAlreadyExists(err) {
		_, err = in.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationUpdate, metrics.ErrorStatus(err, metrics.StatusError))
		return err
	}

	metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationCreate, metrics.ErrorStatus(err, metrics.StatusError))
	return err
}

func (in *Synchronizer) deleteKubernetesSecret(ctx context.Context, msg google.PubSubMessage) error {
	namespace, err := in.getNamespaceFromProjectID(ctx, msg.GetProjectID())
	if err != nil {
		return fmt.Errorf("getting namespace: %+v", err)
	}
	in.logger.Debugf("deleting k8s secret '%s'", msg.GetSecretName())
	err = in.clientset.CoreV1().Secrets(namespace).Delete(ctx, msg.GetSecretName(), metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}

	metrics.LogRequest(metrics.SystemKubernetes, metrics.OperationDelete, metrics.ErrorStatus(err, metrics.StatusError))

	return err
}

func (in *Synchronizer) getNamespaceFromProjectID(ctx context.Context, projectID string) (string, error) {
	if namespace, ok := in.projectNamespaceCache[projectID]; ok {
		return namespace, nil
	}

	log.Infof("cache miss for project id: %s, updating cache", projectID)

	namespaces, err := in.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing namespaces: %+v", err)
	}

	for _, namespace := range namespaces.Items {
		if projectID, ok := namespace.Annotations[ProjectIDAnnotation]; ok {
			in.projectNamespaceCache[projectID] = namespace.Name
			log.Debugf("caching: %s=%s", projectID, namespace.Name)
		}
	}

	if namespace, ok := in.projectNamespaceCache[projectID]; ok {
		return namespace, nil
	} else {
		return "", fmt.Errorf("no namespace found for project ID: %s", projectID)
	}
}

func ToSecretData(msg google.PubSubMessage, namespace string, payload map[string]string) kubernetes.SecretData {
	return kubernetes.SecretData{
		Name:           msg.GetSecretName(),
		Namespace:      namespace,
		LastModified:   msg.GetTimestamp(),
		LastModifiedBy: msg.GetPrincipalEmail(),
		SecretVersion:  msg.GetSecretVersion(),
		Payload:        payload,
	}
}

func SecretPayload(metadata *secretmanagerpb.Secret, raw []byte) (map[string]string, error) {
	if secretContainsEnvironmentVariables(metadata) {
		return godotenv.Unmarshal(string(raw))
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
