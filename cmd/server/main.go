package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"memory-mcp/internal/db"
	"memory-mcp/internal/embedder"
)

var logFile *os.File

func logCall(tool string, args map[string]any, result *mcp.CallToolResult) {
	if logFile == nil {
		return
	}
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"tool":      tool,
		"args":      args,
		"isError":   result.IsError,
	}
	for _, c := range result.Content {
		if t, ok := c.(mcp.TextContent); ok {
			entry["result"] = t.Text
			break
		}
	}
	b, _ := json.Marshal(entry)
	logFile.WriteString(string(b) + "\n")
}

func main() {
	dbPath := db.DBPath()

	sqlDB, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}

	emb := embedder.New(
		os.Getenv("AGENT_OLLAMA_HOST"),
		os.Getenv("AGENT_OLLAMA_MODEL"),
	)

	if logPath := os.Getenv("AGENT_MEMORY_LOG"); logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("warning: could not open log file %s: %v", logPath, err)
		} else {
			logFile = f
			defer f.Close()
		}
	}

	s := server.NewMCPServer("memory", "1.0.0",
		server.WithToolCapabilities(false),
	)

	addTools(s, sqlDB, emb)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func addTools(s *server.MCPServer, sqlDB *sql.DB, emb *embedder.Client) {

	s.AddTool(
		mcp.NewTool("memory_init",
			mcp.WithDescription(
				"Call at session start. Always returns 'user' and 'agent' profiles "+
					"plus the requested domain profile and top-k relevant context chunks. "+
					"Pass the user's first message as `query` for better retrieval "+
					"relevance than seeding with domain name alone."),
			mcp.WithString("domain",
				mcp.Required(),
				mcp.Description("Domain for this session: 'code', 'physics', etc.")),
			mcp.WithString("query",
				mcp.Description("First user message text — used as KNN seed.")),
			mcp.WithNumber("k",
				mcp.Description("Fragments to return (default 5).")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{
				"domain": req.GetString("domain", ""),
				"query":  req.GetString("query", ""),
				"k":      req.GetFloat("k", 0),
			}
			domain := req.GetString("domain", "")
			query := req.GetString("query", "")
			if query == "" {
				query = domain
			}
			k := 5
			if kv := req.GetFloat("k", 0); kv > 0 {
				k = int(kv)
			}

			vec, err := emb.Embed(ctx, query)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("embed: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}

			userProfile, err := db.GetProfile(ctx, sqlDB, "user")
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("user profile: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}
			agentProfile, err := db.GetProfile(ctx, sqlDB, "agent")
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("agent profile: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}

			domainProfile, err := db.GetProfile(ctx, sqlDB, domain)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("profile: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}
			frags, err := db.SearchFragments(ctx, sqlDB, vec, domain, k)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("fragments: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}
			eps, err := db.SearchEpisodes(ctx, sqlDB, vec, domain, 3)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("episodes: %v", err))
				logCall("memory_init", args, result)
				return result, nil
			}

			userFrags, _ := db.SearchFragments(ctx, sqlDB, vec, "user", 3)
			agentFrags, _ := db.SearchFragments(ctx, sqlDB, vec, "agent", 3)

			b, _ := json.Marshal(map[string]any{
				"user_profile":    userProfile,
				"agent_profile":   agentProfile,
				"domain_profile":  domainProfile,
				"fragments":       frags,
				"user_fragments":  userFrags,
				"agent_fragments": agentFrags,
				"episodes":        eps,
			})
			result := mcp.NewToolResultText(string(b))
			logCall("memory_init", args, result)
			return result, nil
		},
	)

	s.AddTool(
		mcp.NewTool("memory_search",
			mcp.WithDescription(
				"Semantic search over stored fragments and episodes. Call mid-conversation "+
					"when context relevant to the current topic appears to be missing."),
			mcp.WithString("query", mcp.Required()),
			mcp.WithString("domain", mcp.Description("Filter by domain.")),
			mcp.WithNumber("k", mcp.Description("Results per type (default 5).")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{
				"query":  req.GetString("query", ""),
				"domain": req.GetString("domain", ""),
				"k":      req.GetFloat("k", 0),
			}
			query := req.GetString("query", "")
			domain := req.GetString("domain", "")
			k := 5
			if kv := req.GetFloat("k", 0); kv > 0 {
				k = int(kv)
			}

			vec, err := emb.Embed(ctx, query)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("embed: %v", err))
				logCall("memory_search", args, result)
				return result, nil
			}
			frags, err := db.SearchFragments(ctx, sqlDB, vec, domain, k)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("fragments: %v", err))
				logCall("memory_search", args, result)
				return result, nil
			}
			eps, err := db.SearchEpisodes(ctx, sqlDB, vec, domain, 3)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("episodes: %v", err))
				logCall("memory_search", args, result)
				return result, nil
			}

			b, _ := json.Marshal(map[string]any{"fragments": frags, "episodes": eps})
			result := mcp.NewToolResultText(string(b))
			logCall("memory_search", args, result)
			return result, nil
		},
	)

	s.AddTool(
		mcp.NewTool("memory_store",
			mcp.WithDescription(
				"Store a distilled fragment (fact/decision) or episode (conversation summary). "+
					"Always distil content before storing — do not store raw conversation text."),
			mcp.WithString("content", mcp.Required()),
			mcp.WithString("type", mcp.Required(),
				mcp.Description("'fragment' or 'episode'")),
			mcp.WithString("domain", mcp.Required()),
			mcp.WithString("title",
				mcp.Description("Required when type=episode.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{
				"content": req.GetString("content", ""),
				"type":    req.GetString("type", ""),
				"domain":  req.GetString("domain", ""),
				"title":   req.GetString("title", ""),
			}
			content := req.GetString("content", "")
			storeType := req.GetString("type", "")
			domain := req.GetString("domain", "")
			title := req.GetString("title", "")

			if storeType != "fragment" && storeType != "episode" {
				result := mcp.NewToolResultError("type must be 'fragment' or 'episode'")
				logCall("memory_store", args, result)
				return result, nil
			}
			if storeType == "episode" && title == "" {
				result := mcp.NewToolResultError("title required for episodes")
				logCall("memory_store", args, result)
				return result, nil
			}

			vec, err := emb.Embed(ctx, content)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("embed: %v", err))
				logCall("memory_store", args, result)
				return result, nil
			}

			var id int64
			if storeType == "fragment" {
				id, err = db.InsertFragment(ctx, sqlDB, domain, content, vec)
			} else {
				id, err = db.InsertEpisode(ctx, sqlDB, domain, title, content, vec)
			}
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("insert: %v", err))
				logCall("memory_store", args, result)
				return result, nil
			}

			b, _ := json.Marshal(map[string]any{
				"stored": true, "id": id, "type": storeType,
			})
			result := mcp.NewToolResultText(string(b))
			logCall("memory_store", args, result)
			return result, nil
		},
	)

	s.AddTool(
		mcp.NewTool("memory_update_profile",
			mcp.WithDescription(
				"Update one named section of a domain profile. Replaces the entire "+
					"section content — do not use for incremental additions. "+
					"For domain='user': use to correct or rewrite profile sections "+
					"(Identity, Communication Style, Domain Focus). For incremental "+
					"discoveries about the user, use memory_store with type='fragment' "+
					"and domain='user' instead. "+
					"For domain='agent': use only to update reference sections "+
					"(Vocabulary Constraints, Conversation Summary Format). For agent "+
					"self-reflection, use memory_store with type='fragment' and "+
					"domain='agent' instead. "+
					"For other domains: call only on explicit user request."),
			mcp.WithString("domain", mcp.Required()),
			mcp.WithString("section", mcp.Required()),
			mcp.WithString("content", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{
				"domain":  req.GetString("domain", ""),
				"section": req.GetString("section", ""),
				"content": req.GetString("content", ""),
			}
			domain := req.GetString("domain", "")
			section := req.GetString("section", "")
			content := req.GetString("content", "")

			if err := db.UpsertProfileSection(ctx, sqlDB, domain, section, content); err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("upsert: %v", err))
				logCall("memory_update_profile", args, result)
				return result, nil
			}
			b, _ := json.Marshal(map[string]any{
				"updated": true, "domain": domain, "section": section,
			})
			result := mcp.NewToolResultText(string(b))
			logCall("memory_update_profile", args, result)
			return result, nil
		},
	)

	s.AddTool(
		mcp.NewTool("memory_list_episodes",
			mcp.WithDescription(
				"List recent episode metadata without full content. "+
					"Use memory_search to retrieve episode content by topic."),
			mcp.WithString("domain", mcp.Description("Filter by domain.")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{
				"domain": req.GetString("domain", ""),
				"limit":  req.GetFloat("limit", 0),
			}
			domain := req.GetString("domain", "")
			limit := 10
			if lv := req.GetFloat("limit", 0); lv > 0 {
				limit = int(lv)
			}
			eps, err := db.ListEpisodes(ctx, sqlDB, domain, limit)
			if err != nil {
				result := mcp.NewToolResultError(fmt.Sprintf("list: %v", err))
				logCall("memory_list_episodes", args, result)
				return result, nil
			}
			b, _ := json.Marshal(eps)
			result := mcp.NewToolResultText(string(b))
			logCall("memory_list_episodes", args, result)
			return result, nil
		},
	)
}
