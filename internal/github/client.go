package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const baseURL = "https://api.github.com"

type Client struct {
	token  string
	client *http.Client
}

func NewClient(token string) *Client {
	return &Client{token: token, client: &http.Client{}}
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.client.Do(req)
}

func (c *Client) decode(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(b))
	}
	return jsonDecode(resp.Body, v)
}

type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	fn         func(int64, int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.downloaded += int64(n)
	if p.fn != nil {
		p.fn(p.downloaded, p.total)
	}
	return n, err
}
