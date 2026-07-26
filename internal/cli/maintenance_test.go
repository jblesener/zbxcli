package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseMaintenanceTime(t *testing.T) {
	got, err := parseMaintenanceTime("2026-08-01T01:00:00+09:00", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Format(time.RFC3339) != "2026-08-01T01:00:00+09:00" {
		t.Fatalf("time = %s", got.Format(time.RFC3339))
	}
	got, err = parseMaintenanceTime("2026-08-01T01:00", "Asia/Tokyo")
	if err != nil || got.Format(time.RFC3339) != "2026-08-01T01:00:00+09:00" {
		t.Fatalf("local time = %s, err = %v", got.Format(time.RFC3339), err)
	}
	for _, tc := range []string{"", "2026-08-01T01:00:30Z", "2026-08-01T01:00"} {
		if _, err := parseMaintenanceTime(tc, "Not/AZone"); err == nil {
			t.Fatalf("parseMaintenanceTime(%q) unexpectedly succeeded", tc)
		}
	}
	if _, err := parseMaintenanceTime("2026-11-01T01:30", "America/New_York"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous DST time error = %v", err)
	}
}

func TestMaintenanceCreateResolvesNamesAndCreatesWindow(t *testing.T) {
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method != "apiinfo.version" && r.Header.Get("Authorization") != "Bearer stored-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		result := any(nil)
		switch request.Method {
		case "apiinfo.version":
			result = "7.2.0"
		case "host.get":
			result = []map[string]string{{"hostid": "10105", "host": "db-01", "name": "Database 01"}}
		case "hostgroup.get":
			result = []map[string]string{{"groupid": "7", "name": "Databases"}}
		case "maintenance.create":
			created = request.Params
			result = map[string][]string{"maintenanceids": {"42"}}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	cmd := NewRootCmd(Dependencies{Store: hostTestStore(t, server.URL)})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"maintenance", "create", "--name", "Database upgrade", "--start", "2026-08-01T01:00:00Z", "--end", "2026-08-01T02:00:00Z", "--host", "db-01", "--host-group", "Databases", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Created maintenance 42.\n" || !strings.Contains(errOut.String(), "Maintenance preview") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if created["maintenance_type"] != float64(0) || created["active_since"] != float64(1785546000) || created["active_till"] != float64(1785549600) {
		t.Fatalf("create params = %#v", created)
	}
	if got := created["hosts"].([]any)[0].(map[string]any)["hostid"]; got != "10105" {
		t.Fatalf("hosts = %#v", created["hosts"])
	}
	if got := created["groups"].([]any)[0].(map[string]any)["groupid"]; got != "7" {
		t.Fatalf("groups = %#v", created["groups"])
	}
}

func TestMaintenanceRejectsUnsafeInput(t *testing.T) {
	cmd := NewRootCmd(Dependencies{})
	cmd.SetIn(strings.NewReader(""))
	if err := confirmMaintenanceMutation(cmd, false); err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("error = %v", err)
	}

	cmd = NewRootCmd(Dependencies{})
	cmd.SetArgs([]string{"maintenance", "create", "--name", "window", "--start", "2026-08-01T02:00:00Z", "--end", "2026-08-01T01:00:00Z", "--host-id", "1", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--end must be after") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaintenanceListJSONPreservesAPIFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		result := any("7.2.0")
		if request.Method == "maintenance.get" {
			result = []map[string]any{{"maintenanceid": "42", "name": "Window", "extra_field": "preserved"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	cmd := NewRootCmd(Dependencies{Store: hostTestStore(t, server.URL)})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"maintenance", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) != 1 || records[0]["extra_field"] != "preserved" {
		t.Fatalf("records = %#v, err = %v", records, err)
	}
}
