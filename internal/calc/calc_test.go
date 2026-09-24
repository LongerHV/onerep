package calc

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type cases struct {
	RTS []struct {
		Reps int      `json:"reps"`
		RPE  float64  `json:"rpe"`
		Pct  *float64 `json:"pct"`
	} `json:"rts"`
	E1RM []struct {
		WeightKg float64  `json:"weight_kg"`
		Reps     int      `json:"reps"`
		RPE      float64  `json:"rpe"`
		Kg       *float64 `json:"kg"`
		Method   Method   `json:"method"`
	} `json:"e1rm"`
	Round []struct {
		Name         string     `json:"name"`
		TargetKg     float64    `json:"target_kg"`
		FallbackUnit string     `json:"fallback_unit"`
		Equipment    *Equipment `json:"equipment"`
		Kg           float64    `json:"kg"`
		PerSide      []float64  `json:"per_side"`
	} `json:"round"`
	Resolve []struct {
		Name    string      `json:"name"`
		Load    Load        `json:"load"`
		Reps    int         `json:"reps"`
		Ctx     LoadContext `json:"ctx"`
		Kg      *float64    `json:"kg"`
		PerSide []float64   `json:"per_side"`
	} `json:"resolve"`
	Drop []struct {
		Name    string      `json:"name"`
		PrevKg  float64     `json:"prev_kg"`
		Pct     float64     `json:"pct"`
		Ctx     LoadContext `json:"ctx"`
		Kg      float64     `json:"kg"`
		PerSide []float64   `json:"per_side"`
	} `json:"drop"`
}

func loadCases(t *testing.T) cases {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/calc_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c cases
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func samePlates(a, b []float64) bool {
	return slices.EqualFunc(a, b, near) || (len(a) == 0 && len(b) == 0)
}

func TestRTSVectors(t *testing.T) {
	for _, c := range loadCases(t).RTS {
		got, ok := RTSPercent(c.Reps, c.RPE)
		switch {
		case c.Pct == nil && ok:
			t.Errorf("RTSPercent(%d, %v) = %v, want no result", c.Reps, c.RPE, got)
		case c.Pct != nil && (!ok || !near(got, *c.Pct)):
			t.Errorf("RTSPercent(%d, %v) = %v, %v; want %v", c.Reps, c.RPE, got, ok, *c.Pct)
		}
	}
}

func TestE1RMVectors(t *testing.T) {
	for _, c := range loadCases(t).E1RM {
		got, method, ok := E1RM(c.WeightKg, c.Reps, c.RPE)
		switch {
		case c.Kg == nil && ok:
			t.Errorf("E1RM(%v, %d, %v) = %v, want no result", c.WeightKg, c.Reps, c.RPE, got)
		case c.Kg != nil && (!ok || !near(got, *c.Kg) || method != c.Method):
			t.Errorf("E1RM(%v, %d, %v) = %v %s %v; want %v %s", c.WeightKg, c.Reps, c.RPE, got, method, ok, *c.Kg, c.Method)
		}
	}
}

func TestRoundVectors(t *testing.T) {
	for _, c := range loadCases(t).Round {
		got := Round(c.TargetKg, c.Equipment, c.FallbackUnit)
		if !near(got.Kg, c.Kg) || !samePlates(got.PerSide, c.PerSide) {
			t.Errorf("%s: got %v %v, want %v %v", c.Name, got.Kg, got.PerSide, c.Kg, c.PerSide)
		}
	}
}

func TestResolveVectors(t *testing.T) {
	for _, c := range loadCases(t).Resolve {
		got, ok := ResolveLoad(c.Load, c.Reps, c.Ctx)
		switch {
		case c.Kg == nil && ok:
			t.Errorf("%s: got %v, want no result", c.Name, got)
		case c.Kg != nil && (!ok || !near(got.Kg, *c.Kg) || !samePlates(got.PerSide, c.PerSide)):
			t.Errorf("%s: got %v %v %v, want %v %v", c.Name, got.Kg, got.PerSide, ok, *c.Kg, c.PerSide)
		}
	}
}

func TestDropVectors(t *testing.T) {
	for _, c := range loadCases(t).Drop {
		got := DropLoad(c.PrevKg, c.Pct, c.Ctx)
		if !near(got.Kg, c.Kg) || !samePlates(got.PerSide, c.PerSide) {
			t.Errorf("%s: got %v %v, want %v %v", c.Name, got.Kg, got.PerSide, c.Kg, c.PerSide)
		}
	}
}

func TestRoundAbsurdTargetsStayBounded(t *testing.T) {
	bar := &Equipment{Kind: KindBarbell, Unit: UnitKg, Config: EquipmentConfig{Bar: 20, Plates: []float64{25, 1.25}}}
	got := Round(1e9, bar, UnitKg)
	if !near(got.Kg, 20+2*1000) {
		t.Fatalf("huge target: got %v, want capped at 1000 per side", got.Kg)
	}
	if got := Round(math.NaN(), nil, UnitKg); got.Kg != 0 {
		t.Fatalf("NaN target: got %v", got.Kg)
	}
}

func TestEquipmentValidate(t *testing.T) {
	cases := map[string]struct {
		eq   Equipment
		want string
	}{
		"ok barbell":         {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{25}}}, ""},
		"bad unit":           {Equipment{KindBarbell, "st", EquipmentConfig{Bar: 20, Plates: []float64{25}}}, "unit"},
		"no plates":          {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20}}, "plates"},
		"zero plate":         {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{0}}}, "positive"},
		"pairs unknown size": {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{25}, PlatePairs: map[string]int{"20": 1}}}, "plate_pairs"},
		"no dumbbells":       {Equipment{KindDumbbell, UnitKg, EquipmentConfig{}}, "weights"},
		"machine stack":      {Equipment{KindMachine, UnitKg, EquipmentConfig{Stack: []float64{5, 10}}}, ""},
		"machine range":      {Equipment{KindCable, UnitLb, EquipmentConfig{Min: 5, Step: 5, Max: 100}}, ""},
		"machine no step":    {Equipment{KindCable, UnitLb, EquipmentConfig{Min: 5, Max: 100}}, "stack"},
		"bodyweight":         {Equipment{KindBodyweight, UnitKg, EquipmentConfig{}}, ""},
		"unknown kind":       {Equipment{"kettlebell", UnitKg, EquipmentConfig{}}, "unknown"},
	}
	for name, c := range cases {
		err := c.eq.Validate()
		if c.want == "" && err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}

func TestUnitConversionRoundTrips(t *testing.T) {
	if !near(FromKg(ToKg(225, UnitLb), UnitLb), 225) || ToKg(100, UnitKg) != 100 {
		t.Fatal("conversion does not round-trip")
	}
}

// Profiles are user input; rounding must stay fast for any profile Validate accepts.
func TestRoundManyLimitedPlatesIsFast(t *testing.T) {
	var plates []float64
	pairs := map[string]int{}
	for i := 1; i <= maxPlateSizes; i++ {
		v := float64(i) / 100
		plates = append(plates, v)
		pairs[strconv.FormatFloat(v, 'f', -1, 64)] = 1000
	}
	eq := &Equipment{Kind: KindBarbell, Unit: UnitKg, Config: EquipmentConfig{Bar: 20, Plates: plates, PlatePairs: pairs}}
	if err := eq.Validate(); err != nil {
		t.Fatalf("profile should be valid: %v", err)
	}
	start := time.Now()
	Round(2250, eq, UnitKg)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("Round took %v", d)
	}
}

func TestValidateBoundsProfileSize(t *testing.T) {
	many := func(n int) []float64 {
		vs := make([]float64, n)
		for i := range vs {
			vs[i] = float64(i + 1)
		}
		return vs
	}
	bad := map[string]Equipment{
		"too many plate sizes": {KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: many(maxPlateSizes + 1)}},
		"too many dumbbells":   {KindDumbbell, UnitKg, EquipmentConfig{Weights: many(maxListValues + 1)}},
		"too many stack steps": {KindCable, UnitKg, EquipmentConfig{Stack: many(maxListValues + 1)}},
		"plate below 0.01":     {KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{0.001}}},
		"NaN plate":            {KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{math.NaN()}}},
		"NaN bar":              {KindBarbell, UnitKg, EquipmentConfig{Bar: math.NaN(), Plates: []float64{25}}},
		"tiny step":            {KindMachine, UnitKg, EquipmentConfig{Min: 5, Step: 0.001, Max: 100}},
	}
	for name, eq := range bad {
		if err := eq.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := Equipment{KindDumbbell, UnitKg, EquipmentConfig{Weights: many(maxListValues)}}
	if err := ok.Validate(); err != nil {
		t.Errorf("max-size dumbbell list rejected: %v", err)
	}
}
