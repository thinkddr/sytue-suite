// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// fakePospuestos es lo que el fakeStore recuerda de posponer y silenciar.
type fakePospuestos struct {
	lista      []domain.Snooze
	estado     domain.ThreadState
	estadoErr  error
	err        error
	llamadas   int
	hilo       uuid.UUID
	until      time.Time
	quitado    uuid.UUID
	silenciado *bool
	archivados int
}

func (f *fakeStore) SnoozeThread(_ context.Context, _, threadID uuid.UUID, until time.Time) (domain.Snooze, error) {
	f.pospuestos.llamadas++
	f.pospuestos.hilo, f.pospuestos.until = threadID, until
	if f.pospuestos.err != nil {
		return domain.Snooze{}, f.pospuestos.err
	}
	return domain.Snooze{ThreadID: threadID, Until: until.UTC()}, nil
}

func (f *fakeStore) UnsnoozeThread(_ context.Context, _, threadID uuid.UUID) error {
	f.pospuestos.llamadas++
	f.pospuestos.quitado = threadID
	return f.pospuestos.err
}

func (f *fakeStore) ListSnoozes(context.Context, uuid.UUID) ([]domain.Snooze, error) {
	return f.pospuestos.lista, f.pospuestos.err
}

func (f *fakeStore) ThreadState(context.Context, uuid.UUID, uuid.UUID) (domain.ThreadState, error) {
	return f.pospuestos.estado, f.pospuestos.estadoErr
}

func (f *fakeStore) MuteThread(_ context.Context, _, threadID uuid.UUID, muted bool) (int, error) {
	f.pospuestos.llamadas++
	f.pospuestos.hilo = threadID
	f.pospuestos.silenciado = &muted
	return f.pospuestos.archivados, f.pospuestos.err
}

func TestSnoozeThreadLlegaAlStoreConLaFecha(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	hilo := uuid.New()
	until := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	rec := httptest.NewRecorder()
	a.snoozeThread(rec, req(http.MethodPut, "/api/v1/threads/x/snooze",
		`{"until":"`+until.Format(time.RFC3339)+`"}`, true, map[string]string{"threadID": hilo.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.pospuestos.hilo != hilo || !st.pospuestos.until.Equal(until) {
		t.Fatalf("al store llegó %v / %v", st.pospuestos.hilo, st.pospuestos.until)
	}
	var got domain.Snooze
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ThreadID != hilo || !got.Until.Equal(until) {
		t.Fatalf("respuesta = %s (%v)", rec.Body.String(), err)
	}
}

// Una fecha pasada, demasiado próxima o a más de un año es un 400 con código propio
// que el cliente sabe explicar, y no llega al store.
func TestSnoozeThreadRechazaFechasFueraDeRango(t *testing.T) {
	for _, until := range []string{
		time.Now().Add(-time.Hour).Format(time.RFC3339),
		time.Now().Add(10 * time.Second).Format(time.RFC3339),
		time.Now().Add(400 * 24 * time.Hour).Format(time.RFC3339),
	} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		a.snoozeThread(rec, req(http.MethodPut, "/api/v1/threads/x/snooze", `{"until":"`+until+`"}`, true,
			map[string]string{"threadID": uuid.NewString()}))
		if rec.Code != http.StatusBadRequest || decodeEnvelopeCode(t, rec) != "invalid_snooze" {
			t.Fatalf("%s: código = %d (%s), quiero 400 invalid_snooze", until, rec.Code, rec.Body.String())
		}
		if st.pospuestos.llamadas != 0 {
			t.Fatalf("%s: lo rechazado llegó al store", until)
		}
	}
}

func TestSnoozeThreadSinNadaEnRecibidosEs409(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	st.pospuestos.err = domain.ErrNothingToSnooze
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.snoozeThread(rec, req(http.MethodPut, "/api/v1/threads/x/snooze",
		`{"until":"`+time.Now().Add(time.Hour).Format(time.RFC3339)+`"}`, true,
		map[string]string{"threadID": uuid.NewString()}))
	if rec.Code != http.StatusConflict || decodeEnvelopeCode(t, rec) != "nothing_to_snooze" {
		t.Fatalf("código = %d (%s), quiero 409 nothing_to_snooze", rec.Code, rec.Body.String())
	}
}

func TestSnoozeThreadIdInvalidoEs400(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.snoozeThread(rec, req(http.MethodPut, "/api/v1/threads/x/snooze", `{}`, true,
		map[string]string{"threadID": "no-es-un-uuid"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestUnsnoozeThreadDevuelve204Y404SiNoEstabaPospuesto(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	hilo := uuid.New()
	rec := httptest.NewRecorder()
	a.unsnoozeThread(rec, req(http.MethodDelete, "/api/v1/threads/x/snooze", "", true,
		map[string]string{"threadID": hilo.String()}))
	if rec.Code != http.StatusNoContent || st.pospuestos.quitado != hilo {
		t.Fatalf("código = %d, quitado = %v", rec.Code, st.pospuestos.quitado)
	}
	st.pospuestos.err = domain.ErrNotFound
	rec = httptest.NewRecorder()
	a.unsnoozeThread(rec, req(http.MethodDelete, "/api/v1/threads/x/snooze", "", true,
		map[string]string{"threadID": hilo.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestListSnoozesVaciaEsUnaLista(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.listSnoozes(rec, req(http.MethodGet, "/api/v1/snoozes", "", true, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"snoozes":[]`) {
		t.Fatalf("código = %d, cuerpo = %s; quiero 200 con snoozes:[]", rec.Code, rec.Body.String())
	}
}

func TestMuteThreadSilenciaYDiceCuantosArchivo(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	st.pospuestos.archivados = 3
	a := newAPI(st, nil, nil)
	hilo := uuid.New()
	rec := httptest.NewRecorder()
	a.muteThread(rec, req(http.MethodPut, "/api/v1/threads/x/mute", `{"muted":true}`, true,
		map[string]string{"threadID": hilo.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.pospuestos.hilo != hilo || st.pospuestos.silenciado == nil || !*st.pospuestos.silenciado {
		t.Fatalf("al store llegó %v / %v", st.pospuestos.hilo, st.pospuestos.silenciado)
	}
	if !strings.Contains(rec.Body.String(), `"archived":3`) || !strings.Contains(rec.Body.String(), `"muted":true`) {
		t.Fatalf("respuesta = %s", rec.Body.String())
	}
}

// Un cuerpo sin `muted` no se lee como «deja de silenciar».
func TestMuteThreadSinElCampoEs400(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.muteThread(rec, req(http.MethodPut, "/api/v1/threads/x/mute", `{}`, true,
		map[string]string{"threadID": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest || st.pospuestos.llamadas != 0 {
		t.Fatalf("código = %d, llamadas = %d; quiero 400 sin llegar al store", rec.Code, st.pospuestos.llamadas)
	}
}

// El hilo lleva su estado: el lector pinta «Pospuesto hasta…» y «Silenciada» sin
// otra petición.
func TestListThreadMessagesLlevaElEstadoDelHilo(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	until := time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	st.pospuestos.estado = domain.ThreadState{Muted: true, SnoozedUntil: &until}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, req(http.MethodGet, "/api/v1/threads/x/messages", "", true,
		map[string]string{"threadID": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Muted        bool       `json:"muted"`
		SnoozedUntil *time.Time `json:"snoozed_until"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Muted || got.SnoozedUntil == nil || !got.SnoozedUntil.Equal(until) {
		t.Fatalf("estado = %+v (%s)", got, rec.Body.String())
	}
}
