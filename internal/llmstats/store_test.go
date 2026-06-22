package llmstats

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE llm_message_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT,
		channel_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		message_id TEXT NOT NULL,
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		llm_turns INTEGER NOT NULL DEFAULT 0,
		tool_calls INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStoreCreateAndListByGuild(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	log, err := store.Create(context.Background(), CreateMessageLogParams{
		GuildID:          "g",
		ChannelID:        "c",
		UserID:           "u",
		MessageID:        "m",
		PromptTokens:     12,
		CompletionTokens: 5,
		TotalTokens:      17,
		LLMTurns:         2,
		ToolCalls:        1,
		Latency:          1500 * time.Millisecond,
		Status:           StatusSuccess,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if log.ID == 0 {
		t.Fatalf("expected id")
	}
	if log.TotalTokens != 17 || log.LLMTurns != 2 || log.ToolCalls != 1 || log.Latency != 1500*time.Millisecond {
		t.Fatalf("log mismatch: %+v", log)
	}

	logs, err := store.ListByGuild(context.Background(), "g", 10)
	if err != nil {
		t.Fatalf("ListByGuild: %v", err)
	}
	if len(logs) != 1 || logs[0].MessageID != "m" {
		t.Fatalf("logs = %+v, want message m", logs)
	}
}
