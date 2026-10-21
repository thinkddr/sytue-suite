// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// registroDeMentira habla lo justo de la API v2, con reto Bearer como Scaleway.
type registroDeMentira struct {
	mu        sync.Mutex
	borrados  []string
	etiquetas map[string]string // etiqueta → digest
}

func (r *registroDeMentira) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/token" {
		if u, p, _ := req.BasicAuth(); u != "nologin" || p != "clave" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"token":"tok"}`))
		return
	}
	if req.Header.Get("Authorization") != "Bearer tok" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="http://`+req.Host+`/token",service="registry",scope="repository:sytue/web:*"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case req.URL.Path == "/v2/sytue/web/tags/list":
		var tags []string
		for t := range r.etiquetas {
			tags = append(tags, `"`+t+`"`)
		}
		_, _ = w.Write([]byte(`{"name":"sytue/web","tags":[` + strings.Join(tags, ",") + `]}`))
	case req.Method == http.MethodHead && strings.HasPrefix(req.URL.Path, "/v2/sytue/web/manifests/"):
		d, ok := r.etiquetas[strings.TrimPrefix(req.URL.Path, "/v2/sytue/web/manifests/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Docker-Content-Digest", d)
	case req.Method == http.MethodDelete:
		r.borrados = append(r.borrados, strings.TrimPrefix(req.URL.Path, "/v2/sytue/web/manifests/"))
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestPurgaDelRegistryConRetoBearer(t *testing.T) {
	reg := &registroDeMentira{etiquetas: map[string]string{"abc1234": "sha256:1", "def5678": "sha256:2"}}
	srv := httptest.NewServer(reg)
	defer srv.Close()
	purgar := newRegistryPurger(config.Config{RegistryURL: srv.URL + "/sytue", RegistryUsername: "nologin", RegistryPassword: "clave"})
	if err := purgar(context.Background(), "web"); err != nil {
		t.Fatalf("purga: %v", err)
	}
	if len(reg.borrados) != 2 {
		t.Fatalf("borrados = %v", reg.borrados)
	}
	// Un repositorio que no existe no es un error: no hay nada que borrar.
	if err := purgar(context.Background(), "nada"); err != nil {
		t.Fatalf("repositorio inexistente: %v", err)
	}
}

type purgaStore struct {
	storeAPI
	pend     []store.ProjectToPurge
	marcados []uuid.UUID
}

func (p *purgaStore) ListProjectsToPurge(context.Context, int) ([]store.ProjectToPurge, error) {
	return p.pend, nil
}

func (p *purgaStore) MarkProjectPurged(_ context.Context, id uuid.UUID) error {
	p.marcados = append(p.marcados, id)
	return nil
}

// CLOUD-08: el slug solo se libera si las imágenes se borraron de verdad; si la
// misma organización ya lo recuperó, sus imágenes no se tocan.
func TestPurgaDeProyectosBorrados(t *testing.T) {
	vivo, borrado, roto := uuid.New(), uuid.New(), uuid.New()
	st := &purgaStore{pend: []store.ProjectToPurge{
		{ID: vivo, Slug: "vuelto", SlugVivo: true},
		{ID: borrado, Slug: "web"},
	}}
	var purgados []string
	c := &Controller{store: st, purgeRepo: func(_ context.Context, slug string) error {
		purgados = append(purgados, slug)
		return nil
	}}
	if err := c.reconcilePurge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(purgados) != 1 || purgados[0] != "web" || len(st.marcados) != 2 {
		t.Fatalf("purgados=%v marcados=%v", purgados, st.marcados)
	}

	st = &purgaStore{pend: []store.ProjectToPurge{{ID: roto, Slug: "web"}}}
	c = &Controller{store: st, purgeRepo: func(context.Context, string) error { return errors.New("405") }}
	if err := c.reconcilePurge(context.Background()); err == nil || len(st.marcados) != 0 {
		t.Fatalf("un registry que no borra no puede liberar el slug: err=%v marcados=%v", err, st.marcados)
	}
}
