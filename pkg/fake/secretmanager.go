package fake

import (
	"context"
	"github.com/nais/hunter2/pkg/google"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type secretManagerClientImpl struct {
	data     []byte
	metadata *secretmanagerpb.Secret
	err      error
}

func (s *secretManagerClientImpl) GetSecretMetadata(context.Context, string) (*secretmanagerpb.Secret, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.metadata, nil
}

func (s *secretManagerClientImpl) GetSecretData(context.Context, string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func NewSecretManagerClient(data []byte, metadata *secretmanagerpb.Secret, err error) google.SecretManagerClient {
	return &secretManagerClientImpl{data: data, metadata: metadata, err: err}
}
