package guildflags

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"

	"database/sql"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE guild_flags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		guidance TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(guild_id, name)
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
	ctx := context.Background()

	created, err := store.Create(ctx, "guild-1", "no_spoilers", "Message references a spoiler.", "Refuse briefly without confirming details.")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created.ID = 0, want nonzero")
	}

	if _, err := store.Create(ctx, "guild-2", "other", "desc", "guidance"); err != nil {
		t.Fatalf("Create guild-2: %v", err)
	}

	flags, err := store.ListByGuild(ctx, "guild-1")
	if err != nil {
		t.Fatalf("ListByGuild: %v", err)
	}
	if len(flags) != 1 || flags[0].Name != "no_spoilers" {
		t.Fatalf("flags = %+v, want one no_spoilers flag", flags)
	}
}

func TestStoreUpdateAndDelete(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()

	created, err := store.Create(ctx, "guild-1", "flag", "old desc", "old guidance")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := store.Update(ctx, created.ID, "guild-1", "new desc", "new guidance")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Description != "new desc" || updated.Guidance != "new guidance" {
		t.Fatalf("updated = %+v, want new desc/guidance", updated)
	}

	removed, err := store.Delete(ctx, created.ID, "guild-1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !removed {
		t.Fatalf("removed = false, want true")
	}

	flags, err := store.ListByGuild(ctx, "guild-1")
	if err != nil {
		t.Fatalf("ListByGuild after delete: %v", err)
	}
	if len(flags) != 0 {
		t.Fatalf("flags = %+v, want none after delete", flags)
	}
}

func TestStoreCreateRequiresFields(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()

	if _, err := store.Create(ctx, "", "name", "desc", "guidance"); err == nil {
		t.Fatal("expected error for missing guild id")
	}
	if _, err := store.Create(ctx, "guild-1", "", "desc", "guidance"); err == nil {
		t.Fatal("expected error for missing name")
	}
	if _, err := store.Create(ctx, "guild-1", "name", "", "guidance"); err == nil {
		t.Fatal("expected error for missing description")
	}
	if _, err := store.Create(ctx, "guild-1", "name", "desc", ""); err == nil {
		t.Fatal("expected error for missing guidance")
	}
}
