package plan

// DiffLine is one line of a line diff: Op is ' ' (same), '-' (removed) or '+' (added).
type DiffLine struct {
	Op   byte
	Text string
}

// maxDiffCells bounds the LCS table; larger inputs are shown as replaced.
const maxDiffCells = 4_000_000

// DiffLines returns a minimal line diff turning a into b (LCS based).
func DiffLines(a, b []string) []DiffLine {
	if len(a)*len(b) > maxDiffCells {
		var out []DiffLine
		for _, s := range a {
			out = append(out, DiffLine{'-', s})
		}
		for _, s := range b {
			out = append(out, DiffLine{'+', s})
		}
		return out
	}
	// lcs[i][j] = LCS length of a[i:] and b[j:].
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, DiffLine{' ', a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{'-', a[i]})
			i++
		default:
			out = append(out, DiffLine{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, DiffLine{'-', a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, DiffLine{'+', b[j]})
	}
	return out
}

// Changed reports whether a diff has any added or removed lines.
func Changed(d []DiffLine) bool {
	for _, l := range d {
		if l.Op != ' ' {
			return true
		}
	}
	return false
}
