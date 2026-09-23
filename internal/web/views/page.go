// Package views holds the templ components that render onerep's HTML.
package views

import (
	"encoding/json"

	"github.com/LongerHV/onerep/internal/auth"
)

// Page is the data every full page needs.
type Page struct {
	Title    string
	Identity *auth.Identity // nil for anonymous pages
}

// csrfHeaders returns the hx-headers value that makes htmx send the CSRF token.
func csrfHeaders(token string) string {
	b, _ := json.Marshal(map[string]string{auth.CSRFHeader: token})
	return string(b)
}
