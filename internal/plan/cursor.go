package plan

// CursorKind says what a version change does to the followed plan's cursor.
type CursorKind string

const (
	// CursorKept: the day still exists; nothing changes.
	CursorKept CursorKind = "kept"
	// CursorStaysComplete: the lifter had finished the plan; it stays complete.
	CursorStaysComplete CursorKind = "stays-complete"
	// CursorPastEnd: the cursor's week is past the new last week, so the plan
	// counts as complete.
	CursorPastEnd CursorKind = "past-end"
	// CursorDayMissing: the week exists but the day doesn't; the cursor moves
	// on to the next day that does (possibly completing the plan).
	CursorDayMissing CursorKind = "day-missing"
)

// CursorMove is where a version change puts the cursor. Week and Day are
// 1-based and 0-based like store.ActivePlan.
type CursorMove struct {
	Kind              CursorKind
	FromWeek, FromDay int
	Week, Day         int
	Complete          bool // the new position is past the last week
}

// Moved reports whether the cursor changes.
func (m CursorMove) Moved() bool { return m.Week != m.FromWeek || m.Day != m.FromDay }

// FitCursor fits a cursor at (week, day) into doc, the version being made
// active. oldWeeks is the length of the version it replaces (0 when unknown):
// a cursor past it means the lifter had finished the plan. A version change
// never restarts the plan: finished and past-the-end cursors become complete,
// and a missing day moves on to the next day that exists.
func FitCursor(oldWeeks int, doc Doc, week, day int) CursorMove {
	m := CursorMove{Kind: CursorKept, FromWeek: week, FromDay: day, Week: week, Day: day}
	complete := func(kind CursorKind) CursorMove {
		m.Kind, m.Week, m.Day, m.Complete = kind, doc.Weeks+1, 0, true
		return m
	}
	switch {
	case oldWeeks > 0 && week > oldWeeks:
		return complete(CursorStaysComplete)
	case week > doc.Weeks:
		return complete(CursorPastEnd)
	case ValidPosition(doc, week, day):
		return m
	}
	m.Kind = CursorDayMissing
	for m.Week, m.Day = max(week+1, 1), 0; m.Week <= doc.Weeks; m.Week++ {
		if ValidPosition(doc, m.Week, 0) {
			return m
		}
	}
	return complete(CursorDayMissing)
}
