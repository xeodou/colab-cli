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

const uploadChunkSize = 3 * 1024 * 1024 // 3MB raw → ~4MB base64, well within WS limits

// KernelUpload writes a local file to the runtime filesystem via kernel execution.
// This is more reliable than the Contents API because it writes directly to the
// kernel's filesystem (typically /content/).
func KernelUpload(ctx context.Context, kc *KernelClient, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}

	if remotePath == "" {
		remotePath = filepath.Base(localPath)
	}

	// For small files, write in one shot
	if len(data) <= uploadChunkSize {
		encoded := base64.StdEncoding.EncodeToString(data)
		code := fmt.Sprintf(
			"import base64, os\nos.makedirs(os.path.dirname(%q) or '.', exist_ok=True)\nwith open(%q, 'wb') as f:\n    f.write(base64.b64decode(%q))\nprint('ok')",
			remotePath, remotePath, encoded,
		)
		output, err := kc.Execute(ctx, code)
		if err != nil {
			return fmt.Errorf("kernel write: %w", err)
		}
		if !strings.Contains(output, "ok") {
			return fmt.Errorf("kernel write failed: %s", output)
		}
		return nil
	}

	// For large files, write in chunks
	// First chunk: create/truncate the file
	for i := 0; i < len(data); i += uploadChunkSize {
		end := i + uploadChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := base64.StdEncoding.EncodeToString(data[i:end])

		mode := "ab" // append
		if i == 0 {
			mode = "wb" // first chunk: create/truncate
		}

		code := fmt.Sprintf(
			"import base64, os\nos.makedirs(os.path.dirname(%q) or '.', exist_ok=True)\nwith open(%q, %q) as f:\n    f.write(base64.b64decode(%q))\nprint('ok')",
			remotePath, remotePath, mode, chunk,
		)
		output, err := kc.Execute(ctx, code)
		if err != nil {
			return fmt.Errorf("kernel write chunk %d: %w", i/uploadChunkSize, err)
		}
		if !strings.Contains(output, "ok") {
			return fmt.Errorf("kernel write chunk %d failed: %s", i/uploadChunkSize, output)
		}
	}

	return nil
}

// KernelDownload reads a file from the runtime filesystem via kernel execution.
func KernelDownload(ctx context.Context, kc *KernelClient, remotePath, localPath string) error {
	// Get file size first
	sizeCode := fmt.Sprintf("import os; print(os.path.getsize(%q))", remotePath)
	sizeOut, err := kc.Execute(ctx, sizeCode)
	if err != nil {
		return fmt.Errorf("get remote file size: %w", err)
	}

	var fileSize int
	if _, err := fmt.Sscanf(strings.TrimSpace(sizeOut), "%d", &fileSize); err != nil {
		return fmt.Errorf("parse file size %q: %w", strings.TrimSpace(sizeOut), err)
	}

	if localPath == "" {
		localPath = filepath.Base(remotePath)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create local directory: %w", err)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer f.Close()

	// Download in chunks
	for offset := 0; offset < fileSize; offset += uploadChunkSize {
		code := fmt.Sprintf(
			"import base64\nwith open(%q, 'rb') as f:\n    f.seek(%d)\n    print(base64.b64encode(f.read(%d)).decode())",
			remotePath, offset, uploadChunkSize,
		)
		output, err := kc.Execute(ctx, code)
		if err != nil {
			return fmt.Errorf("kernel read chunk at offset %d: %w", offset, err)
		}

		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(output))
		if err != nil {
			return fmt.Errorf("decode chunk at offset %d: %w", offset, err)
		}

		if _, err := f.Write(decoded); err != nil {
			return fmt.Errorf("write chunk at offset %d: %w", offset, err)
		}
	}

	return nil
}

// FileClient handles Jupyter Contents API file operations.
// Kept for internal use where Contents API works (e.g., kernel-adjacent operations).
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

// Upload sends a local file to the Colab runtime via Contents API.
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

// Download retrieves a file from the Colab runtime to a local path via Contents API.
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
