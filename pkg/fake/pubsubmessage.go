package fake

import (
	"github.com/nais/hunter2/pkg/google"
	"time"
)

type pubSubMessageImpl struct {
	principalEmail string
	projectID      string
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

func (p *pubSubMessageImpl) GetProjectID() string {
	return p.projectID
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

func NewPubSubMessage(principalEmail, secretName, secretVersion, projectID string, timestamp time.Time) google.PubSubMessage {
	return &pubSubMessageImpl{
		principalEmail: principalEmail,
		secretName:     secretName,
		secretVersion:  secretVersion,
		timestamp:      timestamp,
		projectID:      projectID,
	}
}
