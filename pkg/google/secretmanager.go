package google
import (
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"context"
	"fmt"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type SecretManagerClient struct {
	*secretmanager.Client
}

func NewSecretManagerClient(ctx context.Context) (*SecretManagerClient, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating secret manager client: %w", err)
	}
	return &SecretManagerClient{client}, nil
}

func (in *SecretManagerClient) GetSecretData(ctx context.Context, projectID, secretName string) ([]byte, error) {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	result, err := in.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret version: %v", err)
	}
	return result.Payload.Data, nil
}
