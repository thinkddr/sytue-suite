// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// El hilo se pide por su id y sale ENTERO, con mailbox_id por mensaje: es lo que
// permite al cliente enseñar en la misma conversación lo recibido y la copia de
// Enviados. Sin mailbox_id en el sobre, la fusión en el cliente sería a ciegas.
func TestListThreadMessagesOK(t *testing.T) {
	inbox, sent, thread := uuid.New(), uuid.New(), uuid.New()
	st := &fakeStore{
		acc: domain.Account{ID: uuid.New()},
		threadMsgs: []domain.Message{
			{ID: uuid.New(), ThreadID: thread, MailboxID: sent, Subject: "Re: Presupuesto"},
			{ID: uuid.New(), ThreadID: thread, MailboxID: inbox, Subject: "Presupuesto"},
		},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, req(http.MethodGet, "/api/v1/threads/x/messages", "", true,
		map[string]string{"threadID": thread.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	if st.lastThreadID != thread {
		t.Fatalf("threadID pasado al store = %s, quiero %s", st.lastThreadID, thread)
	}
	var got struct {
		Messages []struct {
			MailboxID uuid.UUID `json:"mailbox_id"`
			ThreadID  uuid.UUID `json:"thread_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("mensajes = %d, quiero 2", len(got.Messages))
	}
	// La gracia del endpoint: el hilo cruza carpetas. Si las dos vinieran del
	// mismo buzón, esto sería el listado de carpeta de siempre.
	if got.Messages[0].MailboxID != sent || got.Messages[1].MailboxID != inbox {
		t.Fatalf("mailbox_id no viaja en el sobre: %+v", got.Messages)
	}
	if got.Messages[0].ThreadID != thread {
		t.Fatalf("thread_id no viaja en el sobre: %+v", got.Messages[0])
	}
}

// Un hilo sin mensajes propios (ajeno o inexistente) sale VACÍO, no 404: no se
// confirma si el hilo existe. Y `messages` debe ser [] y no null, o el cliente
// tendría que distinguir dos formas del mismo «nada».
func TestListThreadMessagesAjenoSaleVacio(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}} // el store no devuelve nada
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, req(http.MethodGet, "/api/v1/threads/x/messages", "", true,
		map[string]string{"threadID": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Messages []domain.Message `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Messages == nil || len(got.Messages) != 0 {
		t.Fatalf("messages = %v, quiero [] (no null)", got.Messages)
	}
}

func TestListThreadMessagesIDInvalido(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, req(http.MethodGet, "/api/v1/threads/x/messages", "", true,
		map[string]string{"threadID": "no-es-un-uuid"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestListThreadMessagesErrorDelStore(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, threadErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, req(http.MethodGet, "/api/v1/threads/x/messages", "", true,
		map[string]string{"threadID": uuid.New().String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestListThreadMessagesSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.listThreadMessages(rec, noAuthReq(http.MethodGet, "/api/v1/threads/x/messages", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("código = %d, quiero 401", rec.Code)
	}
}
