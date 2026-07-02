package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"itsasecret.dev/cli/internal/crypto"
)

type Client struct {
	baseURL    string
	token      string
	sessionKey []byte
	http       *http.Client
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

func (c *Client) WithSessionKey(key []byte) *Client {
	c.sessionKey = key
	return c
}

type LoginRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ClientPublicKey string `json:"clientPublicKey"`
}

type LoginResponse struct {
	Token           string            `json:"token"`
	ServerPublicKey string            `json:"serverPublicKey"`
	WrappedOrgKeys  map[string]string `json:"wrappedOrgKeys"`
}

func (c *Client) Login(ctx context.Context, email, password, clientPubKey string) (*LoginResponse, error) {
	body := LoginRequest{
		Email:           email,
		Password:        password,
		ClientPublicKey: clientPubKey,
	}
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

type PullResponse struct {
	Vars    map[string]string `json:"vars"`
	Secrets map[string]string `json:"secrets"`
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
	var out PullResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(out.Vars)+len(out.Secrets))
	for k, v := range out.Vars {
		result[k] = v
	}
	for k, encrypted := range out.Secrets {
		if len(c.sessionKey) == 0 {
			return nil, fmt.Errorf("cannot decrypt secret %q: no session key", k)
		}
		plaintext, err := crypto.DecryptString(c.sessionKey, encrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %q: %w", k, err)
		}
		result[k] = plaintext
	}
	return result, nil
}

func (c *Client) SetSecret(ctx context.Context, projectID, envName, key, value string) error {
	if len(c.sessionKey) == 0 {
		return fmt.Errorf("no session key — cannot encrypt secret")
	}
	encrypted, err := crypto.EncryptString(c.sessionKey, value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	path := fmt.Sprintf("/api/projects/%s/envs/%s/secrets/%s", projectID, envName, key)
	body := map[string]string{"value": encrypted}
	return c.doNoBody(ctx, "PUT", path, body)
}

func (c *Client) SetVar(ctx context.Context, projectID, envName, key, value string) error {
	path := fmt.Sprintf("/api/projects/%s/envs/%s/vars/%s", projectID, envName, key)
	body := map[string]string{"value": value}
	return c.doNoBody(ctx, "PUT", path, body)
}

func (c *Client) GetSecret(ctx context.Context, projectID, envName, key string) (string, error) {
	if len(c.sessionKey) == 0 {
		return "", fmt.Errorf("no session key — cannot decrypt secret")
	}
	path := fmt.Sprintf("/api/projects/%s/envs/%s/secrets/%s", projectID, envName, key)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("get secret: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	plaintext, err := crypto.DecryptString(c.sessionKey, out.Value)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func (c *Client) GetVar(ctx context.Context, projectID, envName, key string) (string, error) {
	path := fmt.Sprintf("/api/projects/%s/envs/%s/vars/%s", projectID, envName, key)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("get var: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Value, nil
}

func (c *Client) ForkEnv(ctx context.Context, projectID, envName, newName string) error {
	path := fmt.Sprintf("/api/projects/%s/envs/%s/fork", projectID, envName)
	body := map[string]string{"name": newName}
	resp, err := c.do(ctx, "POST", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("fork: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) doNoBody(ctx context.Context, method, path string, body any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	return nil
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
