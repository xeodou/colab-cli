package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func runAuth(args []string) error {
	jsonOutput := hasFlag(args, "--json")

	if hasFlag(args, "--status") {
		status, err := tokenStatus()
		if err != nil {
			return err
		}

		// Determine credential source
		credSource := "colab-vscode (default)"
		if os.Getenv("COLAB_CLIENT_ID") != "" {
			credSource = "custom (COLAB_CLIENT_ID)"
		}

		if jsonOutput {
			tok, _ := loadCachedToken()
			out := map[string]interface{}{
				"status":      status,
				"credentials": credSource,
			}
			if tok != nil {
				out["expires"] = tok.Expiry.Format(time.RFC3339)
				out["valid"] = tok.Expiry.After(time.Now()) || tok.RefreshToken != ""
			} else {
				out["valid"] = false
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println(status)
			fmt.Printf("Credentials:    %s\n", credSource)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("Starting OAuth2 authentication...")
	tok, err := doOAuthLogin(ctx)
	if err != nil {
		return err
	}

	if err := saveToken(tok); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status":  "authenticated",
			"expires": tok.Expiry.Format(time.RFC3339),
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println("Authentication successful!")
		fmt.Printf("Token expires: %s\n", tok.Expiry.Format(time.RFC3339))
	}

	return nil
}
