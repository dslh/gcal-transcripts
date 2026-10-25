package gapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestService(t *testing.T, h http.HandlerFunc) *Service {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Service{HTTP: srv.Client(), CalendarBase: srv.URL + "/calendar", DriveBase: srv.URL + "/drive"}
}

func TestListEventsPages(t *testing.T) {
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/calendars/primary/events" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"items":[{"id":"a"}],"nextPageToken":"p2"}`)
		} else {
			fmt.Fprint(w, `{"items":[{"id":"b","attachments":[{"fileId":"f","mimeType":"application/vnd.google-apps.document"}]}]}`)
		}
	})
	events, err := svc.ListEvents(context.Background(), ListOptions{CalendarID: "primary", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].ID != "b" || events[1].Attachments[0].FileID != "f" {
		t.Errorf("events = %+v", events)
	}
}

func TestExportErrors(t *testing.T) {
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"nope","errors":[{"reason":"insufficientFilePermissions"}]}}`)
	})
	_, err := svc.ExportMarkdown(context.Background(), "f")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.IsNoAccess() || apiErr.Message != "nope" {
		t.Errorf("err = %v, want a no-access APIError", err)
	}
}

func TestExportFallsBackToLink(t *testing.T) {
	var base string
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/files/f/export":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"errors":[{"reason":"exportSizeLimitExceeded"}]}}`)
		case "/drive/files/f":
			fmt.Fprintf(w, `{"exportLinks":{"text/markdown":%q}}`, base+"/big")
		case "/big":
			fmt.Fprint(w, "# big doc")
		}
	})
	base = svc.DriveBase[:len(svc.DriveBase)-len("/drive")]
	got, err := svc.ExportMarkdown(context.Background(), "f")
	if err != nil || string(got) != "# big doc" {
		t.Errorf("ExportMarkdown = %q, %v", got, err)
	}
}

func TestRetriesRateLimits(t *testing.T) {
	calls := 0
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if calls++; calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"capabilities":{"canDownload":false}}`)
	})
	err := svc.CheckAccess(context.Background(), "f")
	var apiErr *APIError
	if calls != 2 || !errors.As(err, &apiErr) || !apiErr.IsDownloadDisabled() {
		t.Errorf("calls = %d, err = %v", calls, err)
	}
}
