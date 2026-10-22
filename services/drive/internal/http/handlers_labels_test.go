// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// idStub simula la API de taxonomía de Sytue ID (GET /api/v1/sensitivity-labels). Su
// contenido es MUTABLE para poder cambiar la taxonomía a mitad de un test.
type idStub struct {
	*httptest.Server
	body   string
	status int
	// gotAuthz guarda el Authorization que recibió: debe ser el DEL USUARIO.
	gotAuthz string
}

func newIDStub(t *testing.T, body string) *idStub {
	t.Helper()
	s := &idStub{body: body, status: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.gotAuthz = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/sensitivity-labels" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func handlersWithID(svc driveService, idURL string) *Handlers {
	return &Handlers{svc: svc, cfg: config.Config{
		PublicURL: "https://drive.sytue.eu", MaxUploadBytes: 1024, IDBaseURL: idURL,
	}}
}

// labelReq construye una petición autenticada CON cabecera Authorization (la que se
// reenvía a Sytue ID).
func labelReq(method, target, body string, params map[string]string) *http.Request {
	r := mkReq(method, target, body, params)
	r.Header.Set("Authorization", "Bearer token-de-usuario")
	return r
}

const taxonomiaConfidencial = `{"items":[
	{"id":"11111111-1111-1111-1111-111111111111","name":"Interno","rank":10,"block_external_share":false},
	{"id":"22222222-2222-2222-2222-222222222222","name":"Confidencial","rank":30,"block_external_share":true}
]}`

// Etiquetar resuelve la etiqueta contra la TAXONOMÍA: el cliente solo manda el id y el
// servidor pone nombre, nivel y política. Lo que el cliente intente colar se ignora.
func TestSetFileLabelResolvesFromTaxonomy(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	svc := &fakeSvc{}
	h := handlersWithID(svc, stub.URL)

	fileID := uuid.New()
	rec := httptest.NewRecorder()
	// El cliente SOLO manda el id: nombre, nivel y política los pone el servidor.
	body := `{"label_id":"22222222-2222-2222-2222-222222222222"}`
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label", body, map[string]string{"id": fileID.String()}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.labelSet == nil {
		t.Fatal("no se llamó a SetFileLabel")
	}
	got := *svc.labelSet
	if got.ID != "22222222-2222-2222-2222-222222222222" || got.Name != "Confidencial" ||
		got.Rank != 30 || !got.BlockExternalShare {
		t.Fatalf("el snapshot no viene de la taxonomía: %+v", got)
	}
	if svc.labelPerm != domain.PermWrite {
		t.Fatalf("etiquetar debe exigir permiso de escritura, got %q", svc.labelPerm)
	}
	if stub.gotAuthz != "Bearer token-de-usuario" {
		t.Fatalf("no se reenvió el Bearer del usuario a Sytue ID: %q", stub.gotAuthz)
	}
}

// Intentar colar la POLÍTICA en el cuerpo (nombre bonito, block_external_share=false)
// se rechaza de plano: el contrato solo acepta label_id, así que no hay forma de
// marcar un fichero como «Confidencial» sin su bloqueo.
func TestSetFileLabelRejectsClientSuppliedPolicy(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	svc := &fakeSvc{}
	h := handlersWithID(svc, stub.URL)

	rec := httptest.NewRecorder()
	body := `{"label_id":"22222222-2222-2222-2222-222222222222","name":"Público","block_external_share":false,"rank":0}`
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label", body, map[string]string{"id": uuid.NewString()}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.labelSet != nil {
		t.Fatal("no debió etiquetarse nada")
	}
}

// Una etiqueta que no está en la taxonomía de la org (inexistente, o de OTRA
// organización) no se puede poner: 400, sin tocar el fichero.
func TestSetFileLabelRejectsUnknownLabel(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	svc := &fakeSvc{}
	h := handlersWithID(svc, stub.URL)

	rec := httptest.NewRecorder()
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label",
		`{"label_id":"99999999-9999-9999-9999-999999999999"}`, map[string]string{"id": uuid.NewString()}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.labelSet != nil {
		t.Fatal("no debió etiquetarse nada")
	}
}

// Sin permiso de escritura sobre el fichero el servicio devuelve ErrPermission ⇒ 403.
func TestSetFileLabelForbiddenWithoutWrite(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	svc := &fakeSvc{labelErr: domain.ErrPermission}
	h := handlersWithID(svc, stub.URL)

	rec := httptest.NewRecorder()
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label",
		`{"label_id":"22222222-2222-2222-2222-222222222222"}`, map[string]string{"id": uuid.NewString()}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Desetiquetar limpia el snapshot y NO consulta la taxonomía (no hace falta).
func TestClearFileLabel(t *testing.T) {
	svc := &fakeSvc{}
	// Sin URL de Sytue ID a propósito: si el DELETE la consultara, fallaría.
	h := handlersWithID(svc, "")

	rec := httptest.NewRecorder()
	h.ClearFileLabel(rec, labelReq(http.MethodDelete, "/api/v1/files/x/label", "", map[string]string{"id": uuid.NewString()}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.labelSet == nil || svc.labelSet.ID != "" {
		t.Fatalf("desetiquetar debe guardar un snapshot vacío: %+v", svc.labelSet)
	}
}

// EL SNAPSHOT SOBREVIVE A UN CAMBIO DE LA TAXONOMÍA: tras etiquetar, el admin cambia
// la etiqueta en Sytue ID (le quita el bloqueo y la renombra) y el fichero conserva lo
// que se le puso. Re-etiquetar es una acción explícita, no un efecto colateral.
func TestLabelSnapshotSurvivesTaxonomyChange(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	svc := &fakeSvc{}
	h := handlersWithID(svc, stub.URL)
	fileID := uuid.New()

	rec := httptest.NewRecorder()
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label",
		`{"label_id":"22222222-2222-2222-2222-222222222222"}`, map[string]string{"id": fileID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("etiquetar: status=%d body=%s", rec.Code, rec.Body.String())
	}
	snapshot := *svc.labelSet

	// El admin relaja la taxonomía: misma etiqueta, ya sin bloqueo y con otro nombre.
	stub.body = `{"items":[{"id":"22222222-2222-2222-2222-222222222222","name":"Interno amplio","rank":5,"block_external_share":false}]}`

	// El fichero (lo que el servicio persistió) NO cambia: sigue Confidencial y sigue
	// bloqueando la compartición externa.
	if snapshot.Name != "Confidencial" || !snapshot.BlockExternalShare || snapshot.Rank != 30 {
		t.Fatalf("el snapshot se alteró con la taxonomía: %+v", snapshot)
	}
	// Y la nueva taxonomía sí se ve al volver a listar (el selector se actualiza).
	rec = httptest.NewRecorder()
	h.ListSensitivityLabels(rec, labelReq(http.MethodGet, "/api/v1/sensitivity-labels", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("listar taxonomía: status=%d", rec.Code)
	}
	var got struct {
		Labels []taxonomyLabel `json:"labels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta: %v (%s)", err, rec.Body.String())
	}
	if len(got.Labels) != 1 || got.Labels[0].Name != "Interno amplio" || got.Labels[0].BlockExternalShare {
		t.Fatalf("la taxonomía nueva no se refleja: %+v", got.Labels)
	}
}

// Si Sytue ID no responde, etiquetar falla en claro (503) en vez de guardar un
// snapshot inventado.
func TestSetFileLabelIDUnavailable(t *testing.T) {
	stub := newIDStub(t, taxonomiaConfidencial)
	stub.status = http.StatusInternalServerError
	svc := &fakeSvc{}
	h := handlersWithID(svc, stub.URL)

	rec := httptest.NewRecorder()
	h.SetFileLabel(rec, labelReq(http.MethodPut, "/api/v1/files/x/label",
		`{"label_id":"22222222-2222-2222-2222-222222222222"}`, map[string]string{"id": uuid.NewString()}))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.labelSet != nil {
		t.Fatal("no debió etiquetarse nada")
	}
}
