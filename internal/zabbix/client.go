package zabbix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	endpoint string
	http     *http.Client
	version  Version
}

type Version struct {
	Major, Minor, Patch int
	Raw                 string
}

func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

func ParseVersion(raw string) (Version, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("invalid API version %q", raw)
	}
	values := [3]int{}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return Version{}, fmt.Errorf("invalid API version %q", raw)
		}
		values[i] = n
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2], Raw: raw}, nil
}

func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Zabbix URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Zabbix URL %q", raw)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("Zabbix URL must use http or https")
	}
	if strings.HasSuffix(u.Path, "/api_jsonrpc.php") {
		return u.String(), nil
	}
	u.Path = path.Join(u.Path, "api_jsonrpc.php")
	return u.String(), nil
}

func New(endpoint string, insecure bool) (*Client, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = insecure // #nosec G402 -- supplied only by explicit CLI flag.
	return &Client{endpoint: endpoint, http: &http.Client{Transport: transport, Timeout: 30 * time.Second}}, nil
}

type rpcRequest struct {
	JSONRPC string  `json:"jsonrpc"`
	Method  string  `json:"method"`
	Params  any     `json:"params"`
	ID      int     `json:"id"`
	Auth    *string `json:"auth,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	ID     int             `json:"id"`
}

func (c *Client) call(ctx context.Context, method string, params any, auth string) (json.RawMessage, error) {
	req := rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1}
	if auth != "" && !c.version.AtLeast(7, 2) {
		req.Auth = &auth
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json-rpc")
	if auth != "" && c.version.AtLeast(7, 2) {
		httpReq.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", method, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("call %s: server returned HTTP %d", method, res.StatusCode)
	}
	var response rpcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("Zabbix API %s failed (%d): %s", method, response.Error.Code, strings.TrimSpace(strings.Join([]string{response.Error.Message, response.Error.Data}, " ")))
	}
	if response.Result == nil {
		return nil, fmt.Errorf("Zabbix API %s returned no result", method)
	}
	return response.Result, nil
}

func (c *Client) Discover(ctx context.Context) (Version, error) {
	raw, err := c.call(ctx, "apiinfo.version", map[string]any{}, "")
	if err != nil {
		return Version{}, err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return Version{}, fmt.Errorf("decode API version: %w", err)
	}
	v, err := ParseVersion(value)
	if err != nil {
		return Version{}, err
	}
	if !v.AtLeast(6, 0) {
		return Version{}, fmt.Errorf("Zabbix API %s is unsupported; zbxcli requires 6.0+", value)
	}
	c.version = v
	return v, nil
}

func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	raw, err := c.call(ctx, "user.login", map[string]string{"username": username, "password": password}, "")
	if err != nil {
		return "", err
	}
	var session string
	if err := json.Unmarshal(raw, &session); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if session == "" {
		return "", errors.New("Zabbix API returned an empty session")
	}
	return session, nil
}

type IssuedToken struct {
	ID, Value string
	ExpiresAt *time.Time
}

func (c *Client) CreateToken(ctx context.Context, session, name string, expiresAt *time.Time) (IssuedToken, error) {
	params := map[string]any{"name": name}
	if expiresAt != nil {
		params["expires_at"] = strconv.FormatInt(expiresAt.Unix(), 10)
	}
	raw, err := c.call(ctx, "token.create", params, session)
	if err != nil {
		return IssuedToken{}, err
	}
	var created struct {
		IDs []string `json:"tokenids"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || len(created.IDs) != 1 {
		if err != nil {
			return IssuedToken{}, fmt.Errorf("decode token creation: %w", err)
		}
		return IssuedToken{}, errors.New("Zabbix API returned no token ID")
	}
	id := created.IDs[0]
	raw, err = c.call(ctx, "token.generate", []string{id}, session)
	if err != nil {
		return IssuedToken{ID: id, ExpiresAt: expiresAt}, err
	}
	var generated []struct {
		ID    string `json:"tokenid"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &generated); err != nil || len(generated) != 1 || generated[0].Token == "" {
		if err != nil {
			return IssuedToken{ID: id, ExpiresAt: expiresAt}, fmt.Errorf("decode token generation: %w", err)
		}
		return IssuedToken{ID: id, ExpiresAt: expiresAt}, errors.New("Zabbix API returned no generated token")
	}
	return IssuedToken{ID: id, Value: generated[0].Token, ExpiresAt: expiresAt}, nil
}

func (c *Client) DeleteToken(ctx context.Context, session, id string) error {
	_, err := c.call(ctx, "token.delete", []string{id}, session)
	return err
}
func (c *Client) Logout(ctx context.Context, session string) error {
	_, err := c.call(ctx, "user.logout", []any{}, session)
	return err
}

type ResourceQuery struct {
	Params map[string]any
	Limit  int
}

var dataCollectionMethods = map[string]string{
	"host":              "host.get",
	"host-group":        "hostgroup.get",
	"host-interface":    "hostinterface.get",
	"template":          "template.get",
	"item":              "item.get",
	"trigger":           "trigger.get",
	"graph":             "graph.get",
	"discovery-rule":    "discoveryrule.get",
	"item-prototype":    "itemprototype.get",
	"trigger-prototype": "triggerprototype.get",
	"graph-prototype":   "graphprototype.get",
	"host-prototype":    "hostprototype.get",
	"web-scenario":      "httptest.get",
}

// ResourceNames returns the supported read-only data-collection resources.
func ResourceNames() []string {
	names := make([]string, 0, len(dataCollectionMethods))
	for name := range dataCollectionMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetResource retrieves a supported data-collection resource. Params are
// resource-specific Zabbix API get parameters; output and limit are managed by
// this client to keep the command's output and pagination behavior consistent.
func (c *Client) GetResource(ctx context.Context, token, resource string, query ResourceQuery) (json.RawMessage, error) {
	method, ok := dataCollectionMethods[strings.ToLower(strings.TrimSpace(resource))]
	if !ok {
		return nil, fmt.Errorf("unsupported resource %q; supported resources: %s", resource, strings.Join(ResourceNames(), ", "))
	}
	if query.Limit <= 0 {
		return nil, errors.New("resource query limit must be positive")
	}
	params := make(map[string]any, len(query.Params)+2)
	for key, value := range query.Params {
		if key == "output" || key == "limit" {
			return nil, fmt.Errorf("resource query parameter %q is managed by the CLI", key)
		}
		params[key] = value
	}
	params["output"] = "extend"
	params["limit"] = query.Limit
	return c.call(ctx, method, params, token)
}
