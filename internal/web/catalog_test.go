package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
)

// session opens the app as the dev user and returns a CSRF token.
func session(t *testing.T, srv *httptest.Server, c *http.Client) string {
	t.Helper()
	m := csrfInput.FindStringSubmatch(read(t, mustGet(t, c, srv.URL+"/")))
	if m == nil {
		t.Fatal("no CSRF token on home page")
	}
	return m[1]
}

func post(t *testing.T, c *http.Client, u, csrf string, form url.Values) (*http.Response, string) {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set(auth.CSRFField, csrf)
	resp, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp, read(t, resp)
}

func htmx(t *testing.T, c *http.Client, u, target string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", target)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d", u, resp.StatusCode)
	}
	return read(t, resp)
}

func TestExerciseSearchFragment(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)

	full := read(t, mustGet(t, c, srv.URL+"/exercises"))
	if !strings.Contains(full, "Barbell Back Squat") || !strings.Contains(full, "<html") {
		t.Fatal("catalog page incomplete")
	}
	frag := htmx(t, c, srv.URL+"/exercises?q=bench+dumbbell", "exercise-results")
	if strings.Contains(frag, "<html") || !strings.Contains(frag, "Dumbbell Bench Press") || strings.Contains(frag, "Barbell Back Squat") {
		t.Fatalf("search fragment:\n%s", frag)
	}
}

func TestCreateCustomizeAndDeleteExercise(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)

	form := url.Values{"name": {"Zercher Squat"}, "slug": {"zercher-squat"}, "measurement": {"weight_reps"},
		"equipment_kind": {"barbell"}, "primary_muscles": {"quads", "glutes"}, "aliases": {"zercher, "}}
	resp, _ := post(t, c, srv.URL+"/exercises", csrf, form)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/exercises/zercher-squat" {
		t.Fatalf("create: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	page := read(t, mustGet(t, c, srv.URL+"/exercises/zercher-squat"))
	if !strings.Contains(page, "Zercher Squat") || !strings.Contains(page, "custom") {
		t.Fatalf("detail page:\n%s", page)
	}

	form.Set("slug", "zercher-squat")
	resp, body := post(t, c, srv.URL+"/exercises", csrf, form)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "already exists") {
		t.Fatalf("duplicate slug: %d", resp.StatusCode)
	}

	// Customizing a seeded exercise, then resetting it.
	form.Set("name", "Squat (low bar)")
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat", csrf, form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("customize: %d", resp.StatusCode)
	}
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, "Reset to default") {
		t.Fatal("customized exercise offers no reset")
	}
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat/delete", csrf, nil)
	if resp.Header.Get("Location") != "/exercises/barbell-back-squat" {
		t.Fatalf("reset redirect: %s", resp.Header.Get("Location"))
	}
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, "Barbell Back Squat") {
		t.Fatal("reset did not restore the seeded exercise")
	}

	resp, _ = post(t, c, srv.URL+"/exercises/zercher-squat/delete", csrf, nil)
	if resp.Header.Get("Location") != "/exercises" {
		t.Fatalf("delete redirect: %s", resp.Header.Get("Location"))
	}
	if resp := mustGet(t, c, srv.URL+"/exercises/zercher-squat"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted exercise: %d", resp.StatusCode)
	}
}

func TestTrainingMaxAndCalculator(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)

	resp, _ := post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"140"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("set TM: %d", resp.StatusCode)
	}
	frag := htmx(t, c, srv.URL+"/exercises/barbell-back-squat/calc?pct=75", "calc-result")
	for _, want := range []string{"75% of 140 kg", "105 kg", "25 · 15 · 2.5 kg on Barbell"} {
		if !strings.Contains(frag, want) {
			t.Fatalf("calculator missing %q:\n%s", want, frag)
		}
	}

	// A decimal comma is accepted.
	post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"142,5"}})
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, `value="142.5"`) {
		t.Fatal("decimal comma TM not saved as 142.5")
	}
	post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"140"}})

	resp, body := post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"heavy"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "enter a number") {
		t.Fatalf("bad TM: %d", resp.StatusCode)
	}

	// Switching to pounds shows the same TM converted; nothing is re-stored.
	resp, _ = post(t, c, srv.URL+"/settings", csrf, url.Values{"unit": {"lb"}, "e1rm_window_days": {"30"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("settings: %d", resp.StatusCode)
	}
	page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat"))
	if !strings.Contains(page, `value="308.65"`) || !strings.Contains(page, "Training max (lb)") {
		t.Fatalf("TM not shown in lb:\n%s", page)
	}
	resp, body = post(t, c, srv.URL+"/settings", csrf, url.Values{"unit": {"stone"}, "e1rm_window_days": {"30"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "unit must be") {
		t.Fatalf("invalid settings: %d", resp.StatusCode)
	}
}

func TestAlternativesAddRemove(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	base := srv.URL + "/exercises/pull-up"

	resp, _ := post(t, c, base+"/alternatives", csrf, url.Values{"alternative": {"inverted-row"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	if page := read(t, mustGet(t, c, base)); !strings.Contains(page, "/exercises/pull-up/alternatives/inverted-row/delete") {
		t.Fatal("user-added alternative has no remove button")
	}
	resp, body := post(t, c, base+"/alternatives", csrf, url.Values{"alternative": {"pull-up"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "its own alternative") {
		t.Fatalf("self: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, base+"/alternatives/inverted-row/delete", csrf, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("remove: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, base+"/alternatives/chin-up/delete", csrf, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("removing a seeded alternative: %d", resp.StatusCode)
	}
}

func TestEquipmentPages(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	csrf := session(t, srv, c)

	list := read(t, mustGet(t, c, srv.URL+"/equipment"))
	if !strings.Contains(list, "20 kg bar, plates 1.25, 2.5, 5-25/5 kg") || !strings.Contains(list, "2-50/2 kg") {
		t.Fatalf("starter equipment missing:\n%s", list)
	}

	form := url.Values{"kind": {"dumbbell"}, "name": {"Home DBs"}, "unit": {"kg"}, "weights": {"2-20/2, 22.5"}, "is_default": {"1"}}
	resp, _ := post(t, c, srv.URL+"/equipment", csrf, form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	list = read(t, mustGet(t, c, srv.URL+"/equipment"))
	if !strings.Contains(list, "Home DBs") || !strings.Contains(list, "2-20/2, 22.5 kg") {
		t.Fatalf("new profile missing:\n%s", list)
	}

	form.Set("weights", "2-20")
	resp, body := post(t, c, srv.URL+"/equipment", csrf, form)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "a range needs a step") {
		t.Fatalf("bad weights: %d", resp.StatusCode)
	}

	// Another user's profile is invisible.
	bob, err := db.UpsertOIDCUser(context.Background(), "iss", "bob", "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := db.SaveEquipment(context.Background(), store.Equipment{UserID: bob.ID, Name: "Bob's bar",
		Spec: calc.Equipment{Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{20}}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp := mustGet(t, c, srv.URL+"/equipment/"+theirs.ID+"/edit"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other user's equipment: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/equipment/"+theirs.ID+"/delete", csrf, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting other user's equipment: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"equipment_id": {theirs.ID}})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("linking other user's equipment: %d", resp.StatusCode)
	}
}
