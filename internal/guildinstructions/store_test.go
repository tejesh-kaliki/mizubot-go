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

func TestStoreDelete(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	ctx := context.Background()

	removed, err := store.Delete(ctx, "guild-1")
	if err != nil {
		t.Fatalf("Delete on empty table: %v", err)
	}
	if removed {
		t.Fatalf("removed = true for a guild with no instructions")
	}

	if _, err := store.Upsert(ctx, "guild-1", "be nice"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := store.Upsert(ctx, "guild-2", "be terse"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	removed, err = store.Delete(ctx, "guild-1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !removed {
		t.Fatalf("removed = false, want true")
	}
	if _, ok, err := store.Get(ctx, "guild-1"); err != nil || ok {
		t.Fatalf("Get after delete: ok=%v err=%v, want ok=false", ok, err)
	}

	// Deleting one guild must not touch another.
	if _, ok, err := store.Get(ctx, "guild-2"); err != nil || !ok {
		t.Fatalf("guild-2 instructions lost: ok=%v err=%v", ok, err)
	}
}

func TestStoreDeleteRequiresGuildID(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if _, err := NewStore(db).Delete(context.Background(), "  "); err == nil {
		t.Fatalf("Delete with a blank guild id should error")
	}
}
