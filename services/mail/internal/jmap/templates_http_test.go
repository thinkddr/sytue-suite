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

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// fakePlantillas es lo que el fakeStore recuerda de las plantillas de respuesta.
type fakePlantillas struct {
	lista     []domain.Template
	err       error
	lastID    uuid.UUID
	lastName  string
	lastBody  string
	llamadas  int
	borradoID uuid.UUID
}

func (f *fakeStore) ListTemplates(context.Context, uuid.UUID) ([]domain.Template, error) {
	return f.plantillas.lista, f.plantillas.err
}

func (f *fakeStore) CreateTemplate(_ context.Context, _ uuid.UUID, name, body string) (domain.Template, error) {
	f.plantillas.llamadas++
	f.plantillas.lastName, f.plantillas.lastBody = name, body
	if f.plantillas.err != nil {
		return domain.Template{}, f.plantillas.err
	}
	return domain.Template{ID: uuid.New(), Name: strings.TrimSpace(name), Body: body}, nil
}

func (f *fakeStore) UpdateTemplate(_ context.Context, _, id uuid.UUID, name, body string) (domain.Template, error) {
	f.plantillas.llamadas++
	f.plantillas.lastID, f.plantillas.lastName, f.plantillas.lastBody = id, name, body
	if f.plantillas.err != nil {
		return domain.Template{}, f.plantillas.err
	}
	return domain.Template{ID: id, Name: strings.TrimSpace(name), Body: body}, nil
}

func (f *fakeStore) DeleteTemplate(_ context.Context, _, id uuid.UUID) error {
	f.plantillas.llamadas++
	f.plantillas.borradoID = id
	return f.plantillas.err
}

func TestListTemplatesDevuelveLasDeLaCuenta(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	st.plantillas.lista = []domain.Template{{ID: uuid.New(), Name: "Horario", Body: "De 9 a 14."}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listTemplates(rec, req(http.MethodGet, "/api/v1/templates", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	var got struct {
		Templates []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Templates) != 1 || got.Templates[0].Name != "Horario" || got.Templates[0].Body != "De 9 a 14." {
		t.Fatalf("plantillas = %+v", got.Templates)
	}
}

// Sin ninguna, la lista es [] y no null: el cliente la recorre sin comprobar.
func TestListTemplatesVaciaEsUnaLista(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.listTemplates(rec, req(http.MethodGet, "/api/v1/templates", "", true, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"templates":[]`) {
		t.Fatalf("código = %d, cuerpo = %s; quiero 200 con templates:[]", rec.Code, rec.Body.String())
	}
}

func TestCreateTemplateLlegaAlStoreYDevuelve201(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createTemplate(rec, req(http.MethodPost, "/api/v1/templates",
		`{"name":"Gracias","body":"Gracias por escribir.\nUn saludo"}`, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.plantillas.lastName != "Gracias" || st.plantillas.lastBody != "Gracias por escribir.\nUn saludo" {
		t.Fatalf("al store llegó %q / %q", st.plantillas.lastName, st.plantillas.lastBody)
	}
	var got domain.Template
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ID == uuid.Nil || got.Name != "Gracias" {
		t.Fatalf("respuesta = %s (%v)", rec.Body.String(), err)
	}
}

// Lo inválido se rechaza con un 400 con código propio, y no llega al store.
func TestCreateTemplateRechazaLoInvalido(t *testing.T) {
	largo := strings.Repeat("x", domain.TemplateBodyMaxBytes+1)
	for _, cuerpo := range []string{
		`{"name":"","body":"x"}`,
		`{"name":"   ","body":"x"}`,
		`{"name":"` + strings.Repeat("a", domain.TemplateNameMax+1) + `","body":"x"}`,
		`{"name":"ok","body":"` + largo + `"}`,
	} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		a.createTemplate(rec, req(http.MethodPost, "/api/v1/templates", cuerpo, true, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%.40s…: código = %d, quiero 400", cuerpo, rec.Code)
		}
		if code := decodeEnvelopeCode(t, rec); code != "invalid_template" {
			t.Fatalf("%.40s…: código de error = %q", cuerpo, code)
		}
		if st.plantillas.llamadas != 0 {
			t.Fatalf("%.40s…: lo rechazado llegó al store", cuerpo)
		}
	}
}

func TestCreateTemplateNombreRepetidoYTopeSon409(t *testing.T) {
	for err, code := range map[error]string{
		domain.ErrTemplateExists: "template_exists",
		domain.ErrTemplateLimite: "template_limit",
	} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		st.plantillas.err = err
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		a.createTemplate(rec, req(http.MethodPost, "/api/v1/templates", `{"name":"Horario","body":"x"}`, true, nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%v: código = %d, quiero 409", err, rec.Code)
		}
		if got := decodeEnvelopeCode(t, rec); got != code {
			t.Fatalf("%v: código de error = %q, quiero %q", err, got, code)
		}
	}
}

func TestUpdateTemplateDevuelveLaPlantilla(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	id := uuid.New()
	rec := httptest.NewRecorder()
	a.updateTemplate(rec, req(http.MethodPut, "/api/v1/templates/"+id.String(),
		`{"name":"Horario de verano","body":"De 8 a 15."}`, true, map[string]string{"templateID": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.plantillas.lastID != id || st.plantillas.lastName != "Horario de verano" || st.plantillas.lastBody != "De 8 a 15." {
		t.Fatalf("al store llegó %v %q %q", st.plantillas.lastID, st.plantillas.lastName, st.plantillas.lastBody)
	}
}

func TestUpdateYDeleteTemplateIDInvalidoEs400(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	params := map[string]string{"templateID": "no-es-un-uuid"}
	rec := httptest.NewRecorder()
	a.updateTemplate(rec, req(http.MethodPut, "/api/v1/templates/x", `{"name":"a","body":""}`, true, params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("put: código = %d, quiero 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.deleteTemplate(rec, req(http.MethodDelete, "/api/v1/templates/x", "", true, params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete: código = %d, quiero 400", rec.Code)
	}
	if st.plantillas.llamadas != 0 {
		t.Fatal("un id inválido llegó al store")
	}
}

// La de otra cuenta (o que no existe) es un 404: el store no la encuentra porque
// filtra por la cuenta del token.
func TestTemplateAjenaEs404(t *testing.T) {
	id := uuid.New()
	params := map[string]string{"templateID": id.String()}
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	st.plantillas.err = domain.ErrNotFound
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.updateTemplate(rec, req(http.MethodPut, "/api/v1/templates/"+id.String(), `{"name":"a","body":""}`, true, params))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("put: código = %d, quiero 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.deleteTemplate(rec, req(http.MethodDelete, "/api/v1/templates/"+id.String(), "", true, params))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete: código = %d, quiero 404", rec.Code)
	}
}

func TestDeleteTemplateEs204(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	id := uuid.New()
	rec := httptest.NewRecorder()
	a.deleteTemplate(rec, req(http.MethodDelete, "/api/v1/templates/"+id.String(), "", true,
		map[string]string{"templateID": id.String()}))
	if rec.Code != http.StatusNoContent || st.plantillas.borradoID != id {
		t.Fatalf("código = %d, borrado = %v", rec.Code, st.plantillas.borradoID)
	}
}

func TestTemplatesSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	for nombre, h := range map[string]http.HandlerFunc{
		"list": a.listTemplates, "create": a.createTemplate,
		"update": a.updateTemplate, "delete": a.deleteTemplate,
	} {
		rec := httptest.NewRecorder()
		h(rec, noAuthReq(http.MethodPost, "/api/v1/templates", `{"name":"a","body":""}`))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: código = %d, quiero 401", nombre, rec.Code)
		}
	}
}

// Guardar una plantilla no cambia lo que enseña el widget de Mail: va sin señal,
// como la firma y el resto de ajustes.
func TestTemplatesVanSinSenalDeCambios(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/templates", http.NoBody)
	if !sinSenalDeCambios(r) {
		t.Fatal("/api/v1/templates dispara la señal de cambios del widget")
	}
}
