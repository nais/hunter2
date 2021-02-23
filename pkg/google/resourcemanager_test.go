package google_test

import (
	"github.com/nais/hunter2/pkg/google"
	"github.com/stretchr/testify/assert"
	"testing"
)

var resourcemanagerTests = []testcase{
	{
		input:  "project-environment",
		output: "project",
	},
	{
		input:  "project-something-environment",
		output: "project-something",
	},
	{
		input:  "project-something-nice-environment",
		output: "project-something-nice",
	},
	{
		input:  "project",
		output: "project",
	},
}

func TestExtractProjectName(t *testing.T) {
	for _, test := range resourcemanagerTests {
		output := google.ExtractProjectName(test.input)
		assert.Equal(t, test.output, output)
	}
}
