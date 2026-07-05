package commands

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"itsasecret.dev/cli/internal/config"
)

// fakeTokenBearer/fakeTokenKey are the two halves of a well-formed access
// token: both standard base64, joined by "." under the shht_ prefix.
var (
	fakeTokenBearer = base64.StdEncoding.EncodeToString([]byte("bearer-bytes-bearer-bytes-32bts!"))
	fakeTokenKey    = base64.StdEncoding.EncodeToString([]byte("key-bytes-key-bytes-key-bytes-32"))
	fakeToken       = "shht_" + fakeTokenBearer + "." + fakeTokenKey
)

// startTokenServer accepts exactly the fake bearer and reports a token
// session with the given expiry.
func startTokenServer(t *testing.T, expiresAt time.Time) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeTokenBearer {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]string{"email": "ci@example.com"},
			"session": map[string]string{
				"kind":      "token",
				"expiresAt": expiresAt.UTC().Format(time.RFC3339),
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthStoresTokenSession(t *testing.T) {
	expiresAt := time.Now().Add(60 * 24 * time.Hour).Truncate(time.Second)
	srv := startTokenServer(t, expiresAt)
	setupConfig(t, srv.URL, false)
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "", "auth", fakeToken)
	if err != nil {
		t.Fatalf("auth failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Authenticated to "+srv.URL+" as ci@example.com") {
		t.Errorf("missing success line, got:\n%s", out)
	}
	if !strings.Contains(out, "token expires "+expiresAt.Local().Format("2006-01-02")) {
		t.Errorf("missing expiry, got:\n%s", out)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := cfg.Session(srv.URL)
	if !ok {
		t.Fatalf("no session stored for %s", srv.URL)
	}
	if stored.Token != fakeTokenBearer {
		t.Errorf("stored token = %q, want the bearer half %q", stored.Token, fakeTokenBearer)
	}
	if stored.Email != "ci@example.com" {
		t.Errorf("stored email = %q", stored.Email)
	}
	if stored.Expired() {
		t.Error("freshly stored token session counts as expired")
	}
	wantKey := base64.RawURLEncoding.EncodeToString([]byte("key-bytes-key-bytes-key-bytes-32"))
	if stored.SessionKey != wantKey {
		t.Errorf("stored session key = %q, want %q", stored.SessionKey, wantKey)
	}
}

func TestAuthNeverExpiringToken(t *testing.T) {
	srv := startTokenServer(t, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	setupConfig(t, srv.URL, false)
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "", "auth", fakeToken)
	if err != nil {
		t.Fatalf("auth failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "(token does not expire)") {
		t.Errorf("expected does-not-expire wording, got:\n%s", out)
	}
}

func TestAuthRejectsMalformedToken(t *testing.T) {
	setupConfig(t, "https://example.invalid", false)
	t.Chdir(t.TempDir())

	if _, err := runCmd(t, "", "auth", "not-a-token"); err == nil || !strings.Contains(err.Error(), "shht_") {
		t.Errorf("want prefix error, got %v", err)
	}
	if _, err := runCmd(t, "", "auth", "shht_missing-separator"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("want malformed error, got %v", err)
	}
}

func TestAuthRejectedByServer(t *testing.T) {
	srv := startTokenServer(t, time.Now().Add(time.Hour))
	setupConfig(t, srv.URL, false)
	t.Chdir(t.TempDir())

	otherBearer := base64.StdEncoding.EncodeToString([]byte("some-other-bearer-value-32-bytes"))
	wrong := "shht_" + otherBearer + "." + fakeTokenKey
	_, err := runCmd(t, "", "auth", wrong)
	if err == nil || !strings.Contains(err.Error(), "token rejected by "+srv.URL) {
		t.Errorf("want rejection error, got %v", err)
	}

	// Nothing must be persisted for a rejected token.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored, ok := cfg.Session(srv.URL); ok && stored.Token != "" {
		t.Errorf("rejected token was persisted: %+v", stored)
	}
}

// The auth command honors a url override from .shh.project, like login does.
func TestAuthHonorsProjectURLOverride(t *testing.T) {
	srv := startTokenServer(t, time.Now().Add(time.Hour))
	setupConfig(t, "https://global.example.invalid", false)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".shh.project"), []byte("project = proj1\nurl = "+srv.URL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCmd(t, "", "auth", fakeToken)
	if err != nil {
		t.Fatalf("auth failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Authenticating to "+srv.URL+" (from ") {
		t.Errorf("missing override notice, got:\n%s", out)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Session(srv.URL); !ok {
		t.Errorf("session not stored under the override URL %s", srv.URL)
	}
}
