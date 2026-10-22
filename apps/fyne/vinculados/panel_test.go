// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vinculados

import (
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func panelPrueba(t *testing.T, servicio string, handler http.HandlerFunc) *Panel {
	t.Helper()
	a := test.NewApp()
	a.Settings().SetTheme(tema.Sytue{})
	w := a.NewWindow("Fixture")
	w.SetContent(container.NewStack())
	w.Resize(fyne.NewSize(390, 844))
	t.Cleanup(a.Quit)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	tr, err := i18n.New("en", i18n.Common(), catalogos.Mail())
	if err != nil {
		t.Fatal(err)
	}
	domain := catalogos.Calendar()
	if servicio == "contacts" {
		domain = catalogos.Contacts()
	}
	if servicio == "notes" {
		domain = catalogos.Notes()
	}
	dtr, err := i18n.New("en", i18n.Common(), domain)
	if err != nil {
		t.Fatal(err)
	}
	p := &Panel{tr: tr, dominio: dtr, svc: api.NewIntegraciones(server.URL, nil, nil), servicio: servicio, lienzo: w.Canvas(), identidad: func() string { return "subject/generation" }, enFondo: func(f func()) { f() }, enUI: func(f func()) { f() }}
	p.actor = p.identidad()
	p.ctx, p.cancelar = context.WithCancel(context.Background())
	p.dialogo = ui.NuevoDialogo("Fixture", nil)
	p.cuenta = api.ExternalMailAccount{InstanceID: "owned", Provider: "microsoft", Address: "original@example.eu", Connected: true, Services: []string{"mail", servicio}}
	p.coleccion = api.RemoteCollection{ID: "opaque collection/?&", Name: "Fixture"}
	p.desde = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	p.hasta = p.desde.AddDate(0, 1, 0)
	t.Cleanup(p.Cerrar)
	return p
}

func TestEmptyAccountConnectSelectsExactConsentedIdentity(t *testing.T) {
	requests := 0
	p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v1/external-mail/accounts":
			_, _ = w.Write([]byte(`{"items":[{"instance_id":"old","provider":"gmail","address":"old@example.eu","connected":true,"services":["mail","calendar"]},{"instance_id":"new","provider":"gmail","address":"new@example.eu","connected":true,"services":["mail","calendar"]}]}`))
		case "/api/v1/linked-services/new/calendar/collections":
			if r.URL.Query().Get("expected_address") != "new@example.eu" {
				t.Errorf("wrong identity %s", r.URL)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"new-calendar","name":"New"}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	p.cuentas = api.ExternalMailAccounts{Providers: []api.ExternalMailProvider{{Provider: "gmail", Available: true, Authorization: "oauth2", Services: []api.LinkedServiceCapability{{Service: "calendar", Available: true}}}}}
	p.cuenta = api.ExternalMailAccount{}
	p.autorizar = func(_ context.Context, provider, instance string, services ...string) (string, error) {
		if provider != "gmail" || instance != "" {
			t.Errorf("fresh connection reused account: %q %q", provider, instance)
		}
		return "new", nil
	}
	p.paso = "cuentas"
	p.pintar()
	test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("externalMail.connect")))
	test.Tap(eleccionConTexto(t, p.dialogo.Cuerpo, "Gmail"))
	test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("externalMail.authorize")))
	if p.cuenta.InstanceID != "new" || p.cuenta.Address != "new@example.eu" || len(p.colecciones) != 1 || requests != 2 {
		t.Fatalf("did not adopt exact new identity: %#v requests=%d", p.cuenta, requests)
	}
}

func TestConsentMissingOrUngrantedNeverFallsBackToOldAccount(t *testing.T) {
	for _, items := range []string{`[{"instance_id":"old","connected":true,"services":["calendar"]}]`, `[{"instance_id":"new","connected":true,"services":["mail"]}]`, `[{"instance_id":"new","connected":false,"services":["calendar"]}]`} {
		t.Run(items, func(t *testing.T) {
			p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/external-mail/accounts" {
					t.Errorf("used an unconfirmed identity %s", r.URL)
				}
				_, _ = w.Write([]byte(`{"items":` + items + `}`))
			})
			p.cuenta = api.ExternalMailAccount{}
			p.cargarCuentas("new")
			if p.cuenta.InstanceID != "" || p.fallo != p.t("linkedServices.notGranted") {
				t.Fatalf("unconfirmed consent appeared connected: %#v %s", p.cuenta, p.fallo)
			}
		})
	}
}

func TestDeniedConsentRetriesTheSameDraft(t *testing.T) {
	calls := 0
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accounts") {
			_, _ = w.Write([]byte(`{"items":[{"instance_id":"draft","provider":"gmail","address":"new@example.eu","connected":true,"services":["mail","contacts"]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	p.autorizar = func(_ context.Context, provider, instance string, services ...string) (string, error) {
		calls++
		if !reflect.DeepEqual(services, []string{"contacts"}) {
			t.Errorf("new linked account requested unrelated mail: %v", services)
		}
		if calls == 1 {
			if instance != "" {
				t.Error("first connection reused another account")
			}
			return "draft", errors.New("synthetic denial")
		}
		if instance != "draft" {
			t.Errorf("denial retried using a new draft: %q", instance)
		}
		return "draft", nil
	}
	p.conectar("gmail", "")
	p.conectar("gmail", "")
	if calls != 2 || p.cuenta.InstanceID != "draft" {
		t.Fatalf("draft not adopted %d %#v", calls, p.cuenta)
	}
}

func TestLinkedConsentPreservesExistingServicesAndReviewsBeforeBrowser(t *testing.T) {
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"items":[]}`)) })
	p.cuentas.Items = []api.ExternalMailAccount{{InstanceID: "owned", Provider: "microsoft", Services: []string{"mail", "calendar", "notes", "unknown", "calendar"}}}
	p.paso = "cuentas"
	p.prepararConsentimiento("microsoft", "owned")
	if !reflect.DeepEqual(p.consentServices, []string{"contacts", "mail", "calendar", "notes"}) {
		t.Fatalf("existing service lost or unknown permission forwarded: %v", p.consentServices)
	}
	mostrados := map[string]bool{}
	for _, objeto := range p.dialogo.Cuerpo.(*fyne.Container).Objects {
		if parrafo, ok := objeto.(*ui.Parrafo); ok {
			mostrados[parrafo.Texto] = true
		}
	}
	for _, key := range []string{"dataflow.item.mail", "dataflow.item.contacts", "dataflow.item.events", "app.notes.description"} {
		if !mostrados[p.t(key)] {
			t.Errorf("permission %s requested without being shown", key)
		}
	}
	calls := 0
	p.autorizar = func(_ context.Context, _, instance string, services ...string) (string, error) {
		calls++
		if instance != "owned" || !reflect.DeepEqual(services, p.consentServices) {
			t.Errorf("review differs from requested scopes: %s %v", instance, services)
		}
		return instance, errors.New("synthetic stop")
	}
	if calls != 0 || p.paso != "consentimiento" {
		t.Fatal("browser opened before permission review")
	}
	test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("externalMail.authorize")))
	if calls != 1 {
		t.Fatal("review could not open consent")
	}
}

func TestConfirmedConsentRefreshesExactAccountWithoutRepeatingConsent(t *testing.T) {
	inventarios := 0
	p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accounts") {
			inventarios++
			if inventarios == 1 {
				_, _ = w.Write([]byte(`{"items":[{"instance_id":"old","connected":true,"services":["mail","calendar"]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"instance_id":"new","address":"new@example.eu","connected":true,"services":["mail","calendar"]}]}`))
			return
		}
		if !strings.Contains(r.URL.Path, "/new/calendar/") {
			t.Errorf("wrong account %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	p.cuenta = api.ExternalMailAccount{}
	p.cargarCuentas("new")
	if p.esperadaID != "new" {
		t.Fatal("lost exact confirmed identity before inventory convergence")
	}
	p.cargarCuentas(p.esperadaID)
	if inventarios != 2 || p.cuenta.InstanceID != "new" || p.esperadaID != "" {
		t.Fatalf("inventory refresh failed %#v %q", p.cuenta, p.esperadaID)
	}
}

func TestEventPatchPreservesRichBodyGuestsRecurrenceAndExactETag(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			expected := map[string]any{"expected_address": "original@example.eu", "collection": "opaque collection/?&", "id": "event/?&", "etag": "revision-1", "patch": map[string]any{"summary": "Changed"}}
			if !reflect.DeepEqual(body, expected) {
				t.Errorf("flattened remote event or lost identity/ETag: %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"event/?&"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	p.evento = api.RemoteEvent{RemoteResource: api.RemoteResource{ID: "event/?&", ETag: "revision-1"}, Summary: "Original", Description: "Rendered body", Start: time.Date(2026, 10, 20, 10, 0, 37, 0, time.UTC), End: time.Date(2026, 10, 20, 11, 0, 37, 0, time.UTC), Attendees: []api.RemoteAttendee{{Email: "guest@example.eu"}}}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.campos["title"].PonerTexto("Changed")
	p.guardar()
	if posts != 1 || p.paso != "items" {
		t.Fatalf("save failed: posts=%d step=%s error=%s", posts, p.paso, p.fallo)
	}
}

func TestNewExternalEventsAndContactsUseEmptyIdentityAndCannotReplayUnknownCreate(t *testing.T) {
	for _, service := range []string{"calendar", "contacts"} {
		t.Run(service, func(t *testing.T) {
			posts, reads := 0, 0
			p := panelPrueba(t, service, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					reads++
					_, _ = w.Write([]byte(`{"items":[]}`))
					return
				}
				posts++
				var body struct {
					ID      string          `json:"id"`
					ETag    string          `json:"etag"`
					Address string          `json:"expected_address"`
					Patch   json.RawMessage `json:"patch"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.ID != "" || body.ETag != "" || body.Address != "original@example.eu" {
					t.Fatalf("create reused resource identity: %#v", body)
				}
				if service == "calendar" {
					var patch api.RemoteEventPatch
					_ = json.Unmarshal(body.Patch, &patch)
					if patch.Summary == nil || *patch.Summary != "New fixture" || patch.Start == nil || patch.End == nil || patch.AllDay == nil || !*patch.AllDay || patch.TimeZone == nil || *patch.TimeZone != "Europe/Madrid" {
						t.Fatalf("incomplete new event: %#v", patch)
					}
					if patch.End.Sub(*patch.Start) != 24*time.Hour {
						t.Errorf("all-day create failed to use exclusive following day: %s %s", patch.Start, patch.End)
					}
				} else {
					var patch api.RemoteContactPatch
					_ = json.Unmarshal(body.Patch, &patch)
					if patch.Name == nil || *patch.Name != "New fixture" {
						t.Fatal("new contact lost its name")
					}
				}
				// A truncated success may follow a completed create at the provider.
				_, _ = w.Write([]byte(`{"id":`))
			})
			p.coleccion.TimeZone = "Europe/Madrid"
			p.paso = "items"
			p.pintar()
			label := "nav.createEvent"
			if service == "contacts" {
				label = "nav.newContact"
			}
			test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.dt(label)))
			if !p.nuevoRegistro || p.guardarBoton == nil {
				t.Fatal("new resource form is missing or read-only")
			}
			field := "title"
			if service == "contacts" {
				field = "name"
			}
			p.campos[field].PonerTexto("New fixture")
			if service == "calendar" {
				p.inicio.Fecha.PonerTexto("21/10/2026")
				p.fin.Fecha.PonerTexto("21/10/2026")
				p.inicio.Hora.PonerTexto("09:00")
				p.fin.Hora.PonerTexto("10:00")
				test.Tap(p.diaCompleto)
			}
			p.guardar()
			p.guardar()
			if posts != 1 || !p.escrituraIncierta {
				t.Fatalf("unknown create replayed: %d %s", posts, p.fallo)
			}
			test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("nav.refresh")))
			if reads != 1 || p.paso != "items" {
				t.Fatal("unknown create retried or attempted to read an empty resource ID")
			}
		})
	}
}

func TestOnlyExplicitNewResourceCanOmitItsVersion(t *testing.T) {
	for _, service := range []string{"calendar", "contacts"} {
		t.Run(service, func(t *testing.T) {
			p := panelPrueba(t, service, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unversioned existing record reached provider") })
			p.evento = api.RemoteEvent{RemoteResource: api.RemoteResource{ID: "existing"}, Summary: "Existing", Start: time.Now(), End: time.Now().Add(time.Hour)}
			p.contacto = api.RemoteContact{RemoteResource: api.RemoteResource{ID: "existing"}, Name: "Existing"}
			p.formulario = p.construirDetalle()
			p.guardar()
			if p.guardarBoton != nil {
				t.Fatal("existing unversioned record became editable")
			}
			p.nuevoRegistro = true
			p.formulario = p.construirDetalle()
			p.guardar()
		})
	}
}

func TestExistingTimedEventCanBecomeAllDayWithoutLosingItsVersion(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		posts++
		var body struct {
			ID    string               `json:"id"`
			ETag  string               `json:"etag"`
			Patch api.RemoteEventPatch `json:"patch"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		v := body.Patch
		if body.ID != "existing" || body.ETag != "v1" || v.AllDay == nil || !*v.AllDay || v.Start == nil || v.End == nil || v.Start.Hour() != 0 || v.End.Sub(*v.Start) != 24*time.Hour {
			t.Errorf("all-day toggle lost identity or date group: %#v", body)
		}
		_, _ = w.Write([]byte(`{"id":"existing"}`))
	})
	p.evento = api.RemoteEvent{RemoteResource: api.RemoteResource{ID: "existing", ETag: "v1"}, Summary: "Existing", Start: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC), TimeZone: "UTC"}
	p.formulario = p.construirDetalle()
	test.Tap(p.diaCompleto)
	p.guardar()
	if posts != 1 {
		t.Fatal("all-day toggle was not saved")
	}
}

func TestRecurringAndReadonlyEventsCannotBeWritten(t *testing.T) {
	for _, resource := range []api.RemoteEvent{{RemoteResource: api.RemoteResource{ID: "series", ETag: "version"}, Recurrence: []string{"RRULE:FREQ=WEEKLY"}}, {RemoteResource: api.RemoteResource{ID: "occurrence", ETag: "version", ReadOnly: true}}} {
		p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("read-only event sent %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
		})
		p.evento = resource
		p.formulario = p.construirDetalle()
		p.guardar()
	}
}

func TestAllDayEditsSendCompleteTimeGroupInTheSelectedZone(t *testing.T) {
	for _, name := range []string{"Europe/Madrid", "America/New_York"} {
		t.Run(name, func(t *testing.T) {
			zone, err := time.LoadLocation(name)
			if err != nil {
				t.Fatal(err)
			}
			posts := 0
			p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"items":[]}`))
					return
				}
				posts++
				var body struct {
					Patch api.RemoteEventPatch `json:"patch"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				v := body.Patch
				if v.Start == nil || v.End == nil || v.AllDay == nil || !*v.AllDay || v.TimeZone == nil || *v.TimeZone != name {
					t.Fatalf("partial or mixed-zone time group %#v", v)
				}
				if v.Start.In(zone).Format(time.DateOnly) != "2026-10-21" || v.End.In(zone).Format(time.DateOnly) != "2026-10-22" {
					t.Errorf("all-day date shifted: %s %s", v.Start.In(zone), v.End.In(zone))
				}
				if v.Summary != nil || v.Description != nil || v.Location != nil {
					t.Error("time change rebuilt other fields")
				}
				_, _ = w.Write([]byte(`{"id":"day"}`))
			})
			p.evento = api.RemoteEvent{RemoteResource: api.RemoteResource{ID: "day", ETag: "v1"}, Summary: "All day", Start: time.Date(2026, 10, 20, 0, 0, 0, 0, zone), End: time.Date(2026, 10, 21, 0, 0, 0, 0, zone), AllDay: true, TimeZone: name}
			p.paso = "detalle"
			p.formulario = p.construirDetalle()
			p.inicio.Fecha.PonerTexto("21/10/2026")
			p.fin.Fecha.PonerTexto("22/10/2026")
			p.guardar()
			if posts != 1 {
				t.Fatalf("all-day save not sent: %s", p.fallo)
			}
		})
	}
}

func TestTimedEditPreservesTheUnchangedEndAndSendsPair(t *testing.T) {
	posts := 0
	end := time.Date(2026, 10, 20, 11, 0, 37, 0, time.UTC)
	p := panelPrueba(t, "calendar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		posts++
		var body struct {
			Patch api.RemoteEventPatch `json:"patch"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		v := body.Patch
		if v.Start == nil || v.End == nil || !v.End.Equal(end) || v.AllDay == nil || *v.AllDay || v.TimeZone == nil || *v.TimeZone != "UTC" {
			t.Errorf("incomplete time pair or lost precision: %#v", v)
		}
		_, _ = w.Write([]byte(`{"id":"timed"}`))
	})
	p.evento = api.RemoteEvent{RemoteResource: api.RemoteResource{ID: "timed", ETag: "v1"}, Summary: "Timed", Start: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC), End: end, TimeZone: "UTC"}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.inicio.Hora.PonerTexto("10:30")
	p.guardar()
	if posts != 1 {
		t.Fatalf("timed save not sent: %s", p.fallo)
	}
}

func TestChangedProviderVersionRequiresFreshReadAndUsesConflictMessage(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, _ *http.Request) {
		posts++
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"external_resource_changed"}}`))
	})
	p.contacto = api.RemoteContact{RemoteResource: api.RemoteResource{ID: "person", ETag: "old"}, Name: "Before"}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.campos["name"].PonerTexto("After")
	p.guardar()
	p.guardar()
	if posts != 1 || !p.escrituraIncierta || p.fallo != p.t("linkedServices.changed") {
		t.Fatalf("stale edit replayed or labelled unknown: %d %s", posts, p.fallo)
	}
}

func TestReadReconnectionFailureOffersConsentForTheExactAccount(t *testing.T) {
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"external_reconnect_required"}}`))
	})
	p.paso = "items"
	p.cargarDetalle("person")
	if !p.reconectarNecesario {
		t.Fatal("lost expired provider consent")
	}
	calls := 0
	p.autorizar = func(_ context.Context, provider, instance string, services ...string) (string, error) {
		calls++
		if provider != "microsoft" || instance != "owned" {
			t.Errorf("reconnected wrong identity %q %q", provider, instance)
		}
		return instance, errors.New("synthetic consent stopped")
	}
	test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("externalMail.connect")))
	test.Tap(botonConTexto(t, p.dialogo.Cuerpo, p.t("externalMail.authorize")))
	if calls != 1 {
		t.Fatalf("no direct reconnect route %d", calls)
	}
}

func TestContactPatchPreservesTypedPhonesAddressesAndOpaqueIDs(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["id"] != "people/opaque?&" || body["etag"] != "version-a" || body["expected_address"] != "original@example.eu" || !reflect.DeepEqual(body["patch"], map[string]any{"name": "Renamed"}) {
				t.Errorf("contact rebuilt: %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"people/opaque?&"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	p.contacto = api.RemoteContact{RemoteResource: api.RemoteResource{ID: "people/opaque?&", ETag: "version-a"}, Name: "Before", Emails: []string{"one@example.eu"}, Phones: []string{"+1234567"}, Addresses: []string{"Existing structured address"}}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.campos["name"].PonerTexto("Renamed")
	p.guardar()
	if posts != 1 {
		t.Fatalf("expected one patch, got %d", posts)
	}
}

func TestOneNoteAppendUnknownDoesNotReplayOrReplaceRichPage(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "notes", func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["text"] != "New paragraph" || body["id"] != "page/?&" || body["title"] != "" || body["expected_address"] != "original@example.eu" || strings.Contains(body["text"], "Original rich page") {
			t.Errorf("destructive notes write: %#v", body)
		}
		w.WriteHeader(http.StatusBadGateway)
	})
	p.nota = api.RemoteNote{RemoteResource: api.RemoteResource{ID: "page/?&"}, Title: "Title", Text: "Original rich page", CanAppend: true}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.campos["text"].PonerTexto("New paragraph")
	p.guardar()
	p.guardar()
	if posts != 1 || !p.escrituraIncierta || p.campos["text"].Texto() != "New paragraph" || p.fallo != p.t("linkedServices.writeUnknown") {
		t.Fatalf("uncertain append replay/lost draft: posts=%d uncertain=%v error=%s", posts, p.escrituraIncierta, p.fallo)
	}
}

func TestNewNoteUnknownDoesNotCreateDuplicate(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "notes", func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["id"] != "" || body["text"] != "Draft" || body["title"] != "New" {
			t.Errorf("wrong create %#v", body)
		}
		w.WriteHeader(http.StatusBadGateway)
	})
	p.crearNota()
	p.campos["title"].PonerTexto("New")
	p.campos["text"].PonerTexto("Draft")
	p.guardar()
	p.guardar()
	if posts != 1 || !p.escrituraIncierta {
		t.Fatalf("new note replayed %d", posts)
	}
}

func TestAcceptedResponseWithTruncatedBodyNeverReplaysCreate(t *testing.T) {
	posts := 0
	p := panelPrueba(t, "notes", func(w http.ResponseWriter, _ *http.Request) { posts++; _, _ = w.Write([]byte(`{"id":`)) })
	p.crearNota()
	p.campos["title"].PonerTexto("New")
	p.campos["text"].PonerTexto("Accepted remotely")
	p.guardar()
	p.guardar()
	if posts != 1 || !p.escrituraIncierta || p.fallo != p.t("linkedServices.writeUnknown") {
		t.Fatalf("invalid success body replayed: %d %v %s", posts, p.escrituraIncierta, p.fallo)
	}
}

func TestLateProviderResponseCannotCrossSessionOrClosedPanel(t *testing.T) {
	for _, closePanel := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-session", true: "closed"}[closePanel], func(t *testing.T) {
			p := panelPrueba(t, "contacts", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"items":[{"id":"private","name":"Old account contact"}]}`))
			})
			var apply func()
			p.enUI = func(f func()) { apply = f }
			p.cargarItems(false)
			if apply == nil {
				t.Fatal("request did not complete")
			}
			if closePanel {
				p.Cerrar()
			} else {
				p.identidad = func() string { return "subject/new-generation" }
			}
			apply()
			if len(p.contactos) != 0 {
				t.Fatal("old account content survived generation/close")
			}
		})
	}
}

func TestPaginationKeepsProviderCursorAndDropsDuplicateRecords(t *testing.T) {
	reads := 0
	p := panelPrueba(t, "contacts", func(w http.ResponseWriter, r *http.Request) {
		reads++
		q := r.URL.Query()
		if q.Get("collection") != "opaque collection/?&" || q.Get("expected_address") != "original@example.eu" {
			t.Errorf("lost collection/identity %s", r.URL)
		}
		if reads == 1 {
			_, _ = w.Write([]byte(`{"items":[{"id":"one","name":"One"}],"next_cursor":"opaque/?&"}`))
			return
		}
		if q.Get("cursor") != "opaque/?&" {
			t.Errorf("cursor reconstructed %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"one","name":"One"},{"id":"two","name":"Two"}],"next_cursor":"opaque/?&"}`))
	})
	p.cargarItems(false)
	p.cargarItems(true)
	if reads != 2 || len(p.contactos) != 2 || p.cursor != "" {
		t.Fatalf("pagination duplicates/loop: %d %d %q", reads, len(p.contactos), p.cursor)
	}
}

func TestMissingCapabilityIsUnavailableAndNoUnsupportedProviderConsent(t *testing.T) {
	for _, provider := range []api.ExternalMailProvider{{Provider: "gmail", Available: true}, {Provider: "microsoft", Available: false, Services: []api.LinkedServiceCapability{{Service: "notes", Available: true}}}, {Provider: "gmail", Available: true, Services: []api.LinkedServiceCapability{{Service: "notes", Available: false, Reason: "google_keep_requires_admin"}}}} {
		if ready, _ := capacidad(provider, "notes"); ready {
			t.Fatalf("unavailable provider offered: %#v", provider)
		}
	}
}

func botonConTexto(t *testing.T, objeto fyne.CanvasObject, texto string) *ui.Boton {
	t.Helper()
	var encontrado *ui.Boton
	var visitar func(fyne.CanvasObject)
	visitar = func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == texto {
			encontrado = b
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, hijo := range c.Objects {
				visitar(hijo)
			}
		}
	}
	visitar(objeto)
	if encontrado == nil {
		t.Fatalf("button %q missing", texto)
	}
	return encontrado
}

func eleccionConTexto(t *testing.T, objeto fyne.CanvasObject, texto string) *ui.TarjetaEleccion {
	t.Helper()
	var encontrado *ui.TarjetaEleccion
	var visitar func(fyne.CanvasObject)
	visitar = func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.TarjetaEleccion); ok && c.Titulo == texto {
			encontrado = c
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, hijo := range c.Objects {
				visitar(hijo)
			}
		}
	}
	visitar(objeto)
	if encontrado == nil {
		t.Fatalf("choice %q missing", texto)
	}
	return encontrado
}

func TestNativeLinkedProviderPickerPhoneLayout(t *testing.T) {
	for _, servicio := range []string{"calendar", "contacts", "notes"} {
		t.Run(servicio, func(t *testing.T) {
			p := panelPrueba(t, servicio, func(w http.ResponseWriter, r *http.Request) { t.Errorf("render triggered network %s", r.URL) })
			p.cuenta = api.ExternalMailAccount{}
			p.paso = "proveedores"
			p.cuentas.Providers = []api.ExternalMailProvider{{Provider: "gmail", Available: true, Authorization: "oauth2", Services: []api.LinkedServiceCapability{{Service: servicio, Available: servicio != "notes", Reason: "google_keep_requires_admin"}}}, {Provider: "microsoft", Available: true, Authorization: "oauth2", Services: []api.LinkedServiceCapability{{Service: servicio, Available: true}}}, {Provider: "yahoo", Available: true, Authorization: "oauth2", Services: []api.LinkedServiceCapability{{Service: servicio, Available: servicio != "notes"}}}}
			p.pintar()
			p.dialogo.Titulo = p.t("linkedServices.connected")
			p.dialogo.Mostrar(p.lienzo)
			cerrar := botonConTexto(t, p.dialogo.Pie, p.t("ui.close"))
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(cerrar)
			if pos.Y+cerrar.Size().Height > p.lienzo.Size().Height {
				t.Fatalf("footer is outside phone viewport: %v %v", pos, cerrar.Size())
			}
			if eleccionConTexto(t, p.dialogo.Cuerpo, "Microsoft Outlook").Inerte {
				t.Fatal("configured provider unavailable")
			}
			if servicio == "notes" && !eleccionConTexto(t, p.dialogo.Cuerpo, "Gmail").Inerte {
				t.Fatal("personal Keep offered as supported")
			}
			if directorio := os.Getenv("SYTUE_CAPTURAS"); directorio != "" {
				f, err := os.Create(filepath.Join(directorio, servicio+"-providers-phone.png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, p.lienzo.Capture())
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
