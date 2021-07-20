package google

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/nais/hunter2/pkg/metrics"
	log "github.com/sirupsen/logrus"
)

type PubSubClient struct {
	*pubsub.Subscription
}

type PubSubMessage interface {
	Ack()
	GetPrincipalEmail() string
	GetProjectID() string
	GetSecretName() string
	GetSecretVersion() string
	GetTimestamp() time.Time
}

type pubSubMessage struct {
	ProjectID  string
	SecretName string
	LogMessage logMessage
	pubsub.Message
}

func (p *pubSubMessage) GetPrincipalEmail() string {
	return p.LogMessage.ProtoPayload.AuthenticationInfo.PrincipalEmail
}

func (p *pubSubMessage) GetProjectID() string {
	return p.ProjectID
}

func (p *pubSubMessage) GetSecretName() string {
	return p.SecretName
}

func (p *pubSubMessage) GetSecretVersion() string {
	return ParseSecretVersion(p.LogMessage.ProtoPayload.ResourceName)
}

func (p *pubSubMessage) GetTimestamp() time.Time {
	return p.LogMessage.Timestamp
}

type logMessage struct {
	Timestamp    time.Time `json:"timestamp"`
	ProtoPayload struct {
		ResourceName       string `json:"resourceName"`
		AuthenticationInfo struct {
			PrincipalEmail string `json:"principalEmail"`
		} `json:"authenticationInfo"`
	} `json:"protoPayload"`
	Resource struct {
		Labels struct {
			ProjectID string `json:"project_id"`
		} `json:"labels"`
	} `json:"resource"`
}

func NewPubSubClient(ctx context.Context, projectID, subscriptionID string) (*PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	sub := client.Subscription(subscriptionID)
	return &PubSubClient{Subscription: sub}, nil
}

func ParseSecretName(resourceName string) (string, error) {
	tokens := strings.Split(resourceName, "/")
	if len(tokens) < 4 || tokens[0] != "projects" || tokens[2] != "secrets" {
		return "", fmt.Errorf("resource name does not contain a secret")
	}
	return tokens[3], nil
}

func ParseSecretVersion(resourceName string) string {
	tokens := strings.Split(resourceName, "/")
	if len(tokens) < 6 || tokens[0] != "projects" || tokens[2] != "secrets" {
		return "1"
	}
	return tokens[5]
}

func ParseProjectID(resourceName string) (string, error) {
	tokens := strings.Split(resourceName, "/")
	if len(tokens) < 4 || tokens[0] != "projects" || tokens[2] != "secrets" {
		return "", fmt.Errorf("resource name does not contain a secret")
	}
	return tokens[1], nil
}

func (in *PubSubClient) Consume(ctx context.Context) chan PubSubMessage {
	messages := make(chan PubSubMessage)

	go func(ctx context.Context, messages chan PubSubMessage) {
		defer close(messages)
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		err := in.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
			var logMessage logMessage
			var secretName string
			var projectID string

			err := json.Unmarshal(msg.Data, &logMessage)
			if err != nil {
				metrics.LogRequest(metrics.SystemPubSub, metrics.OperationRead, metrics.StatusInvalidData)
				log.Warnf("unmarshalling message: %v", err)
				return
			}

			secretName, err = ParseSecretName(logMessage.ProtoPayload.ResourceName)
			if err != nil {
				metrics.LogRequest(metrics.SystemPubSub, metrics.OperationRead, metrics.StatusInvalidData)
				log.Errorf("parsing secret name: %v", err)
				return
			}

			projectID = logMessage.Resource.Labels.ProjectID
			if projectID == "" {
				log.Errorf("no project ID found in: %+v", logMessage)
				return
			}

			messages <- &pubSubMessage{
				ProjectID:  projectID,
				SecretName: secretName,
				LogMessage: logMessage,
				Message:    *msg,
			}
		})

		metrics.LogRequest(metrics.SystemPubSub, metrics.OperationRead, metrics.ErrorStatus(err, metrics.StatusError))
		if err != nil {
			log.Errorf("pulling message from subscription: %v", err)
		}
	}(ctx, messages)

	return messages
}
