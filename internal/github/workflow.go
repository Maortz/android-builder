package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

func jsonDecode(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func (c *Client) TriggerWorkflow(ctx context.Context, owner, repo, workflowFile string, inputs map[string]string) error {
	type payload struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	b, _ := json.Marshal(payload{Ref: "main", Inputs: inputs})
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflowFile)
	resp, err := c.do(ctx, "POST", path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("workflow %s not found — run: builder init", workflowFile)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger workflow: %d %s", resp.StatusCode, body)
	}
	return nil
}

func (c *Client) PollForWorkflowStart(ctx context.Context, owner, repo, workflowFile string, after time.Time, timeout time.Duration) (*WorkflowRun, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		created := url.QueryEscape(">=" + after.UTC().Format(time.RFC3339))
		path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/runs?event=workflow_dispatch&created=%s&per_page=5",
			owner, repo, workflowFile, created)
		resp, err := c.do(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		var result listRunsResponse
		if err := c.decode(resp, &result); err != nil {
			return nil, err
		}
		for i, run := range result.WorkflowRuns {
			if run.CreatedAt.After(after) || run.CreatedAt.Equal(after) {
				return &result.WorkflowRuns[i], nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return nil, fmt.Errorf("workflow did not start within %s", timeout)
}

func (c *Client) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var run WorkflowRun
	return &run, c.decode(resp, &run)
}

func (c *Client) PollForArtifact(ctx context.Context, owner, repo string, runID int64, artifactName string, timeout time.Duration) (*Artifact, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/artifacts", owner, repo, runID)
		resp, err := c.do(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		var result listArtifactsResponse
		if err := c.decode(resp, &result); err != nil {
			return nil, err
		}
		for i, a := range result.Artifacts {
			if a.Name == artifactName {
				return &result.Artifacts[i], nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return nil, fmt.Errorf("artifact %q not available after %s", artifactName, timeout)
}

func (c *Client) DownloadArtifactWithProgress(ctx context.Context, owner, repo string, artifactID int64, onProgress func(int64, int64)) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/artifacts/%d/zip", owner, repo, artifactID)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download artifact: %d %s", resp.StatusCode, b)
	}
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, fn: onProgress}
	return io.ReadAll(pr)
}
