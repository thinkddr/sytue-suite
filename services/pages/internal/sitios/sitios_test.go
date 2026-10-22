// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sitios

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
)

type mockObjStore struct {
	files map[string][]byte
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
	return nil
}

func (m *mockObjStore) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	b, ok := m.files[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func TestSitios_CabecerasDeSeguridad(t *testing.T) {
	// Verificar que las cabeceras por defecto de httpx tienen COEP require-corp y subdominios en HSTS
	defSec := httpx.DefaultSecurityHeaders()
	if !defSec.HSTSIncludeSubdomains || !defSec.HSTSPreload {
		t.Errorf("DefaultSecurityHeaders debería tener HSTS con subdominios y preload")
	}
	if defSec.CrossOriginEmbedderPolicy != "require-corp" {
		t.Errorf("DefaultSecurityHeaders debería tener COEP require-corp")
	}

	obj := newMockObjStore()
	lector := mapa.NuevoLector(obj, time.Minute)
	_ = lector.Refrescar(context.Background())

	cfg := config.Sitios{
		SitesDomain: "sites.sytue.localhost",
		CastOrigin:  "https://cast.sytue.localhost",
	}

	handler := NuevoRouter("v1.0", cfg, lector, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	hsts := rec.Header().Get("Strict-Transport-Security")
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("HSTS = %q; want max-age=31536000", hsts)
	}
	if strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS en sitios NO debe incluir includeSubDomains (§8.1)")
	}
	if strings.Contains(hsts, "preload") {
		t.Errorf("HSTS en sitios NO debe incluir preload (§8.1)")
	}

	coep := rec.Header().Get("Cross-Origin-Embedder-Policy")
	if coep != "" {
		t.Errorf("COEP en sitios debe estar vacía para no romper iframes (got %q)", coep)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-src https://www.openstreetmap.org https://cast.sytue.localhost") {
		t.Errorf("CSP frame-src no contiene fuentes requeridas: %q", csp)
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP frame-ancestors debe ser none: %q", csp)
	}

	ref := rec.Header().Get("Referrer-Policy")
	if ref != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q; want strict-origin-when-cross-origin", ref)
	}

	perm := rec.Header().Get("Permissions-Policy")
	if perm != "camera=(), microphone=(), geolocation=(), payment=()" {
		t.Errorf("Permissions-Policy = %q", perm)
	}

	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie nunca debe emitirse en respuestas de sitio")
	}
}

func TestSitios_SondasConXForwardedFor(t *testing.T) {
	obj := newMockObjStore()
	lector := mapa.NuevoLector(obj, time.Minute)
	_ = lector.Refrescar(context.Background())

	handler := NuevoRouter("v1.0", config.Sitios{}, lector, nil)

	// Sonda con X-Forwarded-For debe responder 404 (§8.1)
	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/healthz con X-Forwarded-For = %d; want 404", rec.Code)
	}

	// Sonda directa sin X-Forwarded-For debe responder 200
	reqOk := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	recOk := httptest.NewRecorder()
	handler.ServeHTTP(recOk, reqOk)
	if recOk.Code != http.StatusOK {
		t.Errorf("/healthz directo = %d; want 200", recOk.Code)
	}
}

func TestSitios_HealthzInformaVersionYCommitAlMonitor(t *testing.T) {
	handler := NuevoRouter("v3.9.7", config.Sitios{}, nil, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("el monitor necesita HTTP 200 y JSON: %d %s (%s)", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta ilegible para el monitor: %v (%s)", err, rec.Body)
	}
	if body.Status != "ok" || body.Version != "v3.9.7" {
		t.Errorf("estado/versión del proceso = %+v", body)
	}
	if want := os.Getenv("SYTUE_BUILD_COMMIT"); want != "" && body.Commit != want {
		t.Errorf("commit = %q; want %q", body.Commit, want)
	}
}

func TestSitios_MetodosYPrefijosReservados(t *testing.T) {
	obj := newMockObjStore()
	lector := mapa.NuevoLector(obj, time.Minute)
	_ = lector.Refrescar(context.Background())

	handler := NuevoRouter("v1.0", config.Sitios{}, lector, nil)

	// POST en servidor de sitios da 405 (§8.1)
	reqPost := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	reqPost.Host = "ejemplo.sites.sytue.localhost"
	recPost := httptest.NewRecorder()
	handler.ServeHTTP(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d; want 405", recPost.Code)
	}

	// Prefijos reservados /_pages y /_sytue dan 404 (§8.1)
	for _, p := range []string{"/_pages/algo", "/_sytue/login"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		req.Host = "ejemplo.sites.sytue.localhost"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d; want 404", p, rec.Code)
		}
	}

	// Path traversal o caracteres inválidos dan 404
	for _, p := range []string{"/../privado", "/doble//barra"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		req.Host = "ejemplo.sites.sytue.localhost"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d; want 404", p, rec.Code)
		}
	}
}

func TestSitios_CicloCompletoSitio(t *testing.T) {
	ctx := context.Background()
	obj := newMockObjStore()

	siteIDLive := uuid.New().String()
	siteIDUnpub := uuid.New().String()
	siteIDSusp := uuid.New().String()

	tokenPreview := "secreto1234567890abcdef"
	hToken := sha256.Sum256([]byte(tokenPreview))
	hashPreview := hex.EncodeToString(hToken[:16])

	// Subir mapa a S3
	mapaDoc := mapa.HostsMapa{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		Sites: map[string]mapa.SiteMapa{
			siteIDLive: {
				Tenant:          "acme",
				Slug:            "panaderia",
				Number:          2,
				Status:          "live",
				Canonical:       "www.panaderia.es",
				Languages:       []string{"es", "en"},
				DefaultLanguage: "es",
				Previews: map[string]string{
					hashPreview: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
			siteIDUnpub: {
				Tenant:          "beta",
				Slug:            "construccion",
				Number:          0,
				Status:          "unpublished",
				Canonical:       "construccion.sites.sytue.localhost",
				Languages:       []string{"es"},
				DefaultLanguage: "es",
				Previews: map[string]string{
					hashPreview: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
			siteIDSusp: {
				Tenant:          "gamma",
				Slug:            "spam",
				Number:          1,
				Status:          "suspended",
				Canonical:       "spam.sites.sytue.localhost",
				Languages:       []string{"es"},
				DefaultLanguage: "es",
				Previews: map[string]string{
					hashPreview: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
		},
		Hosts: map[string]string{
			"panaderia.sites.sytue.localhost":    siteIDLive,
			"www.panaderia.es":                   siteIDLive,
			"construccion.sites.sytue.localhost": siteIDUnpub,
			"spam.sites.sytue.localhost":         siteIDSusp,
		},
	}
	mBytes, _ := json.Marshal(mapaDoc)
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, mapa.ClaveHostsJSON, bytes.NewReader(mBytes), int64(len(mBytes)), objectstore.PutOptions{})

	// Subir contenido simulado
	// Página /es/index.html
	claveIndexES := "shared/sitios/" + siteIDLive + "/v/2/es/index.html"
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, claveIndexES, strings.NewReader("<html><body>Inicio ES</body></html>"), 38, objectstore.PutOptions{ContentType: "text/html"})

	// Página /en/index.html
	claveIndexEN := "shared/sitios/" + siteIDLive + "/v/2/en/index.html"
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, claveIndexEN, strings.NewReader("<html><body>Home EN</body></html>"), 35, objectstore.PutOptions{ContentType: "text/html"})

	// Recurso _astro/logo.png
	claveRecurso := "shared/sitios/" + siteIDLive + "/recursos/logo.png"
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, claveRecurso, strings.NewReader("pngbytes"), 8, objectstore.PutOptions{ContentType: "image/png"})

	// Recurso PDF
	clavePDF := "shared/sitios/" + siteIDLive + "/recursos/ficha.pdf"
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, clavePDF, strings.NewReader("%PDF-1.4..."), 11, objectstore.PutOptions{ContentType: "application/pdf"})

	// Vista previa
	claveVista := "shared/sitios/" + siteIDLive + "/vistas/" + hashPreview + "/index.html"
	_ = obj.Put(ctx, tenancy.Tenant{Slug: "shared"}, claveVista, strings.NewReader("<html><body>Vista previa</body></html>"), 39, objectstore.PutOptions{ContentType: "text/html"})

	lector := mapa.NuevoLector(obj, time.Minute)
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatalf("Refrescar() error = %v", err)
	}

	cfg := config.Sitios{SitesDomain: "sites.sytue.localhost"}
	handler := NuevoRouter("v1.0", cfg, lector, nil)

	// 1. Sitio suspendido da 503 con Retry-After ANTES de cualquier regla (§8.1)
	reqSusp := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqSusp.Host = "spam.sites.sytue.localhost"
	recSusp := httptest.NewRecorder()
	handler.ServeHTTP(recSusp, reqSusp)
	if recSusp.Code != http.StatusServiceUnavailable {
		t.Errorf("sitio suspendido = %d; want 503", recSusp.Code)
	}
	if recSusp.Header().Get("Retry-After") != "3600" {
		t.Errorf("Retry-After = %q; want 3600", recSusp.Header().Get("Retry-After"))
	}
	if !strings.Contains(recSusp.Body.String(), "Esta web no está disponible") {
		t.Errorf("cuerpo de suspendido no contiene texto esperado: %s", recSusp.Body.String())
	}

	// 2. Vista previa en sitio suspendido TAMBIÉN da 503
	reqSuspVista := httptest.NewRequest(http.MethodGet, "/_vista/"+tokenPreview+"/", http.NoBody)
	reqSuspVista.Host = "spam.sites.sytue.localhost"
	recSuspVista := httptest.NewRecorder()
	handler.ServeHTTP(recSuspVista, reqSuspVista)
	if recSuspVista.Code != http.StatusServiceUnavailable {
		t.Errorf("vista previa en suspendido = %d; want 503", recSuspVista.Code)
	}

	// 3. Sitio unpublished da 404
	reqUnpub := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqUnpub.Host = "construccion.sites.sytue.localhost"
	recUnpub := httptest.NewRecorder()
	handler.ServeHTTP(recUnpub, reqUnpub)
	if recUnpub.Code != http.StatusNotFound {
		t.Errorf("sitio unpublished = %d; want 404", recUnpub.Code)
	}
	if !strings.Contains(recUnpub.Body.String(), "Esta web aún no está publicada") {
		t.Errorf("cuerpo de unpublished no contiene texto esperado: %s", recUnpub.Body.String())
	}

	// 4. Host no canónico redirige 301 al canónico
	reqRedir := httptest.NewRequest(http.MethodGet, "/es/", http.NoBody)
	reqRedir.Host = "panaderia.sites.sytue.localhost"
	recRedir := httptest.NewRecorder()
	handler.ServeHTTP(recRedir, reqRedir)
	if recRedir.Code != http.StatusMovedPermanently {
		t.Errorf("host no canónico = %d; want 301", recRedir.Code)
	}
	if recRedir.Header().Get("Location") != "https://www.panaderia.es/es/" {
		t.Errorf("Location = %q; want https://www.panaderia.es/es/", recRedir.Header().Get("Location"))
	}

	// 5. Raíz / redirige 302 con negociación de idioma
	reqRaiz := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqRaiz.Host = "www.panaderia.es"
	reqRaiz.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")
	recRaiz := httptest.NewRecorder()
	handler.ServeHTTP(recRaiz, reqRaiz)
	if recRaiz.Code != http.StatusFound {
		t.Errorf("raíz = %d; want 302", recRaiz.Code)
	}
	if recRaiz.Header().Get("Location") != "/en/" {
		t.Errorf("Location raíz = %q; want /en/", recRaiz.Header().Get("Location"))
	}
	if recRaiz.Header().Get("Vary") != "Accept-Language" {
		t.Errorf("Vary = %q; want Accept-Language", recRaiz.Header().Get("Vary"))
	}

	// 6. Sin barra final en subruta redirige 301 con barra (§8.1)
	reqSinBarra := httptest.NewRequest(http.MethodGet, "/es/productos", http.NoBody)
	reqSinBarra.Host = "www.panaderia.es"
	recSinBarra := httptest.NewRecorder()
	handler.ServeHTTP(recSinBarra, reqSinBarra)
	if recSinBarra.Code != http.StatusMovedPermanently {
		t.Errorf("sin barra = %d; want 301", recSinBarra.Code)
	}
	if recSinBarra.Header().Get("Location") != "/es/productos/" {
		t.Errorf("Location sin barra = %q; want /es/productos/", recSinBarra.Header().Get("Location"))
	}
}

// WEB-09. Con el dominio de clientes puesto, la dirección vieja de una web, bajo el
// dominio de la suite, redirige a la nueva y no sirve nada (ni una vista previa), aunque
// el mapa todavía la tenga.
func TestSitios_LaDireccionViejaSoloRedirige(t *testing.T) {
	obj := newMockObjStore()
	id := uuid.New().String()
	doc, err := json.Marshal(mapa.HostsMapa{
		Version: 1, GeneratedAt: time.Now().UTC(),
		Sites: map[string]mapa.SiteMapa{id: {
			Tenant: "acme", Slug: "tienda", Number: 1, Status: "live",
			Canonical: "tienda.sites.sytue.eu", Languages: []string{"es"}, DefaultLanguage: "es",
		}},
		Hosts: map[string]string{"tienda.sites.sytue.eu": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	obj.files[mapa.ClaveHostsJSON] = doc
	lector := mapa.NuevoLector(obj, time.Minute)
	if err := lector.Refrescar(context.Background()); err != nil {
		t.Fatal(err)
	}
	pedir := func(cfg config.Sitios, host, ruta string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		req.Host = host
		rec := httptest.NewRecorder()
		NuevoRouter("v1.0", cfg, lector, nil).ServeHTTP(rec, req)
		return rec
	}

	if rec := pedir(config.Sitios{SitesDomain: "sites.sytue.eu"}, "tienda.sites.sytue.eu", "/es/"); rec.Code == http.StatusMovedPermanently {
		t.Fatal("sin dominio de clientes la dirección de hoy redirige")
	}
	clientes := config.Sitios{SitesDomain: "sites.clientes.example", SitesDomainAnterior: "sites.sytue.eu"}
	for ruta, destino := range map[string]string{
		"/es/?a=1":         "https://tienda.sites.clientes.example/es/?a=1",
		"/_vista/secreto/": "https://tienda.sites.clientes.example/_vista/secreto/",
	} {
		rec := pedir(clientes, "tienda.sites.sytue.eu", ruta)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != destino {
			t.Errorf("GET %s en la dirección vieja: %d → %q; quiero 301 → %q", ruta, rec.Code, rec.Header().Get("Location"), destino)
		}
	}
	if rec := pedir(clientes, "a.b.sites.sytue.eu", "/"); rec.Code != http.StatusNotFound {
		t.Errorf("un host de dos niveles bajo la zona vieja: %d; quiero 404", rec.Code)
	}
}
