package google

import (
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"context"
	"fmt"
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
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", in.projectID, secretName)
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	result, err := in.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret version: %v", err)
	}
	return result.Payload.Data, nil
}
