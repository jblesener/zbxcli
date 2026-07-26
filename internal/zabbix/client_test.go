package zabbix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTokenWorkflowUsesVersionAppropriateAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		bearer        bool
	}{
		{name: "6.0 auth property", version: "6.0.0"},
		{name: "7.2 bearer header", version: "7.2.0", bearer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string  `json:"method"`
					Auth   *string `json:"auth"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				methods = append(methods, request.Method)
				if request.Method != "apiinfo.version" && request.Method != "user.login" {
					if tc.bearer {
						if got := r.Header.Get("Authorization"); got != "Bearer session" {
							t.Fatalf("Authorization = %q", got)
						}
						if request.Auth != nil {
							t.Fatalf("unexpected auth property: %q", *request.Auth)
						}
					} else if request.Auth == nil || *request.Auth != "session" {
						t.Fatalf("auth = %#v", request.Auth)
					}
				}
				result := any(nil)
				switch request.Method {
				case "apiinfo.version":
					result = tc.version
				case "user.login":
					result = "session"
				case "token.create":
					result = map[string]any{"tokenids": []string{"42"}}
				case "token.generate":
					result = []map[string]string{{"tokenid": "42", "token": "secret"}}
				case "user.logout":
					result = true
				default:
					t.Fatalf("unexpected method %q", request.Method)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
			}))
			defer server.Close()
			client, err := New(server.URL, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			session, err := client.Login(context.Background(), "admin", "password")
			if err != nil {
				t.Fatal(err)
			}
			issued, err := client.CreateToken(context.Background(), session, "zbxcli/default", nil)
			if err != nil {
				t.Fatal(err)
			}
			if issued.ID != "42" || issued.Value != "secret" {
				t.Fatalf("issued = %#v", issued)
			}
			if err := client.Logout(context.Background(), session); err != nil {
				t.Fatal(err)
			}
			if got, want := len(methods), 5; got != want {
				t.Fatalf("method count = %d, want %d", got, want)
			}
		})
	}
}

func TestDeleteTokenUsesVersionAppropriateAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		bearer        bool
	}{
		{name: "6.0 auth property", version: "6.0.0"},
		{name: "7.2 bearer header", version: "7.2.0", bearer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string  `json:"method"`
					Auth   *string `json:"auth"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Method != "token.delete" {
					t.Fatalf("method = %q", request.Method)
				}
				if tc.bearer {
					if got := r.Header.Get("Authorization"); got != "Bearer token" {
						t.Fatalf("Authorization = %q", got)
					}
					if request.Auth != nil {
						t.Fatalf("unexpected auth property: %q", *request.Auth)
					}
				} else if request.Auth == nil || *request.Auth != "token" {
					t.Fatalf("auth = %#v", request.Auth)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": map[string][]string{"tokenids": {"42"}}, "id": 1})
			}))
			defer server.Close()

			client, err := New(server.URL, false)
			if err != nil {
				t.Fatal(err)
			}
			version, err := ParseVersion(tc.version)
			if err != nil {
				t.Fatal(err)
			}
			client.version = version
			if err := client.DeleteToken(context.Background(), "token", "42"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://zabbix.example", "https://zabbix.example/api_jsonrpc.php"},
		{"https://zabbix.example/zabbix/", "https://zabbix.example/zabbix/api_jsonrpc.php"},
		{"https://zabbix.example/zabbix/api_jsonrpc.php", "https://zabbix.example/zabbix/api_jsonrpc.php"},
	} {
		got, err := NormalizeEndpoint(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("NormalizeEndpoint(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDiscoverRejectsUnsupportedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": "5.4.0", "id": 1})
	}))
	defer server.Close()
	client, _ := New(server.URL, false)
	if _, err := client.Discover(context.Background()); err == nil {
		t.Fatal("Discover unexpectedly succeeded")
	}
}

func TestGetResourceUsesRegisteredMethodAndPreservesParams(t *testing.T) {
	expected := map[string]string{
		"host": "host.get", "host-group": "hostgroup.get", "host-interface": "hostinterface.get",
		"template": "template.get", "item": "item.get", "trigger": "trigger.get", "graph": "graph.get",
		"discovery-rule": "discoveryrule.get", "item-prototype": "itemprototype.get",
		"trigger-prototype": "triggerprototype.get", "graph-prototype": "graphprototype.get",
		"host-prototype": "hostprototype.get", "web-scenario": "httptest.get",
	}
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		if request.Params["output"] != "extend" || request.Params["limit"] != float64(3) {
			t.Fatalf("params = %#v", request.Params)
		}
		if request.Params["filter"].(map[string]any)["name"] != "example" {
			t.Fatalf("filter = %#v", request.Params["filter"])
		}
		received = append(received, request.Method)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": []map[string]string{{"custom": "preserved"}}, "id": 1})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	client.version = Version{Major: 7, Minor: 2}
	names := ResourceNames()
	for _, resource := range names {
		raw, err := client.GetResource(context.Background(), "token", resource, ResourceQuery{Params: map[string]any{"filter": map[string]string{"name": "example"}}, Limit: 3})
		if err != nil {
			t.Fatalf("GetResource(%q): %v", resource, err)
		}
		var result []map[string]string
		if err := json.Unmarshal(raw, &result); err != nil || len(result) != 1 || result[0]["custom"] != "preserved" {
			t.Fatalf("result for %q = %#v, err = %v", resource, result, err)
		}
	}
	if len(received) != len(names) {
		t.Fatalf("method count = %d, want %d", len(received), len(names))
	}
	for i, resource := range names {
		if received[i] != expected[resource] {
			t.Errorf("method for %q = %q, want %q", resource, received[i], expected[resource])
		}
	}
}

func TestGetResourceRejectsInvalidQuery(t *testing.T) {
	client := &Client{}
	for _, tc := range []struct {
		name, resource string
		query          ResourceQuery
		want           string
	}{
		{name: "unknown resource", resource: "event", query: ResourceQuery{Limit: 1}, want: "unsupported resource"},
		{name: "non-positive limit", resource: "item", query: ResourceQuery{}, want: "limit must be positive"},
		{name: "output parameter", resource: "item", query: ResourceQuery{Params: map[string]any{"output": "count"}, Limit: 1}, want: "managed by the CLI"},
		{name: "limit parameter", resource: "item", query: ResourceQuery{Params: map[string]any{"limit": 10}, Limit: 1}, want: "managed by the CLI"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.GetResource(context.Background(), "token", tc.resource, tc.query)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGetResourceUsesVersionAppropriateAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		bearer        bool
	}{
		{name: "6.0 auth property", version: "6.0.0"},
		{name: "7.2 bearer header", version: "7.2.0", bearer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string  `json:"method"`
					Auth   *string `json:"auth"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Method != "hostgroup.get" {
					t.Fatalf("method = %q", request.Method)
				}
				if tc.bearer {
					if got := r.Header.Get("Authorization"); got != "Bearer token" {
						t.Fatalf("Authorization = %q", got)
					}
					if request.Auth != nil {
						t.Fatalf("unexpected auth property: %q", *request.Auth)
					}
				} else if request.Auth == nil || *request.Auth != "token" {
					t.Fatalf("auth = %#v", request.Auth)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": []any{}, "id": 1})
			}))
			defer server.Close()

			client, err := New(server.URL, false)
			if err != nil {
				t.Fatal(err)
			}
			version, err := ParseVersion(tc.version)
			if err != nil {
				t.Fatal(err)
			}
			client.version = version
			if _, err := client.GetResource(context.Background(), "token", "host-group", ResourceQuery{Limit: 1}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceClientUsesExpectedMethodsAndPayload(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		methods = append(methods, request.Method)
		if request.Method != "apiinfo.version" && r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		result := any(nil)
		switch request.Method {
		case "apiinfo.version":
			result = "7.2.0"
		case "maintenance.create":
			var params map[string]any
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params["maintenance_type"] != float64(0) {
				t.Fatalf("params = %#v", params)
			}
			result = map[string][]string{"maintenanceids": {"42"}}
		case "maintenance.get":
			result = []map[string]any{{"maintenanceid": "42", "name": "Window", "active_since": "1785546000", "active_till": "1785549600", "groups": []map[string]string{{"groupid": "7", "name": "Databases"}}}}
		case "maintenance.delete":
			result = []string{"42"}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	if id, err := client.CreateMaintenance(context.Background(), "token", MaintenanceCreate{Name: "Window", Start: start, End: start.Add(time.Hour), HostIDs: []string{"10105"}}); err != nil || id != "42" {
		t.Fatalf("create = %q, %v", id, err)
	}
	maintenances, err := client.ListMaintenances(context.Background(), "token", 10)
	if err != nil || len(maintenances) != 1 || maintenances[0].Groups[0].ID != "7" {
		t.Fatalf("list = %#v, %v", maintenances, err)
	}
	if err := client.DeleteMaintenances(context.Background(), "token", []string{"42"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(methods, ","); got != "apiinfo.version,maintenance.create,maintenance.get,maintenance.delete" {
		t.Fatalf("methods = %s", got)
	}
}
