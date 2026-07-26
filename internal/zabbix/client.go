package zabbix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
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
	"sync"
	"time"
)

type Client struct {
	endpoint      string
	http          *http.Client
	version       Version
	tlsMu         sync.Mutex
	tlsThumbprint string
}

// TLSOptions controls trust for HTTPS endpoints. A thumbprint pins the leaf
// certificate and replaces normal CA verification; InsecureSkipVerify is only
// appropriate while establishing a new pin for a trusted private endpoint.
type TLSOptions struct {
	Thumbprint         string
	InsecureSkipVerify bool
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

func New(endpoint string, tlsOptions TLSOptions) (*Client, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	pinned, err := normalizeThumbprint(tlsOptions.Thumbprint)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	client := &Client{endpoint: endpoint, http: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
	if pinned != "" || tlsOptions.InsecureSkipVerify {
		transport.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- a saved pin or explicit bootstrap verifies the peer below.
	}
	transport.TLSClientConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("TLS peer presented no certificates")
		}
		observed := certificateThumbprint(state.PeerCertificates[0].Raw)
		if pinned != "" && observed != pinned {
			return fmt.Errorf("TLS certificate thumbprint mismatch: got %s, want %s", observed, pinned)
		}
		client.tlsMu.Lock()
		client.tlsThumbprint = observed
		client.tlsMu.Unlock()
		return nil
	}
	return client, nil
}

func normalizeThumbprint(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "", nil
	}
	const prefix = "sha256:"
	if !strings.HasPrefix(raw, prefix) || len(raw) != len(prefix)+sha256.Size*2 {
		return "", fmt.Errorf("invalid TLS certificate thumbprint %q", raw)
	}
	for _, r := range raw[len(prefix):] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return "", fmt.Errorf("invalid TLS certificate thumbprint %q", raw)
		}
	}
	return raw, nil
}

func certificateThumbprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum)
}

// TLSThumbprint returns the leaf certificate fingerprint observed during a
// successful HTTPS connection, or an empty string for HTTP connections.
func (c *Client) TLSThumbprint() string {
	c.tlsMu.Lock()
	defer c.tlsMu.Unlock()
	return c.tlsThumbprint
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

// MaintenanceTarget identifies a host or host group assigned to maintenance.
type MaintenanceTarget struct {
	ID   string `json:"hostid,omitempty"`
	Name string `json:"name"`
	Host string `json:"host,omitempty"`
}

type MaintenanceGroup struct {
	ID   string `json:"groupid"`
	Name string `json:"name"`
}

// Maintenance is the subset of a Zabbix maintenance record shown by the CLI.
type Maintenance struct {
	ID              string              `json:"maintenanceid"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	MaintenanceType string              `json:"maintenance_type"`
	ActiveSince     string              `json:"active_since"`
	ActiveTill      string              `json:"active_till"`
	Hosts           []MaintenanceTarget `json:"hosts"`
	Groups          []MaintenanceGroup  `json:"groups"`
}

// MaintenanceCreate contains one one-time, data-collecting maintenance window.
type MaintenanceCreate struct {
	Name, Description string
	Start, End        time.Time
	HostIDs, GroupIDs []string
}

func (c *Client) FindHost(ctx context.Context, token, name string) ([]MaintenanceTarget, error) {
	raw, err := c.call(ctx, "host.get", map[string]any{
		"output":      []string{"hostid", "host", "name"},
		"search":      map[string]string{"host": name, "name": name},
		"searchByAny": true,
	}, token)
	if err != nil {
		return nil, err
	}
	var records []MaintenanceTarget
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("decode host lookup: %w", err)
	}
	matched := records[:0]
	for _, record := range records {
		if record.Host == name || record.Name == name {
			matched = append(matched, record)
		}
	}
	return matched, nil
}

func (c *Client) FindHostGroup(ctx context.Context, token, name string) ([]MaintenanceTarget, error) {
	raw, err := c.call(ctx, "hostgroup.get", map[string]any{
		"output": []string{"groupid", "name"},
		"search": map[string]string{"name": name},
	}, token)
	if err != nil {
		return nil, err
	}
	var wire []struct {
		ID   string `json:"groupid"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode host group lookup: %w", err)
	}
	matched := make([]MaintenanceTarget, 0, len(wire))
	for _, record := range wire {
		if record.Name == name {
			matched = append(matched, MaintenanceTarget{ID: record.ID, Name: record.Name})
		}
	}
	return matched, nil
}

func (c *Client) CreateMaintenance(ctx context.Context, token string, input MaintenanceCreate) (string, error) {
	if strings.TrimSpace(input.Name) == "" {
		return "", errors.New("maintenance name is required")
	}
	if !input.End.After(input.Start) {
		return "", errors.New("maintenance end must be after its start")
	}
	if len(input.HostIDs) == 0 && len(input.GroupIDs) == 0 {
		return "", errors.New("maintenance requires at least one host or host group")
	}
	period := int64(input.End.Sub(input.Start).Seconds())
	params := map[string]any{
		"name":             input.Name,
		"description":      input.Description,
		"active_since":     input.Start.Unix(),
		"active_till":      input.End.Unix(),
		"maintenance_type": 0,
		"timeperiods": []map[string]any{{
			"timeperiod_type": 0,
			"start_date":      input.Start.Unix(),
			"period":          period,
		}},
	}
	if len(input.HostIDs) > 0 {
		hosts := make([]map[string]string, 0, len(input.HostIDs))
		for _, id := range input.HostIDs {
			hosts = append(hosts, map[string]string{"hostid": id})
		}
		params["hosts"] = hosts
	}
	if len(input.GroupIDs) > 0 {
		groups := make([]map[string]string, 0, len(input.GroupIDs))
		for _, id := range input.GroupIDs {
			groups = append(groups, map[string]string{"groupid": id})
		}
		params["groups"] = groups
	}
	raw, err := c.call(ctx, "maintenance.create", params, token)
	if err != nil {
		return "", err
	}
	var created struct {
		IDs []string `json:"maintenanceids"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("decode maintenance creation: %w", err)
	}
	if len(created.IDs) != 1 || created.IDs[0] == "" {
		return "", errors.New("Zabbix API returned no maintenance ID")
	}
	return created.IDs[0], nil
}

func (c *Client) ListMaintenances(ctx context.Context, token string, limit int) ([]Maintenance, error) {
	raw, err := c.ListMaintenancesRaw(ctx, token, limit)
	if err != nil {
		return nil, err
	}
	var records []Maintenance
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("decode maintenance list: %w", err)
	}
	return records, nil
}

// ListMaintenancesRaw retrieves complete maintenance records for JSON output.
func (c *Client) ListMaintenancesRaw(ctx context.Context, token string, limit int) (json.RawMessage, error) {
	if limit <= 0 {
		return nil, errors.New("maintenance list limit must be positive")
	}
	raw, err := c.call(ctx, "maintenance.get", map[string]any{
		"output":            "extend",
		"selectHosts":       "extend",
		"selectGroups":      "extend",
		"selectTags":        "extend",
		"selectTimeperiods": "extend",
		"sortfield":         "name",
		"sortorder":         "ASC",
		"limit":             limit,
	}, token)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) DeleteMaintenances(ctx context.Context, token string, ids []string) error {
	if len(ids) == 0 {
		return errors.New("at least one maintenance ID is required")
	}
	_, err := c.call(ctx, "maintenance.delete", ids, token)
	return err
}
