package config

import "errors"

var ErrConfigNotFound = errors.New("builder.json not found")

type Config struct {
	Project     string            `json:"project"`
	Platform    string            `json:"platform"`
	GitHub      GitHubConfig      `json:"github"`
	Android     AndroidConfig     `json:"android,omitempty"`
	Flutter     FlutterConfig     `json:"flutter,omitempty"`
	ReactNative ReactNativeConfig `json:"reactNative,omitempty"`
}

type GitHubConfig struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch,omitempty"`
}

type AndroidConfig struct {
	BuildType   string `json:"buildType,omitempty"`
	Flavor      string `json:"flavor,omitempty"`
	PackageName string `json:"packageName,omitempty"`
	DeviceID    string `json:"deviceId,omitempty"`
	Signing     bool   `json:"signing,omitempty"`
}

type ReactNativeConfig struct {
	MetroPort int `json:"metroPort,omitempty"`
}

type FlutterConfig struct {
	Version string      `json:"version,omitempty"`
	Watch   WatchConfig `json:"watch,omitempty"`
}

type WatchConfig struct {
	Dirs     []string `json:"dirs,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	Ignore   []string `json:"ignore,omitempty"`
	Debounce int      `json:"debounce,omitempty"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "config: " + e.Field + ": " + e.Message
}

func (c *Config) Validate() error {
	if c.Project == "" {
		return &ValidationError{Field: "project", Message: "required"}
	}
	if c.GitHub.Owner == "" {
		return &ValidationError{Field: "github.owner", Message: "required"}
	}
	if c.GitHub.Repo == "" {
		return &ValidationError{Field: "github.repo", Message: "required"}
	}
	return nil
}
