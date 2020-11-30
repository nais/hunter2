package fake

import (
	"context"
	"github.com/nais/hunter2/pkg/google"
)

type secretManagerClientImpl struct {
	data []byte
	err  error
}

func (s *secretManagerClientImpl) GetSecretData(context.Context, string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func NewSecretManagerClient(data []byte, err error) google.SecretManagerClient {
	return &secretManagerClientImpl{data: data, err: err}
}
