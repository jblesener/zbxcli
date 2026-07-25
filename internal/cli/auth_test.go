package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jblesener/zbxcli/internal/profile"
)

type memorySecrets struct {
	values  map[string]string
	failSet bool
}

func (m *memorySecrets) Get(name string) (string, error) {
	value, ok := m.values[name]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}
func (m *memorySecrets) Set(name, value string) error {
	if m.failSet {
		return errors.New("keychain unavailable")
	}
	m.values[name] = value
	return nil
}
func (m *memorySecrets) Delete(name string) error { delete(m.values, name); return nil }

func TestLoginStoresTokenAndRevokesPreviousToken(t *testing.T) {
	var deleted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := any(true)
		switch req.Method {
		case "apiinfo.version":
			result = "6.0.0"
		case "user.login":
			result = "session"
		case "token.create":
			result = map[string]any{"tokenids": []string{"new-token"}}
		case "token.generate":
			result = []map[string]string{{"tokenid": "new-token", "token": "new-secret"}}
		case "token.delete":
			deleted = "old-token"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()
	secrets := &memorySecrets{values: map[string]string{"default": "old-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: "http://old", TokenID: "old-token"}}}); err != nil {
		t.Fatal(err)
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: store, Password: func(_ io.Writer) (string, error) { return "password", nil }})
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"auth", "login", "--url", server.URL, "--username", "Admin"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if deleted != "old-token" {
		t.Fatalf("deleted = %q", deleted)
	}
	if got := secrets.values["default"]; got != "new-secret" {
		t.Fatalf("stored token = %q", got)
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["default"].TokenID != "new-token" {
		t.Fatalf("profile = %#v", cfg.Profiles["default"])
	}
	if out.Len() == 0 || errOut.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestParseExpiry(t *testing.T) {
	if expiry, err := parseExpiry("never", now()); err != nil || expiry != nil {
		t.Fatalf("never = %v, %v", expiry, err)
	}
	if _, err := parseExpiry("0d", now()); err == nil {
		t.Fatal("zero-day expiry accepted")
	}
	if _, err := parseExpiry("90d", now()); err != nil {
		t.Fatal(err)
	}
}

func TestTokenShowWritesSelectedToken(t *testing.T) {
	secrets := &memorySecrets{values: map[string]string{"production": "production-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"production": {URL: "https://zabbix.example"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"auth", "token", "show", "--profile", "production"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "production-secret\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestTokenShowRejectsMissingProfileAndToken(t *testing.T) {
	t.Run("profile", func(t *testing.T) {
		store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
		cmd := NewRootCmd(Dependencies{Store: store})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"auth", "token", "show"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("auth token show succeeded for missing profile")
		}
		if got := out.String(); got != "" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("token", func(t *testing.T) {
		store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
		if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: "https://zabbix.example"}}}); err != nil {
			t.Fatal(err)
		}
		cmd := NewRootCmd(Dependencies{Store: store})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"auth", "token", "show"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("auth token show succeeded for missing token")
		}
		if got := out.String(); got != "" {
			t.Fatalf("stdout = %q", got)
		}
	})
}

func now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
