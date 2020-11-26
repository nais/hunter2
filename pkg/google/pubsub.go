package google

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/pubsub"
	log "github.com/sirupsen/logrus"
)

type PubSubClient struct {
	*pubsub.Subscription
}

type PubSubMessage struct {
	SecretName string
	pubsub.Message
}

type LogMessage struct {
	ProtoPayload ProtoPayload `json:"protoPayload"`
}

type ProtoPayload struct {
	ResourceName string `json:"resourceName"`
}

func NewPubSubClient(ctx context.Context, projectID, subscriptionID string) (*PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	sub := client.Subscription(subscriptionID)
	return &PubSubClient{sub}, nil
}

func ParseSecretName(resourceName string) (string, error) {
	tokens := strings.Split(resourceName, "/")
	if len(tokens) < 4 || tokens[0] != "projects" || tokens[2] != "secrets" {
		return "", fmt.Errorf("resource name does not contain a secret")
	}
	return tokens[3], nil
}

func (in *PubSubClient) Consume(ctx context.Context) chan PubSubMessage {
	messages := make(chan PubSubMessage)

	go func(ctx context.Context, messages chan PubSubMessage) {
		defer close(messages)
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		err := in.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
			var logMessage LogMessage
			var secretName string
			err := json.Unmarshal(msg.Data, &logMessage)
			if err != nil {
				log.Warnf("failed to unmarshal message: %v", err)
				return
			}
			secretName, err = ParseSecretName(logMessage.ProtoPayload.ResourceName)
			if err != nil {
				log.Errorf("invalid message format: %s")
				return
			}
			messages <- PubSubMessage{secretName, *msg}
		})
		if err != nil {
			log.Errorf("pulling message from subscription: %v", err)
		}
	}(ctx, messages)

	return messages
}
