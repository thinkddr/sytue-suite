// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mapa

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

type mockStore struct {
	sitios       []store.DatosSitioMapa
	previews     []store.DatosPreviewMapa
	dominios     []store.DatosDominioMapa
	retiredSlugs []store.DatosRetiredSlugMapa
	err          error
}

func (m *mockStore) ObtenerDatosParaMapa(ctx context.Context) ([]store.DatosSitioMapa, []store.DatosPreviewMapa, []store.DatosDominioMapa, []store.DatosRetiredSlugMapa, error) {
	if m.err != nil {
		return nil, nil, nil, nil, m.err
	}
	return m.sitios, m.previews, m.dominios, m.retiredSlugs, nil
}

type mockObjStore struct {
	files    map[string][]byte
	putCount int
}

func newMockObjStore() *mockObjStore {
	return &mockObjStore{files: make(map[string][]byte)}
}

func (m *mockObjStore) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.files[key] = b
	m.putCount++
	return nil
}

func (m *mockObjStore) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	b, ok := m.files[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func TestMapa_EscritorYLector(t *testing.T) {
	ctx := context.Background()
	obj := newMockObjStore()

	siteID1 := uuid.New()
	pubNum1 := 3
	canHost1 := "www.panaderia.es"

	siteID2 := uuid.New() // unpublished

	siteID3 := uuid.New() // suspended
	pubNum3 := 1

	st := &mockStore{
		sitios: []store.DatosSitioMapa{
			{
				ID:              siteID1,
				Tenant:          "acme",
				Slug:            "panaderia",
				Status:          "active",
				PublishedNumber: &pubNum1,
				CanonicalHost:   &canHost1,
				LanguagesRaw:    []byte(`{"default":"es","others":["en"]}`),
			},
			{
				ID:              siteID2,
				Tenant:          "beta",
				Slug:            "taller",
				Status:          "active",
				PublishedNumber: nil,
			},
			{
				ID:              siteID3,
				Tenant:          "gamma",
				Slug:            "malicioso",
				Status:          "suspended",
				PublishedNumber: &pubNum3,
			},
		},
		previews: []store.DatosPreviewMapa{
			{
				SiteID:     siteID1,
				PreviewKey: "abc123def456",
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			},
		},
		dominios: []store.DatosDominioMapa{
			{
				SiteID: siteID1,
				Host:   "panaderia.es",
				Status: "active",
			},
			// El canónico del sitio: vale porque es un dominio ACTIVO suyo.
			{
				SiteID: siteID1,
				Host:   "www.panaderia.es",
				Status: "active",
			},
		},
		retiredSlugs: []store.DatosRetiredSlugMapa{
			{
				SiteID: siteID1,
				Slug:   "panaderia-antigua",
			},
		},
	}

	escritor := NuevoEscritor(st, obj, "sites.sytue.localhost")
	if err := escritor.Regenerar(ctx); err != nil {
		t.Fatalf("Regenerar() error = %v", err)
	}

	if obj.putCount != 1 {
		t.Errorf("putCount = %d; want 1", obj.putCount)
	}

	// Idempotencia: segundo regenerar sin cambios no debe hacer Put
	if err := escritor.Regenerar(ctx); err != nil {
		t.Fatalf("Regenerar() segunda vez error = %v", err)
	}
	if obj.putCount != 1 {
		t.Errorf("putCount tras segundo Regenerar = %d; want 1", obj.putCount)
	}

	// Probar Lector
	lector := NuevoLector(obj, 10*time.Second)
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatalf("Refrescar() error = %v", err)
	}
	if !lector.Listo() {
		t.Errorf("lector no está listo tras Refrescar()")
	}

	// Verificar búsqueda por host
	sID, sm, ok := lector.ObtenerPorHost("www.panaderia.es")
	if !ok || sID != siteID1.String() {
		t.Fatalf("ObtenerPorHost(www.panaderia.es) = %s, %v; want %s", sID, ok, siteID1)
	}
	if sm.Status != "live" {
		t.Errorf("sm.Status = %s; want live", sm.Status)
	}
	if sm.Number != 3 {
		t.Errorf("sm.Number = %d; want 3", sm.Number)
	}
	if sm.Canonical != "www.panaderia.es" {
		t.Errorf("sm.Canonical = %s; want www.panaderia.es", sm.Canonical)
	}
	if len(sm.Previews) != 1 || sm.Previews["abc123def456"] == "" {
		t.Errorf("previews inválidas: %#v", sm.Previews)
	}

	// Verificar slug retirado
	sIDRet, _, okRet := lector.ObtenerPorHost("panaderia-antigua.sites.sytue.localhost")
	if !okRet || sIDRet != siteID1.String() {
		t.Errorf("ObtenerPorHost(panaderia-antigua) = %s, %v; want %s", sIDRet, okRet, siteID1)
	}

	// Verificar sitio suspended
	_, smSusp, okSusp := lector.ObtenerPorHost("malicioso.sites.sytue.localhost")
	if !okSusp || smSusp.Status != "suspended" {
		t.Errorf("smSusp.Status = %s, %v; want suspended", smSusp.Status, okSusp)
	}

	// Verificar sitio unpublished
	_, smUnpub, okUnpub := lector.ObtenerPorHost("taller.sites.sytue.localhost")
	if !okUnpub || smUnpub.Status != "unpublished" {
		t.Errorf("smUnpub.Status = %s, %v; want unpublished", smUnpub.Status, okUnpub)
	}
}

func TestMapa_LectorNotFound(t *testing.T) {
	ctx := context.Background()
	obj := newMockObjStore() // empty, no hosts.json

	lector := NuevoLector(obj, 10*time.Second)
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatalf("Refrescar() error = %v; want nil en NotFound", err)
	}
	if !lector.Listo() {
		t.Errorf("lector debe estar listo con mapa vacío cuando S3 devuelve NotFound")
	}

	_, _, ok := lector.ObtenerPorHost("cualquiera.es")
	if ok {
		t.Errorf("ObtenerPorHost dio true en mapa vacío")
	}
}

// WEB-01. `sites.canonical_host` fue texto libre del navegador: un sitio escribía ahí el
// host de otro y el mapa se lo daba (el sitio más nuevo pisaba al más antiguo). El
// canónico solo vale si es un dominio ACTIVO de ese mismo sitio; lo demás se ignora y
// ningún host cambia de dueño.
func TestElCanonicoDeUnSitioNoSeQuedaConElHostDeOtro(t *testing.T) {
	uno := 1
	victima, atacante := uuid.New(), uuid.New()
	casos := []struct{ nombre, canonico string }{
		{"el host por slug de la víctima", "victima.sites.example.test"},
		{"el dominio activo de la víctima", "www.victima.example"},
		{"un host que no es de nadie", "nadie.example"},
		{"un dominio suyo todavía sin activar", "www.atacante.example"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			canonico := c.canonico
			m := construirMapa(
				[]store.DatosSitioMapa{
					{ID: victima, Tenant: "victima", Slug: "victima", Status: "active", PublishedNumber: &uno},
					// Más nuevo que la víctima: es el orden en el que su canónico pisaba.
					{ID: atacante, Tenant: "atacante", Slug: "atacante", Status: "active", PublishedNumber: &uno, CanonicalHost: &canonico},
				},
				nil,
				[]store.DatosDominioMapa{
					{SiteID: victima, Host: "www.victima.example", Status: "active"},
					{SiteID: atacante, Host: "www.atacante.example", Status: "verified"},
				},
				nil, "sites.example.test")

			for _, host := range []string{"victima.sites.example.test", "www.victima.example"} {
				if got := m.Hosts[host]; got != victima.String() {
					t.Errorf("Hosts[%s] = %s; ha de seguir siendo de la víctima (%s)", host, got, victima)
				}
			}
			if got, ok := m.Hosts["nadie.example"]; ok {
				t.Errorf("un host sin dominio verificado entró en el mapa por un canónico: %s", got)
			}
			if got := m.Sites[atacante.String()].Canonical; got != "atacante.sites.example.test" {
				t.Errorf("canónico del atacante = %q; un canónico sin dominio activo propio no cuenta", got)
			}
		})
	}
}

// Un dominio nunca pisa un host que ya tiene dueño: el slug de un sitio es suyo aunque
// alguna fila de `domains` llegara a nombrarlo.
func TestUnDominioNoPisaElSlugDeOtroSitio(t *testing.T) {
	victima, otro := uuid.New(), uuid.New()
	m := construirMapa(
		[]store.DatosSitioMapa{
			{ID: victima, Tenant: "victima", Slug: "victima", Status: "active"},
			{ID: otro, Tenant: "otro", Slug: "otro", Status: "active"},
		},
		nil,
		[]store.DatosDominioMapa{{SiteID: otro, Host: "victima.sites.example.test", Status: "active"}},
		nil, "sites.example.test")
	if got := m.Hosts["victima.sites.example.test"]; got != victima.String() {
		t.Fatalf("Hosts[victima.sites.example.test] = %s; want %s", got, victima)
	}
}
