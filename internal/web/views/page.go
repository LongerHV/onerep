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

// htmxConfig makes htmx swap every response except 204 No Content: error
// pages and forms re-rendered with errors (422) must reach the page too.
const htmxConfig = `{"responseHandling":[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"[45]..","swap":true,"error":true}]}`
