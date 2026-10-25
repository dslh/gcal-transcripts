package gapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const markdownMime = "text/markdown"

// ExportMarkdown converts a Google Doc to markdown using Drive's own export,
// so the conversion is Google's rather than ours. A multi-tab Doc comes back
// as one stream with each tab under a level-1 heading of its name.
func (s *Service) ExportMarkdown(ctx context.Context, fileID string) ([]byte, error) {
	q := url.Values{"mimeType": {markdownMime}}
	body, err := s.get(ctx, s.DriveBase+"/files/"+url.PathEscape(fileID)+"/export?"+q.Encode())
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Reason == "exportSizeLimitExceeded" {
		return s.exportViaLink(ctx, fileID)
	}
	return body, err
}

// exportViaLink is the fallback for Docs over files.export's 10 MB limit,
// which a long meeting with Gemini's screenshots embedded can reach. The
// file's export link serves the same conversion without that cap.
func (s *Service) exportViaLink(ctx context.Context, fileID string) ([]byte, error) {
	var f struct {
		ExportLinks map[string]string `json:"exportLinks"`
	}
	if err := s.getJSON(ctx, s.fileURL(fileID, "exportLinks"), &f); err != nil {
		return nil, err
	}
	link := f.ExportLinks[markdownMime]
	if link == "" {
		return nil, fmt.Errorf("file %s is too large to export and offers no markdown export link", fileID)
	}
	return s.get(ctx, link)
}

// CheckAccess returns nil if the signed-in account can export the file, or
// the *APIError an export would fail with.
func (s *Service) CheckAccess(ctx context.Context, fileID string) error {
	var f struct {
		Capabilities struct {
			CanDownload bool `json:"canDownload"`
		} `json:"capabilities"`
	}
	if err := s.getJSON(ctx, s.fileURL(fileID, "capabilities(canDownload)"), &f); err != nil {
		return err
	}
	if !f.Capabilities.CanDownload {
		return &APIError{Status: http.StatusForbidden, Reason: "cannotExportFile",
			Message: "the owner has disabled downloading for viewers"}
	}
	return nil
}

func (s *Service) fileURL(fileID, fields string) string {
	q := url.Values{"fields": {fields}, "supportsAllDrives": {"true"}}
	return s.DriveBase + "/files/" + url.PathEscape(fileID) + "?" + q.Encode()
}
