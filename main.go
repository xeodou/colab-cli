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
  exec <file>           Execute .py or .ipynb on Colab GPU
  exec -c "code"        Execute inline Python code on Colab GPU
  quota                 Show GPU quota, CCU balance, eligible accelerators
  upload <local> [remote]   Upload file to Colab runtime
  download <remote> [local] Download file from Colab runtime
  status                Show runtime info (GPU, memory, idle time)
  stop                  Release the Colab runtime

Options:
  --json                Machine-readable JSON output
  --gpu t4|l4|a100      GPU type (default: t4)
  --timeout 30m         Execution timeout (default: 30m)
  -h, --help            Show this help
  -v, --version         Show version

Examples:
  colab auth
  colab quota
  colab exec train.py
  colab exec notebook.ipynb
  colab exec -c "import torch; print(torch.cuda.get_device_name(0))"
  colab exec --gpu a100 train.py
  colab upload data.zip
  colab download output/model.bin ./model.bin
  colab stop
`)
}
