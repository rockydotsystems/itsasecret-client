package api

import (
	"bytes"
	"context"
	"encoding/base64"
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
	ClientPublicKey string `json:"clientPubkey"`
}

type LoginResponse struct {
	Token           string            `json:"token"`
	ServerPublicKey string            `json:"serverPubkey"`
	WrappedOrgKeys  map[string]string `json:"orgKeys"`
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
	Vars []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"vars"`
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
	for _, v := range out.Vars {
		result[v.Key] = v.Value
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

// environment is the subset of an environment row the CLI needs to map a
// human-facing env name to the opaque env ID the secret/var routes key on.
type environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// resolveEnvID maps (projectID, envName) to the opaque env ID. The single
// secret/var routes are keyed by env ID, not project + env name (only /pull is
// project+name shaped), so every secret/var call resolves the ID first.
func (c *Client) resolveEnvID(ctx context.Context, projectID, envName string) (string, error) {
	path := fmt.Sprintf("/api/projects/%s/envs", projectID)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("list envs: HTTP %d", resp.StatusCode)
	}
	var envs []environment
	if err := json.NewDecoder(resp.Body).Decode(&envs); err != nil {
		return "", err
	}
	for _, e := range envs {
		if e.Name == envName {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("environment %q not found in project %s", envName, projectID)
}

func (c *Client) SetSecret(ctx context.Context, projectID, envName, key, value string) error {
	if len(c.sessionKey) == 0 {
		return fmt.Errorf("no session key — cannot encrypt secret")
	}
	encrypted, err := crypto.EncryptString(c.sessionKey, value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return err
	}
	// cipher defaults to 'session' server-side: the server decrypts with the
	// transport session key and re-encrypts under the org key.
	path := fmt.Sprintf("/api/envs/%s/secrets/%s", envID, key)
	body := map[string]string{"encryptedValue": encrypted}
	return c.doNoBody(ctx, "PUT", path, body)
}

func (c *Client) SetVar(ctx context.Context, projectID, envName, key, value string) error {
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/envs/%s/vars/%s", envID, key)
	body := map[string]string{"value": value}
	return c.doNoBody(ctx, "PUT", path, body)
}

func (c *Client) GetSecret(ctx context.Context, projectID, envName, key string) (string, error) {
	if len(c.sessionKey) == 0 {
		return "", fmt.Errorf("no session key — cannot decrypt secret")
	}
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("/api/envs/%s/secrets/%s", envID, key)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("get secret: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Key            string `json:"key"`
		EncryptedValue string `json:"encryptedValue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	plaintext, err := crypto.DecryptString(c.sessionKey, out.EncryptedValue)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

// GetVar reads a single var. There is no single-var GET route (vars are
// plaintext), so it lists the env's vars and picks the key.
func (c *Client) GetVar(ctx context.Context, projectID, envName, key string) (string, error) {
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("/api/envs/%s/vars", envID)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("get var: HTTP %d", resp.StatusCode)
	}
	var vars []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vars); err != nil {
		return "", err
	}
	for _, v := range vars {
		if v.Key == key {
			return v.Value, nil
		}
	}
	return "", fmt.Errorf("var %q not found", key)
}

func (c *Client) ForkEnv(ctx context.Context, projectID, envName, newName string) error {
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/projects/%s/envs/%s/fork", projectID, envID)
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
	if len(c.sessionKey) > 0 {
		req.Header.Set("X-Session-Key", base64.StdEncoding.EncodeToString(c.sessionKey))
	}
	return c.http.Do(req)
}
