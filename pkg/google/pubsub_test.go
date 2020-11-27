package google_test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/nais/hunter2/pkg/google"
)

type testcase struct {
	input  string
	output string
	err    error
}

var secretNameTests = []testcase{
	{
		input:  "projects/12345/secrets/foobar",
		output: "foobar",
	},
	{
		input:  "projects/12345/secrets/foobar/versions/2",
		output: "foobar",
	},
	{
		input:  "projects/12345/secrets",
		output: "",
		err:    fmt.Errorf("resource name does not contain a secret"),
	},
}

func TestParseSecretName(t *testing.T) {
	for _, test := range secretNameTests {
		output, err := google.ParseSecretName(test.input)
		assert.Equal(t, test.output, output)
		assert.Equal(t, test.err, err)
	}
}

var secretVersionTests = []testcase{
	{
		input:  "projects/12345/secrets/foobar",
		output: "1",
	},
	{
		input:  "projects/12345/secrets/foobar/versions/2",
		output: "2",
	},
	{
		input:  "projects/12345/secrets",
		output: "1",
	},
}

func TestParseSecretVersion(t *testing.T) {
	for _, test := range secretVersionTests {
		output := google.ParseSecretVersion(test.input)
		assert.Equal(t, test.output, output)
	}
}
