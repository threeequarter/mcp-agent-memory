# MCP Memory Server

Simple implementation of persistent semantic memory for AI agents. Provides structured storage and retrieval of facts, decisions, conversation summaries, and domain profiles through the Model Context Protocol (MCP).

## Architecture

| Component | Technology |
|-----------|-----------|
| Storage | SQLite via `modernc.org/sqlite` |
| Vector search | Full table scan + cosine similarity |
| Embeddings | Ollama HTTP API |
| Transport | MCP over stdio |

## MCP Tools

| Tool | When to Call | What It Returns |
|------|-------------|-----------------|
| `memory_init` | Session start | User profile, agent profile, domain profile, and top-k relevant fragments + episodes |
| `memory_search` | Mid-conversation when context is missing | Semantically matched fragments and episodes |
| `memory_store` | On "remember this" or "save session" | Confirmation with stored ID |
| `profile_read` | Before replacing a section, to inspect current content | Full profile as JSON object |
| `profile_replace_section` | On explicit request to rewrite a profile section | Confirmation with previous content of replaced section |
| `memory_list_episodes` | When listing recent conversations without full content | Episode metadata (id, domain, title, date) |

## Quick Start

### Prerequisites

- Go 1.23+
- [Ollama](https://ollama.com) running locally with an embedding model pulled:

```bash
ollama pull nomic-embed-text
```

### Build

```bash
go build -o memory-mcp ./cmd/server
```

### Configure

Environment variables (set via shell or MCP client config):

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_MEMORY_DATABASE` | `memory.db` next to binary | Path to SQLite database |
| `AGENT_OLLAMA_HOST` | `http://localhost:11434` | Ollama API host |
| `AGENT_OLLAMA_MODEL` | `nomic-embed-text` | Embedding model name |
| `AGENT_MEMORY_LOG` | *(disabled)* | Path to append-only JSON log file for tool call debugging |

### Connect to an MCP Client

**Zed** — add to `settings.json`:

```json
"memory-mcp": {
  "command": "/path/to/memory-mcp",
  "args": [],
  "env": {
    "AGENT_MEMORY_DATABASE": "/path/to/memory.db",
    "AGENT_OLLAMA_HOST": "http://localhost:11434",
    "AGENT_OLLAMA_MODEL": "nomic-embed-text"
  }
}

```

**Claude Desktop** — add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memory-mcp": {
      "command": "/path/to/memory-mcp",
      "env": {
        "AGENT_MEMORY_DATABASE": "/path/to/memory.db",
        "AGENT_OLLAMA_HOST": "http://localhost:11434",
        "AGENT_OLLAMA_MODEL": "nomic-embed-text"
      }
    }
  }
}
```

Verify by opening your MCP client and confirming `memory-mcp` appears as a connected context server with all 6 tools listed.

## Integrating with Agent Prompts

This section explains how to configure an AI agent's system prompt to use the memory server effectively. The pattern below works for any MCP-compatible agent (Zed, Claude Desktop, custom clients) and any domain.

### The MEMORY Block

Add this to your agent's system prompt, replacing `<DOMAIN>` with the relevant domain name:

```markdown
**Session start:** Call `memory_init` with `domain="<DOMAIN>"` and
`query=<first user message text>`. The returned `user_profile`,
`agent_profile`, and domain-specific context are supplementary
background — they enrich the already-assembled context, not replace it.

Domain prompts override the `domain` parameter. When a domain prompt is
active, use its specified domain instead of `"<DOMAIN>"`.

**Domain selection:** If the user's request clearly maps to a specific
domain, note it. Otherwise, `"<DOMAIN>"` is appropriate. New domains are
created implicitly — calling `profile_replace_section` or `memory_store`
with a previously unused domain name creates it automatically. No setup
step is required. On a fresh database, `user` and `agent` profiles are
seeded with placeholder sections to guide initial population.

**Mid-conversation:** Call `memory_search` when context relevant to the
current topic appears to be missing. Do not call speculatively on every turn.

**Storing — fragments:** On "remember this" or similar, call `memory_store`
with `type="fragment"` and an appropriate domain. Store a concise distilled
version of the fact, decision, or preference — not raw conversation text.

**Storing — user insights:** When you discover an important preference,
pattern, or constraint about the user that is not in the user profile, call
`memory_store` with `type="fragment"`, `domain="user"`. Store a concise
distilled observation — not raw conversation text. To correct or rewrite
a profile section (e.g., user changed age or role), use `profile_replace_section`.

**Storing — agent reflection:** When you reflect on your own operation and
identify a pattern that will help in future sessions, call `memory_store`
with `type="fragment"`, `domain="agent"`. Each reflection is an independent
insight — do not attempt to reconstruct previous reflections.

**Storing — episodes:** On "save session" or "archive this", call
`memory_store` with `type="episode"` and a structured summary (context,
key topics, outcomes). Then call `memory_store` again for each notable
fragment surfaced during the session.

**Profile read:** Call `profile_read` to inspect the current content of a
domain profile mid-session. Use this before replacing a section with
`profile_replace_section` so you know what already exists.

**Profile replace:** Call `profile_replace_section` only to rewrite an entire
section on explicit user request. This tool REPLACES the section content —
any existing content is overwritten and lost. The response includes
`previous_content` showing what was replaced. For incremental additions (new
learnings, patterns, preferences), always use `memory_store` with
`type="fragment"` instead.
```

### Domain naming convention

Use short, lowercase domain names that map to the agent's area of expertise:

| Domain | Purpose |
|--------|---------|
| `user` | User identity, preferences, communication style |
| `agent` | Agent self-reflection and behavioral configuration |
| `code` | Programming, tooling, development practices |
| `architect` | System design, microservices, compliance |

New domains are created implicitly. Calling `profile_replace_section` with a previously unused domain name inserts a new row. Calling `memory_store` with a new domain creates fragments in that domain without requiring a profile. No setup step or registration is needed.

On a fresh database, `user` and `agent` profiles are automatically seeded with placeholder sections to guide initial population:

| Profile | Default Sections |
|---------|-----------------|
| `user` | Identity, Communication Style, Domain Focus |
| `agent` | Behavioral Boundaries, Communication Structure Constraints, Vocabulary Constraints |

Placeholder values are short descriptions (e.g., `"Name, age, profession, key interests."` for Identity). The agent replaces them with actual content via `profile_replace_section` as it learns about the user.

### What `memory_init` returns

```json
{
  "user_profile": { "Identity": "...", "Communication Style": "...", "Domain Focus": "..." },
  "agent_profile": { "Vocabulary Constraints": "...", "Conversation Summary Format": "..." },
  "domain_profile": { "Approach & Patterns": "...", "Current State": "..." },
  "fragments": [ { "content": "...", "score": 0.87 } ],
  "user_fragments": [ { "content": "...", "score": 0.65 } ],
  "agent_fragments": [],
  "episodes": [ { "title": "...", "content": "...", "score": 0.72 } ]
}
```

- **Profiles** are JSON objects mapping section names to text content.
- **Fragments** are semantically matched to the query, scored by cosine similarity (0–1, higher is more relevant).
- **Episodes** include full context/topics/outcomes content.
- `user_fragments` and `agent_fragments` provide cross-domain context (user preferences and agent self-reflections relevant to the current topic).

### Example: reading a profile

```
profile_read(domain="user")
```

### Example: replacing a profile section

```
profile_replace_section(domain="user", section="Communication Style",
  content="Evidence-based, direct, no AI filler words, tables over lists, conclusions first")
```

The response includes `previous_content` showing what was replaced, so you can verify no data was lost.

## Database Schema

Three tables in SQLite with WAL journal mode:

```sql
CREATE TABLE profile (
    domain      TEXT PRIMARY KEY,
    content     TEXT NOT NULL,   -- JSON: section_name → text
    updated_at  TEXT NOT NULL
);

CREATE TABLE fragments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    domain      TEXT NOT NULL,
    content     TEXT NOT NULL,
    embedding   BLOB NOT NULL,   -- float32 LE, dims depend on model
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_fragments_domain ON fragments(domain);

CREATE TABLE episodes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    domain      TEXT NOT NULL,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    embedding   BLOB NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_episodes_domain ON episodes(domain);
```

### Bootstrapping

When the database is empty (0 rows in the `profile` table), `seedDefaults` automatically inserts starter `user` and `agent` profiles with placeholder sections. This gives the agent structure to fill in immediately, rather than starting with a blank slate where it doesn't know what sections exist. Seeding runs once — if any profile data exists, no defaults are inserted.

Embeddings are stored as raw `BLOB` (float32 little-endian). Dimension depends on the Ollama model (`nomic-embed-text` → 768, `qwen3-embedding` → 1024). Vector search is exact KNN.

## Project Structure

```
├── cmd/
│   └── server/main.go       # MCP server entry point
├── internal/
│   ├── db/
│   │   ├── db.go            # SQLite connection + schema
│   │   ├── fragments.go     # fragment insert + semantic search
│   │   ├── episodes.go      # episode insert + search + list
│   │   └── profile.go       # profile get + upsert section
│   ├── embedder/
│   │   └── ollama.go        # Ollama HTTP embedding client
│   └── vec/
│       └── cosine.go        # cosine similarity + top-k
├── go.mod
└── go.sum
```
