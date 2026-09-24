package exercise

import (
	"errors"
	"sort"
	"strings"
)

// FieldErrors maps form field names to messages. It is returned for invalid input.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	var parts []string
	for k, v := range f {
		parts = append(parts, k+": "+v)
	}
	sort.Strings(parts)
	return "invalid input: " + strings.Join(parts, "; ")
}

// ErrNoTrainingMax is returned when a calculation needs a training max that is not set.
var ErrNoTrainingMax = errors.New("no training max set")
