package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Login runs Google's installed-app flow: it listens on a loopback port,
// sends the browser to the consent screen, and trades the code that comes
// back for a token. open is called with the consent URL, which is also
// printed to out in case no browser opens.
func Login(ctx context.Context, c Client, open func(string) error, out io.Writer) (*Session, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	conf := c.config(redirect)
	verifier := oauth2.GenerateVerifier()
	state := oauth2.GenerateVerifier() // any unguessable string will do
	authURL := conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		// Without prompt=consent, Google only issues a refresh token the
		// first time an account approves the client, so signing in again
		// after a logout would leave a session that can't refresh.
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			// Anything else (a favicon, a prefetch) is not the redirect.
			if r.URL.Path != "/" || q.Get("code") == "" && q.Get("error") == "" {
				http.NotFound(w, r)
				return
			}
			var res result
			switch {
			case q.Get("state") != state:
				res.err = errors.New("the sign-in response didn't match this request")
			case q.Get("error") != "":
				res.err = fmt.Errorf("sign-in refused: %s", q.Get("error"))
			default:
				res.code = q.Get("code")
			}
			writeResultPage(w, res.err)
			select {
			case done <- res:
			default:
			}
		}),
	}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Fprintf(out, "Opening your browser to sign in to Google. If it doesn't open, visit:\n\n  %s\n\n", authURL)
	if open != nil {
		if err := open(authURL); err != nil {
			fmt.Fprintf(out, "(couldn't open a browser: %v)\n", err)
		}
	}

	var res result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-done:
	}
	if res.err != nil {
		return nil, res.err
	}

	tok, err := conf.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("Google returned no refresh token")
	}
	scope, _ := tok.Extra("scope").(string)
	return &Session{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		ClientSource: c.Source,
		Account:      idTokenEmail(tok),
		Scopes:       strings.Fields(scope),
		Token:        tok,
	}, nil
}

// idTokenEmail reads the account's address out of the ID token. It came
// straight from Google's token endpoint over TLS, so there is no signature
// worth checking; it only labels the session.
func idTokenEmail(tok *oauth2.Token) string {
	raw, _ := tok.Extra("id_token").(string)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	json.Unmarshal(payload, &claims)
	return claims.Email
}

func writeResultPage(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	msg := "Signed in. You can close this tab and go back to the terminal."
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		msg = "Sign-in failed: " + err.Error()
	}
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>gcal-transcripts</title>`+
		`<body style="font:16px system-ui,sans-serif;margin:3rem auto;max-width:32rem">`+
		`<h1 style="font-size:1.25rem">gcal-transcripts</h1><p>%s</p>`, html.EscapeString(msg))
}
