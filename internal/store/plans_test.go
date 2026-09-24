package store

import (
	"context"
	"errors"
	"testing"
)

func TestPlanVersions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	p, v1, err := db.CreatePlan(ctx, alice.ID, "PPL", []byte(`{"v":1}`), PlanActive, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v1.Status != PlanActive {
		t.Fatalf("v1 = %+v", v1)
	}
	v2, err := db.SavePlanVersion(ctx, alice.ID, p.ID, "PPL v2", []byte(`{"v":2}`), PlanDraft, "mcp", "harder")
	if err != nil || v2.Version != 2 {
		t.Fatalf("v2 = %+v, %v", v2, err)
	}
	if got, _ := db.PlanByID(ctx, alice.ID, p.ID); got.Name != "PPL v2" {
		t.Fatalf("plan not renamed: %q", got.Name)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v1.ID {
		t.Fatal("a draft must not replace the active version")
	}

	if err := db.ActivateVersion(ctx, alice.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	vs, err := db.PlanVersions(ctx, alice.ID, p.ID)
	if err != nil || len(vs) != 2 || vs[0].Status != PlanActive || vs[1].Status != PlanSuperseded || string(vs[0].Doc) != `{"v":2}` {
		t.Fatalf("versions = %+v, %v", vs, err)
	}

	// Saving straight to active supersedes the current one.
	v3, err := db.SavePlanVersion(ctx, alice.ID, p.ID, "PPL", []byte(`{"v":3}`), PlanActive, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v3.ID || active.Version != 3 {
		t.Fatalf("active = %+v", active)
	}

	// Rolling back to an older version is allowed.
	if err := db.ActivateVersion(ctx, alice.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v1.ID {
		t.Fatal("rollback failed")
	}

	// Isolation.
	if _, err := db.PlanVersionByID(ctx, bob.ID, v1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob reads alice's version: %v", err)
	}
	if _, err := db.SavePlanVersion(ctx, bob.ID, p.ID, "x", []byte(`{}`), PlanDraft, "web", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob saves into alice's plan: %v", err)
	}
	if err := db.ActivateVersion(ctx, bob.ID, v2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob activates alice's version: %v", err)
	}
	if vs, _ := db.PlanVersions(ctx, bob.ID, p.ID); len(vs) != 0 {
		t.Fatal("bob lists alice's versions")
	}
}

func TestDeleteDraftOnly(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	p, v1, _ := db.CreatePlan(ctx, u.ID, "P", []byte(`{}`), PlanActive, "web", "")
	draft, _ := db.SavePlanVersion(ctx, u.ID, p.ID, "P", []byte(`{}`), PlanDraft, "web", "")
	if err := db.DeleteDraft(ctx, u.ID, v1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting the active version: %v", err)
	}
	if err := db.DeleteDraft(ctx, u.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	// A deleted draft's number is reused by the next version.
	next, _ := db.SavePlanVersion(ctx, u.ID, p.ID, "P", []byte(`{}`), PlanDraft, "web", "")
	if next.Version != 2 {
		t.Fatalf("next version = %d", next.Version)
	}
}

func TestActivePlanCursor(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	p, _, _ := db.CreatePlan(ctx, alice.ID, "P", []byte(`{}`), PlanActive, "web", "")

	if _, err := db.ActivePlan(ctx, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no active plan yet: %v", err)
	}
	if err := db.SetActivePlan(ctx, alice.ID, ActivePlan{PlanID: p.ID, Week: 1, Day: 0}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetActivePlan(ctx, alice.ID, ActivePlan{PlanID: p.ID, Week: 2, Day: 1}); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.ActivePlan(ctx, alice.ID); a != (ActivePlan{PlanID: p.ID, Week: 2, Day: 1}) {
		t.Fatalf("cursor = %+v", a)
	}
	if err := db.SetActivePlan(ctx, bob.ID, ActivePlan{PlanID: p.ID, Week: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob follows alice's plan: %v", err)
	}
	if err := db.ClearActivePlan(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ActivePlan(ctx, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cleared plan still active")
	}
}
