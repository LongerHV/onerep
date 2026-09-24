package plan

// DiffLine is one line of a line diff: Op is ' ' (same), '-' (removed) or '+' (added).
type DiffLine struct {
	Op   byte
	Text string
}

// maxDiffCells bounds the LCS table; larger inputs are shown as replaced.
const maxDiffCells = 4_000_000

// DiffLines returns a minimal line diff turning a into b (LCS based). The
// common prefix and suffix are matched first, so a local edit in a long
// document only runs the LCS over the edited region.
func DiffLines(a, b []string) []DiffLine {
	var head, tail []DiffLine
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		head = append(head, DiffLine{' ', a[0]})
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		tail = append(tail, DiffLine{' ', a[len(a)-1]})
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	out := append(head, diffMiddle(a, b)...)
	for i := len(tail) - 1; i >= 0; i-- {
		out = append(out, tail[i])
	}
	return out
}

func diffMiddle(a, b []string) []DiffLine {
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
