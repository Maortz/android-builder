package auth

import "testing"

func TestDeleteTokenClearsFallbackFile(t *testing.T) {
	tmp := t.TempDir()
	old := tokenFile
	tokenFile = tmp + "/token"
	defer func() { tokenFile = old }()

	if err := SetToken("dummy-token"); err != nil {
		t.Fatalf("SetToken failed: %v", err)
	}

	DeleteToken()

	if _, err := GetToken(); err == nil {
		t.Fatal("expected GetToken to fail after DeleteToken, got nil error")
	}
}
