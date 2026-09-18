package typesafestats

import (
	"context"
	"path/filepath"
	"testing"

	"mizubot-go/internal/db"

	"github.com/pressly/goose/v3"
)

const migrationsDir = "../../db/migrations"

func TestKindMigrationBackfillsExistingRows(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	goose.SetDialect("sqlite3")
	goose.SetTableName("goose_db_version")
	if err := goose.UpTo(database, migrationsDir, 13); err != nil {
		t.Fatalf("migrate to 13: %v", err)
	}

	// Rows as they existed before the kind column: plain classifications,
	// plus the judge rows that labelled themselves in selected_tools.
	insert := func(selected string) {
		t.Helper()
		_, err := database.Exec(`INSERT INTO typesafe_classification_logs(
			channel_id, user_id, model, request_state, request_questions, response_answers,
			selected_tools, input_tokens, output_tokens, latency_ms, status, error, created_at, message_id, matched_flags)
			VALUES('c', 'u', 'jev', '{}', '{}', '{}', ?, 1, 1, 1, 'success', '', 1, 'm', '[]')`, selected)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(`["reminder_create"]`)
	insert(`[]`)
	insert(`["anilist_pick","42"]`)
	insert(`["embed_filter","Frieren"]`)

	if err := goose.Up(database, migrationsDir); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	rows, err := database.Query(`SELECT selected_tools, kind FROM typesafe_classification_logs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct{ selected, kind string }{
		{`["reminder_create"]`, KindClassifier},
		{`[]`, KindClassifier},
		{`["anilist_pick","42"]`, KindAniListPick},
		{`["embed_filter","Frieren"]`, KindEmbedFilter},
	}
	i := 0
	for rows.Next() {
		var selected, kind string
		if err := rows.Scan(&selected, &kind); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || selected != want[i].selected || kind != want[i].kind {
			t.Errorf("row %d = (%s, %s), want %+v", i, selected, kind, want[min(i, len(want)-1)])
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("rows = %d, want %d", i, len(want))
	}
}

func TestStoreCreateDefaultsAndPersistsKind(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := goose.Up(database, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewStore(database)
	ctx := context.Background()

	defaulted, err := store.Create(ctx, CreateClassificationLogParams{GuildID: "g", ChannelID: "c", UserID: "u", Model: "jev"})
	if err != nil {
		t.Fatal(err)
	}
	if defaulted.Kind != KindClassifier {
		t.Errorf("empty kind stored as %q, want %q", defaulted.Kind, KindClassifier)
	}

	picked, err := store.Create(ctx, CreateClassificationLogParams{GuildID: "g", ChannelID: "c", UserID: "u", Model: "jev", Kind: KindAniListPick})
	if err != nil {
		t.Fatal(err)
	}
	if picked.Kind != KindAniListPick {
		t.Errorf("kind = %q, want %q", picked.Kind, KindAniListPick)
	}

	listed, err := store.ListByGuild(ctx, "g", 10)
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListByGuild = %d rows, err %v", len(listed), err)
	}
	kinds := map[string]bool{listed[0].Kind: true, listed[1].Kind: true}
	if !kinds[KindClassifier] || !kinds[KindAniListPick] {
		t.Errorf("listed kinds = %v", kinds)
	}
}
