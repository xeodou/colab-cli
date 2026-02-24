package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Colab API overview:
//  1. OAuth2 auth → get Google access token
//  2. GET  /tun/m/assign → receive XSRF token (or existing assignment)
//  3. POST /tun/m/assign with X-Goog-Colab-Token → get runtime endpoint + proxy token
//  4. Open Jupyter session via runtime proxy URL (POST /api/sessions)
//  5. Execute code over WebSocket (Jupyter wire protocol v5.3)
//  6. Periodic keep-alive every 60s to prevent ~90min idle timeout
//  7. Release via GET+POST /tun/m/unassign (same XSRF pattern)
//
// API reference: reverse-engineered from github.com/googlecolab/colab-vscode

const (
	colabBackendURL   = "https://colab.research.google.com"
	colabGAPIURL      = "https://colab.pa.googleapis.com" // Google API proxy for Colab services
	keepAliveInterval = 60 * time.Second                  // must be < 90min idle timeout

	// clientAgent must be "vscode" — Colab's backend validates this header and
	// rejects unknown user-agents. Taken from the official colab-vscode extension.
	clientAgent = "vscode"
)

// Runtime holds Colab runtime assignment info.
type Runtime struct {
	Endpoint    string `json:"endpoint"`
	ProxyToken  string `json:"proxyToken"`
	ProxyURL    string `json:"proxyUrl"`
	Accelerator string `json:"accelerator"`
	NbHash      string `json:"nbHash"`

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ColabClient interacts with the Colab backend API.
type ColabClient struct {
	token      string
	httpClient *http.Client
}

// NewColabClient creates a new Colab API client.
func NewColabClient(accessToken string) *ColabClient {
	return &ColabClient{
		token:      accessToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// colabRequest executes an HTTP request with standard Colab headers.
func (c *ColabClient) colabRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Colab-Client-Agent", clientAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

// outcomeError maps Colab assignment outcome codes to user-friendly errors.
// Outcomes: 0=undefined (ok), 1=quota denied, 2=quota exceeded, 4=success, 5=denylisted.
func outcomeError(code int) error {
	switch code {
	case 1:
		return fmt.Errorf("GPU quota denied — try again later or upgrade to Colab Pro")
	case 2:
		return fmt.Errorf("GPU quota exceeded — you've used too much GPU time recently")
	case 5:
		return fmt.Errorf("account denylisted from GPU access")
	}
	return nil
}

// stripXSSI removes the XSSI prefix ")]}'\n" from Colab responses.
// Colab prepends this to all JSON responses as a JSON hijacking countermeasure.
func stripXSSI(data []byte) []byte {
	prefix := []byte(")]}'\n")
	if len(data) > len(prefix) && string(data[:len(prefix)]) == string(prefix) {
		return data[len(prefix):]
	}
	return data
}

// uuidToNbHash converts a UUID to Colab's notebook hash (nbh) format.
// Colab uses a custom encoding: dashes → underscores, padded with dots to 44 chars.
// Example: "550e8400-e29b-41d4-a716-446655440000" → "550e8400_e29b_41d4_a716_446655440000.."
func uuidToNbHash(u string) string {
	s := strings.ReplaceAll(u, "-", "_")
	for len(s) < 44 {
		s += "."
	}
	return s
}

// AssignRuntime requests a Colab GPU runtime.
func (c *ColabClient) AssignRuntime(ctx context.Context, gpu string) (*Runtime, error) {
	nbHash := uuidToNbHash(uuid.New().String())

	params := fmt.Sprintf("?nbh=%s&variant=GPU&accelerator=%s&authuser=0", nbHash, strings.ToUpper(gpu))
	assignURL := colabBackendURL + "/tun/m/assign" + params

	// Step 1: GET to obtain XSRF token (or existing assignment)
	resp, err := c.colabRequest(ctx, "GET", assignURL, nil)
	if err != nil {
		return nil, fmt.Errorf("XSRF GET: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("XSRF GET failed (status %d): %s", resp.StatusCode, stripXSSI(body))
	}

	cleaned := stripXSSI(body)

	// Try parsing as existing assignment first
	var existingAssignment assignPostResponse
	if err := json.Unmarshal(cleaned, &existingAssignment); err == nil && existingAssignment.RuntimeProxyInfo.Token != "" {
		// Already assigned — reuse
		rt := &Runtime{
			Endpoint:    existingAssignment.Endpoint,
			ProxyToken:  existingAssignment.RuntimeProxyInfo.Token,
			ProxyURL:    existingAssignment.RuntimeProxyInfo.URL,
			Accelerator: existingAssignment.Accelerator,
			NbHash:      nbHash,
		}
		kaCtx, cancel := context.WithCancel(ctx)
		rt.cancel = cancel
		rt.wg.Add(1)
		go c.keepAlive(kaCtx, rt)
		return rt, nil
	}

	// Parse as XSRF token response
	var xsrfResp struct {
		Token   string `json:"token"`
		Acc     string `json:"acc"`
		Nbh     string `json:"nbh"`
		Variant string `json:"variant"`
	}
	if err := json.Unmarshal(cleaned, &xsrfResp); err != nil {
		return nil, fmt.Errorf("parse XSRF response: %w (body: %s)", err, cleaned)
	}

	if xsrfResp.Token == "" {
		return nil, fmt.Errorf("no XSRF token in response: %s", cleaned)
	}

	// Step 2: POST with XSRF token
	req, err := http.NewRequestWithContext(ctx, "POST", assignURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create assign request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Colab-Client-Agent", clientAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Goog-Colab-Token", xsrfResp.Token)

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("assign POST: %w", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cleaned = stripXSSI(body)
		// Check for quota/outcome errors
		var errResp struct {
			Outcome int `json:"outcome"`
		}
		if json.Unmarshal(cleaned, &errResp) == nil {
			if err := outcomeError(errResp.Outcome); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("assign failed (status %d): %s", resp.StatusCode, cleaned)
	}

	var assignment assignPostResponse
	if err := json.Unmarshal(stripXSSI(body), &assignment); err != nil {
		return nil, fmt.Errorf("parse assignment: %w", err)
	}

	// Check outcome
	if err := outcomeError(assignment.Outcome); err != nil {
		return nil, err
	}

	if assignment.Endpoint == "" {
		return nil, fmt.Errorf("no endpoint in assignment response: %s", stripXSSI(body))
	}

	rt := &Runtime{
		Endpoint:    assignment.Endpoint,
		ProxyToken:  assignment.RuntimeProxyInfo.Token,
		ProxyURL:    assignment.RuntimeProxyInfo.URL,
		Accelerator: assignment.Accelerator,
		NbHash:      nbHash,
	}

	// Start keep-alive
	kaCtx, cancel := context.WithCancel(ctx)
	rt.cancel = cancel
	rt.wg.Add(1)
	go c.keepAlive(kaCtx, rt)

	return rt, nil
}

type assignPostResponse struct {
	Endpoint    string `json:"endpoint"`
	Accelerator string `json:"accelerator"`
	Outcome     int    `json:"outcome"` // 0=undefined, 1=quota_denied, 2=quota_exceeded, 4=success, 5=denylisted
	Fit         int    `json:"fit"`     // frontend idle timeout in seconds
	Sub         int    `json:"sub"`     // subscription state: 1=unsubscribed, 2=recurring, 3=non-recurring
	SubTier     int    `json:"subTier"` // 0=none, 1=pro, 2=pro+
	RuntimeProxyInfo struct {
		Token              string `json:"token"`
		TokenExpiresInSecs int    `json:"tokenExpiresInSeconds"`
		URL                string `json:"url"`
	} `json:"runtimeProxyInfo"`
}

// keepAlive sends periodic keep-alive requests to prevent runtime idle timeout.
func (c *ColabClient) keepAlive(ctx context.Context, rt *Runtime) {
	defer rt.wg.Done()
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			kaURL := colabBackendURL + "/tun/m/" + rt.Endpoint + "/keep-alive/"
			req, err := http.NewRequestWithContext(ctx, "GET", kaURL, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+c.token)
			req.Header.Set("X-Colab-Client-Agent", clientAgent)
			req.Header.Set("X-Colab-Tunnel", "Google")
			req.Header.Set("X-Colab-Runtime-Proxy-Token", rt.ProxyToken)
			resp, err := c.httpClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
		}
	}
}

// UnassignRuntime releases the Colab runtime using the XSRF GET+POST pattern.
func (c *ColabClient) UnassignRuntime(ctx context.Context, rt *Runtime) error {
	// Stop keep-alive
	if rt.cancel != nil {
		rt.cancel()
		rt.wg.Wait()
	}

	unassignURL := colabBackendURL + "/tun/m/unassign/" + rt.Endpoint + "?authuser=0"

	// Step 1: GET to obtain XSRF token
	resp, err := c.colabRequest(ctx, "GET", unassignURL, nil)
	if err != nil {
		return fmt.Errorf("unassign XSRF GET: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unassign XSRF GET failed (status %d): %s", resp.StatusCode, stripXSSI(body))
	}

	var xsrfResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(stripXSSI(body), &xsrfResp); err != nil {
		return fmt.Errorf("parse unassign XSRF: %w", err)
	}

	// Step 2: POST with XSRF token
	req, err := http.NewRequestWithContext(ctx, "POST", unassignURL, nil)
	if err != nil {
		return fmt.Errorf("create unassign request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Colab-Client-Agent", clientAgent)
	req.Header.Set("Accept", "application/json")
	if xsrfResp.Token != "" {
		req.Header.Set("X-Goog-Colab-Token", xsrfResp.Token)
	}

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unassign POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unassign failed (status %d): %s", resp.StatusCode, stripXSSI(body))
	}

	return nil
}

// ListAssignments returns currently assigned runtimes.
func (c *ColabClient) ListAssignments(ctx context.Context) ([]assignPostResponse, error) {
	url := colabBackendURL + "/tun/m/assignments?authuser=0"
	resp, err := c.colabRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list assignments failed (status %d): %s", resp.StatusCode, stripXSSI(body))
	}

	cleaned := stripXSSI(body)

	// Response could be an array or an object wrapping assignments
	var assignments []assignPostResponse
	if err := json.Unmarshal(cleaned, &assignments); err != nil {
		// Try as a wrapper object with various possible keys
		var wrapper map[string]json.RawMessage
		if err2 := json.Unmarshal(cleaned, &wrapper); err2 != nil {
			return nil, fmt.Errorf("parse assignments: %w (body: %s)", err, cleaned)
		}
		// Try common keys
		for _, key := range []string{"assignments", "servers", "items"} {
			if raw, ok := wrapper[key]; ok {
				if json.Unmarshal(raw, &assignments) == nil {
					return assignments, nil
				}
			}
		}
		// If it's a single assignment object, wrap in array
		var single assignPostResponse
		if json.Unmarshal(cleaned, &single) == nil && single.Endpoint != "" {
			return []assignPostResponse{single}, nil
		}
		return nil, fmt.Errorf("parse assignments: unexpected format: %s", cleaned)
	}

	return assignments, nil
}

// RefreshProxyToken refreshes the runtime proxy token via the GAPI endpoint.
func (c *ColabClient) RefreshProxyToken(ctx context.Context, rt *Runtime) error {
	url := fmt.Sprintf("%s/v1/runtime-proxy-token?endpoint=%s&port=8080", colabGAPIURL, rt.Endpoint)
	resp, err := c.colabRequest(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("refresh proxy token: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh proxy token failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		Token    string `json:"token"`
		TokenTTL string `json:"tokenTtl"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parse proxy token: %w", err)
	}

	rt.ProxyToken = tokenResp.Token
	if tokenResp.URL != "" {
		rt.ProxyURL = tokenResp.URL
	}

	return nil
}

// UserInfo holds Colab account and quota information.
type UserInfo struct {
	Tier           string    `json:"tier"`
	PaidBalance    float64   `json:"paid_balance"`
	BurnRate       float64   `json:"burn_rate_hourly"`
	ActiveRuntimes int       `json:"active_runtimes"`
	FreeRemaining  float64   `json:"free_remaining"`
	FreeRefillTime time.Time `json:"free_refill_time,omitempty"`
	EligibleGPUs   []string  `json:"eligible_gpus"`
	EligibleTPUs   []string  `json:"eligible_tpus"`
	IneligibleGPUs []string  `json:"ineligible_gpus,omitempty"`
	IneligibleTPUs []string  `json:"ineligible_tpus,omitempty"`
}

// GetUserInfo retrieves account tier, quota, and eligible accelerators.
func (c *ColabClient) GetUserInfo(ctx context.Context) (*UserInfo, error) {
	url := colabGAPIURL + "/v1/user-info?get_ccu_consumption_info=true"
	resp, err := c.colabRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user info failed (status %d): %s", resp.StatusCode, body)
	}

	// GAPI responses may or may not have XSSI prefix
	cleaned := stripXSSI(body)

	var raw struct {
		SubscriptionTier     string  `json:"subscriptionTier"`
		PaidBalance          float64 `json:"paidComputeUnitsBalance"`
		BurnRate             float64 `json:"consumptionRateHourly"`
		AssignmentsCount     int     `json:"assignmentsCount"`
		EligibleAccelerators []struct {
			Variant string   `json:"variant"`
			Models  []string `json:"models"`
		} `json:"eligibleAccelerators"`
		IneligibleAccelerators []struct {
			Variant string   `json:"variant"`
			Models  []string `json:"models"`
		} `json:"ineligibleAccelerators"`
		FreeCCUQuotaInfo *struct {
			RemainingTokens     string `json:"remainingTokens"`
			NextRefillTimestamp int64  `json:"nextRefillTimestampSec"`
		} `json:"freeCcuQuotaInfo"`
	}
	if err := json.Unmarshal(cleaned, &raw); err != nil {
		return nil, fmt.Errorf("parse user info: %w", err)
	}

	// Map subscription tier
	tier := "Free"
	switch raw.SubscriptionTier {
	case "SUBSCRIPTION_TIER_PRO":
		tier = "Pro"
	case "SUBSCRIPTION_TIER_PRO_PLUS":
		tier = "Pro+"
	}

	info := &UserInfo{
		Tier:           tier,
		PaidBalance:    raw.PaidBalance,
		BurnRate:       raw.BurnRate,
		ActiveRuntimes: raw.AssignmentsCount,
	}

	// Free quota (tokens are in milli-CCUs)
	if raw.FreeCCUQuotaInfo != nil {
		var milliCCU float64
		fmt.Sscanf(raw.FreeCCUQuotaInfo.RemainingTokens, "%f", &milliCCU)
		info.FreeRemaining = milliCCU / 1000.0
		if raw.FreeCCUQuotaInfo.NextRefillTimestamp > 0 {
			info.FreeRefillTime = time.Unix(raw.FreeCCUQuotaInfo.NextRefillTimestamp, 0)
		}
	}

	// Parse eligible/ineligible accelerators
	for _, ea := range raw.EligibleAccelerators {
		switch ea.Variant {
		case "VARIANT_GPU":
			info.EligibleGPUs = append(info.EligibleGPUs, ea.Models...)
		case "VARIANT_TPU":
			info.EligibleTPUs = append(info.EligibleTPUs, ea.Models...)
		}
	}
	for _, ia := range raw.IneligibleAccelerators {
		switch ia.Variant {
		case "VARIANT_GPU":
			info.IneligibleGPUs = append(info.IneligibleGPUs, ia.Models...)
		case "VARIANT_TPU":
			info.IneligibleTPUs = append(info.IneligibleTPUs, ia.Models...)
		}
	}

	return info, nil
}

// RuntimeStatus holds runtime status information.
type RuntimeStatus struct {
	GPU       string `json:"gpu"`
	MemoryMB  int    `json:"memory_mb"`
	IdleSecs  int    `json:"idle_seconds"`
	Connected bool   `json:"connected"`
}

// GetStatus queries the runtime for status information.
func (c *ColabClient) GetStatus(ctx context.Context, rt *Runtime) (*RuntimeStatus, error) {
	code := `
import json, subprocess
try:
    import torch
    gpu = torch.cuda.get_device_name(0) if torch.cuda.is_available() else "No GPU"
    mem = torch.cuda.get_device_properties(0).total_mem // (1024*1024) if torch.cuda.is_available() else 0
except:
    gpu, mem = "Unknown", 0

result = subprocess.run(['cat', '/proc/uptime'], capture_output=True, text=True)
idle = int(float(result.stdout.split()[1])) if result.returncode == 0 else 0
print(json.dumps({"gpu": gpu, "memory_mb": mem, "idle_seconds": idle, "connected": True}))
`
	kc, err := NewKernelClient(ctx, rt)
	if err != nil {
		return nil, err
	}
	defer kc.Close()

	output, err := kc.Execute(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("status query: %w", err)
	}

	var status RuntimeStatus
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			if err := json.Unmarshal([]byte(line), &status); err == nil {
				return &status, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to parse status from output: %s", output)
}
