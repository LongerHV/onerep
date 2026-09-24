package exercise

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestParseWeights(t *testing.T) {
	cases := map[string][]float64{
		"2-10/2, 12.5":     {2, 4, 6, 8, 10, 12.5},
		"25, 20, 20, 1.25": {1.25, 20, 25},
		"1.25-5/1.25":      {1.25, 2.5, 3.75, 5},
		" ":                nil,
		"5-6/2":            {5},
	}
	for in, want := range cases {
		got, err := ParseWeights(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ParseWeights(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "-5", "0", "2-10", "10-2/2", "2-10/0", "1-100000/0.01"} {
		if _, err := ParseWeights(bad); err == nil {
			t.Errorf("ParseWeights(%q) should fail", bad)
		}
	}
}

func TestFormatWeightsRoundTrips(t *testing.T) {
	for _, in := range []string{"2-50/2", "1.25, 2.5, 5, 10, 15, 20, 25", "5-100/5, 110, 120.5"} {
		vs, err := ParseWeights(in)
		if err != nil {
			t.Fatal(err)
		}
		again, err := ParseWeights(FormatWeights(vs))
		if err != nil || !slices.Equal(again, vs) {
			t.Errorf("%q: round trip via %q gave %v", in, FormatWeights(vs), again)
		}
	}
	if got := FormatWeights([]float64{2, 4, 6, 8}); got != "2-8/2" {
		t.Errorf("FormatWeights = %q, want 2-8/2", got)
	}
}

func TestPlatePairs(t *testing.T) {
	got, err := ParsePlatePairs("1.25:2, 2.50:4")
	if err != nil || !maps.Equal(got, map[string]int{"1.25": 2, "2.5": 4}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if FormatPlatePairs(got) != "1.25:2, 2.5:4" {
		t.Fatalf("format: %q", FormatPlatePairs(got))
	}
	if got, err := ParsePlatePairs(""); err != nil || got != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	if _, err := ParsePlatePairs("1.25"); err == nil || !strings.Contains(err.Error(), "1.25:2") {
		t.Fatalf("missing count should explain the format, got %v", err)
	}
}

func TestFormatWeightsSortsInput(t *testing.T) {
	in := []float64{25, 20, 15, 10, 5, 2.5, 1.25}
	if got := FormatWeights(in); got != "1.25, 2.5, 5-25/5" {
		t.Fatalf("FormatWeights(descending) = %q", got)
	}
	if in[0] != 25 {
		t.Fatal("FormatWeights must not reorder its argument")
	}
}
