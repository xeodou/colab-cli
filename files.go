package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileClient handles Jupyter Contents API file operations.
type FileClient struct {
	endpoint   string
	proxyToken string
	httpClient *http.Client
}

// NewFileClient creates a file operations client for a runtime.
func NewFileClient(rt *Runtime) *FileClient {
	return &FileClient{
		endpoint:   rt.ProxyURL,
		proxyToken: rt.ProxyToken,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// Upload sends a local file to the Colab runtime.
func (fc *FileClient) Upload(ctx context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}

	if remotePath == "" {
		remotePath = filepath.Base(localPath)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	body := map[string]interface{}{
		"content": encoded,
		"format":  "base64",
		"type":    "file",
		"name":    filepath.Base(remotePath),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal upload body: %w", err)
	}

	url := fc.endpoint + "/api/contents/" + strings.TrimPrefix(remotePath, "/")
	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("X-Colab-Runtime-Proxy-Token", fc.proxyToken)
	req.Header.Set("X-Colab-Client-Agent", clientAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// Download retrieves a file from the Colab runtime to a local path.
func (fc *FileClient) Download(ctx context.Context, remotePath, localPath string) error {
	url := fc.endpoint + "/api/contents/" + strings.TrimPrefix(remotePath, "/") + "?content=1"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("X-Colab-Runtime-Proxy-Token", fc.proxyToken)
	req.Header.Set("X-Colab-Client-Agent", clientAgent)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (status %d): %s", resp.StatusCode, respBody)
	}

	var contentsResp struct {
		Content string `json:"content"`
		Format  string `json:"format"`
		Type    string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&contentsResp); err != nil {
		return fmt.Errorf("parse download response: %w", err)
	}

	var fileData []byte
	switch contentsResp.Format {
	case "base64":
		fileData, err = base64.StdEncoding.DecodeString(contentsResp.Content)
		if err != nil {
			return fmt.Errorf("decode base64 content: %w", err)
		}
	case "text":
		fileData = []byte(contentsResp.Content)
	default:
		return fmt.Errorf("unsupported format: %s", contentsResp.Format)
	}

	if localPath == "" {
		localPath = filepath.Base(remotePath)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create local directory: %w", err)
	}

	if err := os.WriteFile(localPath, fileData, 0644); err != nil {
		return fmt.Errorf("write local file: %w", err)
	}

	return nil
}
