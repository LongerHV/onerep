package exercise

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ParseWeights parses a comma-separated weight list where "a-b/s" expands to
// a, a+s, ... up to b. "2-10/2, 12.5" gives [2 4 6 8 10 12.5]. The result is
// sorted and de-duplicated.
func ParseWeights(s string) ([]float64, error) {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, rest, ok := strings.Cut(part, "-"); ok && lo != "" {
			hi, step, ok := strings.Cut(rest, "/")
			if !ok {
				return nil, fmt.Errorf("%q: a range needs a step, like 2-50/2", part)
			}
			a, err1 := parsePositive(lo)
			b, err2 := parsePositive(hi)
			st, err3 := parsePositive(step)
			if err1 != nil || err2 != nil || err3 != nil || b < a {
				return nil, fmt.Errorf("%q: not a valid range", part)
			}
			if (b-a)/st > 1000 {
				return nil, fmt.Errorf("%q: range has too many values", part)
			}
			for i := 0; ; i++ {
				v := math.Round((a+float64(i)*st)*100) / 100
				if v > b+1e-9 {
					break
				}
				out = append(out, v)
			}
			continue
		}
		v, err := parsePositive(part)
		if err != nil {
			return nil, fmt.Errorf("%q: not a positive number", part)
		}
		out = append(out, v)
	}
	sort.Float64s(out)
	dedup := out[:0]
	for i, v := range out {
		if i == 0 || v != out[i-1] {
			dedup = append(dedup, v)
		}
	}
	return dedup, nil
}

func parsePositive(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 || math.IsInf(v, 0) {
		return 0, fmt.Errorf("not a positive number: %q", s)
	}
	return v, nil
}

// FormatWeights is the inverse of ParseWeights: values are sorted and runs of
// three or more equally spaced values are written as ranges.
func FormatWeights(vs []float64) string {
	vs = slices.Sorted(slices.Values(vs))
	var parts []string
	for i := 0; i < len(vs); {
		j := i + 1
		if j < len(vs) {
			step := vs[j] - vs[i]
			for j+1 < len(vs) && math.Abs(vs[j+1]-vs[j]-step) < 1e-9 {
				j++
			}
			if j-i >= 2 {
				parts = append(parts, FormatNumber(vs[i])+"-"+FormatNumber(vs[j])+"/"+FormatNumber(step))
				i = j + 1
				continue
			}
		}
		parts = append(parts, FormatNumber(vs[i]))
		i++
	}
	return strings.Join(parts, ", ")
}

// FormatNumber prints v with at most two decimals and no trailing zeros.
func FormatNumber(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// ParsePlatePairs parses "1.25:2, 2.5:4" (plate size: number of pairs).
func ParsePlatePairs(s string) (map[string]int, error) {
	out := map[string]int{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		size, count, ok := strings.Cut(part, ":")
		v, err1 := parsePositive(size)
		n, err2 := strconv.Atoi(strings.TrimSpace(count))
		if !ok || err1 != nil || err2 != nil || n < 0 {
			return nil, fmt.Errorf("%q: write plate size and number of pairs, like 1.25:2", part)
		}
		out[FormatNumber(v)] = n
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// FormatPlatePairs is the inverse of ParsePlatePairs.
func FormatPlatePairs(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := strconv.ParseFloat(keys[i], 64)
		b, _ := strconv.ParseFloat(keys[j], 64)
		return a < b
	})
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
