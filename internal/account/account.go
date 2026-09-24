// Package account manages a user's own preferences.
package account

import (
	"context"
	"errors"

	"github.com/LongerHV/onerep/internal/calc"
)

// Store is what the service needs. *store.DB implements it.
type Store interface {
	UpdateUserSettings(ctx context.Context, userID, unit string, e1rmWindowDays int) error
}

type Service struct {
	Store Store
}

// ErrInvalidSettings is returned for a unit other than kg/lb or a window outside 7–365 days.
var ErrInvalidSettings = errors.New("unit must be kg or lb and the e1RM window 7 to 365 days")

// UpdateSettings changes the display unit and the e1RM look-back window.
// Stored weights are always kg, so changing the unit converts nothing.
func (s *Service) UpdateSettings(ctx context.Context, userID, unit string, e1rmWindowDays int) error {
	if (unit != calc.UnitKg && unit != calc.UnitLb) || e1rmWindowDays < 7 || e1rmWindowDays > 365 {
		return ErrInvalidSettings
	}
	return s.Store.UpdateUserSettings(ctx, userID, unit, e1rmWindowDays)
}
