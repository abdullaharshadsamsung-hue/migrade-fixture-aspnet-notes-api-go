# Notes API (Go)

An HTTP API for managing notes, migrated from an ASP.NET (C#) codebase to Go using the standard library toolchain. The service runs as a single binary built from `cmd/server`.

> **Migration status:** All 3 modules migrated. The build, the target unit tests and a behavioral comparison against the original ASP.NET application have all passed. The 97% confidence figure is the migration model's own estimate. It is not a measured result.

## Tech Stack

- **Language:** Go (see `go.mod` for the module path and Go version)
- **HTTP layer:** Go standard library (`net/http`), with routing defined in `cmd/server/router.go`
- **Dependencies:** managed with Go modules (`go.mod`)
- **Containerization:** `Dockerfile` and `docker-compose.yml` are included

## Prerequisites

- Go toolchain matching the version declared in `go.mod`
- Write access to `/app/bin`, because the install step writes the binary to `/app/bin/server`. This path is designed for the container environment. On a local machine you may need the appropriate permissions to create it.
- A reachable database, with its connection string supplied through `DATABASE_URL`
- Optional: Docker and Docker Compose, if you want to use the provided `Dockerfile` and `docker-compose.yml`

## Getting Started

Run these steps in order, from the repository root, starting with a fresh clone.

### 1. Install and build

```sh
mkdir -p /app/bin && (go mod download || true) && if [ -d ./cmd/server ]; then go mod tidy && go build -o /app/bin/server ./cmd/server; elif ls ./*.go >/dev/null 2>&1; then go mod tidy && go build -o /app/bin/server .; else echo 'WARNING: no Go source files found in /app (only go.mod is present); nothing to build'; fi
```

This command:

1. Creates `/app/bin`.
2. Downloads module dependencies. Errors from this step are ignored because of `|| true`.
3. Runs `go mod tidy`.
4. Builds `./cmd/server` into `/app/bin/server`.

### 2. Resolve dependencies

This step was run and succeeded during the migration. Run it after Install and before Run:

```sh
go mod tidy
```

### 3. Configure environment variables

Export the required variables. See [Environment Variables](#environment-variables) for details.

```sh
export DATABASE_URL="<your-database-connection-string>"
export PORT="<port-to-listen-on>"
export JWT_SECRET="<your-jwt-signing-secret>"
```

### 4. Database setup

No database setup command was detected for this project. Make sure the database referenced by `DATABASE_URL` exists and is reachable before starting the server. Check `internal/notes/store.go` to see what schema the store expects.

### 5. Run

```sh
/app/bin/server
```

### Docker

The repository includes a `Dockerfile` and a `docker-compose.yml`. Read those files directly for container build and run configuration. No container-specific commands were verified as part of this migration.

## Running Tests

```sh
go test ./...
```

The target unit tests passed during migration.

## Environment Variables

| Variable       | Required | Description                                                                 |
|----------------|----------|-----------------------------------------------------------------------------|
| `DATABASE_URL` | Yes      | Connection string for the database backing the notes store.                 |
| `PORT`         | Yes      | Port the HTTP server listens on.                                            |
| `JWT_SECRET`   | Yes      | Secret used for JWT handling, such as signing and validating tokens.        |

Check `internal/config/config.go` for how these variables are loaded and whether any defaults or validation apply.

## Architecture Overview

```
.
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── cmd/
│   └── server/
│       ├── main.go        # Entry point: loads config, wires dependencies, starts HTTP server
│       └── router.go      # Route registration / mapping of URLs to handlers
└── internal/
    ├── config/
    │   └── config.go      # Reads configuration (DATABASE_URL, PORT, JWT_SECRET)
    ├── httpapi/
    │   └── handlers.go    # HTTP handlers (request parsing, responses)
    └── notes/
        ├── note.go        # Note domain model
        └── store.go       # Persistence layer for notes
```

The packages fit together as follows:

- **`cmd/server`** is the executable. `main.go` bootstraps the application and `router.go` defines the HTTP routes.
- **`internal/config`** centralizes environment-based configuration.
- **`internal/httpapi`** contains the HTTP-facing logic and translates requests into calls on the notes package.
- **`internal/notes`** holds the domain type (`note.go`) and data access (`store.go`).

Because these packages live under `internal/`, they cannot be imported from outside this module.

## Migration Notes

The original ASP.NET concepts map to the Go code as follows:

| ASP.NET concept                                   | Go replacement                                   |
|---------------------------------------------------|--------------------------------------------------|
| Application startup / host builder                | `cmd/server/main.go`                             |
| Route configuration / attribute routing           | `cmd/server/router.go`                           |
| Controllers / action methods                      | `internal/httpapi/handlers.go`                   |
| Entity / model classes                            | `internal/notes/note.go`                         |
| Data access (repository / DbContext-style layer)  | `internal/notes/store.go`                        |
| `appsettings` / configuration binding             | `internal/config/config.go` (environment variables) |
| NuGet package references / project file           | `go.mod`                                         |

Other changes from the ASP.NET version:

- Configuration is supplied only through environment variables (`DATABASE_URL`, `PORT`, `JWT_SECRET`).
- The application compiles to a single static binary (`/app/bin/server`) instead of running on the .NET runtime.
- A behavioral comparison between this Go implementation and the original ASP.NET application passed.

## Known Limitations

- **No components were marked unmigratable,** and no setup commands failed during migration.
- **No DB setup step exists.** No database setup or migration command was detected. Schema creation, if the store requires it, must be handled outside the documented steps.
- **Dependency download errors are hidden.** The install command uses `go mod download || true`, so a failed download will not stop the command. A later `go mod tidy` or `go build` failure is the first visible sign of the problem.
- **The binary path is container-oriented.** Install and Run use the absolute path `/app/bin/server`, which may need permissions or adjustment outside the container.

## Manual Review Required

No modules were flagged as low confidence. Because the 97% confidence figure is self-assessed, review the following before production use:

- **`internal/config/config.go`:** Confirm that all three environment variables are read. Check how missing or invalid values are handled, for example whether startup fails fast or falls back to defaults.
- **`internal/httpapi/handlers.go`:** Verify how `JWT_SECRET` is used for authentication and authorization, and confirm that auth checks match the original ASP.NET behavior. Also verify status codes, error response formats and input validation.
- **`internal/notes/store.go`:** Check how the connection from `DATABASE_URL` is handled, along with connection lifecycle and query correctness. Confirm the schema assumptions, since no DB setup step is provided.
- **`cmd/server/router.go`:** Confirm that every route and HTTP method from the original API is registered.
- **`cmd/server/main.go`:** Review startup, shutdown and the listen address built from `PORT`.
- **`Dockerfile` and `docker-compose.yml`:** Confirm that they pass the required environment variables and match the build and run steps documented above.