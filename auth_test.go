package main

import (
	"os"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE() error: %v", err)
	}

	if len(verifier) == 0 {
		t.Error("verifier is empty")
	}
	if len(challenge) == 0 {
		t.Error("challenge is empty")
	}
	if verifier == challenge {
		t.Error("verifier and challenge should differ")
	}

	// Verify uniqueness across calls
	v2, c2, err := generatePKCE()
	if err != nil {
		t.Fatalf("second generatePKCE() error: %v", err)
	}
	if verifier == v2 {
		t.Error("two calls produced the same verifier")
	}
	if challenge == c2 {
		t.Error("two calls produced the same challenge")
	}
}

func TestGetOAuthConfig_Default(t *testing.T) {
	// Clear env vars to ensure defaults
	os.Unsetenv("COLAB_CLIENT_ID")
	os.Unsetenv("COLAB_CLIENT_SECRET")

	cfg := getOAuthConfig()
	if cfg.ClientID != defaultClientID {
		t.Errorf("ClientID = %q, want default", cfg.ClientID)
	}
	if cfg.ClientSecret != defaultClientSecret {
		t.Errorf("ClientSecret = %q, want default", cfg.ClientSecret)
	}
}

func TestGetOAuthConfig_CustomEnv(t *testing.T) {
	os.Setenv("COLAB_CLIENT_ID", "custom-id")
	os.Setenv("COLAB_CLIENT_SECRET", "custom-secret")
	defer func() {
		os.Unsetenv("COLAB_CLIENT_ID")
		os.Unsetenv("COLAB_CLIENT_SECRET")
	}()

	cfg := getOAuthConfig()
	if cfg.ClientID != "custom-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "custom-id")
	}
	if cfg.ClientSecret != "custom-secret" {
		t.Errorf("ClientSecret = %q, want %q", cfg.ClientSecret, "custom-secret")
	}
}

func TestGetOAuthConfig_PartialEnv(t *testing.T) {
	os.Setenv("COLAB_CLIENT_ID", "custom-id")
	os.Unsetenv("COLAB_CLIENT_SECRET")
	defer os.Unsetenv("COLAB_CLIENT_ID")

	cfg := getOAuthConfig()
	if cfg.ClientID != "custom-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "custom-id")
	}
	if cfg.ClientSecret != defaultClientSecret {
		t.Errorf("ClientSecret = %q, want default", cfg.ClientSecret)
	}
}
