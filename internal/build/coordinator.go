package build

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/github"
	"github.com/google/uuid"
)

const (
	DefaultTimeout  = 30 * time.Minute
	WorkflowFile    = "android-build.yml"
	APKArtifactName = "apk"
)

type Coordinator struct {
	config   *config.Config
	github   *github.Client
	progress *Progress
}

func NewCoordinator(cfg *config.Config, gh *github.Client) *Coordinator {
	return &Coordinator{config: cfg, github: gh, progress: NewProgress(os.Stdout)}
}

type BuildOptions struct {
	OutputDir string
	Timeout   time.Duration
	Release   bool
	Unsigned  bool
}

type BuildResult struct {
	BuildID     string
	APKPath     string
	Duration    time.Duration
	WorkflowURL string
	APKSize     int64
}

func (c *Coordinator) Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
	start := time.Now()
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	buildID := uuid.New().String()[:8]
	c.progress.Start(buildID)

	buildType := "debug"
	if opts.Release {
		buildType = "release"
	}

	c.progress.Update(PhaseTriggering, "Triggering GitHub Actions build...")
	triggerTime := time.Now()
	inputs := map[string]string{"build_id": buildID, "build_type": buildType}
	if c.config.Flutter.Version != "" {
		inputs["flutter_version"] = c.config.Flutter.Version
	}
	if opts.Unsigned {
		inputs["unsigned"] = "true"
	}
	branch := c.config.GitHub.Branch
	if branch == "" {
		branch = "master"
	}
	if err := c.github.TriggerWorkflow(ctx, c.config.GitHub.Owner, c.config.GitHub.Repo, WorkflowFile, branch, inputs); err != nil {
		c.progress.Error(PhaseTriggering, err)
		return nil, fmt.Errorf("trigger: %w", err)
	}
	c.progress.Complete(PhaseTriggering, "Workflow triggered")

	c.progress.Update(PhaseWaitingStart, "Waiting for workflow to start...")
	run, err := c.github.PollForWorkflowStart(ctx, c.config.GitHub.Owner, c.config.GitHub.Repo, WorkflowFile, triggerTime, 2*time.Minute)
	if err != nil {
		c.progress.Error(PhaseWaitingStart, err)
		return nil, fmt.Errorf("start: %w", err)
	}
	c.progress.Complete(PhaseWaitingStart, fmt.Sprintf("Workflow started (run #%d)", run.ID))
	c.progress.SetWorkflowURL(run.HTMLURL)

	c.progress.Update(PhaseBuilding, "Building APK... (ubuntu-latest, ~3–5 min)")
	artifact, err := c.github.PollForArtifact(ctx, c.config.GitHub.Owner, c.config.GitHub.Repo, run.ID, APKArtifactName, opts.Timeout)
	if err != nil {
		c.progress.Error(PhaseBuilding, err)
		return nil, fmt.Errorf("build: %w", err)
	}
	c.progress.Complete(PhaseBuilding, "Build complete")

	c.progress.Update(PhaseDownloading, "Downloading APK...")
	apkPath, apkSize, err := c.downloadAPK(ctx, opts.OutputDir, artifact.ID, buildID)
	if err != nil {
		c.progress.Error(PhaseDownloading, err)
		return nil, fmt.Errorf("download: %w", err)
	}
	c.progress.Complete(PhaseDownloading, fmt.Sprintf("Downloaded (%.2f MB)", float64(apkSize)/(1024*1024)))
	c.progress.Finish()

	return &BuildResult{
		BuildID:     buildID,
		APKPath:     apkPath,
		Duration:    time.Since(start),
		WorkflowURL: run.HTMLURL,
		APKSize:     apkSize,
	}, nil
}

func (c *Coordinator) downloadAPK(ctx context.Context, outputDir string, artifactID int64, buildID string) (string, int64, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", 0, err
	}
	zipData, err := c.github.DownloadArtifactWithProgress(ctx, c.config.GitHub.Owner, c.config.GitHub.Repo, artifactID, func(d, t int64) {
		c.progress.UpdateDownloadProgress(d, t)
	})
	if err != nil {
		return "", 0, err
	}
	dest := filepath.Join(outputDir, fmt.Sprintf("%s-%s.apk", c.config.Project, buildID))
	size, err := extractAPKFromZip(zipData, dest)
	return dest, size, err
}

func extractAPKFromZip(zipData []byte, destPath string) (int64, error) {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return 0, err
	}
	for _, f := range r.File {
		if filepath.Ext(f.Name) == ".apk" {
			rc, err := f.Open()
			if err != nil {
				return 0, err
			}
			defer rc.Close()
			out, err := os.Create(destPath)
			if err != nil {
				return 0, err
			}
			defer out.Close()
			return io.Copy(out, rc)
		}
	}
	return 0, fmt.Errorf("no .apk found in artifact zip")
}
