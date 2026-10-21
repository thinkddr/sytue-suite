// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// fakeTemplateSvc añade el alta desde plantilla al fake de la capa de
// aplicación (que ya cumple cloudService).
type fakeTemplateSvc struct {
	*fakeSvc
	in  *service.ApplyTemplateInput
	out service.TemplateApplied
	err error
}

func (f *fakeTemplateSvc) AppCatalog() (domain.AppCatalog, error) {
	return domain.AppCatalog{}, nil
}

func (f *fakeTemplateSvc) ApplyTemplate(_ context.Context, in service.ApplyTemplateInput) (service.TemplateApplied, string, error) {
	f.in = &in
	return f.out, "scl_webhook_secret", f.err
}

func templateHandlers(f *fakeTemplateSvc) *Handlers {
	return &Handlers{svc: f, cfg: config.Config{WebhookSecret: "s3cr3t"}}
}

func TestCreateProjectFromTemplate(t *testing.T) {
	f := &fakeTemplateSvc{fakeSvc: &fakeSvc{}, out: service.TemplateApplied{
		Project:    domain.Project{Slug: "mi-api"},
		TemplateID: "go-api-postgres",
	}}
	h := templateHandlers(f)
	rr := httptest.NewRecorder()
	body := `{"template_id":" go-api-postgres ","slug":" mi-api ","repo_url":"https://x/y.git",
		"answers":{"ADMIN_EMAIL":"a@b.eu"},"instance_size":"standard"}`
	h.CreateProjectFromTemplate(rr, req(http.MethodPost, "/projects:from-template", "", body))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.in == nil || f.in.TemplateID != "go-api-postgres" || f.in.Slug != "mi-api" {
		t.Fatalf("entrada no normalizada: %+v", f.in)
	}
	if f.in.ActorSub != "user-1" || f.in.Answers["ADMIN_EMAIL"] != "a@b.eu" {
		t.Fatalf("entrada no propagada: %+v", f.in)
	}
	// El humano que crea el proyecto recibe el secreto del webhook una vez.
	if m := decodeEnvelope(t, rr); m["webhook_secret"] != "scl_webhook_secret" {
		t.Fatalf("el humano creador debe ver el secreto: %v", m)
	}
}

// Un token de API no recibe NUNCA el secreto del webhook (misma regla que
// CreateProject: con él, un token de scope write firmaría pushes y escalaría a
// deploy).
func TestCreateProjectFromTemplateHidesSecretFromToken(t *testing.T) {
	f := &fakeTemplateSvc{fakeSvc: &fakeSvc{}}
	h := templateHandlers(f)
	rt := req(http.MethodPost, "/projects:from-template", "", `{"template_id":"go-api","slug":"api"}`)
	rt = rt.WithContext(context.WithValue(rt.Context(), tokenCtxKey{}, &apiCaller{Scope: domain.ScopeDeploy}))
	rr := httptest.NewRecorder()
	h.CreateProjectFromTemplate(rr, rt)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := decodeEnvelope(t, rr)["webhook_secret"]; ok {
		t.Fatalf("un token NUNCA debe recibir el secreto: %s", rr.Body.String())
	}
}

func TestCreateProjectFromTemplateErrores(t *testing.T) {
	cases := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"json inválido", nil, ``, http.StatusBadRequest},
		{"plantilla desconocida", domain.ErrNotFound, `{"template_id":"x"}`, http.StatusNotFound},
		{"answers no declaradas", fmt.Errorf("%w: clave no declarada", domain.ErrInvalidInput), `{"template_id":"x"}`, http.StatusBadRequest},
		{"cuota", domain.ErrQuotaExceeded, `{"template_id":"x"}`, http.StatusPaymentRequired},
		{"sin suite", domain.ErrSuiteRequired, `{"template_id":"x"}`, http.StatusPaymentRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := templateHandlers(&fakeTemplateSvc{fakeSvc: &fakeSvc{}, err: c.err})
			rr := httptest.NewRecorder()
			h.CreateProjectFromTemplate(rr, req(http.MethodPost, "/projects:from-template", "", c.body))
			if rr.Code != c.want {
				t.Fatalf("status=%d; se esperaba %d (%s)", rr.Code, c.want, rr.Body.String())
			}
		})
	}
}

func (f *fakeTemplateSvc) PublicURL() string { return "https://cloud.sytue.localhost" }
