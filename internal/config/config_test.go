package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maortz/android-builder/internal/config"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	mgr := config.NewManager()
	cfg := &config.Config{
		Project:  "test-app",
		Platform: "android",
		GitHub:   config.GitHubConfig{Owner: "acme", Repo: "app"},
		Android:  config.AndroidConfig{BuildType: "debug"},
	}
	if err := mgr.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "builder.json")); err != nil {
		t.Fatalf("builder.json not created")
	}
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Project != "test-app" {
		t.Errorf("Project: got %q want %q", loaded.Project, "test-app")
	}
	if loaded.Android.BuildType != "debug" {
		t.Errorf("BuildType: got %q want debug", loaded.Android.BuildType)
	}
}

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	_, err := config.NewManager().Load()
	if err != config.ErrConfigNotFound {
		t.Errorf("want ErrConfigNotFound, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{"valid", config.Config{Project: "x", GitHub: config.GitHubConfig{Owner: "a", Repo: "b"}}, false},
		{"no project", config.Config{GitHub: config.GitHubConfig{Owner: "a", Repo: "b"}}, true},
		{"no owner", config.Config{Project: "x", GitHub: config.GitHubConfig{Repo: "b"}}, true},
		{"no repo", config.Config{Project: "x", GitHub: config.GitHubConfig{Owner: "a"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
