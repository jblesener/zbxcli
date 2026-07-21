package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jblesener/zbxcli/internal/profile"
)

func TestHostGetLoadsProfileAndRendersTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "apiinfo.version":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": "7.2.0", "id": 1})
		case "host.get":
			if got := r.Header.Get("Authorization"); got != "Bearer stored-token" {
				t.Fatalf("Authorization = %q", got)
			}
			if request.Params["limit"] != float64(3) {
				t.Fatalf("limit = %#v", request.Params["limit"])
			}
			search, ok := request.Params["search"].(map[string]any)
			if !ok || search["host"] != "web" || search["name"] != "web" || request.Params["searchByAny"] != true {
				t.Fatalf("search params = %#v", request.Params)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": []map[string]string{
				{"hostid": "1", "host": "web-01", "name": "Web 01", "status": "0"},
				{"hostid": "2", "host": "web-02", "name": "Web 02", "status": "1"},
			}, "id": 1})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	store := hostTestStore(t, server.URL)
	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"host", "get", "web", "--limit", "3"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, value := range []string{"HOSTID", "web-01", "Web 01", "Monitored", "web-02", "Disabled"} {
		if !strings.Contains(got, value) {
			t.Fatalf("table output %q does not contain %q", got, value)
		}
	}
}

func TestHostGetJSONPreservesRawAPIFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		result := any("7.2.0")
		if request.Method == "host.get" {
			result = []map[string]string{{"hostid": "1", "host": "web-01", "custom_field": "preserved"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	store := hostTestStore(t, server.URL)
	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"host", "get", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var hosts []map[string]string
	if err := json.Unmarshal(out.Bytes(), &hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0]["custom_field"] != "preserved" {
		t.Fatalf("json output = %#v", hosts)
	}
}

func TestWriteHostTableHandlesEmptyResult(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{})
	cmd.SetOut(out)
	if err := writeHostTable(cmd, json.RawMessage("[]")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "HOSTID") || strings.Count(strings.TrimSpace(got), "\n") != 0 {
		t.Fatalf("empty table output = %q", got)
	}
}

func TestHostGetValidatesOptionsAndProfile(t *testing.T) {
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"host", "get", "--limit", "0"})
	if err := cmd.Execute(); err == nil || err.Error() != "--limit must be positive" {
		t.Fatalf("limit error = %v", err)
	}
	cmd = NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"host", "get", "--output", "yaml"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported --output") {
		t.Fatalf("output error = %v", err)
	}
	cmd = NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"host", "get", "--profile", "missing"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("profile error = %v", err)
	}
}

func TestHostGetReportsMissingStoredToken(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: server.URL}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"host", "get"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "load API token") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("token error = %v", err)
	}
}

func hostTestStore(t *testing.T, endpoint string) *profile.Store {
	t.Helper()
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{"default": "stored-token"}}}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: endpoint, APIVersion: "7.2.0"}}}); err != nil {
		t.Fatal(err)
	}
	return store
}
