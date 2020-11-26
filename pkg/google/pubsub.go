package google

import (
	"cloud.google.com/go/pubsub"
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
)

type PubSubClient struct {
	*pubsub.Subscription
}

func NewPubSubClient(ctx context.Context, projectID, subscriptionID string) (*PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	sub := client.Subscription(subscriptionID)
	return &PubSubClient{sub}, nil
}

func (in *PubSubClient) Consume(ctx context.Context) chan pubsub.Message {
	messages := make(chan pubsub.Message)

	go func(ctx context.Context, messages chan pubsub.Message) {
		defer close(messages)
		cctx, cancel := context.WithCancel(ctx)

		err := in.Receive(cctx, func (ctx context.Context, msg *pubsub.Message) {
			messages <- *msg
		})
		if err != nil {
			cancel()
			log.Errorf("pulling message from subscription: %v", err)
		}
	}(ctx, messages)

	return messages
}
