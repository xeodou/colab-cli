package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func runExec(args []string) error {
	jsonOutput := hasFlag(args, "--json")
	gpu := getFlagValue(args, "--gpu", "t4")
	timeoutStr := getFlagValue(args, "--timeout", "30m")
	inlineCode := getFlagValue(args, "-c", "")
	session := getFlagValue(args, "--session", "")

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", timeoutStr, err)
	}

	// Determine what to execute
	var codeCells []string // one or more code blocks to execute sequentially
	var filename string
	var isNotebook bool

	if inlineCode != "" {
		codeCells = []string{inlineCode}
	} else {
		// Parse positional arg (skip flags)
		skipNext := false
		for _, arg := range args {
			if skipNext {
				skipNext = false
				continue
			}
			if arg == "--gpu" || arg == "--timeout" || arg == "-c" || arg == "--session" {
				skipNext = true
				continue
			}
			if strings.HasPrefix(arg, "--") {
				continue
			}
			filename = arg
			break
		}

		if filename == "" {
			return fmt.Errorf("usage: colab exec <file.py|file.ipynb> or colab exec -c \"code\"")
		}

		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		if strings.HasSuffix(filename, ".ipynb") {
			// Parse notebook and extract code cells
			isNotebook = true
			cells, err := parseNotebookCells(data)
			if err != nil {
				return fmt.Errorf("parse notebook: %w", err)
			}
			if len(cells) == 0 {
				return fmt.Errorf("notebook has no code cells")
			}
			codeCells = cells
		} else {
			codeCells = []string{string(data)}
		}
	}

	// Set up context with timeout and signal handling
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Authenticate
	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)

	// Get runtime: resume specific session or assign fresh
	var rt *Runtime
	if session != "" {
		if !jsonOutput {
			fmt.Printf("Connecting to session %s...\n", session)
		}
		rt, err = client.ResumeRuntime(ctx, gpu, session)
	} else {
		if !jsonOutput {
			fmt.Printf("Requesting %s GPU runtime...\n", strings.ToUpper(gpu))
		}
		rt, err = client.AssignRuntime(ctx, gpu)
	}
	if err != nil {
		return fmt.Errorf("assign runtime: %w", err)
	}

	// Without --session: release runtime on exit (fresh environment each time)
	// With --session: keep runtime alive for reuse
	if session == "" {
		var cleanupOnce sync.Once
		cleanup := func() {
			cleanupOnce.Do(func() {
				if !jsonOutput {
					fmt.Println("\nReleasing runtime...")
				}
				cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanCancel()
				_ = client.UnassignRuntime(cleanCtx, rt)
			})
		}

		go func() {
			<-sigCh
			cleanup()
			os.Exit(130)
		}()
		defer cleanup()
	} else {
		go func() {
			<-sigCh
			if rt.cancel != nil {
				rt.cancel()
			}
			os.Exit(130)
		}()
	}

	if !jsonOutput {
		fmt.Printf("Runtime: %s (%s)\n", rt.Accelerator, rt.Endpoint)
	}

	// Connect to kernel
	if !jsonOutput {
		fmt.Println("Connecting to kernel...")
	}

	kc, err := NewKernelClient(ctx, rt)
	if err != nil {
		return fmt.Errorf("connect kernel: %w", err)
	}
	defer kc.Close()

	startTime := time.Now()
	var allOutput strings.Builder
	var execErr error

	streamFn := func(stream, text string) {
		if stream == "stderr" {
			fmt.Fprint(os.Stderr, text)
		} else {
			fmt.Print(text)
		}
	}

	for i, code := range codeCells {
		if isNotebook && !jsonOutput {
			fmt.Printf("--- Cell %d/%d ---\n", i+1, len(codeCells))
		} else if i == 0 && !jsonOutput {
			fmt.Println("--- Output ---")
		}

		var output string
		if jsonOutput {
			output, execErr = kc.Execute(ctx, code)
		} else {
			output, execErr = kc.ExecuteStream(ctx, code, streamFn)
		}
		allOutput.WriteString(output)

		if execErr != nil {
			break
		}
	}

	duration := time.Since(startTime)

	if execErr != nil {
		if jsonOutput {
			out := map[string]interface{}{
				"status":   "error",
				"error":    execErr.Error(),
				"output":   allOutput.String(),
				"duration": duration.Seconds(),
				"gpu":      rt.Accelerator,
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
			return nil // JSON output already includes the error; don't double-print to stderr
		}
		return fmt.Errorf("execution: %w", execErr)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"status":   "success",
			"output":   allOutput.String(),
			"duration": duration.Seconds(),
			"gpu":      rt.Accelerator,
		}
		if isNotebook {
			out["cells"] = len(codeCells)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("--- Done (%.1fs) ---\n", duration.Seconds())
	}

	return nil
}

// parseNotebookCells extracts code cell sources from a .ipynb file.
func parseNotebookCells(data []byte) ([]string, error) {
	var nb struct {
		Cells []struct {
			CellType string      `json:"cell_type"`
			Source   interface{} `json:"source"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, err
	}

	var cells []string
	for _, cell := range nb.Cells {
		if cell.CellType != "code" {
			continue
		}
		code := extractSource(cell.Source)
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		cells = append(cells, code)
	}
	return cells, nil
}

// extractSource handles the .ipynb source field which can be a string or []string.
func extractSource(src interface{}) string {
	switch v := src.(type) {
	case string:
		return v
	case []interface{}:
		var lines []string
		for _, line := range v {
			if s, ok := line.(string); ok {
				lines = append(lines, s)
			}
		}
		return strings.Join(lines, "")
	}
	return ""
}
