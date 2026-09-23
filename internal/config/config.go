// Package config loads onerep configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

const (
	EnvProd = "prod"
	EnvDev  = "dev"
)

type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
}

type Config struct {
	Env         string
	Listen      string
	BaseURL     string
	DBPath      string
	AutoMigrate bool
	DevUser     string
	OIDC        OIDC
}

// Dev reports whether onerep runs in development mode.
func (c Config) Dev() bool { return c.Env == EnvDev }

// SecureCookies reports whether cookies must carry the Secure attribute.
func (c Config) SecureCookies() bool {
	u, err := url.Parse(c.BaseURL)
	return err == nil && u.Scheme == "https"
}

// Load reads configuration using getenv (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	c := Config{
		Env:     get("ONEREP_ENV", EnvProd),
		Listen:  get("ONEREP_LISTEN", ":8080"),
		BaseURL: get("ONEREP_BASE_URL", ""),
		DBPath:  get("ONEREP_DB", "./onerep.db"),
		DevUser: getenv("ONEREP_DEV_USER"),
		OIDC: OIDC{
			Issuer:       getenv("ONEREP_OIDC_ISSUER"),
			ClientID:     getenv("ONEREP_OIDC_CLIENT_ID"),
			ClientSecret: getenv("ONEREP_OIDC_CLIENT_SECRET"),
		},
	}

	autoMigrate, err := strconv.ParseBool(get("ONEREP_AUTO_MIGRATE", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("ONEREP_AUTO_MIGRATE: %w", err)
	}
	c.AutoMigrate = autoMigrate

	switch c.Env {
	case EnvProd, EnvDev:
	default:
		return Config{}, fmt.Errorf("ONEREP_ENV must be %q or %q, got %q", EnvProd, EnvDev, c.Env)
	}

	if c.BaseURL == "" && c.Dev() {
		c.BaseURL = "http://localhost:8080"
	}
	if c.BaseURL == "" {
		return Config{}, errors.New("ONEREP_BASE_URL is required")
	}
	if u, err := url.Parse(c.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("ONEREP_BASE_URL must be an absolute URL, got %q", c.BaseURL)
	}

	if c.DevUser != "" && !c.Dev() {
		return Config{}, errors.New("ONEREP_DEV_USER is only allowed with ONEREP_ENV=dev")
	}

	oidcSet := c.OIDC.Issuer != "" || c.OIDC.ClientID != "" || c.OIDC.ClientSecret != ""
	oidcComplete := c.OIDC.Issuer != "" && c.OIDC.ClientID != "" && c.OIDC.ClientSecret != ""
	if oidcSet && !oidcComplete {
		return Config{}, errors.New("ONEREP_OIDC_ISSUER, ONEREP_OIDC_CLIENT_ID and ONEREP_OIDC_CLIENT_SECRET must all be set")
	}
	if !oidcComplete && c.DevUser == "" {
		return Config{}, errors.New("configure OIDC (ONEREP_OIDC_*) or, in dev, ONEREP_DEV_USER")
	}

	return c, nil
}
