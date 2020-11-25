package google

import (
	"cloud.google.com/go/pubsub"
	"context"
	"fmt"
)

type PubSubClient struct {
	*pubsub.Client
}

func NewPubSubClient(ctx context.Context, projectID string) (*PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	return &PubSubClient{client}, nil
}

func (in *PubSubClient) pull(ctx context.Context, subscriptionID string) ([]byte, error) {
	sub := in.Subscription(subscriptionID)
	var payload []byte
	err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		payload = msg.Data
		// TODO
		msg.Ack()
	})
	if err != nil {
		return nil, fmt.Errorf("pulling message from subscription: %w", err)
	}
	return payload, nil
}
