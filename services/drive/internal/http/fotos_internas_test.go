// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// La copia al Drive de otra persona NO se abre desde fuera: por el ingress (con
// X-Forwarded-For) es 404 y sin el secreto de la suite, 401; el servicio no llega a
// ejecutarse en ninguno de los dos.
func TestLaCopiaDeFotosSoloDesdeDentroYConSecreto(t *testing.T) {
	r := NewRouter(Deps{Cfg: config.Config{PublicURL: "https://drive.sytue.eu", HubInternalSecret: "s3creto"}})
	pedir := func(secreto, xff string) int {
		req := httptest.NewRequest(http.MethodPost, RutaCopiaFotos,
			strings.NewReader(`{"tenant":"acme","file_id":"`+uuid.NewString()+`","owner_sub":"ada","editor_sub":"bob"}`))
		if secreto != "" {
			req.Header.Set("Authorization", "Bearer "+secreto)
		}
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if c := pedir("", ""); c != http.StatusUnauthorized {
		t.Errorf("sin secreto: %d", c)
	}
	if c := pedir("otro", ""); c != http.StatusUnauthorized {
		t.Errorf("con otro secreto: %d", c)
	}
	if c := pedir("s3creto", "203.0.113.7"); c != http.StatusNotFound {
		t.Errorf("por el ingress, con el secreto bueno: %d", c)
	}
}

type copiadorDoble struct {
	tenant, duenya, editora string
	src                     uuid.UUID
}

func (c *copiadorDoble) CopiaParaFotos(ctx context.Context, src uuid.UUID, duenya, editora string) (domain.File, bool, error) {
	t, _ := tenancy.From(ctx)
	c.tenant, c.duenya, c.editora, c.src = t.Slug, duenya, editora, src
	return domain.File{ID: uuid.New(), OwnerSub: duenya}, true, nil
}

// Con todo en regla, copia en la organización del cuerpo y a nombre de quien dice; lo
// que falta es 400.
func TestLaCopiaDeFotosLeeElCuerpo(t *testing.T) {
	c := &copiadorDoble{}
	h := copiaFotos(c)
	src := uuid.New()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, RutaCopiaFotos,
		strings.NewReader(`{"tenant":"acme","file_id":"`+src.String()+`","owner_sub":"ada","editor_sub":"bob"}`)))
	var out struct {
		Reused bool `json:"reused"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || !out.Reused || c.tenant != "acme" || c.duenya != "ada" || c.editora != "bob" || c.src != src {
		t.Fatalf("%d %s %+v", w.Code, w.Body, c)
	}
	for _, malo := range []string{
		`{"tenant":"","file_id":"` + src.String() + `","owner_sub":"ada","editor_sub":"bob"}`,
		`{"tenant":"acme","file_id":"x","owner_sub":"ada"}`, `{"tenant":"acme","file_id":"` + src.String() + `","owner_sub":""}`,
		`{"tenant":"acme","file_id":"` + src.String() + `","owner_sub":"ada"}`,
	} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, RutaCopiaFotos, strings.NewReader(malo)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", malo, w.Code)
		}
	}
}
