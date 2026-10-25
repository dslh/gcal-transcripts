package gapi

import (
	"context"
	"net/url"
	"time"
)

// Event is the part of a Calendar event gcal-transcripts reads.
type Event struct {
	ID          string       `json:"id"`
	Summary     string       `json:"summary"`
	HTMLLink    string       `json:"htmlLink"`
	Start       EventTime    `json:"start"`
	End         EventTime    `json:"end"`
	Organizer   Person       `json:"organizer"`
	Attendees   []Person     `json:"attendees"`
	Attachments []Attachment `json:"attachments"`
}

// EventTime is either a moment (timed events) or a date (all-day events).
type EventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
}

// Time returns the moment in local time. An all-day event starts at local
// midnight on its date.
func (t EventTime) Time() (tm time.Time, allDay bool, err error) {
	if t.DateTime != "" {
		tm, err = time.Parse(time.RFC3339, t.DateTime)
		return tm.Local(), false, err
	}
	tm, err = time.ParseInLocation(time.DateOnly, t.Date, time.Local)
	return tm, true, err
}

// String is the time as the API gave it.
func (t EventTime) String() string {
	if t.DateTime != "" {
		return t.DateTime
	}
	return t.Date
}

type Person struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Self        bool   `json:"self"`
	Resource    bool   `json:"resource"`
}

type Attachment struct {
	FileID   string `json:"fileId"`
	FileURL  string `json:"fileUrl"`
	Title    string `json:"title"`
	MimeType string `json:"mimeType"`
}

const eventFields = "id,summary,htmlLink,start,end,organizer(email,displayName)," +
	"attendees(email,displayName,self,resource),attachments(fileId,fileUrl,title,mimeType)"

type ListOptions struct {
	CalendarID string
	From, To   time.Time
	// Query is Calendar's free-text search over titles, descriptions,
	// locations and guests.
	Query string
}

// ListEvents returns the events overlapping [From, To), recurring ones
// expanded into their instances, in start order.
func (s *Service) ListEvents(ctx context.Context, o ListOptions) ([]Event, error) {
	var events []Event
	page := ""
	for {
		q := url.Values{
			"timeMin":      {o.From.Format(time.RFC3339)},
			"timeMax":      {o.To.Format(time.RFC3339)},
			"singleEvents": {"true"},
			"orderBy":      {"startTime"},
			"maxResults":   {"250"},
			"fields":       {"items(" + eventFields + "),nextPageToken"},
		}
		if o.Query != "" {
			q.Set("q", o.Query)
		}
		if page != "" {
			q.Set("pageToken", page)
		}
		var resp struct {
			Items         []Event `json:"items"`
			NextPageToken string  `json:"nextPageToken"`
		}
		if err := s.getJSON(ctx, s.eventsURL(o.CalendarID)+"?"+q.Encode(), &resp); err != nil {
			return nil, err
		}
		events = append(events, resp.Items...)
		if resp.NextPageToken == "" {
			return events, nil
		}
		page = resp.NextPageToken
	}
}

// GetEvent fetches one event by the ID ListEvents gave it.
func (s *Service) GetEvent(ctx context.Context, calendarID, eventID string) (Event, error) {
	var ev Event
	u := s.eventsURL(calendarID) + "/" + url.PathEscape(eventID) + "?" + url.Values{"fields": {eventFields}}.Encode()
	err := s.getJSON(ctx, u, &ev)
	return ev, err
}

func (s *Service) eventsURL(calendarID string) string {
	return s.CalendarBase + "/calendars/" + url.PathEscape(calendarID) + "/events"
}
