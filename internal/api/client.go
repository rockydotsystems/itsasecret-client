package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"itsasecret.dev/cli/internal/crypto"
)

type Client struct {
	baseURL    string
	token      string
	sessionKey []byte
	onNewToken func(token string, expiresAt time.Time)
	onReauth   func(ctx context.Context) (token string, sessionKey []byte, err error)
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

// WithTokenSaver registers a callback for rolled session tokens: CLI
// sessions rotate on every successful request, and the new token must be
// persisted or the next command is locked out once the grace window closes.
func (c *Client) WithTokenSaver(fn func(token string, expiresAt time.Time)) *Client {
	c.onNewToken = fn
	return c
}

// WithReauth registers a recovery callback for rejected sessions: when the
// server answers 401 (rolled token lost, session revoked or expired
// server-side) the callback re-authenticates - typically by prompting for
// the master password - and the request is retried once with the fresh
// credentials.
func (c *Client) WithReauth(fn func(ctx context.Context) (token string, sessionKey []byte, err error)) *Client {
	c.onReauth = fn
	return c
}

// ErrUnauthorized reports that the server rejected the session token - the
// session has expired or was revoked.
var ErrUnauthorized = errors.New("session rejected by server")

// Me returns the logged-in user's email, or ErrUnauthorized when the session
// token is no longer valid.
func (c *Client) Me(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, "GET", "/api/auth/me", nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", ErrUnauthorized
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("me: HTTP %d", resp.StatusCode)
	}
	var out struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.User.Email, nil
}

type LoginRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ClientPublicKey string `json:"clientPubkey"`
	// Client identifies the session kind; "cli" sessions are short-lived and
	// roll their token on every successful request.
	Client string `json:"client,omitempty"`
}

type LoginResponse struct {
	Token           string            `json:"token"`
	ServerPublicKey string            `json:"serverPubkey"`
	WrappedOrgKeys  map[string]string `json:"orgKeys"`
	// MasterWrappedOrgKeys are wrapped under the master-password-derived key
	// - the only form of org keys the CLI persists.
	MasterWrappedOrgKeys map[string]string `json:"masterWrappedOrgKeys"`
	SessionExpiresAt     time.Time         `json:"sessionExpiresAt"`
}

func (c *Client) Login(ctx context.Context, email, password, clientPubKey string) (*LoginResponse, error) {
	body := LoginRequest{
		Email:           email,
		Password:        password,
		ClientPublicKey: clientPubKey,
		Client:          "cli",
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

// Org is the subset of an org row the CLI needs.
type Org struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Project is the subset of a project row the CLI needs.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Environment is the subset of an environment row the CLI needs, including
// the opaque env ID the secret/var routes key on.
type Environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListOrgs returns the orgs the logged-in user is a member of.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	var orgs []Org
	if err := c.getJSON(ctx, "/api/orgs/", "list orgs", &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// ListProjects returns the projects in an org.
func (c *Client) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	var projects []Project
	path := fmt.Sprintf("/api/orgs/%s/projects", orgID)
	if err := c.getJSON(ctx, path, "list projects", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListEnvs returns the environments in a project.
func (c *Client) ListEnvs(ctx context.Context, projectID string) ([]Environment, error) {
	var envs []Environment
	path := fmt.Sprintf("/api/projects/%s/envs", projectID)
	if err := c.getJSON(ctx, path, "list envs", &envs); err != nil {
		return nil, err
	}
	return envs, nil
}

// resolveEnvID maps (projectID, envName) to the opaque env ID. The single
// secret/var routes are keyed by env ID, not project + env name (only /pull is
// project+name shaped), so every secret/var call resolves the ID first.
func (c *Client) resolveEnvID(ctx context.Context, projectID, envName string) (string, error) {
	envs, err := c.ListEnvs(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, e := range envs {
		if e.Name == envName {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("environment %q not found in project %s", envName, projectID)
}

// ListSecrets returns the secret keys in an environment, sorted. Values are
// never returned by the list route - use `secret get` to reveal one.
func (c *Client) ListSecrets(ctx context.Context, projectID, envName string) ([]string, error) {
	envID, err := c.resolveEnvID(ctx, projectID, envName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/envs/%s/secrets", envID)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list secrets: HTTP %d", resp.StatusCode)
	}
	var rows []struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (c *Client) SetSecret(ctx context.Context, projectID, envName, key, value string) error {
	if len(c.sessionKey) == 0 {
		return fmt.Errorf("no session key - cannot encrypt secret")
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
		return "", fmt.Errorf("no session key - cannot decrypt secret")
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

// getJSON GETs path and decodes the 200 response into out; what names the
// operation in error messages.
func (c *Client) getJSON(ctx context.Context, path, what string, out any) error {
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d", what, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	// A rejected session (rolled token lost, revoked, expired server-side)
	// gets one shot at recovery: re-authenticate and retry the request.
	if resp.StatusCode == 401 && c.token != "" && c.onReauth != nil {
		reauth := c.onReauth
		c.onReauth = nil // one retry per client, no loops
		_ = resp.Body.Close()
		token, sessionKey, err := reauth(ctx)
		if err != nil {
			return nil, err
		}
		c.token, c.sessionKey = token, sessionKey
		return c.do(ctx, method, path, body)
	}
	// Rolling sessions: successful responses carry a fresh token that must
	// replace the one we sent.
	if newToken := resp.Header.Get("X-New-Session-Token"); newToken != "" && c.onNewToken != nil {
		expiresAt, _ := time.Parse(time.RFC3339, resp.Header.Get("X-Session-Expires-At"))
		c.token = newToken
		c.onNewToken(newToken, expiresAt)
	}
	return resp, nil
}
