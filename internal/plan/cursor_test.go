package plan

import "testing"

// A version change never restarts the plan: a finished plan stays complete,
// a missing week counts as complete and a missing day moves on to the next one.
func TestFitCursor(t *testing.T) {
	doc := exampleDoc(t) // 4 weeks; weeks 1-3: 2 days, week 4: 3 days
	cases := []struct {
		name             string
		oldWeeks         int
		week, day        int
		kind             CursorKind
		wantWeek, wanDay int
		complete         bool
	}{
		{"day still exists", 4, 2, 1, CursorKept, 2, 1, false},
		{"complete at the same length", 4, 5, 0, CursorStaysComplete, 5, 0, true},
		{"finished a shorter version", 2, 3, 0, CursorStaysComplete, 5, 0, true},
		{"finished a longer version", 8, 9, 0, CursorStaysComplete, 5, 0, true},
		{"week past the new end", 8, 7, 1, CursorPastEnd, 5, 0, true},
		{"missing day moves to the next week", 4, 3, 2, CursorDayMissing, 4, 0, false},
		{"missing last day completes", 4, 4, 5, CursorDayMissing, 5, 0, true},
		{"no old version known", 0, 5, 0, CursorPastEnd, 5, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := FitCursor(c.oldWeeks, doc, c.week, c.day)
			if m.Kind != c.kind || m.Week != c.wantWeek || m.Day != c.wanDay || m.Complete != c.complete ||
				m.FromWeek != c.week || m.FromDay != c.day {
				t.Fatalf("FitCursor(%d, doc, %d, %d) = %+v", c.oldWeeks, c.week, c.day, m)
			}
			if m.Moved() != ([2]int{c.week, c.day} != [2]int{m.Week, m.Day}) {
				t.Fatalf("Moved() = %v", m.Moved())
			}
		})
	}
}
