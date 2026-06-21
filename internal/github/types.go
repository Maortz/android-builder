package github

import "time"

type WorkflowRun struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
}

type Artifact struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type listRunsResponse struct {
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type listArtifactsResponse struct {
	Artifacts []Artifact `json:"artifacts"`
}
