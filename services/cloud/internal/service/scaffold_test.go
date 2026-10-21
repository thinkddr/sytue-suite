// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/forge"
)

// forjaDeMentira responde como Sytue Git a las tres llamadas del alta y anota
// cuántos repositorios se pidieron. Ese recuento es la aserción que importa en el
// caso de reintento: crear dos repositorios por recargar una pestaña es basura en
// la cuenta de alguien.
type forjaDeMentira struct {
	*httptest.Server
	creados int
}

func nuevaForja(t *testing.T) *forjaDeMentira {
	t.Helper()
	f := &forjaDeMentira{}
	f.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/login/oauth/access_token"):
			_, _ = fmt.Fprint(w, `{"access_token":"tok_de_ana"}`)
		case strings.HasSuffix(r.URL.Path, "/api/v1/user"):
			_, _ = fmt.Fprint(w, `{"login":"ana"}`)
		case strings.HasSuffix(r.URL.Path, "/generate"):
			f.creados++
			_, _ = fmt.Fprint(w, `{"full_name":"ana/mi-web",
				"clone_url":"https://git.sytue.eu/ana/mi-web.git",
				"html_url":"https://git.sytue.eu/ana/mi-web"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// servicioConForja monta un Service con caché real (miniredis) y la forja de
// mentira. Sin sellador a propósito: el alta se para justo DESPUÉS de crear el
// repositorio, con un error reconocible, y así se puede afirmar que llegó hasta
// ahí sin necesitar PostgreSQL.
func servicioConForja(t *testing.T, f *forjaDeMentira) *Service {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// El servidor de prueba escucha en 127.0.0.1 y con su propio certificado, que
	// es justo lo que el cliente de producción rechaza: se le presta el suyo.
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	t.Cleanup(restore)

	cl := forge.New(f.URL, "cloud", "secreto", "https://cloud.sytue.eu/api/forge/callback")
	if cl == nil {
		t.Fatal("el cliente de la forja no se construyó")
	}
	cl.UsarHTTPParaTest(f.Client())

	s := &Service{store: &fakeStore{}}
	s.UsarCache(c)
	s.SetForge(cl)
	return s
}

// plantillaConEsqueleto instala un catálogo de dos plantillas —una con esqueleto
// y otra sin él— y devuelve el id de la primera.
//
// Va fabricado y no leído del catálogo real a propósito: hoy ninguna plantilla
// declara `starter_repo` (faltan los repositorios esqueleto en la forja), así que
// atar el test al catálogo lo dejaba SALTÁNDOSE entero, que es la forma más
// silenciosa de no probar nada. Y el día que se declaren, este test seguiría
// probando lo mismo en vez de cambiar de significado.
func plantillaConEsqueleto(t *testing.T) string {
	t.Helper()
	base := domain.TemplateManifest{
		Name: "Prueba", Description: "Plantilla de prueba.", Version: "1.0.0",
		License: "MIT", Maintainer: "Sytue", Framework: domain.FrameworkAstro,
	}
	con, sin := base, base
	con.ID, con.StarterRepo = "con-esqueleto", "sytue-starters/astro"
	sin.ID = "sin-esqueleto"
	t.Cleanup(domain.SetStarterTemplatesForTest([]domain.TemplateManifest{con, sin}))
	return con.ID
}

func intencion(id string) ScaffoldIntent {
	return ScaffoldIntent{
		ActorSub: "ana", TemplateID: id, Slug: "mi-web", Name: "Mi web", RepoName: "mi-web",
	}
}

// TestElConsentimientoDeUnoNoSirveAOtro es la puerta CSRF del rodeo por la forja.
// Sin ella, un `state` filtrado dejaría que otra persona completase el alta —y
// creara el repositorio en SU cuenta— dentro de la organización de la víctima.
func TestElConsentimientoDeUnoNoSirveAOtro(t *testing.T) {
	f := nuevaForja(t)
	s := servicioConForja(t, f)
	ctx := tenantCtx(t)
	id := plantillaConEsqueleto(t)

	url, err := s.StartScaffold(ctx, intencion(id))
	if err != nil {
		t.Fatalf("StartScaffold: %v", err)
	}
	state := estadoDe(t, url)

	if _, _, err := s.FinishScaffold(ctx, state, "code", "otro-usuario"); err == nil {
		t.Fatal("se completó el alta con la sesión de otra persona")
	}
	if f.creados != 0 {
		t.Errorf("se creó un repositorio (%d) para una autorización ajena", f.creados)
	}
}

// TestElConsentimientoNoCruzaDeOrganizacion: el mismo usuario puede estar en dos
// organizaciones, y el proyecto tiene que nacer en la que lo pidió.
func TestElConsentimientoNoCruzaDeOrganizacion(t *testing.T) {
	f := nuevaForja(t)
	s := servicioConForja(t, f)
	id := plantillaConEsqueleto(t)

	url, err := s.StartScaffold(tenantCtx(t), intencion(id))
	if err != nil {
		t.Fatalf("StartScaffold: %v", err)
	}
	otra, err := tenancy.New("otra-org")
	if err != nil {
		t.Fatal(err)
	}
	ctxOtra := tenancy.Into(context.Background(), otra)
	if _, _, err := s.FinishScaffold(ctxOtra, estadoDe(t, url), "code", "ana"); err == nil {
		t.Fatal("se completó el alta en otra organización")
	}
	if f.creados != 0 {
		t.Errorf("se creó un repositorio (%d) para otra organización", f.creados)
	}
}

// TestLaAutorizacionEsDeUnSOLOUso. Quien recarga la pestaña del retorno no puede
// acabar con dos repositorios en su cuenta, y un `state` capturado no se puede
// reutilizar mañana.
func TestLaAutorizacionEsDeUnSOLOUso(t *testing.T) {
	f := nuevaForja(t)
	s := servicioConForja(t, f)
	ctx := tenantCtx(t)
	id := plantillaConEsqueleto(t)

	url, err := s.StartScaffold(ctx, intencion(id))
	if err != nil {
		t.Fatalf("StartScaffold: %v", err)
	}
	state := estadoDe(t, url)

	// Primera vuelta: crea el repositorio y se para en el alta, que aquí no tiene
	// cifrador. Ese error concreto es la prueba de que llegó hasta el final.
	_, _, err = s.FinishScaffold(ctx, state, "code", "ana")
	if err == nil || !strings.Contains(err.Error(), "cifrador") {
		t.Fatalf("primera vuelta: %v (se esperaba llegar al alta)", err)
	}
	if f.creados != 1 {
		t.Fatalf("repositorios creados = %d, quiero 1", f.creados)
	}

	// Segunda: el estado ya no existe.
	if _, _, err := s.FinishScaffold(ctx, state, "code", "ana"); err == nil {
		t.Fatal("el estado se pudo usar dos veces")
	}
	if f.creados != 1 {
		t.Errorf("se creó un segundo repositorio al repetir el retorno (%d)", f.creados)
	}
}

// TestSinPlantillaConEsqueletoNoSeEmpieza: mandar a alguien a autorizar permisos
// sobre su forja para descubrir después que no hay nada que copiar es gastarle un
// consentimiento a cambio de nada.
func TestSinPlantillaConEsqueletoNoSeEmpieza(t *testing.T) {
	f := nuevaForja(t)
	s := servicioConForja(t, f)
	ctx := tenantCtx(t)

	conEsqueleto := plantillaConEsqueleto(t)
	if _, err := s.StartScaffold(ctx, intencion("sin-esqueleto")); err == nil {
		t.Error("se empezó el rodeo con una plantilla sin esqueleto")
	}
	if _, err := s.StartScaffold(ctx, intencion("no-existe")); err == nil {
		t.Error("se empezó el rodeo con una plantilla inexistente")
	}
	// Y un slug inválido tampoco pasa: el proyecto no se podría crear al volver.
	mala := intencion(conEsqueleto)
	mala.Slug = "Mi Web"
	if _, err := s.StartScaffold(ctx, mala); err == nil {
		t.Error("se empezó el rodeo con un slug inválido")
	}
}

// estadoDe saca el `state` de la URL de consentimiento.
func estadoDe(t *testing.T, authorizeURL string) string {
	t.Helper()
	_, query, ok := strings.Cut(authorizeURL, "?")
	if !ok {
		t.Fatalf("la URL de autorización no lleva parámetros: %s", authorizeURL)
	}
	for _, par := range strings.Split(query, "&") {
		if clave, valor, _ := strings.Cut(par, "="); clave == "state" {
			return valor
		}
	}
	t.Fatalf("la URL de autorización no lleva state: %s", authorizeURL)
	return ""
}
