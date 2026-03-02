package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func runUpload(args []string) error {
	jsonOutput := hasFlag(args, "--json")
	gpu := getFlagValue(args, "--gpu", "t4")
	session := getFlagValue(args, "--session", "")

	positional := positionalArgs(args, "--gpu", "--session")

	if len(positional) < 1 {
		return fmt.Errorf("usage: colab upload <local-file> [remote-path]")
	}

	localPath := positional[0]
	remotePath := ""
	if len(positional) > 1 {
		remotePath = positional[1]
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

	// Connect to kernel for reliable file upload
	kc, err := NewKernelClient(ctx, rt)
	if err != nil {
		return fmt.Errorf("connect kernel: %w", err)
	}
	defer kc.Close()

	if !jsonOutput {
		fmt.Printf("Uploading %s...\n", filepath.Base(localPath))
	}

	if err := KernelUpload(ctx, kc, localPath, remotePath); err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	dest := remotePath
	if dest == "" {
		dest = filepath.Base(localPath)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status": "uploaded",
			"local":  localPath,
			"remote": dest,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Uploaded: %s -> %s\n", localPath, dest)
	}

	return nil
}
