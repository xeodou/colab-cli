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

	positional := positionalArgs(args, "--gpu")

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

	rt, err := client.AssignRuntime(ctx, gpu)
	if err != nil {
		return fmt.Errorf("assign runtime: %w", err)
	}

	fc := NewFileClient(rt)

	if !jsonOutput {
		fmt.Printf("Downloading %s...\n", remotePath)
	}

	if err := fc.Download(ctx, remotePath, localPath); err != nil {
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
