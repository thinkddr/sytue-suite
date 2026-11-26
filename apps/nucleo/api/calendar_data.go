// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// CalendarData is the device-owned Calendar surface. Missing capabilities must
// return an error; the private facade never falls back to the legacy transport.
type CalendarData interface {
	PrivateStorageScope() storage.Scope
	Invitations(context.Context, bool) ([]Invitation, error)
	GetEvent(context.Context, string) (Event, error)
	RespondInvitation(context.Context, string, string) (Invitation, error)
	Reminders(context.Context) ([]Reminder, error)
	DismissReminder(context.Context, string) error
	SnoozeReminder(context.Context, string, int) (time.Time, error)
	EventICS(context.Context, string) ([]byte, error)
	List(context.Context) ([]Calendar, error)
	SearchEvents(context.Context, string, int) ([]Event, error)
	SharedWithMe(context.Context) ([]Calendar, error)
	CreateCalendar(context.Context, CalendarInput) (Calendar, error)
	UpdateCalendar(context.Context, string, CalendarInput) (Calendar, error)
	DeleteCalendar(context.Context, string) error
	Subscribe(context.Context, SubscriptionInput) (Calendar, error)
	Instances(context.Context, string, time.Time, time.Time) ([]Instance, error)
	CreateEvent(context.Context, string, EventInput) (Event, error)
	UpdateEvent(context.Context, string, EventInput) (Event, error)
	DeleteEvent(context.Context, string) error
	SearchEventsPage(context.Context, string, int, string) (Busqueda, error)
	SuggestSlots(context.Context, time.Time, time.Time, int, string, string, string, bool) ([]Hueco, error)
	SetPartStat(context.Context, string, string) error
	AmendRecurrence(context.Context, string, string, time.Time, string) error
	RefreshSubscription(context.Context, string) error
	ExportICS(context.Context, string) ([]byte, error)
	Shares(context.Context, string) ([]CalendarShare, error)
	Share(context.Context, string, string, string) error
	RevokeShare(context.Context, string, string) error
	Resources(context.Context, time.Time, time.Time) ([]Resource, error)
	CreateResource(context.Context, ResourceInput) (Resource, error)
	UpdateResource(context.Context, string, ResourceInput) (Resource, error)
	DeleteResource(context.Context, string) error
	BookResource(context.Context, string, Booking) (string, error)
}

var ErrPrivateCalendarOperation = errors.New("calendar: operation unavailable through private data")

// WithPrivateData keeps the typed API used by existing screens. The caller owns
// the repository and drains its work before closing the SDK or changing account.
func (c *Calendars) WithPrivateData(data CalendarData) (*Calendars, error) {
	if c == nil || c.c == nil || data == nil || c.data != nil {
		return nil, ErrPrivateCalendarOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "calendar" {
		return nil, ErrPrivateCalendarOperation
	}
	return &Calendars{c: privateDataClient(c.c, ErrPrivateCalendarOperation), data: data}, nil
}

// CalendarUIDData contesta a una invitación por el UID del evento (ResponderPorUID).
// Una fachada privada sin él sigue fallando cerrada.
type CalendarUIDData interface {
	ResponderPorUID(ctx context.Context, uid, partstat string) error
}

// CalendarSerieData parte una serie por una ocurrencia (DividirSerie) en un solo paso:
// crea el evento nuevo con lo que contestaron los invitados mientras tanto y recorta la
// serie. Una fachada sin él lo hace como siempre, CreateEvent y AmendRecurrence.
type CalendarSerieData interface {
	DividirSerie(ctx context.Context, calendario, uid string, ocurrencia time.Time, recorte string, in EventInput) (Event, error)
}

// CalendarFiltroData es la búsqueda de eventos con calendario y tramo
// (SearchEventsFiltrada). Una fachada privada sin él sigue fallando cerrada.
type CalendarFiltroData interface {
	SearchEventsFiltrada(ctx context.Context, texto string, f FiltroBusqueda, limite int, cursor string) (Busqueda, error)
}
