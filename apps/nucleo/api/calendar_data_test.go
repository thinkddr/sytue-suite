// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateCalendarFixture struct {
	CalendarData
	scope    storage.Scope
	get      func(context.Context, string) (Event, error)
	create   func(context.Context, string, EventInput) (Event, error)
	resource func(context.Context, string, Booking) (string, error)
	page     func(context.Context, string, int, string) (Busqueda, error)
}

func (f privateCalendarFixture) PrivateStorageScope() storage.Scope { return f.scope }
func (f privateCalendarFixture) GetEvent(ctx context.Context, id string) (Event, error) {
	return f.get(ctx, id)
}

func (f privateCalendarFixture) CreateEvent(ctx context.Context, id string, in EventInput) (Event, error) {
	return f.create(ctx, id, in)
}

func (f privateCalendarFixture) BookResource(ctx context.Context, id string, in Booking) (string, error) {
	return f.resource(ctx, id, in)
}

func (f privateCalendarFixture) SearchEventsPage(ctx context.Context, q string, n int, cursor string) (Busqueda, error) {
	return f.page(ctx, q, n, cursor)
}

func TestPrivateCalendarPreservesPayloadAndFailureWithoutLegacyFallback(t *testing.T) {
	calls := 0
	original := NewCalendars("https://calendar.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"legacy"}`)), Request: r}, nil
	})})
	denied := errors.New("private keys unavailable")
	start := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	minutes := []int{10, 30}
	fixture := privateCalendarFixture{
		scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "calendar"},
		get: func(_ context.Context, id string) (Event, error) {
			if id != "private-event" {
				t.Fatal("changed event identity")
			}
			return Event{}, denied
		},
		create: func(_ context.Context, id string, in EventInput) (Event, error) {
			if id != "private-calendar" || in.Description != "never in legacy HTTP" || in.Reminders == nil || len(*in.Reminders) != 2 || !in.Start.Equal(start) {
				t.Fatal("changed private event input")
			}
			return Event{ID: "created", Description: in.Description}, nil
		},
		resource: func(_ context.Context, id string, in Booking) (string, error) {
			if id != "room" || in.UID != "private-uid" {
				t.Fatal("changed booking identity")
			}
			return "", denied
		},
		page: func(_ context.Context, q string, n int, cursor string) (Busqueda, error) {
			if q != "private text" || n != 2 || cursor != "private cursor" {
				t.Fatal("changed private search cursor")
			}
			return Busqueda{Events: []Event{{ID: "result"}}, Siguiente: "next"}, nil
		},
	}
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.GetEvent(t.Context(), "private-event"); !errors.Is(err, denied) || calls != 0 {
		t.Fatal("failure reached HTTP", err, calls)
	}
	created, err := private.CreateEvent(t.Context(), "private-calendar", EventInput{Start: start, Description: "never in legacy HTTP", Reminders: &minutes})
	if err != nil || created.ID != "created" || calls != 0 {
		t.Fatal("private write failed", err, calls)
	}
	if _, err := private.BookResource(t.Context(), "room", Booking{UID: "private-uid"}); !errors.Is(err, denied) || calls != 0 {
		t.Fatal("resource failure reached HTTP", err, calls)
	}
	page, err := private.SearchEventsPage(t.Context(), "private text", 2, "private cursor")
	if err != nil || len(page.Events) != 1 || page.Siguiente != "next" || calls != 0 {
		t.Fatal("private search changed", err, calls)
	}
	if _, err := private.c.Do(t.Context(), Request{Path: "/future-operation"}); !errors.Is(err, ErrPrivateCalendarOperation) || calls != 0 {
		t.Fatal("unknown operation reached HTTP", err, calls)
	}
	if _, err := private.Peticiones("calendar").Pendientes(t.Context()); !errors.Is(err, ErrPrivateCalendarOperation) || calls != 0 {
		t.Fatal("ACL fell back to HTTP", err, calls)
	}
	legacy, err := original.GetEvent(t.Context(), "event")
	if err != nil || legacy.ID != "legacy" || calls != 1 {
		t.Fatal("original client changed", err, calls)
	}
	if _, err := private.WithPrivateData(fixture); !errors.Is(err, ErrPrivateCalendarOperation) {
		t.Fatal("nested facade accepted")
	}
	for _, scope := range []storage.Scope{{}, {Tenant: "org", Subject: "owner", Product: "contacts"}} {
		fixture.scope = scope
		if _, err := original.WithPrivateData(fixture); !errors.Is(err, ErrPrivateCalendarOperation) {
			t.Fatal("foreign scope accepted")
		}
	}
	if _, err := original.WithPrivateData(nil); !errors.Is(err, ErrPrivateCalendarOperation) {
		t.Fatal("nil repository accepted")
	}
}
