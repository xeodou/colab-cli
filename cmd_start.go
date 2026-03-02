package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func runStart(args []string) error {
	jsonOutput := hasFlag(args, "--json")
	gpu := getFlagValue(args, "--gpu", "t4")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)

	if !jsonOutput {
		fmt.Printf("Requesting %s GPU runtime...\n", strings.ToUpper(gpu))
	}

	rt, err := client.AssignRuntime(ctx, gpu)
	if err != nil {
		return fmt.Errorf("assign runtime: %w", err)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status":   "started",
			"session":  rt.Endpoint,
			"gpu":      rt.Accelerator,
			"proxyUrl": rt.ProxyURL,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Runtime started: %s (%s)\n", rt.Accelerator, rt.Endpoint)
		fmt.Printf("Session: %s\n", rt.Endpoint)
		fmt.Println("\nUse this session with other commands:")
		fmt.Printf("  colab exec --session %s script.py\n", rt.Endpoint)
		fmt.Printf("  colab upload --session %s data.tar.gz\n", rt.Endpoint)
		fmt.Printf("  colab stop\n")
	}

	return nil
}
