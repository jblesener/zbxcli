package zabbix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestGetHostsUsesSearchAndVersionAppropriateAuthorization(t *testing.T) {
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
					Method string                     `json:"method"`
					Params map[string]json.RawMessage `json:"params"`
					Auth   *string                    `json:"auth"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				switch request.Method {
				case "apiinfo.version":
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": tc.version, "id": 1})
				case "host.get":
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
					var search map[string]string
					if err := json.Unmarshal(request.Params["search"], &search); err != nil {
						t.Fatal(err)
					}
					if got, want := search, map[string]string{"host": "web", "name": "web"}; !equalStringMaps(got, want) {
						t.Fatalf("search = %#v, want %#v", got, want)
					}
					var limit int
					if err := json.Unmarshal(request.Params["limit"], &limit); err != nil || limit != 7 {
						t.Fatalf("limit = %d, err = %v", limit, err)
					}
					var output, sortField, sortOrder string
					_ = json.Unmarshal(request.Params["output"], &output)
					_ = json.Unmarshal(request.Params["sortfield"], &sortField)
					_ = json.Unmarshal(request.Params["sortorder"], &sortOrder)
					if output != "extend" || sortField != "name" || sortOrder != "ASC" {
						t.Fatalf("output=%q sortfield=%q sortorder=%q", output, sortField, sortOrder)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": []map[string]string{{"hostid": "42", "host": "web-01", "name": "Web 01", "status": "0", "custom": "value"}}, "id": 1})
				default:
					t.Fatalf("unexpected method %q", request.Method)
				}
			}))
			defer server.Close()

			client, err := New(server.URL, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			raw, err := client.GetHosts(context.Background(), "token", HostQuery{Search: " web ", Limit: 7})
			if err != nil {
				t.Fatal(err)
			}
			var hosts []map[string]string
			if err := json.Unmarshal(raw, &hosts); err != nil {
				t.Fatal(err)
			}
			if len(hosts) != 1 || hosts[0]["custom"] != "value" {
				t.Fatalf("raw hosts = %#v", hosts)
			}
		})
	}
}

func TestGetHostsRejectsNonPositiveLimit(t *testing.T) {
	client := &Client{}
	if _, err := client.GetHosts(context.Background(), "token", HostQuery{}); err == nil || err.Error() != "host query limit must be positive" {
		t.Fatalf("GetHosts error = %v", err)
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

func equalStringMaps(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			return false
		}
	}
	return true
}
