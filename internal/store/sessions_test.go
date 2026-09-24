package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

func newSession(t *testing.T, db *DB, userID, name string) Session {
	t.Helper()
	s, err := db.CreateSession(context.Background(), Session{UserID: userID, Name: name, Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func set(sessionID, id string, kg float64, updated time.Time) Set {
	reps := 5
	done := updated
	return Set{ID: id, SessionID: sessionID, Slug: "barbell-back-squat", Kind: "working", WeightKg: &kg, Reps: &reps,
		DoneAt: &done, UpdatedAt: updated}
}

func TestUpsertSetSyncRules(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	s := newSession(t, db, alice.ID, "A")

	if out, err := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 100, t0), "op-1"); err != nil || out != Applied {
		t.Fatalf("insert: %v %v", out, err)
	}
	// The same op again changes nothing, even with different content.
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 999, t0.Add(time.Hour)), "op-1"); out != Duplicate {
		t.Fatalf("replay: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 102.5, t0.Add(time.Minute)), "op-2"); out != Applied {
		t.Fatalf("newer edit: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 90, t0.Add(30*time.Second)), "op-3"); out != Ignored {
		t.Fatalf("older edit: %v", out)
	}
	sets, _ := db.SessionSets(ctx, alice.ID, s.ID)
	if len(sets) != 1 || *sets[0].WeightKg != 102.5 {
		t.Fatalf("sets = %+v", sets)
	}

	if _, err := db.UpsertSet(ctx, bob.ID, set(s.ID, "set-2", 1, t0), "op-4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob writes into alice's session: %v", err)
	}
	other := newSession(t, db, alice.ID, "B")
	if _, err := db.UpsertSet(ctx, alice.ID, set(other.ID, "set-1", 1, t0.Add(time.Hour)), "op-5"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moving a set to another session: %v", err)
	}
}

func TestDeletedSetsStayDeleted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "A")

	_, _ = db.UpsertSet(ctx, u.ID, set(s.ID, "set-1", 100, t0), "op-1")
	if out, err := db.DeleteSet(ctx, u.ID, s.ID, "set-1", t0.Add(time.Minute), "op-2"); err != nil || out != Applied {
		t.Fatalf("delete: %v %v", out, err)
	}
	// An edit made before the delete but synced after it must not revive the set.
	if out, _ := db.UpsertSet(ctx, u.ID, set(s.ID, "set-1", 105, t0.Add(2*time.Minute)), "op-3"); out != Ignored {
		t.Fatalf("late upsert: %v", out)
	}
	// Deleted before the server ever saw it (both ops queued offline, delivered out of order).
	if out, _ := db.DeleteSet(ctx, u.ID, s.ID, "set-2", t0.Add(time.Minute), "op-4"); out != Ignored {
		t.Fatalf("delete unseen: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, u.ID, set(s.ID, "set-2", 100, t0), "op-5"); out != Ignored {
		t.Fatalf("upsert after tombstone: %v", out)
	}
	if sets, _ := db.SessionSets(ctx, u.ID, s.ID); len(sets) != 0 {
		t.Fatalf("deleted sets visible: %+v", sets)
	}
}

func TestSessionNotesAndFinish(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "A")
	later := time.Now().Add(time.Hour)

	if out, _ := db.SetSessionNotes(ctx, u.ID, s.ID, "felt heavy", later, "op-1"); out != Applied {
		t.Fatalf("notes: %v", out)
	}
	if out, _ := db.SetSessionNotes(ctx, u.ID, s.ID, "older text", later.Add(-time.Minute), "op-2"); out != Ignored {
		t.Fatalf("older notes: %v", out)
	}
	if open, err := db.OpenSession(ctx, u.ID); err != nil || open.ID != s.ID {
		t.Fatalf("open session = %+v, %v", open, err)
	}
	if out, _ := db.FinishSession(ctx, u.ID, s.ID, later, "op-3"); out != Applied {
		t.Fatalf("finish: %v", out)
	}
	if out, _ := db.FinishSession(ctx, u.ID, s.ID, later, "op-4"); out != Ignored {
		t.Fatalf("finish twice: %v", out)
	}
	got, _ := db.SessionByID(ctx, u.ID, s.ID)
	if got.Notes != "felt heavy" || got.FinishedAt == nil {
		t.Fatalf("session = %+v", got)
	}
	if _, err := db.OpenSession(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("finished session still open")
	}
}

func TestHistoryQueries(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	old := newSession(t, db, u.ID, "old")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(old.ID, "a", 100, t0.Add(-60*24*time.Hour)), 130), "")
	prev := newSession(t, db, u.ID, "prev")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(prev.ID, "b", 100, t0), 120), "")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(prev.ID, "c", 105, t0), 125), "")
	_, _ = db.DeleteSet(ctx, u.ID, prev.ID, "c", t0.Add(time.Minute), "")
	cur := newSession(t, db, u.ID, "current")

	best, err := db.BestE1RM(ctx, u.ID, "barbell-back-squat", t0.Add(-30*24*time.Hour))
	if err != nil || best == nil || *best != 120 {
		t.Fatalf("best e1RM in window = %v, %v (older and deleted sets must not count)", best, err)
	}
	if none, _ := db.BestE1RM(ctx, u.ID, "pull-up", t0.Add(-time.Hour)); none != nil {
		t.Fatalf("no sets: %v", *none)
	}
	last, err := db.LastSets(ctx, u.ID, "barbell-back-squat", cur.ID)
	if err != nil || len(last) != 1 || last[0].ID != "b" {
		t.Fatalf("last sets = %+v, %v", last, err)
	}

	list, err := db.ListSessions(ctx, u.ID, 10, 0)
	if err != nil || len(list) != 3 || list[0].Name != "current" || list[1].Sets != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteSession(ctx, u.ID, prev.ID); err != nil {
		t.Fatal(err)
	}
	if sets, _ := db.SessionSets(ctx, u.ID, prev.ID); len(sets) != 0 {
		t.Fatal("sets of a deleted session remain")
	}
}

func withE1RM(s Set, kg float64) Set {
	s.E1RMKg = &kg
	return s
}
