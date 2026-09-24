package training

import (
	"context"
	"time"

	"github.com/LongerHV/onerep/internal/store"
)

// HistoryPageSize is the number of sessions per history page.
const HistoryPageSize = 20

// History lists sessions newest first; more reports whether another page exists.
func (s *Service) History(ctx context.Context, user store.User, page int) (sessions []store.SessionSummary, more bool, err error) {
	if page < 0 {
		page = 0
	}
	list, err := s.Store.ListSessions(ctx, user.ID, HistoryPageSize+1, page*HistoryPageSize)
	if err != nil {
		return nil, false, err
	}
	if len(list) > HistoryPageSize {
		return list[:HistoryPageSize], true, nil
	}
	return list, false, nil
}

// Session returns one of the user's sessions with its sets.
func (s *Service) Session(ctx context.Context, user store.User, id string) (store.Session, []store.Set, error) {
	sess, err := s.Store.SessionByID(ctx, user.ID, id)
	if err != nil {
		return store.Session{}, nil, err
	}
	sets, err := s.Store.SessionSets(ctx, user.ID, id)
	return sess, sets, err
}

// SaveSet creates or corrects a set from the history editor. The edit is
// stamped with the server's clock, so it wins over older companion edits.
func (s *Service) SaveSet(ctx context.Context, user store.User, in SetInput) error {
	in.UpdatedAt = s.now()
	// A companion edit may carry a (clamped) timestamp a few minutes ahead of
	// the server; the history edit is newer from the user's point of view.
	sets, err := s.Store.SessionSets(ctx, user.ID, in.SessionID)
	if err != nil {
		return err
	}
	for _, existing := range sets {
		if existing.ID == in.ID && !in.UpdatedAt.After(existing.UpdatedAt) {
			in.UpdatedAt = existing.UpdatedAt.Add(time.Millisecond)
		}
	}
	set, err := s.toStore(ctx, user, in)
	if err != nil {
		return err
	}
	_, err = s.Store.UpsertSet(ctx, user.ID, set, "")
	return err
}

func (s *Service) DeleteSet(ctx context.Context, user store.User, sessionID, setID string) error {
	_, err := s.Store.DeleteSet(ctx, user.ID, sessionID, setID, s.now(), "")
	return err
}

func (s *Service) SetNotes(ctx context.Context, user store.User, sessionID, notes string) error {
	if len(notes) > 10000 {
		return InvalidError{"notes are too long"}
	}
	_, err := s.Store.SetSessionNotes(ctx, user.ID, sessionID, notes, s.now(), "")
	return err
}

// Finish finishes a session from the history editor (advancing the plan
// like the companion does).
func (s *Service) Finish(ctx context.Context, user store.User, sessionID string) error {
	_, err := s.finish(ctx, user, sessionID, s.now(), "")
	return err
}

func (s *Service) Delete(ctx context.Context, user store.User, sessionID string) error {
	return s.Store.DeleteSession(ctx, user.ID, sessionID)
}

// Sessions lists the user's sessions matching f, newest first (limit 20 by default, at most 100).
func (s *Service) Sessions(ctx context.Context, user store.User, f store.SessionFilter) ([]store.SessionSummary, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	f.Limit = min(f.Limit, 100)
	return s.Store.SearchSessions(ctx, user.ID, f)
}
