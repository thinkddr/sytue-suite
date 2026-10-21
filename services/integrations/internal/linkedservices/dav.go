// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package linkedservices

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"
)

const (
	yahooCalendar = "https://caldav.calendar.yahoo.com"
	yahooContacts = "https://carddav.address.yahoo.com"
)

// The DAV library handles XML and provider properties. Its transport delegates
// to the same bounded, origin-restricted egress path as the JSON adapters.
type davHTTP struct{ client *Client }

func (d davHTTP) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		defer req.Body.Close()
		var err error
		body, err = io.ReadAll(io.LimitReader(req.Body, maxResponse+1))
		if err != nil || len(body) > maxResponse {
			return nil, ErrInvalid
		}
	}
	raw, headers, err := d.client.request(req.Context(), req.Method, req.URL.String(), req.Header.Get("Content-Type"), body, req.Header)
	if err != nil {
		return nil, err
	}
	status := http.StatusOK
	if req.Method == "PROPFIND" || req.Method == "REPORT" {
		status = http.StatusMultiStatus
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw)), Request: req}, nil
}

func (c *Client) calendarDAV() (*caldav.Client, error) {
	return caldav.NewClient(davHTTP{c}, yahooCalendar+"/")
}

func (c *Client) contactsDAV() (*carddav.Client, error) {
	return carddav.NewClient(davHTTP{c}, yahooContacts+"/")
}

func (c *Client) davCollections(ctx context.Context, service string) (Page[Collection], error) {
	result := Page[Collection]{Items: []Collection{}}
	if service == Calendar {
		client, err := c.calendarDAV()
		if err != nil {
			return result, err
		}
		principal, err := client.FindCurrentUserPrincipal(ctx)
		if err != nil {
			return result, err
		}
		home, err := client.FindCalendarHomeSet(ctx, principal)
		if err != nil {
			return result, err
		}
		items, err := client.FindCalendars(ctx, home)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if _, err = davPath(yahooCalendar, item.Path); err != nil {
				return result, err
			}
			result.Items = append(result.Items, Collection{ID: item.Path, Name: item.Name})
		}
		return result, nil
	}
	if service == Contacts {
		client, err := c.contactsDAV()
		if err != nil {
			return result, err
		}
		principal, err := client.FindCurrentUserPrincipal(ctx)
		if err != nil {
			return result, err
		}
		home, err := client.FindAddressBookHomeSet(ctx, principal)
		if err != nil {
			return result, err
		}
		items, err := client.FindAddressBooks(ctx, home)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if _, err = davPath(yahooContacts, item.Path); err != nil {
				return result, err
			}
			result.Items = append(result.Items, Collection{ID: item.Path, Name: item.Name})
		}
		return result, nil
	}
	return result, ErrReadOnly
}

func davPath(base, value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", ErrInvalid
	}
	if u.IsAbs() {
		if u.Scheme != "https" || u.Host != strings.TrimPrefix(base, "https://") {
			return "", ErrInvalid
		}
	} else {
		if u.Host != "" || !strings.HasPrefix(u.Path, "/") {
			return "", ErrInvalid
		}
		u.Scheme = "https"
		u.Host = strings.TrimPrefix(base, "https://")
	}
	if strings.ContainsAny(u.Path, "\r\n\x00\\") || path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/") && u.Path != "/" {
		return "", ErrInvalid
	}
	return u.String(), nil
}

func davObject(base, collection, id string) (string, error) {
	parent, err := davPath(base, collection)
	if err != nil {
		return "", err
	}
	target, err := davPath(base, id)
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSuffix(parent, "/") + "/"
	if !strings.HasPrefix(target, prefix) || strings.Contains(strings.TrimPrefix(target, prefix), "/") {
		return "", ErrInvalid
	}
	return target, nil
}

func davID(object string, start time.Time) string {
	if !start.IsZero() {
		object += "\n" + start.Format(time.RFC3339Nano)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(object))
}

func decodeDAVID(id string) (string, time.Time, error) {
	if len(id) > 8192 {
		return "", time.Time{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", time.Time{}, ErrInvalid
	}
	parts := strings.Split(string(raw), "\n")
	if len(parts) > 2 {
		return "", time.Time{}, ErrInvalid
	}
	var start time.Time
	if len(parts) == 2 {
		start, err = time.Parse(time.RFC3339Nano, parts[1])
		if err != nil {
			return "", start, ErrInvalid
		}
	}
	return parts[0], start, nil
}

func davPage[T any](items []T, cursor string) (Page[T], error) {
	offset := 0
	if cursor != "" {
		var err error
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 || offset > len(items) {
			return Page[T]{}, ErrInvalid
		}
	}
	end := offset + 100
	if end > len(items) {
		end = len(items)
	}
	result := Page[T]{Items: items[offset:end]}
	if end < len(items) {
		result.NextCursor = strconv.Itoa(end)
	}
	return result, nil
}

// go-webdav removes the HTTP quotes when decoding ETags. Restore them at our
// API boundary; callers send this exact opaque value back in If-Match.
func davETag(value string) string {
	if value == "" {
		return ""
	}
	return strconv.Quote(value)
}

func davResponseETag(headers http.Header) string {
	value, _ := strconv.Unquote(headers.Get("ETag"))
	return value
}

func davEventDTO(collection string, object caldav.CalendarObject, event ical.Event) Event {
	start, _ := event.DateTimeStart(time.UTC)
	end, _ := event.DateTimeEnd(time.UTC)
	summary, _ := event.Props.Text(ical.PropSummary)
	description, _ := event.Props.Text(ical.PropDescription)
	location, _ := event.Props.Text(ical.PropLocation)
	result := Event{Resource: Resource{ID: davID(object.Path, time.Time{}), ETag: davETag(object.ETag)}, CollectionID: collection, Summary: summary, Description: description, Location: location, Start: start, End: end}
	if prop := event.Props.Get(ical.PropDateTimeStart); prop != nil {
		result.AllDay = prop.ValueType() == ical.ValueDate
		result.TimeZone = prop.Params.Get("TZID")
	}
	result.ReadOnly = object.ETag == "" || len(object.Data.Events()) != 1 || event.Props.Get(ical.PropRecurrenceRule) != nil || event.Props.Get(ical.PropRecurrenceID) != nil || event.Props.Get(ical.PropRecurrenceDates) != nil
	if result.ReadOnly {
		result.ID = davID(object.Path, start)
	}
	for _, prop := range event.Props.Values(ical.PropAttendee) {
		result.Attendees = append(result.Attendees, Attendee{Email: strings.TrimPrefix(prop.Value, "mailto:"), Name: prop.Params.Get("CN")})
	}
	var raw bytes.Buffer
	if ical.NewEncoder(&raw).Encode(object.Data) == nil {
		result.DAV = raw.String()
	}
	return result
}

func (c *Client) davEvents(ctx context.Context, collection string, from, to time.Time, cursor string) (Page[Event], error) {
	if _, err := davPath(yahooCalendar, collection); err != nil {
		return Page[Event]{}, err
	}
	client, err := c.calendarDAV()
	if err != nil {
		return Page[Event]{}, err
	}
	objects, err := client.QueryCalendar(ctx, collection, &caldav.CalendarQuery{CompRequest: caldav.CalendarCompRequest{AllProps: true, AllComps: true, Expand: &caldav.CalendarExpandRequest{Start: from, End: to}}, CompFilter: caldav.CompFilter{Name: ical.CompCalendar, Comps: []caldav.CompFilter{{Name: ical.CompEvent, Start: from, End: to}}}})
	if err != nil {
		return Page[Event]{}, err
	}
	items := []Event{}
	for _, object := range objects {
		if object.Data == nil {
			return Page[Event]{}, ErrUnavailable
		}
		if _, err = davObject(yahooCalendar, collection, object.Path); err != nil {
			return Page[Event]{}, err
		}
		for _, event := range object.Data.Events() {
			item := davEventDTO(collection, object, event)
			if !item.Start.IsZero() && item.End.After(from) && item.Start.Before(to) {
				items = append(items, item)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
	return davPage(items, cursor)
}

func (c *Client) davEvent(ctx context.Context, collection, id string) (Event, error) {
	objectPath, start, err := decodeDAVID(id)
	if err != nil {
		return Event{}, err
	}
	if _, err = davObject(yahooCalendar, collection, objectPath); err != nil {
		return Event{}, err
	}
	client, err := c.calendarDAV()
	if err != nil {
		return Event{}, err
	}
	if start.IsZero() {
		object, err := client.GetCalendarObject(ctx, objectPath)
		if err != nil {
			return Event{}, err
		}
		if object.Data == nil {
			return Event{}, ErrUnavailable
		}
		events := object.Data.Events()
		if len(events) == 0 {
			return Event{}, ErrNotFound
		}
		return davEventDTO(collection, *object, events[0]), nil
	}
	objects, err := client.MultiGetCalendar(ctx, collection, &caldav.CalendarMultiGet{Paths: []string{objectPath}, CompRequest: caldav.CalendarCompRequest{AllProps: true, AllComps: true, Expand: &caldav.CalendarExpandRequest{Start: start, End: start.Add(time.Second)}}})
	if err != nil {
		return Event{}, err
	}
	for _, object := range objects {
		if object.Data == nil {
			return Event{}, ErrUnavailable
		}
		for _, event := range object.Data.Events() {
			item := davEventDTO(collection, object, event)
			if item.Start.Equal(start) {
				return item, nil
			}
		}
	}
	return Event{}, ErrNotFound
}

func (c *Client) davSaveEvent(ctx context.Context, collection, id, etag string, p EventPatch) (Event, error) {
	parent, err := davPath(yahooCalendar, collection)
	if err != nil {
		return Event{}, err
	}
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Sytue//Linked Calendar//EN")
	event := ical.NewEvent()
	uid := uuid.NewString()
	event.Props.SetText(ical.PropUID, uid)
	cal.Children = append(cal.Children, event.Component)
	target := strings.TrimSuffix(parent, "/") + "/" + uid + ".ics"
	headers := http.Header{"If-None-Match": {"*"}}
	if id != "" {
		current, loadErr := c.davEvent(ctx, collection, id)
		if loadErr != nil {
			return Event{}, loadErr
		}
		if current.ReadOnly {
			return Event{}, ErrReadOnly
		}
		if current.ETag != etag {
			return Event{}, ErrConflict
		}
		headers, err = conditional(etag)
		if err != nil {
			return Event{}, err
		}
		cal, err = ical.NewDecoder(strings.NewReader(current.DAV)).Decode()
		if err != nil || len(cal.Events()) != 1 {
			return Event{}, ErrReadOnly
		}
		events := cal.Events()
		event = &events[0]
		objectPath, _, decodeErr := decodeDAVID(id)
		if decodeErr != nil {
			return Event{}, decodeErr
		}
		target, err = davObject(yahooCalendar, collection, objectPath)
		if err != nil {
			return Event{}, err
		}
	}
	if err := applyDAVEventPatch(event, p); err != nil {
		return Event{}, err
	}
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	var raw bytes.Buffer
	if err = ical.NewEncoder(&raw).Encode(cal); err != nil {
		return Event{}, ErrInvalid
	}
	_, response, err := c.request(ctx, http.MethodPut, target, ical.MIMEType, raw.Bytes(), headers)
	if err != nil {
		return Event{}, err
	}
	parsed, _ := url.Parse(target)
	result := davEventDTO(collection, caldav.CalendarObject{Path: parsed.Path, ETag: davResponseETag(response), Data: cal}, *event)
	// Do not reinterpret a failed read after a confirmed PUT as a failed write.
	// The next explicit refresh supplies a missing provider ETag.
	return result, nil
}

func davContactDTO(collection string, object carddav.AddressObject) Contact {
	card := object.Card
	result := Contact{Resource: Resource{ID: davID(object.Path, time.Time{}), ETag: davETag(object.ETag), ReadOnly: object.ETag == ""}, CollectionID: collection, Name: card.Value(vcard.FieldFormattedName), Organization: card.Value(vcard.FieldOrganization), Title: card.Value(vcard.FieldTitle), Note: card.Value(vcard.FieldNote)}
	if name := card.Name(); name != nil {
		result.GivenName = name.GivenName
		result.FamilyName = name.FamilyName
	}
	for _, f := range card[vcard.FieldEmail] {
		result.Emails = append(result.Emails, f.Value)
	}
	for _, f := range card[vcard.FieldTelephone] {
		result.Phones = append(result.Phones, f.Value)
	}
	for _, f := range card[vcard.FieldAddress] {
		result.Addresses = append(result.Addresses, strings.ReplaceAll(f.Value, ";", ", "))
	}
	var raw bytes.Buffer
	if vcard.NewEncoder(&raw).Encode(card) == nil {
		result.DAV = raw.String()
	}
	return result
}

func (c *Client) davContacts(ctx context.Context, collection, cursor string) (Page[Contact], error) {
	if _, err := davPath(yahooContacts, collection); err != nil {
		return Page[Contact]{}, err
	}
	client, err := c.contactsDAV()
	if err != nil {
		return Page[Contact]{}, err
	}
	objects, err := client.QueryAddressBook(ctx, collection, &carddav.AddressBookQuery{DataRequest: carddav.AddressDataRequest{AllProp: true}})
	if err != nil {
		return Page[Contact]{}, err
	}
	items := []Contact{}
	for _, object := range objects {
		if _, err = davObject(yahooContacts, collection, object.Path); err != nil {
			return Page[Contact]{}, err
		}
		items = append(items, davContactDTO(collection, object))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return davPage(items, cursor)
}

func (c *Client) davContact(ctx context.Context, collection, id string) (Contact, error) {
	objectPath, start, err := decodeDAVID(id)
	if err != nil || !start.IsZero() {
		return Contact{}, ErrInvalid
	}
	if _, err = davObject(yahooContacts, collection, objectPath); err != nil {
		return Contact{}, err
	}
	client, err := c.contactsDAV()
	if err != nil {
		return Contact{}, err
	}
	object, err := client.GetAddressObject(ctx, objectPath)
	if err != nil {
		return Contact{}, err
	}
	return davContactDTO(collection, *object), nil
}

func mergeCardValues(card vcard.Card, key string, values []string) {
	existing := card[key]
	next := make([]*vcard.Field, 0, len(values))
	for _, value := range values {
		var item *vcard.Field
		for _, previous := range existing {
			if previous.Value == value {
				item = previous
				break
			}
		}
		if item == nil {
			item = &vcard.Field{Value: value}
		}
		next = append(next, item)
	}
	card[key] = next
}

func setCardPrimary(card vcard.Card, key, value string) {
	if fields := card[key]; len(fields) > 0 {
		fields[0].Value = value
		return
	}
	card.SetValue(key, value)
}

func (c *Client) davSaveContact(ctx context.Context, collection, id, etag string, p ContactPatch) (Contact, error) {
	parent, err := davPath(yahooContacts, collection)
	if err != nil {
		return Contact{}, err
	}
	card := make(vcard.Card)
	uid := uuid.NewString()
	card.SetValue(vcard.FieldVersion, "3.0")
	card.SetValue(vcard.FieldUID, uid)
	target := strings.TrimSuffix(parent, "/") + "/" + uid + ".vcf"
	headers := http.Header{"If-None-Match": {"*"}}
	if id != "" {
		current, loadErr := c.davContact(ctx, collection, id)
		if loadErr != nil {
			return Contact{}, loadErr
		}
		if current.ReadOnly {
			return Contact{}, ErrReadOnly
		}
		if current.ETag != etag {
			return Contact{}, ErrConflict
		}
		headers, err = conditional(etag)
		if err != nil {
			return Contact{}, err
		}
		card, err = vcard.NewDecoder(strings.NewReader(current.DAV)).Decode()
		if err != nil {
			return Contact{}, ErrReadOnly
		}
		objectPath, _, decodeErr := decodeDAVID(id)
		if decodeErr != nil {
			return Contact{}, decodeErr
		}
		target, err = davObject(yahooContacts, collection, objectPath)
		if err != nil {
			return Contact{}, err
		}
	}
	applyDAVContactPatch(card, p)
	card.SetRevision(time.Now().UTC())
	var raw bytes.Buffer
	if err = vcard.NewEncoder(&raw).Encode(card); err != nil {
		return Contact{}, ErrInvalid
	}
	_, response, err := c.request(ctx, http.MethodPut, target, vcard.MIMEType, raw.Bytes(), headers)
	if err != nil {
		return Contact{}, err
	}
	parsed, _ := url.Parse(target)
	return davContactDTO(collection, carddav.AddressObject{Path: parsed.Path, ETag: davResponseETag(response), Card: card}), nil
}

func applyDAVEventPatch(event *ical.Event, p EventPatch) error {
	if p.Recurrence != nil || p.Attendees != nil {
		return ErrReadOnly
	}
	for name, value := range map[string]*string{ical.PropSummary: p.Summary, ical.PropDescription: p.Description, ical.PropLocation: p.Location} {
		if value != nil {
			event.Props.SetText(name, *value)
		}
	}
	if p.Start != nil || p.End != nil || p.AllDay != nil || p.TimeZone != nil {
		if p.Start == nil || p.End == nil || p.AllDay == nil {
			return ErrInvalid
		}
		for name, value := range map[string]time.Time{ical.PropDateTimeStart: *p.Start, ical.PropDateTimeEnd: *p.End} {
			if *p.AllDay {
				event.Props.SetDate(name, value)
			} else {
				event.Props.SetDateTime(name, value.UTC())
			}
		}
		event.Props.Del(ical.PropDuration)
	}
	return nil
}

func applyDAVContactPatch(card vcard.Card, p ContactPatch) {
	if p.Name != nil {
		setCardPrimary(card, vcard.FieldFormattedName, *p.Name)
	}
	if p.GivenName != nil || p.FamilyName != nil {
		name := card.Name()
		if name == nil {
			name = &vcard.Name{}
		}
		if p.GivenName != nil {
			name.GivenName = *p.GivenName
		}
		if p.FamilyName != nil {
			name.FamilyName = *p.FamilyName
		}
		variants := card[vcard.FieldName]
		card.SetName(name)
		if len(variants) > 1 {
			card[vcard.FieldName] = append(card[vcard.FieldName], variants[1:]...)
		}
	}
	for key, value := range map[string]*string{vcard.FieldOrganization: p.Organization, vcard.FieldTitle: p.Title, vcard.FieldNote: p.Note} {
		if value != nil {
			setCardPrimary(card, key, *value)
		}
	}
	if p.Emails != nil {
		mergeCardValues(card, vcard.FieldEmail, *p.Emails)
	}
	if p.Phones != nil {
		mergeCardValues(card, vcard.FieldTelephone, *p.Phones)
	}
	if card.Value(vcard.FieldFormattedName) == "" {
		if name := card.Name(); name != nil {
			card.SetValue(vcard.FieldFormattedName, strings.TrimSpace(name.GivenName+" "+name.FamilyName))
		}
	}
}
