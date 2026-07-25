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

func TestGetForwardsParamsAndWritesTable(t *testing.T) {
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
		case "item.get":
			if got := r.Header.Get("Authorization"); got != "Bearer stored-token" {
				t.Fatalf("Authorization = %q", got)
			}
			if request.Params["limit"] != float64(3) || request.Params["output"] != "extend" || request.Params["searchByAny"] != true {
				t.Fatalf("params = %#v", request.Params)
			}
			search, ok := request.Params["search"].(map[string]any)
			if !ok || search["name"] != "CPU" {
				t.Fatalf("search = %#v", request.Params["search"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": []map[string]string{{"itemid": "42", "name": "CPU", "key_": "system.cpu.load", "type": "0", "status": "0", "state": "0", "custom": "preserved"}}, "id": 1})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: hostTestStore(t, server.URL)})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"get", "item", "--limit", "3", "--params", `{"search":{"name":"CPU"},"searchByAny":true}`})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"ITEMID", "42", "CPU", "system.cpu.load", "Zabbix agent", "Enabled", "Normal"} {
		if !strings.Contains(out.String(), value) {
			t.Fatalf("table output %q does not contain %q", out.String(), value)
		}
	}
}

func TestGetJSONPreservesRawAPIFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		result := any("7.2.0")
		if request.Method == "item.get" {
			result = []map[string]string{{"itemid": "42", "custom_field": "preserved"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	out := &bytes.Buffer{}
	cmd := NewRootCmd(Dependencies{Store: hostTestStore(t, server.URL)})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"get", "item", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var records []map[string]string
	if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) != 1 || records[0]["custom_field"] != "preserved" {
		t.Fatalf("output = %#v, err = %v", records, err)
	}
}

func TestGetValidatesOptionsAndParams(t *testing.T) {
	store := &profile.Store{Path: filepath.Join(t.TempDir(), "profiles.json"), Secrets: &memorySecrets{values: map[string]string{}}}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "limit", args: []string{"get", "item", "--limit", "0"}, want: "--limit must be positive"},
		{name: "invalid JSON", args: []string{"get", "item", "--params", "["}, want: "parse --params JSON object"},
		{name: "non-object JSON", args: []string{"get", "item", "--params", "[]"}, want: "parse --params JSON object"},
		{name: "null JSON", args: []string{"get", "item", "--params", "null"}, want: "--params must be a JSON object"},
		{name: "reserved output", args: []string{"get", "item", "--params", `{"output":"count"}`}, want: "must not include \"output\""},
		{name: "reserved limit", args: []string{"get", "item", "--params", `{"limit":5}`}, want: "must not include \"limit\""},
		{name: "output", args: []string{"get", "item", "--output", "yaml"}, want: "unsupported --output"},
		{name: "query for non-host", args: []string{"get", "item", "CPU"}, want: "optional query is supported only for the host resource"},
		{name: "query search conflict", args: []string{"get", "host", "web", "--params", `{"search":{"host":"api"}}`}, want: "must not include \"search\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewRootCmd(Dependencies{Store: store})
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
