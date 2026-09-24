package plan

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed plan.schema.json
var schemaJSON []byte

// Schema returns the plan JSON Schema (published at /schema/plan.json).
func Schema() []byte { return schemaJSON }

// schemaID is the $id in plan.schema.json.
const schemaID = "https://onerep.local/schema/plan.json"

var compiled = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaID, doc); err != nil {
		return nil, err
	}
	return c.Compile(schemaID)
})

// Problem is a validation finding located by a JSON Pointer into the document.
type Problem struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}

// Problems is returned as an error when a document has errors.
type Problems []Problem

func (ps Problems) HasErrors() bool {
	return slices.ContainsFunc(ps, func(p Problem) bool { return !p.Warning })
}

func (ps Problems) Error() string {
	var parts []string
	for _, p := range ps {
		if !p.Warning {
			parts = append(parts, p.Pointer+": "+p.Message)
		}
	}
	return "invalid plan: " + strings.Join(parts, "; ")
}

// Lookup answers the questions validation asks about the user's catalog.
type Lookup interface {
	// Exercise reports whether slug exists for the user and whether it is hidden.
	Exercise(slug string) (exists, hidden bool)
	HasTrainingMax(slug string) bool
}

// Validate checks raw against the JSON Schema, then the rules the schema
// cannot express. The Doc is only meaningful when there are no errors.
func Validate(raw []byte, lookup Lookup) (Doc, Problems) {
	var doc Doc
	var syntax *json.SyntaxError
	if err := json.Unmarshal(raw, new(any)); errors.As(err, &syntax) {
		line, col := position(raw, syntax.Offset)
		return doc, Problems{{Pointer: "", Message: fmt.Sprintf("invalid JSON at line %d, column %d: %s", line, col, syntax.Error())}}
	} else if err != nil {
		return doc, Problems{{Pointer: "", Message: "invalid JSON: " + err.Error()}}
	}
	if ps := schemaProblems(raw); len(ps) > 0 {
		return doc, ps
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, Problems{{Pointer: "", Message: err.Error()}}
	}
	return doc, semanticProblems(doc, lookup)
}

func position(raw []byte, offset int64) (line, col int) {
	line, col = 1, 1
	for i := int64(0); i < offset-1 && i < int64(len(raw)); i++ {
		if raw[i] == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

func schemaProblems(raw []byte) Problems {
	sch, err := compiled()
	if err != nil {
		return Problems{{Message: "plan schema failed to compile: " + err.Error()}}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Problems{{Message: "invalid JSON: " + err.Error()}}
	}
	var ve *jsonschema.ValidationError
	if err := sch.Validate(inst); !errors.As(err, &ve) {
		return nil
	}
	var out Problems
	seen := map[string]bool{}
	add := func(e *jsonschema.ValidationError, msg string) {
		ptr := pointer(e.InstanceLocation)
		if key := ptr + "\x00" + msg; !seen[key] {
			seen[key] = true
			out = append(out, Problem{Pointer: ptr, Message: msg})
		}
	}
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if msg, ok := friendlier(e); ok {
			add(e, msg)
			return
		}
		causes := e.Causes
		switch e.ErrorKind.(type) {
		case *kind.OneOf, *kind.AnyOf:
			// A value that fails every alternative: report the alternatives
			// that got past the type check, or else which types are allowed.
			var kept []*jsonschema.ValidationError
			var wants []string
			for _, c := range causes {
				if w, ok := typeMismatch(c, len(e.InstanceLocation)); ok {
					wants = append(wants, w...)
				} else {
					kept = append(kept, c)
				}
			}
			if len(kept) == 0 && len(wants) > 0 {
				add(e, "expected "+strings.Join(slices.Compact(slices.Sorted(slices.Values(wants))), " or "))
				return
			}
			causes = kept
		}
		if len(causes) == 0 {
			add(e, e.ErrorKind.LocalizedString(printer))
			return
		}
		for _, c := range causes {
			walk(c)
		}
	}
	walk(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

var printer = message.NewPrinter(language.English)

// typeMismatch reports whether e (a oneOf branch) failed only because the
// value at depth has the wrong type, and which types it wanted.
func typeMismatch(e *jsonschema.ValidationError, depth int) ([]string, bool) {
	if len(e.InstanceLocation) != depth {
		return nil, false
	}
	if t, ok := e.ErrorKind.(*kind.Type); ok {
		return t.Want, true
	}
	if len(e.Causes) == 0 {
		return nil, false
	}
	var wants []string
	for _, c := range e.Causes {
		w, ok := typeMismatch(c, depth)
		if !ok {
			return nil, false
		}
		wants = append(wants, w...)
	}
	return wants, true
}

// pointer turns an instance location into a JSON Pointer.
func pointer(loc []string) string {
	var b strings.Builder
	for _, tok := range loc {
		b.WriteString("/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(tok))
	}
	return b.String()
}

func semanticProblems(doc Doc, lookup Lookup) Problems {
	if n := totalSets(doc); n > MaxTotalSets {
		return Problems{{Message: fmt.Sprintf("the plan has %d sets in total; at most %d are allowed", n, MaxTotalSets)}}
	}
	var ps Problems
	errorf := func(ptr, format string, args ...any) {
		ps = append(ps, Problem{Pointer: ptr, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(ptr, format string, args ...any) {
		ps = append(ps, Problem{Pointer: ptr, Message: fmt.Sprintf(format, args...), Warning: true})
	}
	perWeek := func(ptr string, n int, list bool) {
		if list && n != doc.Weeks {
			errorf(ptr, "has %d values but the plan has %d weeks", n, doc.Weeks)
		}
	}
	checkSlug := func(ptr, slug string) bool {
		exists, hidden := lookup.Exercise(slug)
		switch {
		case !exists:
			errorf(ptr, "unknown exercise %q", slug)
		case hidden:
			warnf(ptr, "%q is no longer in the catalog", slug)
		}
		return exists
	}

	for w := 1; w <= doc.Weeks; w++ {
		if len(DaysForWeek(doc, w)) == 0 {
			errorf("/days", "week %d has no training days (check only_weeks)", w)
		}
	}
	for di, day := range doc.Days {
		dp := "/days/" + strconv.Itoa(di)
		for i, w := range day.OnlyWeeks {
			if w > doc.Weeks {
				errorf(dp+"/only_weeks/"+strconv.Itoa(i), "week %d is past the plan's %d weeks", w, doc.Weeks)
			}
		}
		for gi, g := range day.Groups {
			gp := dp + "/groups/" + strconv.Itoa(gi)
			perWeek(gp+"/rest_s", len(g.RestS.Values), g.RestS.List)
			for si, slot := range g.Exercises {
				sp := gp + "/exercises/" + strconv.Itoa(si)
				known := checkSlug(sp+"/slug", slot.Slug)
				for ai, alt := range slot.Alternatives {
					checkSlug(sp+"/alternatives/"+strconv.Itoa(ai), alt)
				}
				for li, line := range slot.Sets {
					lp := sp + "/sets/" + strconv.Itoa(li)
					perWeek(lp+"/count", len(line.Count.Values), line.Count.List)
					perWeek(lp+"/reps", len(line.Reps.Values), line.Reps.List)
					perWeek(lp+"/duration_s", len(line.DurationS.Values), line.DurationS.List)
					perWeek(lp+"/rpe", len(line.RPE.Values), line.RPE.List)
					for ri, r := range line.Reps.Values {
						if !r.AMRAP && r.Min > r.Max {
							ptr := lp + "/reps"
							if line.Reps.List {
								ptr += "/" + strconv.Itoa(ri)
							}
							errorf(ptr, "range %s goes down", r)
						}
					}
					if line.Load == nil {
						continue
					}
					l := line.Load
					for name, v := range map[string]PerWeek[float64]{"weight": l.Weight, "pct_tm": l.PctTM, "rpe": l.RPE, "drop_pct": l.DropPct} {
						perWeek(lp+"/load/"+name, len(v.Values), v.List)
					}
					if l.DropPct.Set() && li == 0 {
						errorf(lp+"/load/drop_pct", "a drop set needs a previous set line to drop from")
					}
					if l.PctTM.Set() && known && !lookup.HasTrainingMax(slot.Slug) {
						warnf(lp+"/load/pct_tm", "no training max set for %q, so these weights will be left for you to pick", slot.Slug)
					}
					if l.RPE.Set() && !rpeResolvable(line) {
						warnf(lp+"/load/rpe", "RPE loads need 1-12 reps; the weight will be left for you to pick")
					}
				}
			}
		}
	}
	return ps
}

func rpeResolvable(line SetLine) bool {
	for _, r := range line.Reps.Values {
		if r.AMRAP || r.Min < 1 || r.Min > 12 {
			return false
		}
	}
	return line.Reps.Set()
}

// friendlier replaces library wording for rules people break most often.
func friendlier(e *jsonschema.ValidationError) (string, bool) {
	if _, ok := e.ErrorKind.(*kind.OneOf); !ok || !strings.HasSuffix(e.SchemaURL, "#/$defs/setLine") {
		return "", false
	}
	if len(e.Causes) == 0 { // both alternatives matched
		return "give reps or duration_s, not both", true
	}
	return "give reps (or duration_s for timed sets)", true
}

// MaxTotalSets bounds the sets of a whole plan (a year of 5 days a week with
// 25 sets a day is about 6500). Every preview expands the whole plan.
const MaxTotalSets = 20000

// totalSets counts the sets of every week without expanding them.
func totalSets(doc Doc) int {
	n := 0
	for w := 1; w <= doc.Weeks; w++ {
		for _, di := range DaysForWeek(doc, w) {
			for _, g := range doc.Days[di].Groups {
				for _, slot := range g.Exercises {
					for _, line := range slot.Sets {
						if c := line.Count.At(w); c != nil {
							n += *c
						}
					}
				}
			}
		}
	}
	return n
}
