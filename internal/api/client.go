package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    http.DefaultClient,
	}
}

func (c *Client) WithToken(token string) *Client {
	c.token = token
	return c
}

type LoginResponse struct {
	Token       string            `json:"token"`
	SessionKey  []byte            `json:"sessionKey"`
	OrgKeys     map[string][]byte `json:"orgKeys"`
}

func (c *Client) Login(ctx context.Context, email, password string) (*LoginResponse, error) {
	body := map[string]string{"email": email, "password": password}
	resp, err := c.do(ctx, "POST", "/api/auth/login", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("login: HTTP %d", resp.StatusCode)
	}
	var out LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Pull(ctx context.Context, projectID, envName string) (map[string]string, error) {
	path := fmt.Sprintf("/api/projects/%s/envs/%s/pull", projectID, envName)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pull: HTTP %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}
