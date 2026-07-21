package zabbix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
