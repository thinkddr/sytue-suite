// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la cadena de egress MONTADA (package policy_test): lo que redirect_test.go
// prueba a nivel de función, aquí se comprueba que además está CABLEADO en el cliente
// que reciben los drivers, y que la credencial y el presupuesto de una organización no
// se le entregan a otra.
//
// Qué se rompe si esto falla:
//   - la auth se inyecta salto a salto, así que un redirect mal acotado manda el token
//     de un cliente al SaaS de otro;
//   - el cubo de rate-limit y la fila de auditoría se atribuyen por org:instancia, así
//     que compartirlos mezcla el gasto y la traza de dos organizaciones distintas.
package policy_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// descartar es un logger mudo: estos tests no revisan trazas.
func descartar() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestRedirect_mismoHostReinyectaElBearer establece la PREMISA de todo lo demás: la
// pasarela no pone la cabecera una vez por petición, la pone en cada RoundTrip, así
// que el destino de un redirect recibe el token igual que el original.
//
// Por eso importan las reglas de redirect: sin ellas, «seguir un 302» es «entregar la
// credencial a quien diga el 302». Y es también el caso simétrico que impide arreglar
// esto cortando TODOS los saltos: los proveedores redirigen constantemente.
func TestRedirect_mismoHostReinyectaElBearer(t *testing.T) {
	allowLoopback(t)

	const token = "token-de-la-organizacion"
	vistos := map[string]string{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		vistos[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		if r.URL.Path == "/a" {
			http.Redirect(w, r, "/b", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Bearer: token,
		Limiter: &fakeLimiter{allow: true}, Log: descartar(),
	})
	resp, err := rt.HTTP.Get(srv.URL + "/a")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	mu.Lock()
	defer mu.Unlock()
	if vistos["/a"] != "Bearer "+token {
		t.Fatalf("el original no llevaba el Bearer inyectado: %q", vistos["/a"])
	}
	if vistos["/b"] != "Bearer "+token {
		t.Fatalf("el salto no se siguió o no llevaba el Bearer: %q", vistos["/b"])
	}
}

// TestRedirect_crossHostNoSeSigue comprueba que la regla de same-host está montada en
// el CheckRedirect del cliente, no solo escrita en una función suelta.
//
// El destino (127.0.0.2) pasa la política de IP del test igual que 127.0.0.1, así que
// no lo tapa la barrera SSRF. Y se exige el error CONCRETO: si la regla desapareciera,
// el salto acabaría en un fallo de conexión —también un error, pero otro—, y el test
// caería. Lo que se afirma es qué capa cortó, no que «algo» falló.
func TestRedirect_crossHostNoSeSigue(t *testing.T) {
	allowLoopback(t)

	var salidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		salidas.Add(1)
		_, puerto, err := net.SplitHostPort(r.Host)
		if err != nil {
			t.Errorf("host sin puerto: %q", r.Host)
			return
		}
		u := url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.2", puerto), Path: "/victima"}
		http.Redirect(w, r, u.String(), http.StatusFound)
	}))
	defer srv.Close()

	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Bearer: "token-de-la-organizacion",
		Limiter: &fakeLimiter{allow: true}, Log: descartar(),
	})
	resp, err := rt.HTTP.Get(srv.URL + "/a")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("un redirect a otro host debe cortarse: entregaría el Bearer de esta organización a un destino ajeno")
	}
	if !strings.Contains(err.Error(), "redirect cross-host bloqueado") {
		t.Fatalf("err = %v; se esperaba el corte por same-host, no otro fallo cualquiera", err)
	}
	if n := salidas.Load(); n != 1 {
		t.Errorf("salidas al origen = %d, quiero 1 (el salto no debe llegar a intentarse)", n)
	}
}

// limiterRoto simula un limitador que no responde (Dragonfly caído). Un rate-limit que
// no se puede consultar NO puede degradar a «pasa»: sería justo el momento en el que
// una instancia desbocada se lleva por delante la cuota de toda la organización.
//
// Devuelve Allowed=true JUNTO con el error, y es deliberado: con el valor cero del
// resultado (Allowed=false) este test pasaría igual aunque se borrara la comprobación
// del error, porque lo ataja la comprobación de Allowed que viene justo después. El
// sabotaje me lo enseñó. Así se aísla la rama que se quiere probar: ante un error, el
// resultado NO se mira.
type limiterRoto struct{}

func (limiterRoto) Allow(context.Context, string) (httpx.RateLimitResult, error) {
	return httpx.RateLimitResult{Allowed: true}, errors.New("dragonfly no responde")
}

func TestRateLimit_falloDelLimitadorNoEgresa(t *testing.T) {
	allowLoopback(t)

	var salidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		salidas.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := &fakeSink{}
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Limiter: limiterRoto{},
		Audit: sink, Log: descartar(),
	})
	if _, err := rt.HTTP.Get(srv.URL); err == nil {
		t.Fatal("si el limitador falla, el egress debe fallar cerrado")
	}
	if n := salidas.Load(); n != 0 {
		t.Errorf("salidas = %d, quiero 0: sin consultar el presupuesto no se sale", n)
	}
	if len(sink.records) != 0 {
		t.Errorf("filas de auditoría = %d, quiero 0: no llegó a haber egress", len(sink.records))
	}
}

// limiterCompartido es UN limitador para toda la pasarela (como en producción: uno por
// proceso, compartido entre réplicas por Dragonfly). Registra qué clave pidió cada
// consulta, que es lo único que separa el presupuesto de una organización del de otra.
type limiterCompartido struct {
	mu     sync.Mutex
	claves []string
}

func (l *limiterCompartido) Allow(_ context.Context, key string) (httpx.RateLimitResult, error) {
	l.mu.Lock()
	l.claves = append(l.claves, key)
	l.mu.Unlock()
	return httpx.RateLimitResult{Allowed: true}, nil
}

// TestAislamiento_dosOrganizaciones: dos instancias de organizaciones distintas
// comparten proceso, limitador y sink de auditoría (así es en producción), y aun así
// cada egress sale con SU credencial, contra SU cubo y con SU atribución.
//
// Es el fallo más caro que puede tener este servicio: entregarle a una organización el
// token de otra significa devolverle los datos del SaaS de otra.
func TestAislamiento_dosOrganizaciones(t *testing.T) {
	allowLoopback(t)

	var mu sync.Mutex
	autorizaciones := map[string]string{} // ruta → Authorization recibido
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		autorizaciones[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	limiter := &limiterCompartido{}
	sink := &fakeSink{}
	orgs := []struct{ org, inst, token, ruta string }{
		{"acme", "inst-a", "token-de-acme", "/acme"},
		{"globex", "inst-b", "token-de-globex", "/globex"},
	}
	for _, o := range orgs {
		rt := policy.BuildRuntime(policy.RuntimeParams{
			OrgID: o.org, InstanceID: o.inst, Bearer: o.token,
			Limiter: limiter, Audit: sink, Log: descartar(),
		})
		resp, err := rt.HTTP.Get(srv.URL + o.ruta)
		if err != nil {
			t.Fatalf("GET de %s: %v", o.org, err)
		}
		_ = resp.Body.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	for i, o := range orgs {
		otra := orgs[1-i]
		if got := autorizaciones[o.ruta]; got != "Bearer "+o.token {
			t.Fatalf("%s recibió %q, quiero su propio token", o.org, got)
		}
		if strings.Contains(autorizaciones[o.ruta], otra.token) {
			t.Fatalf("el token de %s viajó en la petición de %s", otra.org, o.org)
		}
		if want := o.org + ":" + o.inst; !slices.Contains(limiter.claves, want) {
			t.Fatalf("claves de rate-limit = %v, falta %q: sin cubo propio una org se come la cuota de la otra", limiter.claves, want)
		}
	}
	if len(sink.records) != 2 {
		t.Fatalf("filas de auditoría = %d, quiero 2", len(sink.records))
	}
	for _, rec := range sink.records {
		if rec.OrgID != "acme" && rec.OrgID != "globex" {
			t.Fatalf("fila auditada sin organización reconocible: %+v", rec)
		}
		if rec.Auth != "REDACTED" {
			t.Fatalf("la auditoría de %s no redactó la auth: %q", rec.OrgID, rec.Auth)
		}
	}
	if sink.records[0].OrgID == sink.records[1].OrgID {
		t.Fatalf("las dos filas se atribuyeron a la misma organización (%s)", sink.records[0].OrgID)
	}
}

// TestDLP_seEvaluaEnCadaSalto: el hook de política se ejecuta dentro del RoundTripper,
// así que un redirect NO es una puerta trasera para esquivarlo. Sin esto, una regla que
// bloquea /admin se saltaría con un 302 desde /publico.
func TestDLP_seEvaluaEnCadaSalto(t *testing.T) {
	allowLoopback(t)

	var salidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		salidas.Add(1)
		if r.URL.Path == "/publico" {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var vistas []string
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Limiter: &fakeLimiter{allow: true},
		DLP: func(req *http.Request) error {
			vistas = append(vistas, req.URL.Path)
			if req.URL.Path == "/admin" {
				return fmt.Errorf("prohibido: %w", policy.ErrBlockedByPolicy)
			}
			return nil
		},
		Log: descartar(),
	})
	if _, err := rt.HTTP.Get(srv.URL + "/publico"); !errors.Is(err, policy.ErrBlockedByPolicy) {
		t.Fatalf("err = %v, quiero ErrBlockedByPolicy en el salto", err)
	}
	if len(vistas) != 2 || vistas[1] != "/admin" {
		t.Fatalf("rutas evaluadas = %v, quiero que el hook viera también el destino del salto", vistas)
	}
	if n := salidas.Load(); n != 1 {
		t.Errorf("salidas = %d, quiero 1: el salto bloqueado no debe llegar al servidor", n)
	}
}

// TestBearer_noViajaEnLaURL: la inyección va en cabecera y NUNCA en la query string.
// Un token en la URL acaba en los logs de acceso del tercero, en su Referer y en el
// historial de cualquier proxy por el que pase.
func TestBearer_noViajaEnLaURL(t *testing.T) {
	allowLoopback(t)

	const token = "token-de-la-organizacion"
	var crudo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crudo = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Bearer: token,
		Limiter: &fakeLimiter{allow: true}, Log: descartar(),
	})
	resp, err := rt.HTTP.Get(srv.URL + "/x?" + url.Values{"q": {"hola"}}.Encode())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if strings.Contains(crudo, token) {
		t.Fatalf("el token apareció en la URL: %q", crudo)
	}
}

// Una URL pre-autenticada (la sesión de subida de OneDrive) no lleva el token de la
// instancia: Graph la rechaza con él y, además, vive en otro host.
func TestSinCredencial_noInyectaElBearer(t *testing.T) {
	allowLoopback(t)
	var cabeceras []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cabeceras = append(cabeceras, r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-1", Bearer: "token-de-la-organizacion",
		Limiter: &fakeLimiter{allow: true}, Log: descartar(),
	})
	for _, ctx := range []context.Context{context.Background(), policy.SinCredencial(context.Background())} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL, http.NoBody)
		resp, err := rt.HTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if len(cabeceras) != 2 || cabeceras[0] != "Bearer token-de-la-organizacion" || cabeceras[1] != "" {
		t.Fatalf("cabeceras = %q", cabeceras)
	}
}
