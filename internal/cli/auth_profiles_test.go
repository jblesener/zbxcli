package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jblesener/zbxcli/internal/profile"
)

func TestProfileListWritesSortedMetadata(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{
		"zulu":  {URL: "https://z.example", Username: "Zed", APIVersion: "7.2.0", TokenID: "3", TLSThumbprint: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		"alpha": {URL: "https://a.example", Username: "Ada", APIVersion: "6.0.0", TokenID: "1", ExpiresAt: &past},
		"beta":  {URL: "https://b.example", Username: "Bea", APIVersion: "7.0.0", TokenID: "2", ExpiresAt: &future},
	}}); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd(Dependencies{Store: store})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "profile", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "alpha") || !strings.HasPrefix(lines[2], "beta") || !strings.HasPrefix(lines[3], "zulu") {
		t.Fatalf("table = %q", out.String())
	}

	out.Reset()
	cmd = NewRootCmd(Dependencies{Store: store})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "profile", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var summaries []profileSummary
	if err := json.Unmarshal(out.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 || summaries[0].Name != "alpha" || summaries[0].ExpiryStatus != "expired" || summaries[1].ExpiryStatus != "active" || summaries[2].ExpiryStatus != "never" || summaries[2].TLSThumbprint == "" {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestRotateReusesSavedUsernameAndRevokesPreviousToken(t *testing.T) {
	var username, deleted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := any(true)
		switch req.Method {
		case "apiinfo.version":
			result = "7.2.0"
		case "user.login":
			var params map[string]string
			_ = json.Unmarshal(req.Params, &params)
			username, result = params["username"], "session"
		case "token.create":
			result = map[string]any{"tokenids": []string{"new-token"}}
		case "token.generate":
			result = []map[string]string{{"tokenid": "new-token", "token": "new-secret"}}
		case "token.delete":
			var ids []string
			_ = json.Unmarshal(req.Params, &ids)
			deleted = strings.Join(ids, ",")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	secrets := &memorySecrets{values: map[string]string{"production": "old-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"production": {URL: server.URL, Username: "Admin", TokenID: "old-token"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store, Password: func(io.Writer) (string, error) { return "password", nil }})
	cmd.SetArgs([]string{"auth", "rotate", "--profile", "production", "--expires-in", "never"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if username != "Admin" || deleted != "old-token" || secrets.values["production"] != "new-secret" {
		t.Fatalf("username=%q deleted=%q secrets=%#v", username, deleted, secrets.values)
	}
	cfg, err := store.Load()
	if err != nil || cfg.Profiles["production"].Username != "Admin" || cfg.Profiles["production"].TokenID != "new-token" {
		t.Fatalf("profile=%#v err=%v", cfg.Profiles["production"], err)
	}
}

func TestRotatePromptsForUsernameForLegacyProfile(t *testing.T) {
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
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()
	secrets := &memorySecrets{values: map[string]string{"default": "old-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: server.URL, TokenID: "old-token"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store, Password: func(io.Writer) (string, error) { return "password", nil }})
	cmd.SetIn(strings.NewReader("Admin\n"))
	cmd.SetArgs([]string{"auth", "rotate", "--expires-in", "never"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil || cfg.Profiles["default"].Username != "Admin" {
		t.Fatalf("profile=%#v err=%v", cfg.Profiles["default"], err)
	}
}

func TestRotateCleansUpNewTokenWhenSecureStorageFails(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
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
			var ids []string
			_ = json.Unmarshal(req.Params, &ids)
			deleted = append(deleted, ids...)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()
	secrets := &memorySecrets{values: map[string]string{"default": "old-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: server.URL, Username: "Admin", TokenID: "old-token"}}}); err != nil {
		t.Fatal(err)
	}
	secrets.failSet = true
	cmd := NewRootCmd(Dependencies{Store: store, Password: func(io.Writer) (string, error) { return "password", nil }})
	cmd.SetArgs([]string{"auth", "rotate", "--expires-in", "never"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "store API token securely") {
		t.Fatalf("error = %v", err)
	}
	if got := strings.Join(deleted, ","); got != "new-token" {
		t.Fatalf("deleted = %q", got)
	}
	if got := secrets.values["default"]; got != "old-secret" {
		t.Fatalf("secret = %q", got)
	}
}

func TestRemoveRevokesRemoteTokenBeforeRemovingLocalProfile(t *testing.T) {
	var deleted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "token.delete" {
			if got := r.Header.Get("Authorization"); got != "Bearer stored-secret" {
				t.Fatalf("authorization = %q", got)
			}
			var ids []string
			_ = json.Unmarshal(req.Params, &ids)
			deleted = strings.Join(ids, ",")
		}
		result := any(true)
		if req.Method == "apiinfo.version" {
			result = "7.2.0"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()
	secrets := &memorySecrets{values: map[string]string{"production": "stored-secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"production": {URL: server.URL, Username: "Admin", TokenID: "42"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"auth", "remove", "production", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil || deleted != "42" || len(cfg.Profiles) != 0 {
		t.Fatalf("deleted=%q profiles=%#v err=%v", deleted, cfg.Profiles, err)
	}
	if _, ok := secrets.values["production"]; ok {
		t.Fatalf("secret was not removed: %#v", secrets.values)
	}
}

func TestRemoveRejectsNonInteractiveUseWithoutYes(t *testing.T) {
	secrets := &memorySecrets{values: map[string]string{"default": "secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: "https://zabbix.example", TokenID: "42"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"auth", "remove", "default"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "without --yes") {
		t.Fatalf("error = %v", err)
	}
	if _, ok := secrets.values["default"]; !ok {
		t.Fatal("secret was removed")
	}
}

func TestRemoveKeepsLocalProfileWhenRemoteRevocationFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "apiinfo.version" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": "6.0.0", "id": 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": -32602, "message": "Invalid params.", "data": "permission denied"}, "id": 1})
	}))
	defer server.Close()
	secrets := &memorySecrets{values: map[string]string{"default": "secret"}}
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: secrets}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: server.URL, TokenID: "42"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"auth", "remove", "default", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "revoke API token") {
		t.Fatalf("error = %v", err)
	}
	cfg, err := store.Load()
	if err != nil || cfg.Profiles["default"].TokenID != "42" {
		t.Fatalf("profiles=%#v err=%v", cfg.Profiles, err)
	}
	if got := secrets.values["default"]; got != "secret" {
		t.Fatalf("secret = %q", got)
	}
}
