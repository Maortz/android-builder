package github

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestEncryptSecretRoundTrip(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(pub[:])

	sealedB64, err := encryptSecret("super-secret-value", pubKeyB64)
	if err != nil {
		t.Fatalf("encryptSecret: %v", err)
	}

	sealed, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		t.Fatalf("decode sealed box: %v", err)
	}

	opened, ok := box.OpenAnonymous(nil, sealed, pub, priv)
	if !ok {
		t.Fatal("failed to open sealed box")
	}
	if string(opened) != "super-secret-value" {
		t.Errorf("got %q, want %q", opened, "super-secret-value")
	}
}
