package account

import (
	"context"
	"errors"
	"testing"

	"github.com/LongerHV/onerep/internal/store/storetest"
)

func TestUpdateSettings(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	u, err := db.UpsertOIDCUser(ctx, "iss", "sub", "", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: db}

	if err := s.UpdateSettings(ctx, u.ID, "lb", 60); err != nil {
		t.Fatal(err)
	}
	got, _ := db.UserByID(ctx, u.ID)
	if got.Unit != "lb" || got.E1RMWindowDays != 60 {
		t.Fatalf("settings not saved: %+v", got)
	}
	for _, bad := range []struct {
		unit string
		days int
	}{{"stone", 30}, {"kg", 6}, {"kg", 366}} {
		if err := s.UpdateSettings(ctx, u.ID, bad.unit, bad.days); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%+v: got %v", bad, err)
		}
	}
}
