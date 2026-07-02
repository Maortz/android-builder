package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/nacl/box"
)

type PublicKey struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

func (c *Client) GetActionsPublicKey(ctx context.Context, owner, repo string) (*PublicKey, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/secrets/public-key", owner, repo)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var pk PublicKey
	if err := c.decode(resp, &pk); err != nil {
		return nil, fmt.Errorf("get actions public key: %w", err)
	}
	return &pk, nil
}

func encryptSecret(plaintext string, pubKeyB64 string) (string, error) {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	if len(pubKeyBytes) != 32 {
		return "", fmt.Errorf("unexpected public key length %d, want 32", len(pubKeyBytes))
	}
	var pubKey [32]byte
	copy(pubKey[:], pubKeyBytes)

	sealed, err := box.SealAnonymous(nil, []byte(plaintext), &pubKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("seal secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *Client) CreateOrUpdateSecret(ctx context.Context, owner, repo, secretName, plaintext string, pubKey *PublicKey) error {
	encryptedValue, err := encryptSecret(plaintext, pubKey.Key)
	if err != nil {
		return fmt.Errorf("encrypt %s: %w", secretName, err)
	}
	type payload struct {
		EncryptedValue string `json:"encrypted_value"`
		KeyID          string `json:"key_id"`
	}
	b, _ := json.Marshal(payload{EncryptedValue: encryptedValue, KeyID: pubKey.KeyID})
	path := fmt.Sprintf("/repos/%s/%s/actions/secrets/%s", owner, repo, secretName)
	resp, err := c.do(ctx, "PUT", path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("set secret %s: HTTP %d", secretName, resp.StatusCode)
	}
	return nil
}
