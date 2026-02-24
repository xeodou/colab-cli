# colab

A Go CLI to execute Python code on Google Colab GPUs from the terminal.

Useful for running training jobs, quick GPU experiments, or file transfers without opening a browser.

## Install

```bash
go install github.com/xeodou/colab@latest
```

Or build from source:

```bash
git clone https://github.com/xeodou/colab.git
cd colab
go build -o colab .
```

## Quick Start

```bash
# 1. Authenticate (opens browser)
colab auth

# 2. Run code on a T4 GPU
colab exec -c "import torch; print(torch.cuda.get_device_name(0))"

# 3. Run a Python script
colab exec train.py

# 4. Release the runtime when done
colab stop
```

## Commands

| Command | Description |
|---------|-------------|
| `auth` | Authenticate via Google OAuth2 (browser flow) |
| `exec <file>` | Execute a `.py` or `.ipynb` file on Colab GPU |
| `exec -c "code"` | Execute inline Python code |
| `quota` | Show CCU balance, burn rate, eligible GPUs |
| `upload <local> [remote]` | Upload a file to the Colab runtime |
| `download <remote> [local]` | Download a file from the Colab runtime |
| `status` | Show runtime info (GPU type, memory, idle time) |
| `stop` | Release the runtime |

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--gpu t4\|l4\|a100` | `t4` | GPU type to request |
| `--timeout 30m` | `30m` | Execution timeout |
| `--json` | off | Machine-readable JSON output |

## Examples

```bash
# Check your GPU quota and remaining compute units
colab quota

# Run a Jupyter notebook cell by cell
colab exec notebook.ipynb

# Request an A100 GPU
colab exec --gpu a100 train.py

# Upload training data, run a script, download the model
colab upload dataset.zip
colab exec train.py --timeout 2h
colab download output/model.bin ./model.bin

# JSON output for scripting
colab exec -c "print('hello')" --json
```

## Custom OAuth2 Credentials

By default, `colab` uses the public OAuth2 credentials from Google's [Colab VS Code extension](https://marketplace.visualstudio.com/items?itemName=GoogleColab.colab-vscode-extension). You can use your own credentials instead.

### Creating Your Own Credentials

1. Go to [Google Cloud Console — Credentials](https://console.cloud.google.com/apis/credentials)
2. Create a project (or select an existing one)
3. Click **Create Credentials** → **OAuth client ID**
4. Application type: **Desktop app**
5. Copy the **Client ID** and **Client secret**
6. Go to [OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent) and add the scope: `https://www.googleapis.com/auth/colaboratory`
7. Add your Google account as a **test user**

For more details, see [Google's OAuth2 guide](https://developers.google.com/identity/protocols/oauth2/native-app).

### Using Custom Credentials

```bash
export COLAB_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export COLAB_CLIENT_SECRET="your-client-secret"
colab auth
```

Check which credentials are in use:

```bash
colab auth --status
```

> **Note:** The `colaboratory` scope is restricted by Google. Custom credentials will show an "unverified app" warning during login and are limited to 100 test users. This is fine for personal use.

## How It Works

1. **Auth** - OAuth2 loopback flow with S256 PKCE. Tokens are cached at `~/.config/colab/token.json` and auto-refreshed.

2. **Runtime** - Requests a GPU runtime via Colab's backend API. A keep-alive goroutine runs every 60s to prevent idle disconnection.

3. **Execution** - Connects to the Jupyter kernel over WebSocket, sends `execute_request` messages, and streams `stdout`/`stderr` back to the terminal in real time.

4. **Files** - Upload and download use the Jupyter Contents API (`/api/contents`).

5. **Cleanup** - `Ctrl+C` during execution gracefully releases the runtime. The `stop` command does the same manually.

## Requirements

- Go 1.21+
- A Google account with Colab access

## License

MIT
