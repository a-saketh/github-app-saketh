# GitHub App - PR Change Statistics

A Go-based GitHub App that extracts and displays pull request change statistics including additions, deletions, and file change status.

## Features

- **GitHub App Authentication** - JWT-based authentication with automatic token generation
- **PR Change Statistics** - Get per-file additions, deletions, and status (added/modified/removed/renamed)
- **Repository Files** - List files in a GitHub repository
- **Webhook Support** - Handle GitHub webhook events

## Quick Start

### Prerequisites

- Go 1.25+
- GitHub App credentials (App ID and private key)

### Setup

1. **Create a `.env` file** in the `server` directory:

```env
GITHUB_APP_ID=your_app_id
GITHUB_PRIVATE_KEY=your_private_key_pem
```

2. **Build the app**:

```bash
cd server
go build -o server.exe
```

3. **Run the server**:

```bash
./server.exe
```

The server listens on `http://localhost:3000`

## API Endpoints

### Authenticate Test

```
GET /auth-test
```

Verifies GitHub App credentials and authentication flow.

### Get PR Changed Files

```
GET /pr-files?owner=USER&repo=REPO&pr=PR_NUMBER
```

Returns per-file change statistics:

- `filename` - File path
- `status` - Change type (added, modified, removed, renamed)
- `additions` - Lines added
- `deletions` - Lines deleted
- `changes` - Total changes

**Example Response:**

```json
{
  "status": "success",
  "pr_number": 123,
  "total_files": 5,
  "total_additions": 156,
  "total_deletions": 42,
  "files": [
    {
      "filename": "src/main.go",
      "status": "modified",
      "additions": 45,
      "deletions": 12
    }
  ]
}
```

### Get Repository Files

```
GET /repo-files?owner=USER&repo=REPO
```

Lists all files in a GitHub repository.

### Webhook

```
POST /webhook
```

Handles GitHub webhook events (pull requests, push, etc.).

## Development

```bash
cd server
go run *.go
```

## Project Structure

```
server/
  ├── main.go          # Server setup and route registration
  ├── auth.go          # JWT and authentication logic
  ├── handlers.go      # HTTP handlers
  ├── pullrequest.go   # PR-specific functionality
  ├── webhook.go       # Webhook handling
  ├── repository.go    # Repository operations
  ├── types.go         # Type definitions
  └── go.mod           # Dependencies
```

## License

MIT
