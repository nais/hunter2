package synchronizer

import (
	"github.com/magiconair/properties/assert"
	"github.com/nais/hunter2/pkg/google"
	"testing"
	"time"
)

type pubSubMessageImpl struct {
	principalEmail string
	secretName     string
	secretVersion  string
	timestamp      time.Time
}

func (p *pubSubMessageImpl) Ack() {
	// no-op
}

func (p *pubSubMessageImpl) GetPrincipalEmail() string {
	return p.principalEmail
}

func (p *pubSubMessageImpl) GetSecretName() string {
	return p.secretName
}

func (p *pubSubMessageImpl) GetSecretVersion() string {
	return p.secretVersion
}

func (p *pubSubMessageImpl) GetTimestamp() time.Time {
	return p.timestamp
}

func newPubSubMessage(principalEmail, secretName, secretVersion string, timestamp time.Time) google.PubSubMessage {
	return &pubSubMessageImpl{
		principalEmail: principalEmail,
		secretName:     secretName,
		secretVersion:  secretVersion,
		timestamp:      timestamp,
	}
}

func TestToSecretData(t *testing.T) {
	principalEmail := "some-principal@domain.test"
	secretName := "some-secret"
	secretVersion := "1"
	timestamp := time.Now()

	msg := newPubSubMessage(principalEmail, secretName, secretVersion, timestamp)

	namespace := "some-namespace"
	payload := []byte("some-payload")

	secretData := ToSecretData(namespace, msg, payload)

	assert.Equal(t, secretData.Namespace, namespace)
	assert.Equal(t, secretData.Name, secretName)
	assert.Equal(t, secretData.Payload, map[string]string{
		StaticSecretDataKey: string(payload),
	})
	assert.Equal(t, secretData.SecretVersion, secretVersion)
	assert.Equal(t, secretData.LastModified, timestamp)
	assert.Equal(t, secretData.LastModifiedBy, principalEmail)
}
