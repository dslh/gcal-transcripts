// Package auth signs in to Google with the installed-app OAuth flow and keeps
// the resulting session between runs.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/oauth2"
)

// Set at release time by GoReleaser (-ldflags -X). They are empty in source
// and in local builds, which then need a client file: see ResolveClient.
//
// Google does not treat a Desktop app client's secret as confidential, since
// anything shipped in a binary can be read back out of it; PKCE is what
// protects the sign-in.
var (
	embeddedClientID     string
	embeddedClientSecret string
)

// ClientEnv names a client JSON file, as an alternative to --client-file.
const ClientEnv = "GCAL_TRANSCRIPTS_CLIENT"

// Scopes are requested at sign-in. openid and email only label the session
// with the account it belongs to; the other two are what the commands use.
var Scopes = []string{
	"openid",
	"email",
	"https://www.googleapis.com/auth/calendar.events.readonly",
	"https://www.googleapis.com/auth/drive.readonly",
}

// requiredScopes are the ones the API calls fail without.
var requiredScopes = Scopes[2:]

var endpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// ErrNoClient means there is no OAuth client to sign in with.
var ErrNoClient = errors.New("no OAuth client: this build has none built in, so pass --client-file " +
	"with a Desktop app client JSON downloaded from Google Cloud, or set $" + ClientEnv + " to its path")

// Client is a Google OAuth client of the Desktop app type.
type Client struct {
	ID     string `json:"client_id"`
	Secret string `json:"client_secret"`
	// Source says where the client came from, for `status`.
	Source string `json:"-"`
}

// ResolveClient picks the client to sign in with: the file at flagPath, then
// the file named by $GCAL_TRANSCRIPTS_CLIENT, then the one built in.
func ResolveClient(flagPath string) (Client, error) {
	path, source := flagPath, "--client-file"
	if path == "" {
		path, source = os.Getenv(ClientEnv), "$"+ClientEnv
	}
	if path != "" {
		c, err := LoadClientFile(path)
		c.Source = fmt.Sprintf("%s (%s)", source, path)
		return c, err
	}
	if embeddedClientID != "" {
		return Client{ID: embeddedClientID, Secret: embeddedClientSecret, Source: "built in"}, nil
	}
	return Client{}, ErrNoClient
}

// LoadClientFile reads a client JSON as Google Cloud's console downloads it
// ({"installed": {...}}), or a flat {"client_id", "client_secret"} object.
func LoadClientFile(path string) (Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Client{}, err
	}
	var f struct {
		Installed *Client         `json:"installed"`
		Web       json.RawMessage `json:"web"`
		Client
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return Client{}, fmt.Errorf("%s: %w", path, err)
	}
	c := f.Client
	switch {
	case f.Installed != nil:
		c = *f.Installed
	case f.Web != nil:
		return Client{}, fmt.Errorf("%s is a web application client; gcal-transcripts needs a Desktop app client", path)
	}
	if c.ID == "" {
		return Client{}, fmt.Errorf("%s: no client_id", path)
	}
	return c, nil
}

func (c Client) config(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		Endpoint:     endpoint,
		RedirectURL:  redirectURL,
		Scopes:       Scopes,
	}
}
