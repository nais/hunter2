package google_test

import (
	"github.com/nais/hunter2/pkg/google"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestToAccessSecretVersionRequest(t *testing.T) {
	projectID := "some-project"
	secretName := "some-secret"

	expected := "projects/some-project/secrets/some-secret/versions/latest"
	actual := google.ToAccessSecretVersionRequest(projectID, secretName)

	assert.Equal(t, expected, actual.GetName())
}

func TestToGetSecretRequest(t *testing.T) {
	projectID := "some-project"
	secretName := "some-secret"

	expected := "projects/some-project/secrets/some-secret"
	actual := google.ToGetSecretRequest(projectID, secretName)

	assert.Equal(t, expected, actual.GetName())
}
