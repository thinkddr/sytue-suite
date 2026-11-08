// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// La web marca un correo como leído con un Email/set de `keywords/$seen` por POST
// /jmap: eso manda la señal de cambios al widget de Mail. Un lote que solo lee
// (Email/get, Email/query), aunque vaya por POST, no la manda. VISTO EN ROJO sin el
// TrasEscribir de jmapConSenal (el $seen no avisaba) y sin el CambioEn de handleJMAP (cada
// lectura avisaba).
func TestUnEmailSetDeSeenPorJMAPMandaLaSenal(t *testing.T) {
	var mu sync.Mutex
	var llegan []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Subject string `json:"subject"`
			Event   string `json:"event"`
			Data    struct {
				Producto string `json:"producto"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Event+" "+e.Data.Producto+" "+e.Subject)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	msg := uuid.New()
	st := &fakeStore{bySubject: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	a.hub = hubnotify.New(hub.URL, "s")
	r := chi.NewRouter()
	r.Method(http.MethodPost, "/jmap", a.jmapConSenal())
	pedir := func(lote string) {
		t.Helper()
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req(http.MethodPost, "/jmap", lote, true, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /jmap: %d %s", rec.Code, rec.Body.String())
		}
	}
	recibido := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), llegan...)
	}

	// Solo lectura: nada. Se espera un poco, porque el envío es asíncrono.
	pedir(`{"methodCalls":[["Email/query",{},"c0"],["Email/get",{"ids":[]},"c1"]]}`)
	time.Sleep(300 * time.Millisecond)
	if got := recibido(); len(got) != 0 {
		t.Fatalf("un lote que solo lee manda la señal: %v", got)
	}

	pedir(`{"methodCalls":[["Email/set",{"update":{"` + msg.String() + `":{"keywords/$seen":true}}},"c0"]]}`)
	limite := time.Now().Add(5 * time.Second)
	for len(recibido()) == 0 {
		if time.Now().After(limite) {
			t.Fatal("marcar como leído por JMAP no manda la señal de cambios")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := recibido(); len(got) != 1 || got[0] != "datos mail user-1" {
		t.Fatalf("llega %v, quiero [datos mail user-1]", got)
	}
}
