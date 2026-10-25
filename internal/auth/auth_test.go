package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestLoadClientFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}
	c, err := LoadClientFile(write("installed.json", `{"installed":{"client_id":"id","client_secret":"s","redirect_uris":["http://localhost"]}}`))
	if err != nil || c.ID != "id" || c.Secret != "s" {
		t.Errorf("installed: %+v, %v", c, err)
	}
	if c, err := LoadClientFile(write("flat.json", `{"client_id":"id2"}`)); err != nil || c.ID != "id2" {
		t.Errorf("flat: %+v, %v", c, err)
	}
	if _, err := LoadClientFile(write("web.json", `{"web":{"client_id":"id"}}`)); err == nil {
		t.Error("a web client was accepted")
	}
}

func TestResolveClientWithoutOne(t *testing.T) {
	t.Setenv(ClientEnv, "")
	if _, err := ResolveClient(""); !errors.Is(err, ErrNoClient) {
		t.Errorf("err = %v, want ErrNoClient", err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "session.json")
	if _, err := LoadSession(path); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("missing session: %v", err)
	}
	s := &Session{ClientID: "id", Scopes: Scopes[:3], Token: &oauth2.Token{RefreshToken: "r"}}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil || got.Token.RefreshToken != "r" {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	var scopeErr *MissingScopesError
	if err := got.Check(); !errors.As(err, &scopeErr) || len(scopeErr.Scopes) != 1 {
		t.Errorf("Check = %v, want drive.readonly missing", err)
	}
}
