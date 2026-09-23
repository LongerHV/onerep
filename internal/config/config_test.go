package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

var oidcEnv = map[string]string{
	"ONEREP_BASE_URL":           "https://gym.example.com",
	"ONEREP_OIDC_ISSUER":        "https://id.example.com",
	"ONEREP_OIDC_CLIENT_ID":     "onerep",
	"ONEREP_OIDC_CLIENT_SECRET": "secret",
}

func with(base map[string]string, kv ...string) map[string]string {
	m := map[string]string{}
	for k, v := range base {
		m[k] = v
	}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(oidcEnv))
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != EnvProd || c.Listen != ":8080" || c.DBPath != "./onerep.db" || !c.AutoMigrate {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if !c.SecureCookies() {
		t.Fatal("https base URL must enable secure cookies")
	}
}

func TestLoadDevUser(t *testing.T) {
	c, err := Load(env(map[string]string{"ONEREP_ENV": "dev", "ONEREP_DEV_USER": "alice"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "http://localhost:8080" || c.SecureCookies() {
		t.Fatalf("dev base URL: %+v", c)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"bad env":           {with(oidcEnv, "ONEREP_ENV", "staging"), "ONEREP_ENV"},
		"dev user in prod":  {with(oidcEnv, "ONEREP_DEV_USER", "alice"), "ONEREP_DEV_USER"},
		"partial oidc":      {with(oidcEnv, "ONEREP_OIDC_CLIENT_SECRET", ""), "must all be set"},
		"no auth at all":    {map[string]string{"ONEREP_ENV": "dev"}, "configure OIDC"},
		"missing base url":  {with(oidcEnv, "ONEREP_BASE_URL", ""), "ONEREP_BASE_URL is required"},
		"relative base url": {with(oidcEnv, "ONEREP_BASE_URL", "gym.example.com"), "absolute URL"},
		"bad auto migrate":  {with(oidcEnv, "ONEREP_AUTO_MIGRATE", "maybe"), "ONEREP_AUTO_MIGRATE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
