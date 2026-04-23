# AGENTS.md

## Project Overview

MCP Memory Server — a persistent memory server for AI agents. Stores facts, decisions, conversation summaries, and domain profiles in SQLite with vector similarity search. Exposes 5 MCP tools over stdio for semantic storage and retrieval.

System context: runs as a Zed context server. Ollama provides embeddings locally. SQLite stores all data in a single file. No external services beyond Ollama on localhost.

## Project Structure

```
mcp-agent-memory/
├── cmd/
│   └── server/
│       └── main.go            # MCP server entry point (5 tools)
├── internal/
│   ├── db/
│   │   ├── db.go              # SQLite connection, schema init, DBPath()
│   │   ├── fragments.go       # Fragment insert + semantic search
│   │   ├── episodes.go        # Episode insert + search + list metadata
│   │   └── profile.go         # Profile get + upsert section (JSON blob)
│   ├── embedder/
│   │   └── ollama.go          # Ollama HTTP embedding client
│   └── vec/
│       └── cosine.go          # Float32 packing + cosine similarity + TopK
├── go.mod
├── go.sum
└── README.md
```

## Architecture

Single-process MCP server over stdio. One SQLite connection (WAL mode, `max_open_conns=1`).

Core pipeline per tool call:

1. Parse tool arguments from `mcp.CallToolRequest`
2. Embed query text via Ollama HTTP API (`/api/embeddings`) — dimension depends on model
3. Execute SQL (profile read, fragment/episode search/insert, profile upsert)
4. Marshal results to JSON text and return via `mcp.NewToolResultText` or `mcp.NewToolResultError`
5. Optional: log call to append-only JSON file (`AGENT_MEMORY_LOG`)

Vector search is exact KNN: scan entire domain-filtered table, unpack BLOB embeddings, compute cosine similarity, sort descending, return top-k. No approximate indexing — table sizes are small (thousands of rows).

Data model:

| Table | Purpose | Key Fields |
|---|---|---|
| `profile` | Domain profiles (JSON blob) | `domain` PK, `content` JSON, `updated_at` |
| `fragments` | Distilled facts/decisions | `id`, `domain`, `content`, `embedding` BLOB, `created_at` |
| `episodes` | Conversation summaries | `id`, `domain`, `title`, `content`, `embedding` BLOB, `created_at` |

Profile content is a JSON object mapping section names to text. `UpsertProfileSection` reads existing JSON, updates one key, marshals back, and does `INSERT ... ON CONFLICT(domain) DO UPDATE`.

## Code Conventions

**Code Style:**
- Standard Go formatting (`gofmt`). Tab indentation.
- Line length: no hard limit, but keep tool descriptions readable.
- Naming: PascalCase exported, camelCase unexported. Acronyms uppercase (`DB`, `ID`, `HTTP`, `SQL`).
- Import order: stdlib, blank line, third-party, blank line, internal (`memory-mcp/...`).
- Struct tags: `json:"field_name"` using snake_case for API response fields.

**Error Handling:**
- Return errors up the call stack. No custom error types.
- In tool handlers, wrap errors with `fmt.Sprintf("context: %v", err)` and return via `mcp.NewToolResultError`.
- `sql.ErrNoRows` is handled explicitly where empty result is valid (e.g., missing profile returns empty map, not error).
- JSON marshal errors in tool handlers are ignored with `_` — they operate on simple maps and should never fail.

**SQL Patterns:**
- Raw `database/sql` queries. No ORM.
- Context-aware: always use `ExecContext`, `QueryContext`, `QueryRowContext`.
- Parameters via `?` placeholders (SQLite driver handles binding).
- WAL mode enabled, foreign keys on, `max_open_conns=1` for SQLite writer serialization.

**Time:**
- Always UTC. Format: `time.Now().UTC().Format(time.RFC3339)`.

**Logging:**
- Standard `log` package for fatal errors and warnings only.
- Optional structured call logging via `logCall()` in `main.go` — writes append-only JSON lines when `AGENT_MEMORY_LOG` is set.

## Shared Utilities & Reusable Components

| Location | What It Does | When to Use |
|---|---|---|
| `internal/db/db.go:Open(path)` | Opens SQLite with WAL + schema init | Any new entry point needing DB |
| `internal/db/db.go:DBPath()` | Resolves DB path from env or binary dir | Standard DB path resolution |
| `internal/vec/cosine.go:Pack(v)` | float32 slice → little-endian BLOB | Before storing embeddings |
| `internal/vec/cosine.go:Unpack(b)` | BLOB → float32 slice | After reading embeddings from DB |
| `internal/vec/cosine.go:Cosine(a, b)` | Cosine similarity between two float32 slices | Custom vector comparison |
| `internal/vec/cosine.go:TopK(query, candidates, k)` | Sorts candidates by cosine score, returns top k | Any semantic ranking |
| `internal/embedder/ollama.go:Client.Embed(ctx, text)` | HTTP call to Ollama embeddings API | Any text that needs embedding |
| `internal/embedder/ollama.go:Client.Unload(ctx)` | Sends keep_alive=0 to free VRAM | Graceful shutdown or memory pressure |

## Development Guidelines

**Adding a new MCP tool:**

1. Add tool definition in `cmd/server/main.go` inside `addTools()`.
2. Use `mcp.NewTool("tool_name", mcp.WithDescription(...), mcp.WithString(...))`.
3. Implement handler as closure over `sqlDB` and `emb`.
4. Parse arguments with `req.GetString("param", "")` / `req.GetFloat("param", 0)`.
5. Call `logCall` wrapper for optional call logging.
6. Return `mcp.NewToolResultText(jsonString)` or `mcp.NewToolResultError(message)`.

**Adding a new database table:**

1. Add `CREATE TABLE IF NOT EXISTS` to `internal/db/db.go:initSchema()`.
2. Create a new file under `internal/db/` (e.g., `internal/db/concept.go`) with struct type and insert/search functions.
3. Follow the fragment/episode pattern: struct with JSON tags, `Insert*` with `vec.Pack(embedding)`, `Search*` with `vec.TopK`.
4. Use `context.Context` and `*sql.DB` as first parameters.

**Adding a new vector utility:**

1. Add to `internal/vec/cosine.go` if general-purpose.
2. Keep functions pure (no side effects, no DB access).
3. Document dimension assumptions — dimension is model-dependent, not hardcoded.

## Development Commands

Build the server:

```bash
cd cmd/server
go build -o memory-mcp
# or from repo root:
go build -o memory-mcp ./cmd/server
```

Run locally (requires Ollama):

```bash
ollama pull nomic-embed-text
export AGENT_OLLAMA_HOST=http://localhost:11434
export AGENT_OLLAMA_MODEL=nomic-embed-text
./memory-mcp
```

## Configuration

All configuration is via environment variables. No config files.

| Variable | Default | Description |
|---|---|---|
| `AGENT_MEMORY_DATABASE` | `memory.db` next to binary | SQLite file path |
| `AGENT_OLLAMA_HOST` | `http://localhost:11434` | Ollama API base URL |
| `AGENT_OLLAMA_MODEL` | `nomic-embed-text` | Embedding model name |
| `AGENT_MEMORY_LOG` | *(disabled)* | Path to append-only JSON call log |

## Key Dependencies

| Package | Version | Role |
|---|---|---|
| `github.com/mark3labs/mcp-go` | v0.49.0 | MCP protocol implementation, server, tool definitions |
| `modernc.org/sqlite` | v1.49.1 | Pure-Go SQLite driver (no CGO) |

## Integration Points

- **Ollama** (`/api/embeddings`, `/api/generate`): Must be running locally. Model must be pulled before use. Embedding dimension is model-dependent (`nomic-embed-text` → 768, `qwen3-embedding` → 1024). If model changes, verify dimension compatibility with `vec.Pack`/`Unpack` and schema BLOB size — mixing dimensions in the same table will cause errors at search time.
- **MCP client** (Zed, Claude Desktop, etc.): Server communicates over stdio. Tool names and descriptions are the contract.

## Important Notes

- SQLite is configured for single-writer concurrency (`max_open_conns=1`). Do not increase this — SQLite WAL handles readers, but multiple writers would cause `database is locked` errors.
- Vector search is full-table scan with exact cosine. No IVF, HNSW, or other approximate indexes. Performance is acceptable up to tens of thousands of rows. If scaling beyond that, consider adding a vector index or switching to a dedicated vector DB.
- `logCall` ignores marshal errors and silently drops log entries if the file is not openable. This is intentional — logging must not break tool execution.
- `go.mod` declares `go 1.25.5`. The `README.md` states Go 1.23+ as the minimum. Use Go 1.23+ features conservatively to maintain compatibility.
- No tests exist in the repository. If adding tests, standard `testing` package is sufficient; no testify or other test dependencies are present.
- The `Unload` method on embedder client sends a request to Ollama to release the model from VRAM (`keep_alive: 0`). It is safe to call on shutdown but not currently wired to any signal handler.
- Profile `content` is stored as a single JSON blob. There is no schema validation — any string key is valid. The application layer (agent prompt) defines expected sections.
