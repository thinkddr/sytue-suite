// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// El buscador global de la suite (services/hub/internal/connectors/mail.go) lee de
// esta respuesta `messages[].{id, subject, from, preview, date}`. No puede importar
// aquel paquete —es `internal` de otro servicio—, así que el contrato se fija desde
// este lado: si un campo cambia de nombre, Mail vuelve a salir como «no ha
// respondido» sin que nada falle.
func TestSearchMessagesParaElBuscadorGlobal(t *testing.T) {
	enviado := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	id := uuid.New()
	st := &fakeStore{
		acc: domain.Account{ID: uuid.New()},
		searchRet: []domain.Message{{
			ID: id, Subject: "Presupuesto", FromAddr: "ana@acme.com",
			Snippet: "Te paso el presupuesto", SentAt: enviado,
		}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.searchMessages(rec, req(http.MethodGet, "/api/v1/messages/search?q=from%3Aana+presupuesto&limit=5", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	// La misma lectura de la consulta que Email/query: el operador sale del texto.
	if st.lastSearch.From != "ana" || st.lastSearch.Text != "presupuesto" || st.lastSearch.Limit != 5 {
		t.Fatalf("filtro pasado al store = %+v", st.lastSearch)
	}

	var got struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("mensajes = %d, quiero 1", len(got.Messages))
	}
	m := got.Messages[0]
	for campo, quiero := range map[string]string{
		"id": id.String(), "subject": "Presupuesto", "from": "ana@acme.com",
		"preview": "Te paso el presupuesto", "date": enviado.Format(time.RFC3339),
	} {
		if m[campo] != quiero {
			t.Errorf("messages[0].%s = %v, quiero %q (lo lee el conector del Hub)", campo, m[campo], quiero)
		}
	}
}

// Sin texto no se pregunta a la base, y `messages` es [] y no null.
func TestSearchMessagesVacia(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, searchErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.searchMessages(rec, req(http.MethodGet, "/api/v1/messages/search?q=++", "", true, nil))
	if rec.Code != http.StatusOK || rec.Body.String() == "" {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Messages []domain.Message `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Messages == nil {
		t.Fatalf("messages = %v (%v), quiero []", got.Messages, err)
	}
}

// El límite se acota: el buscador global no necesita más de una página corta.
func TestSearchMessagesAcotaElLimite(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	a.searchMessages(httptest.NewRecorder(), req(http.MethodGet, "/api/v1/messages/search?q=x&limit=100000", "", true, nil))
	if st.lastSearch.Limit != maxBusquedaSuite {
		t.Fatalf("límite = %d, quiero %d", st.lastSearch.Limit, maxBusquedaSuite)
	}
}

// La búsqueda pagina: `position` llega al store como desplazamiento (el de
// Email/query), y sin él o con basura es la primera página.
func TestSearchMessagesPagina(t *testing.T) {
	for q, quiero := range map[string]int{
		"q=x&limit=50&position=100": 100,
		"q=x&limit=50":              0,
		"q=x&position=-3":           0,
		"q=x&position=abc":          0,
	} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		a.searchMessages(httptest.NewRecorder(), req(http.MethodGet, "/api/v1/messages/search?"+q, "", true, nil))
		if st.lastSearch.Offset != quiero {
			t.Errorf("%s: desplazamiento %d, quiero %d", q, st.lastSearch.Offset, quiero)
		}
	}
}

func TestSearchMessagesSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.searchMessages(rec, noAuthReq(http.MethodGet, "/api/v1/messages/search?q=x", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("código = %d, quiero 401", rec.Code)
	}
}
