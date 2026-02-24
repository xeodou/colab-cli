package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func runStatus(args []string) error {
	jsonOutput := hasFlag(args, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)

	if !jsonOutput {
		fmt.Println("Checking runtime status...")
	}

	assignments, err := client.ListAssignments(ctx)
	if err != nil {
		return fmt.Errorf("list assignments: %w", err)
	}

	if len(assignments) == 0 {
		if jsonOutput {
			out := map[string]interface{}{
				"status":  "no_runtime",
				"message": "No active runtime",
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("No active runtime.")
		}
		return nil
	}

	a := assignments[0]
	rt := &Runtime{
		Endpoint:    a.Endpoint,
		ProxyToken:  a.RuntimeProxyInfo.Token,
		ProxyURL:    a.RuntimeProxyInfo.URL,
		Accelerator: a.Accelerator,
	}

	// If we have a proxy URL, query the kernel for detailed status
	if rt.ProxyURL != "" {
		status, err := client.GetStatus(ctx, rt)
		if err == nil {
			if jsonOutput {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("GPU:        %s\n", status.GPU)
				fmt.Printf("Memory:     %d MB\n", status.MemoryMB)
				fmt.Printf("Idle:       %ds\n", status.IdleSecs)
				fmt.Printf("Connected:  %v\n", status.Connected)
			}
			return nil
		}
		// Fall through to basic info if kernel query fails
	}

	// Basic info from assignment
	if jsonOutput {
		out := map[string]interface{}{
			"gpu":       rt.Accelerator,
			"endpoint":  rt.Endpoint,
			"connected": true,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("GPU:        %s\n", rt.Accelerator)
		fmt.Printf("Endpoint:   %s\n", rt.Endpoint)
		fmt.Printf("Connected:  true\n")
	}

	return nil
}
