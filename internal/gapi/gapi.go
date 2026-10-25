// Package gapi is a small client for the two Google APIs gcal-transcripts
// needs: Calendar, to find events with a transcript attached, and Drive, to
// export the transcript as markdown. It speaks plain HTTP rather than pull in
// Google's generated client libraries.
package gapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	calendarBase = "https://www.googleapis.com/calendar/v3"
	driveBase    = "https://www.googleapis.com/drive/v3"
)

// Service calls the APIs with an already-authorised HTTP client. The base
// URLs are fields so tests can point them at a local server.
type Service struct {
	HTTP         *http.Client
	CalendarBase string
	DriveBase    string
}

func New(h *http.Client) *Service {
	return &Service{HTTP: h, CalendarBase: calendarBase, DriveBase: driveBase}
}

// APIError is an error response from a Google API.
type APIError struct {
	Status  int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Reason != "" {
		return fmt.Sprintf("%d %s: %s", e.Status, e.Reason, msg)
	}
	return fmt.Sprintf("%d: %s", e.Status, msg)
}

// IsAuth reports whether the token itself was rejected, which only signing
// in again fixes.
func (e *APIError) IsAuth() bool { return e.Status == http.StatusUnauthorized }

// IsNoAccess reports whether the signed-in account can't read the file. The
// transcript is attached to the event for every guest, but the Doc behind it
// is shared separately, so a guest can see an attachment they can't open.
// Drive answers 404 rather than 403 for a file it won't admit exists.
func (e *APIError) IsNoAccess() bool {
	return e.Status == http.StatusNotFound ||
		e.Status == http.StatusForbidden && (e.Reason == "insufficientFilePermissions" || e.Reason == "forbidden")
}

// IsDownloadDisabled reports whether the Doc's owner has stopped viewers from
// downloading or copying it, which rules out an export too.
func (e *APIError) IsDownloadDisabled() bool {
	return e.Status == http.StatusForbidden && e.Reason == "cannotExportFile"
}

func (e *APIError) retryable() bool {
	switch e.Status {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusForbidden:
		return e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded"
	}
	return false
}

func parseError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	var wrapper struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &wrapper) == nil {
		e.Message = wrapper.Error.Message
		if len(wrapper.Error.Errors) > 0 {
			e.Reason = wrapper.Error.Errors[0].Reason
		}
	}
	return e
}

// get fetches u, retrying a couple of times on rate limits and server errors.
func (s *Service) get(ctx context.Context, u string) ([]byte, error) {
	var last error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 300 {
			return body, nil
		}
		apiErr := parseError(resp.StatusCode, body)
		if !apiErr.retryable() {
			return nil, apiErr
		}
		last = apiErr
	}
	return nil, last
}

func (s *Service) getJSON(ctx context.Context, u string, out any) error {
	body, err := s.get(ctx, u)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
