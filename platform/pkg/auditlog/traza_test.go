// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auditlog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// autenticar hace lo que RequireAuth: claims y tenant en un contexto DERIVADO,
// que el middleware de la traza, por fuera, no ve.
func autenticar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		ctx := authn.Into(r.Context(), authn.Claims{Subject: "ana", Tenant: "acme"})
		ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func routerDePrueba(reg *Registro) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{Traza: reg.Middleware(
		Regla{Metodo: http.MethodGet, Ruta: "/api/v1/files/{id}/content", Accion: "file.download", Param: "id"},
		Regla{Metodo: http.MethodGet, Ruta: "/api/v1/documents/{id}/", Accion: "doc.open", Param: "id"},
		Regla{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	)})
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(autenticar)
		api.Get("/files/{id}/content", func(w http.ResponseWriter, r *http.Request) {
			if chi.URLParam(r, "id") == "no-existe" {
				httpx.Fail(w, r, httpx.ErrNotFound)
				return
			}
			_, _ = w.Write([]byte("bytes"))
		})
		api.Route("/documents/{id}", func(d chi.Router) {
			d.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		})
		api.Get("/export", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		api.Get("/files/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		api.Post("/shares", func(w http.ResponseWriter, r *http.Request) {
			Anotar(r.Context(), "share.create", "f-9", map[string]any{"role": "viewer"})
			w.WriteHeader(http.StatusCreated)
		})
	})
	r.Get("/public/{token}", func(w http.ResponseWriter, r *http.Request) {
		AnotarComo(r.Context(), "acme", "link:l-1", "link.download", "f-7", nil)
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func pedir(t *testing.T, h http.Handler, metodo, ruta string, cabeceras map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, http.NoBody)
	req.Header.Set("Authorization", "Bearer x")
	for k, v := range cabeceras {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

func encolados(reg *Registro) []Evento {
	var out []Evento
	for {
		select {
		case e := <-reg.cola:
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestMiddlewareAnotaLoQueCasaYTerminaBien(t *testing.T) {
	reg := NuevoRegistro(nil, nil)
	h := routerDePrueba(reg)

	pedir(t, h, http.MethodGet, "/api/v1/files/f-1/content", nil)
	// Del mismo fichero por rangos, solo el primer trozo.
	pedir(t, h, http.MethodGet, "/api/v1/files/f-2/content", map[string]string{"Range": "bytes=0-1023"})
	pedir(t, h, http.MethodGet, "/api/v1/files/f-2/content", map[string]string{"Range": "bytes=1024-2047"})
	// Lo que falla no es un acceso; lo que no tiene regla, tampoco.
	pedir(t, h, http.MethodGet, "/api/v1/files/no-existe/content", nil)
	pedir(t, h, http.MethodGet, "/api/v1/files/f-3", nil)
	// Una subruta con barra final en el patrón de chi.
	pedir(t, h, http.MethodGet, "/api/v1/documents/d-1", nil)
	pedir(t, h, http.MethodGet, "/api/v1/export", nil)
	// Desde el handler: el recurso venía en el cuerpo.
	pedir(t, h, http.MethodPost, "/api/v1/shares", nil)
	// Sin sesión, con el tenant que resolvió el enlace.
	pedir(t, h, http.MethodGet, "/public/tok", nil)

	got := encolados(reg)
	resumen := make([]string, 0, len(got))
	for _, e := range got {
		resumen = append(resumen, e.Tenant+"|"+e.Actor+"|"+e.Accion+"|"+e.Objetivo)
		if e.Detalle["request_id"] == nil || e.Detalle["ip"] == nil {
			t.Errorf("%s sin request_id o ip: %v", e.Accion, e.Detalle)
		}
		if e.Momento.IsZero() {
			t.Errorf("%s sin momento", e.Accion)
		}
	}
	quiero := []string{
		"acme|ana|acceso.file.download|f-1",
		"acme|ana|acceso.file.download|f-2",
		"acme|ana|acceso.doc.open|d-1",
		"acme|ana|acceso.export|",
		"acme|ana|acceso.share.create|f-9",
		"acme|link:l-1|acceso.link.download|f-7",
	}
	if strings.Join(resumen, "\n") != strings.Join(quiero, "\n") {
		t.Fatalf("traza:\n%s\nquiero:\n%s", strings.Join(resumen, "\n"), strings.Join(quiero, "\n"))
	}
	if got[4].Detalle["role"] != "viewer" {
		t.Fatalf("el detalle del handler se perdió: %v", got[4].Detalle)
	}
}

func TestSinSesionNoSeAnotaYLaColaLlenaNoBloquea(t *testing.T) {
	reg := NuevoRegistro(nil, nil)
	h := routerDePrueba(reg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export", http.NoBody)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || len(encolados(reg)) != 0 {
		t.Fatalf("sin sesión: %d, %d eventos", rr.Code, len(encolados(reg)))
	}
	// Una cola llena descarta y cuenta, sin bloquear la petición.
	for range tamCola + 10 {
		reg.anotar(Evento{Tenant: "acme", Actor: "ana", Accion: "acceso.x"})
	}
	if n := reg.perdidos.Load(); n != 10 {
		t.Fatalf("perdidos = %d, quiero 10", n)
	}
	// Un tenant inválido o una acción vacía no entran.
	encolados(reg)
	reg.anotar(Evento{Tenant: "NO VALE", Actor: "ana", Accion: "acceso.x"})
	reg.anotar(Evento{Tenant: "acme", Actor: "ana"})
	if len(encolados(reg)) != 0 {
		t.Fatal("se encoló un evento sin tenant válido o sin acción")
	}
	// Anotar fuera del middleware (un worker, un test) no hace nada ni falla.
	Anotar(t.Context(), "x", "y", nil)
	// Un objetivo larguísimo se recorta por runas.
	reg.anotar(Evento{Tenant: "acme", Actor: "ana", Accion: "acceso.x", Objetivo: strings.Repeat("é", 600)})
	if e := encolados(reg); len(e) != 1 || len(e[0].Objetivo) > maxObjetivo || !strings.HasPrefix(e[0].Objetivo, "é") {
		t.Fatalf("objetivo recortado: %d bytes", len(e[0].Objetivo))
	}
}

func TestConsultaSQLParametriza(t *testing.T) {
	sql, args := consultaSQL("acme", Consulta{Action: "Acceso.file_%", Actor: "Ana"}, 11)
	if !strings.Contains(sql, "starts_with(action, $2)") || !strings.Contains(sql, "strpos(actor, $3) > 0") || !strings.Contains(sql, "LIMIT $4") {
		t.Fatalf("consulta: %s", sql)
	}
	// Los valores van como parámetros, tal cual (sin comodines: no hay LIKE).
	if args[1] != "acceso.file_%" || args[2] != "ana" || args[3] != 11 {
		t.Fatalf("args: %#v", args)
	}
}
