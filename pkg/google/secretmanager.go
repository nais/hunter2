package google

import (
	"context"
	"fmt"
	"github.com/nais/hunter2/pkg/metrics"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type SecretManagerClient interface {
	GetSecretData(ctx context.Context, projectID string) ([]byte, error)
	GetSecretMetadata(ctx context.Context, secretName string) (*secretmanagerpb.Secret, error)
}

type secretManagerClient struct {
	*secretmanager.Client
	projectID string
}

func NewSecretManagerClient(ctx context.Context, projectID string) (SecretManagerClient, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating secret manager client: %w", err)
	}
	return &secretManagerClient{client, projectID}, nil
}

func (in *secretManagerClient) GetSecretData(ctx context.Context, secretName string) ([]byte, error) {
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

func (in *secretManagerClient) GetSecretMetadata(ctx context.Context, secretName string) (*secretmanagerpb.Secret, error) {
	req := ToGetSecretRequest(in.projectID, secretName)
	start := time.Now()
	secret, err := in.GetSecret(ctx, req)
	responseTime := time.Now().Sub(start)
	metrics.GoogleSecretManagerResponseTime.Observe(responseTime.Seconds())
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func ToAccessSecretVersionRequest(projectID, secretName string) *secretmanagerpb.AccessSecretVersionRequest {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	return &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
}

func ToGetSecretRequest(projectID, secretName string) *secretmanagerpb.GetSecretRequest {
	name := fmt.Sprintf("projects/%s/secrets/%s", projectID, secretName)
	return &secretmanagerpb.GetSecretRequest{
		Name: name,
	}
}
