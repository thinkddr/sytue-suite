// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google Calendar and Microsoft Graph use these fixed camelCase and OData wire keys.
package linkedservices

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The JSON names below are provider wire contracts, not suite DTO names.
type remoteTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func (r remoteTime) parse() time.Time {
	zone := time.UTC
	if r.TimeZone != "" {
		if loaded, err := time.LoadLocation(r.TimeZone); err == nil {
			zone = loaded
		}
	}
	if r.Date != "" {
		t, _ := time.ParseInLocation(time.DateOnly, r.Date, zone)
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, r.DateTime); err == nil {
		return t
	}
	t, _ := time.ParseInLocation("2006-01-02T15:04:05.999999999", r.DateTime, zone)
	return t
}

type googleEvent struct {
	ID          string     `json:"id"`
	ETag        string     `json:"etag"`
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	Location    string     `json:"location"`
	Start       remoteTime `json:"start"`
	End         remoteTime `json:"end"`
	Recurrence  []string   `json:"recurrence"`
	RecurringID string     `json:"recurringEventId"`
	Status      string     `json:"status"`
	EventType   string     `json:"eventType"`
	HTMLLink    string     `json:"htmlLink"`
	Attendees   []struct {
		Email string `json:"email"`
		Name  string `json:"displayName"`
	} `json:"attendees"`
}

func (g googleEvent) event(collection string) Event {
	e := Event{Resource: Resource{ID: g.ID, ETag: g.ETag, ProviderURL: safeProviderURL(g.HTMLLink)}, CollectionID: collection, Summary: g.Summary, Description: g.Description, Location: g.Location, Start: g.Start.parse(), End: g.End.parse(), TimeZone: g.Start.TimeZone, AllDay: g.Start.Date != "", Recurrence: g.Recurrence}
	e.ReadOnly = g.Status == "cancelled" || g.RecurringID != "" || len(g.Recurrence) > 0 || (g.EventType != "" && g.EventType != "default")
	for _, a := range g.Attendees {
		e.Attendees = append(e.Attendees, Attendee{Email: a.Email, Name: a.Name})
	}
	return e
}

type graphEvent struct {
	ID      string `json:"id"`
	ETag    string `json:"@odata.etag"`
	Subject string `json:"subject"`
	Body    struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
	} `json:"body"`
	Location struct {
		Name string `json:"displayName"`
	} `json:"location"`
	Start     remoteTime `json:"start"`
	End       remoteTime `json:"end"`
	AllDay    bool       `json:"isAllDay"`
	Type      string     `json:"type"`
	Cancelled bool       `json:"isCancelled"`
	WebLink   string     `json:"webLink"`
	Attendees []struct {
		Address struct {
			Email string `json:"address"`
			Name  string `json:"name"`
		} `json:"emailAddress"`
	} `json:"attendees"`
}

func (g graphEvent) event(collection string) Event {
	e := Event{Resource: Resource{ID: g.ID, ETag: g.ETag, ProviderURL: safeProviderURL(g.WebLink)}, CollectionID: collection, Summary: g.Subject, Description: plainContent(g.Body.Content, g.Body.ContentType), Location: g.Location.Name, Start: g.Start.parse(), End: g.End.parse(), TimeZone: g.Start.TimeZone, AllDay: g.AllDay}
	e.ReadOnly = g.Cancelled || g.Type != "singleInstance" && g.Type != ""
	for _, a := range g.Attendees {
		e.Attendees = append(e.Attendees, Attendee{Email: a.Address.Email, Name: a.Address.Name})
	}
	return e
}

func (c *Client) Collections(ctx context.Context, service string) (Page[Collection], error) {
	if c.provider == "yahoo" {
		return c.davCollections(ctx, service)
	}
	result := Page[Collection]{Items: []Collection{}}
	switch c.provider + "/" + service {
	case "gmail/calendar":
		return c.googleCalendars(ctx)
	case "gmail/contacts":
		result.Items = append(result.Items, Collection{ID: "contacts", Name: "Google Contacts"})
		return result, nil
	case "microsoft/calendar", "microsoft/contacts", "microsoft/notes":
		return c.graphCollections(ctx, service)
	default:
		return result, ErrReadOnly
	}
}

func (c *Client) googleCalendars(ctx context.Context) (Page[Collection], error) {
	result := Page[Collection]{Items: []Collection{}}
	var wire struct {
		Items []struct {
			ID       string `json:"id"`
			Name     string `json:"summary"`
			Color    string `json:"backgroundColor"`
			TimeZone string `json:"timeZone"`
			Access   string `json:"accessRole"`
		} `json:"items"`
		Next string `json:"nextPageToken"`
	}
	page := ""
	for rounds := 0; rounds < 20; rounds++ {
		wire.Items = nil
		wire.Next = ""
		if err := c.json(ctx, http.MethodGet, query(googleCalendar+"/users/me/calendarList", url.Values{"maxResults": {"250"}, "pageToken": {page}}), nil, &wire, nil); err != nil {
			return result, err
		}
		for _, item := range wire.Items {
			result.Items = append(result.Items, Collection{ID: item.ID, Name: item.Name, Color: item.Color, TimeZone: item.TimeZone, ReadOnly: item.Access != "owner" && item.Access != "writer"})
		}
		if wire.Next == "" {
			return result, nil
		}
		page = wire.Next
	}
	return result, ErrUnavailable
}

func (c *Client) graphCollections(ctx context.Context, service string) (Page[Collection], error) {
	result := Page[Collection]{Items: []Collection{}}
	path := "/calendars"
	if service == Contacts {
		path = "/contactFolders"
		// Graph lists child folders here, not the default address book itself.
		// The stable local alias routes reads/writes through /me/contacts,
		// including accounts whose default book is currently empty.
		result.Items = append(result.Items, Collection{ID: "contacts", Name: "Microsoft Outlook"})
	}
	if service == Notes {
		path = "/onenote/sections"
	}
	var wire struct {
		Items []struct {
			ID           string `json:"id"`
			Name         string `json:"displayName"`
			CalendarName string `json:"name"`
			Editable     *bool  `json:"canEdit"`
			Default      bool   `json:"isDefaultCalendar"`
		} `json:"value"`
		Next string `json:"@odata.nextLink"`
	}
	target := query(graph+path, url.Values{"$top": {"100"}})
	for rounds := 0; rounds < 20; rounds++ {
		wire.Items = nil
		wire.Next = ""
		if err := c.json(ctx, http.MethodGet, target, nil, &wire, nil); err != nil {
			return result, err
		}
		for _, item := range wire.Items {
			name := item.Name
			if name == "" {
				name = item.CalendarName
			}
			result.Items = append(result.Items, Collection{ID: item.ID, Name: name, ReadOnly: item.Editable != nil && !*item.Editable})
		}
		if wire.Next == "" {
			return result, nil
		}
		cursor, err := c.cursor(wire.Next, graph+path)
		if err != nil {
			return result, err
		}
		target, err = c.nextURL(cursor, "", graph+path)
		if err != nil {
			return result, err
		}
	}
	return result, ErrUnavailable
}

func (c *Client) Events(ctx context.Context, collection string, from, to time.Time, cursor string) (Page[Event], error) {
	result := Page[Event]{Items: []Event{}}
	if !validID(collection) || !eventWindow(from, to) {
		return result, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davEvents(ctx, collection, from, to, cursor)
	}
	if c.provider == "gmail" {
		var wire struct {
			Items []googleEvent `json:"items"`
			Next  string        `json:"nextPageToken"`
		}
		if len(cursor) > 4096 {
			return result, ErrInvalid
		}
		target := query(googleCalendar+"/calendars/"+url.PathEscape(collection)+"/events", url.Values{"timeMin": {from.Format(time.RFC3339)}, "timeMax": {to.Format(time.RFC3339)}, "singleEvents": {"true"}, "orderBy": {"startTime"}, "maxResults": {"100"}, "pageToken": {cursor}})
		if err := c.json(ctx, http.MethodGet, target, nil, &wire, nil); err != nil {
			return result, err
		}
		for _, item := range wire.Items {
			if item.Status != "cancelled" {
				result.Items = append(result.Items, item.event(collection))
			}
		}
		result.NextCursor = wire.Next
		return result, nil
	}
	prefix := graph + "/calendars/" + url.PathEscape(collection) + "/calendarView"
	target, err := c.nextURL(cursor, query(prefix, url.Values{"startDateTime": {from.Format(time.RFC3339)}, "endDateTime": {to.Format(time.RFC3339)}, "$top": {"100"}}), prefix)
	if err != nil {
		return result, err
	}
	var wire struct {
		Items []graphEvent `json:"value"`
		Next  string       `json:"@odata.nextLink"`
	}
	if err := c.json(ctx, http.MethodGet, target, nil, &wire, calendarHeaders()); err != nil {
		return result, err
	}
	for _, item := range wire.Items {
		if !item.Cancelled {
			result.Items = append(result.Items, item.event(collection))
		}
	}
	result.NextCursor, err = c.cursor(wire.Next, prefix)
	return result, err
}

func calendarHeaders() http.Header {
	return http.Header{"Prefer": {`outlook.timezone="UTC", outlook.body-content-type="text"`}}
}

func (c *Client) Event(ctx context.Context, collection, id string) (Event, error) {
	if !validID(collection) || !validID(id) {
		return Event{}, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davEvent(ctx, collection, id)
	}
	if c.provider == "gmail" {
		var item googleEvent
		err := c.json(ctx, http.MethodGet, googleCalendar+"/calendars/"+url.PathEscape(collection)+"/events/"+url.PathEscape(id), nil, &item, nil)
		return item.event(collection), err
	}
	var item graphEvent
	err := c.json(ctx, http.MethodGet, graph+"/calendars/"+url.PathEscape(collection)+"/events/"+url.PathEscape(id), nil, &item, calendarHeaders())
	return item.event(collection), err
}

func (c *Client) SaveEvent(ctx context.Context, collection, id, etag string, patch EventPatch) (Event, error) {
	if !validID(collection) || id != "" && !validID(id) || !validEventPatch(patch, id == "") {
		return Event{}, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davSaveEvent(ctx, collection, id, etag, patch)
	}
	method := http.MethodPost
	headers := calendarHeaders()
	if id != "" {
		current, err := c.Event(ctx, collection, id)
		if err != nil {
			return Event{}, err
		}
		if current.ReadOnly {
			return Event{}, ErrReadOnly
		}
		if current.ETag != etag {
			return Event{}, ErrConflict
		}
		conditionalHeaders, err := conditional(etag)
		if err != nil {
			return Event{}, err
		}
		headers.Set("If-Match", conditionalHeaders.Get("If-Match"))
		method = http.MethodPatch
	}
	target := graph + "/calendars/" + url.PathEscape(collection) + "/events"
	if c.provider == "gmail" {
		target = googleCalendar + "/calendars/" + url.PathEscape(collection) + "/events"
	}
	if id != "" {
		target += "/" + url.PathEscape(id)
	}
	body, err := cloudEventPatch(c.provider, patch)
	if err != nil {
		return Event{}, err
	}
	if c.provider == "gmail" {
		var item googleEvent
		err := c.json(ctx, method, target, body, &item, headers)
		return item.event(collection), err
	}
	var item graphEvent
	err = c.json(ctx, method, target, body, &item, headers)
	return item.event(collection), err
}

func validEventPatch(p EventPatch, create bool) bool {
	if create && (p.Summary == nil || p.Start == nil || p.End == nil || p.AllDay == nil) {
		return false
	}
	if p.Start != nil && p.End != nil && (!p.End.After(*p.Start) || p.Start.IsZero() || p.End.Sub(*p.Start) > 366*24*time.Hour) {
		return false
	}
	for _, value := range []*string{p.Summary, p.Location, p.Description, p.TimeZone} {
		if value != nil && (len(*value) > 1<<20 || strings.ContainsRune(*value, 0)) {
			return false
		}
	}
	return true
}

func cloudEventPatch(provider string, patch EventPatch) (map[string]any, error) {
	body := map[string]any{}
	if patch.Summary != nil {
		if provider == "gmail" {
			body["summary"] = *patch.Summary
		} else {
			body["subject"] = *patch.Summary
		}
	}
	if patch.Description != nil {
		if provider == "gmail" {
			body["description"] = *patch.Description
		} else {
			body["body"] = map[string]string{"contentType": "text", "content": *patch.Description}
		}
	}
	if patch.Location != nil {
		if provider == "gmail" {
			body["location"] = *patch.Location
		} else {
			body["location"] = map[string]string{"displayName": *patch.Location}
		}
	}
	if patch.Recurrence != nil && len(*patch.Recurrence) > 0 {
		return nil, ErrReadOnly
	}
	if patch.Attendees != nil {
		return nil, ErrReadOnly
	} // Invitations need a separate explicit UI, not an incidental edit.
	if err := cloudEventTimes(provider, patch, body); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, ErrInvalid
	}
	return body, nil
}

func cloudEventTimes(provider string, patch EventPatch, body map[string]any) error {
	if patch.Start != nil || patch.End != nil || patch.AllDay != nil || patch.TimeZone != nil {
		// Time edits must describe the complete pair. Other edits preserve all
		// original time fields, including the provider's timezone representation.
		if patch.Start == nil || patch.End == nil || patch.AllDay == nil {
			return ErrInvalid
		}
		zone := "UTC"
		location := time.UTC
		if patch.TimeZone != nil && *patch.TimeZone != "" {
			zone = *patch.TimeZone
			var err error
			location, err = time.LoadLocation(zone)
			if err != nil {
				return ErrInvalid
			}
		}
		for key, value := range map[string]time.Time{"start": *patch.Start, "end": *patch.End} {
			if provider == "gmail" {
				if *patch.AllDay {
					body[key] = map[string]string{"date": value.In(location).Format(time.DateOnly)}
				} else {
					body[key] = map[string]string{"dateTime": value.Format(time.RFC3339), "timeZone": zone}
				}
			} else {
				date := value.UTC().Format("2006-01-02T15:04:05")
				if *patch.AllDay {
					date = value.In(location).Format(time.DateOnly) + "T00:00:00"
				}
				body[key] = map[string]string{"dateTime": date, "timeZone": "UTC"}
			}
		}
		if provider == "microsoft" {
			body["isAllDay"] = *patch.AllDay
		}
	}
	return nil
}
