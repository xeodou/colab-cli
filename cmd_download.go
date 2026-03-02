package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func runDownload(args []string) error {
	jsonOutput := hasFlag(args, "--json")
	gpu := getFlagValue(args, "--gpu", "t4")
	session := getFlagValue(args, "--session", "")

	positional := positionalArgs(args, "--gpu", "--session")

	if len(positional) < 1 {
		return fmt.Errorf("usage: colab download <remote-path> [local-path]")
	}

	remotePath := positional[0]
	localPath := ""
	if len(positional) > 1 {
		localPath = positional[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)

	if !jsonOutput {
		fmt.Println("Connecting to runtime...")
	}

	var rt *Runtime
	if session != "" {
		rt, err = client.ResumeRuntime(ctx, gpu, session)
	} else {
		rt, err = client.AssignRuntime(ctx, gpu)
	}
	if err != nil {
		return fmt.Errorf("assign runtime: %w", err)
	}

	// Connect to kernel for reliable file download
	kc, err := NewKernelClient(ctx, rt)
	if err != nil {
		return fmt.Errorf("connect kernel: %w", err)
	}
	defer kc.Close()

	if !jsonOutput {
		fmt.Printf("Downloading %s...\n", remotePath)
	}

	if err := KernelDownload(ctx, kc, remotePath, localPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	dest := localPath
	if dest == "" {
		dest = filepath.Base(remotePath)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status": "downloaded",
			"remote": remotePath,
			"local":  dest,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Downloaded: %s -> %s\n", remotePath, dest)
	}

	return nil
}
