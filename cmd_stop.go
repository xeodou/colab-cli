package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func runStop(args []string) error {
	jsonOutput := hasFlag(args, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)

	if !jsonOutput {
		fmt.Println("Looking for active runtime...")
	}

	assignments, err := client.ListAssignments(ctx)
	if err != nil {
		return fmt.Errorf("list assignments: %w", err)
	}

	if len(assignments) == 0 {
		if jsonOutput {
			out := map[string]interface{}{
				"status":  "no_runtime",
				"message": "No active runtime found",
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("No active runtime found.")
		}
		return nil
	}

	released := 0
	for _, a := range assignments {
		rt := &Runtime{
			Endpoint:    a.Endpoint,
			ProxyToken:  a.RuntimeProxyInfo.Token,
			ProxyURL:    a.RuntimeProxyInfo.URL,
			Accelerator: a.Accelerator,
		}
		if !jsonOutput {
			fmt.Printf("Releasing %s (%s)...\n", rt.Accelerator, rt.Endpoint)
		}
		if err := client.UnassignRuntime(ctx, rt); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to release %s: %v\n", rt.Endpoint, err)
			continue
		}
		released++
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status":   "released",
			"released": released,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Released %d runtime(s).\n", released)
	}

	return nil
}
