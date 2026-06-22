package guildinstructions

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE guild_instructions (
		guild_id TEXT NOT NULL PRIMARY KEY,
		instructions TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStoreGetMissingInstruction(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	_, ok, err := store.Get(context.Background(), "guild-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false")
	}
}

func TestStoreUpsertInstruction(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	first, err := store.Upsert(context.Background(), "guild-1", "first rule")
	if err != nil {
		t.Fatalf("Upsert first: %v", err)
	}
	if first.Instructions != "first rule" {
		t.Fatalf("instructions = %q, want first rule", first.Instructions)
	}

	updated, err := store.Upsert(context.Background(), "guild-1", "updated rule")
	if err != nil {
		t.Fatalf("Upsert updated: %v", err)
	}
	if updated.Instructions != "updated rule" {
		t.Fatalf("instructions = %q, want updated rule", updated.Instructions)
	}

	got, ok, err := store.Get(context.Background(), "guild-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || got.Instructions != "updated rule" {
		t.Fatalf("instruction = %+v ok=%v, want updated rule", got, ok)
	}
}

func TestSeedInstructions(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	err := Seed(context.Background(), store, map[string]string{
		"guild-1": "seeded rule",
		"":        "ignored",
		"guild-2": " ",
	})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}

	instruction, ok, err := store.GetGuildInstruction(context.Background(), "guild-1")
	if err != nil {
		t.Fatalf("GetGuildInstruction: %v", err)
	}
	if !ok || instruction != "seeded rule" {
		t.Fatalf("instruction = %q ok=%v, want seeded rule", instruction, ok)
	}

	_, ok, err = store.GetGuildInstruction(context.Background(), "guild-2")
	if err != nil {
		t.Fatalf("GetGuildInstruction ignored: %v", err)
	}
	if ok {
		t.Fatalf("blank seed should be ignored")
	}
}
