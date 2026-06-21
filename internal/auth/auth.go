package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "android-builder"
	accountName = "github-token"
)

var tokenFile = filepath.Join(homeDir(), ".config", "android-builder", "token")

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func GetToken() (string, error) {
	token, err := keyring.Get(serviceName, accountName)
	if err == nil && token != "" {
		return token, nil
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", errors.New("not authenticated")
	}
	token = strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("not authenticated")
	}
	return token, nil
}

func SetToken(token string) error {
	if err := keyring.Set(serviceName, accountName, token); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(tokenFile, []byte(token), 0600)
}

func DeleteToken() {
	_ = keyring.Delete(serviceName, accountName)
	_ = os.Remove(tokenFile)
}
