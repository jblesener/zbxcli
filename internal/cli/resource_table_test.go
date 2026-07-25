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

func TestGetHostQueryLoadsProfileAndRendersTable(t *testing.T) {
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

	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: hostTestStore(t, server.URL)})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"get", "host", "web", "--limit", "3"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"HOSTID", "web-01", "Web 01", "Monitored", "web-02", "Disabled"} {
		if !strings.Contains(out.String(), value) {
			t.Fatalf("table output %q does not contain %q", out.String(), value)
		}
	}
}

func TestResourceTableSchemasAndEmptyResult(t *testing.T) {
	tests := map[string]string{
		"host":              "HOSTID",
		"host-group":        "GROUPID",
		"host-interface":    "INTERFACEID",
		"template":          "TEMPLATEID",
		"item":              "ITEMID",
		"trigger":           "TRIGGERID",
		"graph":             "GRAPHID",
		"discovery-rule":    "ITEMID",
		"item-prototype":    "ITEMID",
		"trigger-prototype": "TRIGGERID",
		"graph-prototype":   "GRAPHID",
		"host-prototype":    "DISCOVERYID",
		"web-scenario":      "HTTPTESTID",
	}
	for resource, header := range tests {
		t.Run(resource, func(t *testing.T) {
			out := &bytes.Buffer{}
			cmd := NewRootCmd(Dependencies{})
			cmd.SetOut(out)
			if err := writeResourceTable(cmd, resource, json.RawMessage("[]")); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); !strings.Contains(got, header) || strings.Count(strings.TrimSpace(got), "\n") != 0 {
				t.Fatalf("empty table output = %q", got)
			}
		})
	}
}

func TestHostCommandIsNotRegistered(t *testing.T) {
	cmd := NewRootCmd(Dependencies{})
	cmd.SetArgs([]string{"host", "get"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command \"host\"") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetReportsMissingStoredToken(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
	if err := store.Save(profile.Config{Profiles: map[string]profile.Profile{"default": {URL: server.URL}}}); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd(Dependencies{Store: store})
	cmd.SetArgs([]string{"get", "host"})
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
