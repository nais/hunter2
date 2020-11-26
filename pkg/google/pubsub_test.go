package google_test

import (
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/nais/hunter2/pkg/google"
)

type secretnametest struct {
	input  string
	output string
	err    error
}

var secretNameTests = []secretnametest{
	{
		input:  "projects/12345/secrets/foobar",
		output: "foobar",
	},
	{
		input:  "projects/12345/secrets/foobar/versions/NEW",
		output: "foobar",
	},
}

func TestParseSecretName(t *testing.T) {
	for _, test := range secretNameTests {
		output, err := google.ParseSecretName(test.input)
		assert.Equal(t, test.output, output)
		assert.Equal(t, test.err, err)
	}
}
