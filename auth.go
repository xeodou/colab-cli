package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/int128/oauth2cli"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultClientID     = "1014160490159-cvot3bea7tgkp72a4m29h20d9ddo6bne.apps.googleusercontent.com"
	defaultClientSecret = "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"
)

// getOAuthConfig returns the OAuth2 config, using environment variables if set.
// Override with COLAB_CLIENT_ID and COLAB_CLIENT_SECRET for custom credentials.
func getOAuthConfig() oauth2.Config {
	clientID := os.Getenv("COLAB_CLIENT_ID")
	if clientID == "" {
		clientID = defaultClientID
	}
	clientSecret := os.Getenv("COLAB_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = defaultClientSecret
	}
	return oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes: []string{
			"profile",
			"email",
			"https://www.googleapis.com/auth/colaboratory",
		},
	}
}

// tokenCachePath returns ~/.config/colab/token.json
func tokenCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "colab", "token.json"), nil
}

// loadCachedToken loads and returns the cached OAuth2 token.
// Returns nil if no cache exists or the token is invalid.
func loadCachedToken() (*oauth2.Token, error) {
	path, err := tokenCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token cache: %w", err)
	}

	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token cache: %w", err)
	}

	return &tok, nil
}

// saveToken persists the token to disk.
func saveToken(tok *oauth2.Token) error {
	path, err := tokenCachePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write token cache: %w", err)
	}

	return nil
}

// getToken returns a valid OAuth2 token, refreshing if needed.
// If no cached token exists, returns an error prompting the user to run `colab auth`.
func getToken(ctx context.Context) (*oauth2.Token, error) {
	tok, err := loadCachedToken()
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return nil, fmt.Errorf("not authenticated. Run: colab auth")
	}

	// Use TokenSource to handle automatic refresh
	cfg := getOAuthConfig()
	ts := cfg.TokenSource(ctx, tok)
	newTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("token refresh failed (re-run: colab auth): %w", err)
	}

	// Save if token was refreshed
	if newTok.AccessToken != tok.AccessToken {
		if err := saveToken(newTok); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cache refreshed token: %v\n", err)
		}
	}

	return newTok, nil
}

// generatePKCE creates a S256 PKCE verifier and challenge pair.
func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// doOAuthLogin performs the browser-based OAuth2 loopback flow.
func doOAuthLogin(ctx context.Context) (*oauth2.Token, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}

	ready := make(chan string, 1)
	cfg := oauth2cli.Config{
		OAuth2Config: getOAuthConfig(),
		AuthCodeOptions: []oauth2.AuthCodeOption{
			oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("prompt", "consent"),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
			oauth2.SetAuthURLParam("code_challenge", challenge),
		},
		TokenRequestOptions: []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("code_verifier", verifier),
		},
		LocalServerReadyChan: ready,
	}

	// Open browser when server is ready
	go func() {
		url := <-ready
		fmt.Printf("Opening browser for authentication...\n")
		fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", url)
		openBrowser(url)
	}()

	tok, err := oauth2cli.GetToken(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("OAuth2 flow: %w", err)
	}

	return tok, nil
}

// tokenStatus returns a human-readable token status string.
func tokenStatus() (string, error) {
	tok, err := loadCachedToken()
	if err != nil {
		return "", err
	}
	if tok == nil {
		return "Not authenticated", nil
	}

	if tok.Expiry.IsZero() {
		return "Authenticated (no expiry set)", nil
	}

	remaining := time.Until(tok.Expiry)
	if remaining <= 0 {
		if tok.RefreshToken != "" {
			return "Token expired (will auto-refresh on next use)", nil
		}
		return "Token expired (re-run: colab auth)", nil
	}

	return fmt.Sprintf("Authenticated (expires in %s)", remaining.Round(time.Second)), nil
}
