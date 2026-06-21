package workflow

import _ "embed"

//go:embed templates/android-build.yml
var androidBuildWorkflow []byte

func GetWorkflowTemplate() ([]byte, error) {
	return androidBuildWorkflow, nil
}
