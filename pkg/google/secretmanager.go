package google

import (
	"context"
	"fmt"
	"github.com/nais/hunter2/pkg/metrics"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type SecretManagerClient struct {
	*secretmanager.Client
	projectID string
}

func NewSecretManagerClient(ctx context.Context, projectID string) (*SecretManagerClient, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating secret manager client: %w", err)
	}
	return &SecretManagerClient{client, projectID}, nil
}

func (in *SecretManagerClient) GetSecretData(ctx context.Context, secretName string) ([]byte, error) {
	req := ToAccessSecretVersionRequest(in.projectID, secretName)
	start := time.Now()
	result, err := in.AccessSecretVersion(ctx, req)
	responseTime := time.Now().Sub(start)
	metrics.GoogleSecretManagerResponseTime.Observe(responseTime.Seconds())
	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func ToAccessSecretVersionRequest(projectID, secretName string) *secretmanagerpb.AccessSecretVersionRequest {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	return &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
}
