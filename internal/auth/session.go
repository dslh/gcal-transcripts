package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/oauth2"
)

// SessionEnv overrides where the session file lives.
const SessionEnv = "GCAL_TRANSCRIPTS_SESSION"

// ErrNotLoggedIn means there is no usable session on disk.
var ErrNotLoggedIn = errors.New("not signed in")

// MissingScopesError means the session predates a scope the commands now
// need. Google keeps refreshing such a token happily and only refuses the
// API call itself, so it is caught up front instead.
type MissingScopesError struct{ Scopes []string }

func (e *MissingScopesError) Error() string {
	return "the session lacks scopes this version needs: " + strings.Join(e.Scopes, ", ")
}

// Session is what a sign-in leaves on disk. It carries its own client, so a
// session made with --client-file keeps working without the flag.
type Session struct {
	ClientID     string        `json:"client_id"`
	ClientSecret string        `json:"client_secret"`
	ClientSource string        `json:"client_source"`
	Account      string        `json:"account,omitempty"`
	Scopes       []string      `json:"scopes"`
	Token        *oauth2.Token `json:"token"`
}

// SessionPath is $GCAL_TRANSCRIPTS_SESSION, or session.json under the user
// config directory, so the same session is found from any working directory.
func SessionPath() (string, error) {
	if p := os.Getenv(SessionEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gcal-transcripts", "session.json"), nil
}

// LoadSession reads the session at path, returning ErrNotLoggedIn if there
// isn't one.
func LoadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Token == nil || s.Token.RefreshToken == "" {
		return nil, ErrNotLoggedIn
	}
	return &s, nil
}

// Save writes the session readable by the user only, replacing any other.
func (s *Session) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Check returns a *MissingScopesError if the session wasn't granted every
// scope the commands use.
func (s *Session) Check() error {
	var missing []string
	for _, want := range requiredScopes {
		if !slices.Contains(s.Scopes, want) {
			missing = append(missing, want)
		}
	}
	if missing != nil {
		return &MissingScopesError{Scopes: missing}
	}
	return nil
}

// HTTPClient makes authorised requests, refreshing the access token as it
// expires.
func (s *Session) HTTPClient(ctx context.Context) *http.Client {
	c := Client{ID: s.ClientID, Secret: s.ClientSecret}
	return c.config("").Client(ctx, s.Token)
}

// Revoke tells Google to invalidate the session's refresh token, and with it
// every access token issued from it.
func (s *Session) Revoke(ctx context.Context) error {
	form := url.Values{"token": {s.Token.RefreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 400 is what Google answers for a token it has already forgotten.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("revoking the token: %s", resp.Status)
	}
	return nil
}
