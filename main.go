package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "auth":
		err = runAuth(args)
	case "start":
		err = runStart(args)
	case "exec":
		err = runExec(args)
	case "upload":
		err = runUpload(args)
	case "download":
		err = runDownload(args)
	case "quota":
		err = runQuota(args)
	case "status":
		err = runStatus(args)
	case "stop":
		err = runStop(args)
	case "version", "--version", "-v":
		fmt.Printf("colab %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`Usage: colab <command> [options]

Commands:
  auth                  Authenticate with Google (OAuth2 browser flow)
  start                 Start a GPU runtime and print the session ID
  exec <file>           Execute .py or .ipynb on Colab GPU
  exec -c "code"        Execute inline Python code on Colab GPU
  upload <local> [remote]   Upload file to Colab runtime
  download <remote> [local] Download file from Colab runtime
  quota                 Show GPU quota, CCU balance, eligible accelerators
  status                Show runtime info (GPU, memory, idle time)
  stop                  Release the Colab runtime

Options:
  --json                Machine-readable JSON output
  --gpu t4|l4|a100      GPU type (default: t4)
  --timeout 30m         Execution timeout (default: 30m)
  --session <id>        Use a specific runtime session
  -h, --help            Show this help
  -v, --version         Show version

Examples:
  colab exec train.py                          # one-shot: assign, run, release
  colab exec --gpu a100 train.py               # one-shot with A100
  colab exec -c "import torch; print(torch.cuda.get_device_name(0))"

  # Long-running session:
  colab start --gpu t4                         # → prints session ID
  colab upload --session <id> data.tar.gz      # upload to that runtime
  colab exec --session <id> train.py           # run on that runtime
  colab download --session <id> model.bin      # download results
  colab stop                                   # release when done
`)
}
