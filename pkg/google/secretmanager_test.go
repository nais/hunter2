package google_test

import (
	"github.com/magiconair/properties/assert"
	"github.com/nais/hunter2/pkg/google"
	"testing"
)

func TestToAccessSecretVersionRequest(t *testing.T) {
	projectID := "some-project"
	secretName := "some-secret"

	expected := "projects/some-project/secrets/some-secret/versions/latest"
	actual := google.ToAccessSecretVersionRequest(projectID, secretName)

	assert.Equal(t, actual.GetName(), expected)
}
