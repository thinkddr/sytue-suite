// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auditsink

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// fakeStore simula la persistencia: sirve eventos desde un cursor y anota lo confirmado.
type fakeStore struct {
	sinks    []store.AuditSink
	rows     []store.AuditRow
	sentTo   int64
	errNoted string
	listErr  error
}

func (f *fakeStore) ListEnabledAuditSinks(context.Context) ([]store.AuditSink, error) {
	return f.sinks, f.listErr
}

func (f *fakeStore) AuditSince(_ context.Context, _ string, afterID int64, limit int) ([]store.AuditRow, error) {
	var out []store.AuditRow
	for _, r := range f.rows {
		if r.ID > afterID {
			out = append(out, r)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeStore) MarkAuditSent(_ context.Context, _ string, lastID int64) error {
	f.sentTo = lastID
	return nil
}

func (f *fakeStore) MarkAuditSinkError(_ context.Context, _, msg string) error {
	f.errNoted = msg
	return nil
}

func filas(n int) []store.AuditRow {
	out := make([]store.AuditRow, n)
	for i := range out {
		out[i] = store.AuditRow{
			ID: int64(i + 1), OccurredAt: time.Unix(int64(1700000000+i), 0).UTC(),
			Action: "user.login", Detail: []byte(`{"ok":true}`),
		}
	}
	return out
}

// El SIEM tiene que recibir los eventos EN ORDEN y con su id, que es por lo que
// deduplica cuando un lote se repite. Y el cursor solo avanza tras un 2xx.
func TestDispatchEntregaEnOrdenYAvanzaElCursor(t *testing.T) {
	var recibidos []int64
	var firma, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var b batch
		if err := json.Unmarshal(body, &b); err != nil {
			t.Errorf("cuerpo ilegible: %v", err)
		}
		for _, e := range b.Events {
			recibidos = append(recibidos, e.ID)
		}
		firma, auth = r.Header.Get("X-Sytue-Signature"), r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs := &fakeStore{
		rows: filas(3),
		sinks: []store.AuditSink{{
			OrgID: "org-1", URL: srv.URL, Enabled: true,
			AuthHeader: "Splunk t0ken", SigningSecret: "s3creto",
		}},
	}
	if err := newWithClient(fs, srv.Client()).DispatchAll(context.Background()); err != nil {
		t.Fatalf("DispatchAll: %v", err)
	}
	if len(recibidos) != 3 || recibidos[0] != 1 || recibidos[2] != 3 {
		t.Fatalf("los eventos deben llegar en orden ascendente: %v", recibidos)
	}
	if fs.sentTo != 3 {
		t.Fatalf("cursor = %d, quiero 3 (el último confirmado)", fs.sentTo)
	}
	if auth != "Splunk t0ken" {
		t.Fatalf("la cabecera de autorización del SIEM no viajó: %q", auth)
	}
	if firma == "" {
		t.Fatal("el lote debe ir firmado")
	}
}

// Si el SIEM está caído, el cursor NO se mueve: es preferible repetir un evento a
// perderlo, y el admin tiene que poder ver POR QUÉ no le llega nada.
func TestDispatchNoAvanzaElCursorSiElSIEMFalla(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "token inválido", http.StatusUnauthorized)
	}))
	defer srv.Close()

	fs := &fakeStore{
		rows:  filas(2),
		sinks: []store.AuditSink{{OrgID: "org-1", URL: srv.URL, Enabled: true, LastSentID: 0}},
	}
	if err := newWithClient(fs, srv.Client()).DispatchAll(context.Background()); err != nil {
		t.Fatalf("un SIEM caído no puede romper el barrido: %v", err)
	}
	if fs.sentTo != 0 {
		t.Fatalf("el cursor no puede avanzar con un fallo: quedó en %d", fs.sentTo)
	}
	if fs.errNoted == "" {
		t.Fatal("el fallo debe quedar anotado para que el admin lo vea")
	}
}

// La firma tiene que ser verificable por el receptor con el secreto compartido: si no,
// no puede distinguir un lote de Sytue de uno que le mande cualquiera.
func TestFirmaVerificable(t *testing.T) {
	t.Parallel()
	body := []byte(`{"events":[]}`)
	const secreto = "s3creto"
	m := hmac.New(sha256.New, []byte(secreto))
	m.Write(body)
	if want := hex.EncodeToString(m.Sum(nil)); sign(body, secreto) != want {
		t.Fatalf("firma = %q, quiero %q", sign(body, secreto), want)
	}
}

// Una organización con problemas no puede dejar sin auditoría a las demás.
func TestDispatchSigueTrasUnFalloDeOrganizacion(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()

	fs := &fakeStore{
		rows: filas(1),
		sinks: []store.AuditSink{
			{OrgID: "rota", URL: "http://127.0.0.1:1/nope", Enabled: true},
			{OrgID: "sana", URL: ok.URL, Enabled: true},
		},
	}
	if err := newWithClient(fs, ok.Client()).DispatchAll(context.Background()); err != nil {
		t.Fatalf("DispatchAll: %v", err)
	}
	if fs.sentTo != 1 {
		t.Fatalf("la organización sana debía recibir su lote; cursor = %d", fs.sentTo)
	}
}

// Un error al LISTAR sí corta: sin la lista no hay nada que barrer y hay que decirlo.
func TestDispatchPropagaElErrorDeListado(t *testing.T) {
	fs := &fakeStore{listErr: errors.New("base caída")}
	if err := New(fs).DispatchAll(context.Background()); err == nil {
		t.Fatal("se esperaba error")
	}
}
