// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analyticshttp_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/services/analytics/internal/config"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
	analyticshttp "git.sytue.eu/sytue/one/services/analytics/internal/http"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

const (
	hostSitio = "sitio.ejemplo.com"
	ipCliente = "203.0.113.44"
	uaHumano  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

type fakeSites map[string]store.Site

func (f fakeSites) SiteByHost(_ context.Context, host string) (store.Site, error) {
	if s, ok := f[host]; ok {
		return s, nil
	}
	return store.Site{}, store.ErrNotFound
}

type fakeSalt []byte

func (f fakeSalt) Today(context.Context) ([]byte, error) { return f, nil }

type fakeSink struct{ eventos []domain.Event }

func (s *fakeSink) Enqueue(e domain.Event) bool {
	s.eventos = append(s.eventos, e)
	return true
}

type colectorOpts struct {
	sink      *fakeSink
	allowlist []string
	rate      float64
	burst     int
	// burstSitio, si no es cero, es la ráfaga del cubo del SITIO (la de IP es burst).
	burstSitio int
}

func nuevoColector(t *testing.T, o colectorOpts) http.Handler {
	t.Helper()
	if o.sink == nil {
		o.sink = &fakeSink{}
	}
	if o.burst == 0 {
		o.burst = 1000
	}
	if o.rate == 0 {
		o.rate = 1000
	}
	return analyticshttp.NewIngestRouter(analyticshttp.IngestDeps{
		Cfg: config.Config{
			SiteCacheTTL:   time.Minute,
			RatePerSite:    o.rate,
			BurstPerSite:   cmp.Or(o.burstSitio, o.burst),
			RatePerIP:      o.rate,
			BurstPerIP:     o.burst,
			TrustedProxies: clientip.DefaultTrustedProxies(),
		},
		Sites: fakeSites{hostSitio: {
			ID: "11111111-1111-1111-1111-111111111111", Tenant: "acme",
			Host: hostSitio, EventAllowlist: o.allowlist,
		}},
		Salts: fakeSalt(bytes.Repeat([]byte{7}, store.SaltLen)),
		Sink:  o.sink,
	})
}

// envia manda un beacon y devuelve el código de respuesta.
func envia(t *testing.T, h http.Handler, body any, cabeceras map[string]string) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://"+hostSitio+"/_sytue/e", bytes.NewReader(raw))
	req.Header.Set("User-Agent", uaHumano)
	req.Header.Set("X-Forwarded-For", ipCliente)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range cabeceras {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// Un peer público no es Traefik. Por tanto, aunque el cliente escriba ambas
// cabeceras, su identidad se deriva del socket y no de texto controlable por él.
func TestColectorIgnoraCabecerasDeIPDeClienteNoConfiable(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	raw, err := json.Marshal(map[string]any{"n": "pageview", "p": "/"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://"+hostSitio+"/_sytue/e", bytes.NewReader(raw))
	req.RemoteAddr = "198.51.100.7:43120"
	req.Header.Set("User-Agent", uaHumano)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Forwarded-For", ipCliente)
	req.Header.Set("X-Real-IP", "203.0.113.99")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || len(sink.eventos) != 1 {
		t.Fatalf("respuesta/eventos = %d/%d, se esperaba 204/1", rec.Code, len(sink.eventos))
	}
	want := domain.Visitor(bytes.Repeat([]byte{7}, store.SaltLen), "11111111-1111-1111-1111-111111111111", "198.51.100.7", uaHumano)
	if !bytes.Equal(sink.eventos[0].Visitor, want) {
		t.Fatal("la identidad del visitante aceptó una cabecera falsificable")
	}
}

// GPC y DNT son innegociables y sin interruptor: el servidor los honra también,
// por si alguien esquiva el script.
func TestGPCyDNTNoRegistranNada(t *testing.T) {
	t.Parallel()
	for _, cab := range []map[string]string{
		{"Sec-GPC": "1"},
		{"DNT": "1"},
	} {
		sink := &fakeSink{}
		h := nuevoColector(t, colectorOpts{sink: sink})
		if code := envia(t, h, map[string]any{"n": "pageview", "p": "/"}, cab); code != http.StatusNoContent {
			t.Errorf("con %v se esperaba 204, salió %d", cab, code)
		}
		if len(sink.eventos) != 0 {
			t.Errorf("con %v se registró un evento: %+v", cab, sink.eventos)
		}
	}
	// Sin la señal, el mismo evento SÍ se registra (si no, el test anterior
	// pasaría con un colector roto).
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	envia(t, h, map[string]any{"n": "pageview", "p": "/"}, nil)
	if len(sink.eventos) != 1 {
		t.Fatalf("sin GPC/DNT debía registrarse 1 evento, hubo %d", len(sink.eventos))
	}
}

// Ni la IP ni el user-agent crudos llegan a la fila. El barrido es por REFLEXIÓN
// sobre todos los campos de domain.Event: si mañana alguien añade uno y mete ahí
// la IP «solo para depurar», este test se entera.
func TestElEventoNoLlevaIPNiUserAgent(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	envia(t, h, map[string]any{"n": "pageview", "p": "/precios"}, nil)
	if len(sink.eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hubo %d", len(sink.eventos))
	}
	ev := sink.eventos[0]

	prohibido := []string{ipCliente, uaHumano, "Mozilla", "AppleWebKit", "203.0.113"}
	v := reflect.ValueOf(ev)
	for i := range v.NumField() {
		campo := v.Type().Field(i).Name
		var texto string
		switch f := v.Field(i); f.Kind() {
		case reflect.String:
			texto = f.String()
		case reflect.Slice, reflect.Array:
			texto = string(fmt.Appendf(nil, "%s", f.Interface()))
		default:
			continue
		}
		for _, mal := range prohibido {
			if strings.Contains(texto, mal) {
				t.Errorf("el campo %s del evento contiene %q: %q", campo, mal, texto)
			}
		}
	}
	// Y sí lleva lo que debe: un identificador de 16 bytes derivado de ambos.
	if len(ev.Visitor) != 16 {
		t.Errorf("el identificador mide %d bytes, deben ser 16", len(ev.Visitor))
	}
	if ev.Browser != "Chrome" || ev.OS != "macOS" || ev.Device != "desktop" {
		t.Errorf("del user-agent deben sobrevivir tres enums: %s/%s/%s", ev.Browser, ev.OS, ev.Device)
	}
}

// La query string se cae: el script ya la descarta, y el servidor —que es
// público y recibe lo que sea— vuelve a cortarla.
func TestLaQueryStringSeCae(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	envia(t, h, map[string]any{
		"n": "pageview",
		"p": "/reset?reset_token=secreto&email=alguien@ejemplo.com",
		"r": "https://noticias.ejemplo.org/articulo?utm_source=x",
		"q": map[string]string{
			"utm_source": "boletin",
			"email":      "alguien@ejemplo.com", // no está en la allowlist
			"session_id": "s3cr3t0",             // tampoco
		},
	}, nil)
	if len(sink.eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hubo %d", len(sink.eventos))
	}
	ev := sink.eventos[0]
	if ev.Path != "/reset" {
		t.Errorf("la ruta conserva la query string: %q", ev.Path)
	}
	if ev.Referrer != "noticias.ejemplo.org" {
		t.Errorf("del referente solo debe quedar el host: %q", ev.Referrer)
	}
	if ev.UTM[0] != "boletin" {
		t.Errorf("utm_source = %q", ev.UTM[0])
	}
	for i, v := range ev.UTM {
		if strings.Contains(v, "@") || strings.Contains(v, "s3cr3t0") {
			t.Errorf("UTM[%d] recogió una clave fuera de la allowlist: %q", i, v)
		}
	}
}

func TestBotsSeDescartan(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	code := envia(t, h, map[string]any{"n": "pageview", "p": "/"}, map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	})
	if code != http.StatusNoContent {
		t.Errorf("se esperaba 204 también para un bot (sin oráculo), salió %d", code)
	}
	if len(sink.eventos) != 0 {
		t.Errorf("un bot no se registra ni se marca: %+v", sink.eventos)
	}
}

// El sitio ES el hostname: un Host que no está dado de alta no mide nada, y no
// hay identificador que falsificar para apuntar a otro.
func TestHostDesconocidoNoMide(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink})
	req := httptest.NewRequest(http.MethodPost, "http://otro-sitio.example/_sytue/e",
		strings.NewReader(`{"n":"pageview","p":"/"}`))
	req.Header.Set("User-Agent", uaHumano)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("se esperaba 204 (sin oráculo), salió %d", rec.Code)
	}
	if len(sink.eventos) != 0 {
		t.Errorf("un host no dado de alta no debe medir: %+v", sink.eventos)
	}
}

func TestRechazaCrossSiteYOrigenDiscordante(t *testing.T) {
	t.Parallel()
	for _, cab := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Origin": "https://otro.ejemplo.net"},
	} {
		sink := &fakeSink{}
		h := nuevoColector(t, colectorOpts{sink: sink})
		envia(t, h, map[string]any{"n": "pageview", "p": "/"}, cab)
		if len(sink.eventos) != 0 {
			t.Errorf("con %v no debía registrarse nada: %+v", cab, sink.eventos)
		}
	}
}

func TestEventoNoDeclaradoSeDescarta(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	h := nuevoColector(t, colectorOpts{sink: sink, allowlist: []string{"alta"}})
	envia(t, h, map[string]any{"n": "alta", "p": "/gracias"}, nil)
	envia(t, h, map[string]any{"n": "pepe@correo.com", "p": "/gracias"}, nil)
	if len(sink.eventos) != 1 || sink.eventos[0].Name != "alta" {
		t.Fatalf("solo debía pasar el evento declarado: %+v", sink.eventos)
	}
}

func TestLimiteDeVelocidad(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	// Una recarga deliberadamente lenta evita que -race o una máquina cargada
	// repongan tokens entre peticiones y conviertan esta prueba en temporal.
	h := nuevoColector(t, colectorOpts{sink: sink, rate: 0.01, burst: 2})
	var ultimo int
	for range 5 {
		ultimo = envia(t, h, map[string]any{"n": "pageview", "p": "/"}, nil)
	}
	if ultimo != http.StatusTooManyRequests {
		t.Errorf("pasado el límite se esperaba 429, salió %d", ultimo)
	}
	if len(sink.eventos) > 2 {
		t.Errorf("el límite no frenó las escrituras: %d eventos", len(sink.eventos))
	}
}

// Nada de píxel GET: se cachea, se prefetchea y ensucia las cuentas.
func TestSoloPOST(t *testing.T) {
	t.Parallel()
	h := nuevoColector(t, colectorOpts{})
	req := httptest.NewRequest(http.MethodGet, "http://"+hostSitio+"/_sytue/e", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /_sytue/e = %d, se esperaba 405", rec.Code)
	}
}

// La sonda de Analytics de verify.sh pregunta con el host del propio colector
// (`analytics.<dominio>`), que NO es un sitio medido. Pedía `/_sytue/s.js` y
// esperaba 200, pero a un host sin dar de alta el script se le niega con 404 a
// propósito: el piloto dio «FALLO Analytics (404)» en tres despliegues seguidos
// con el colector sano. Aquí se ejecuta lo que verify.sh comprueba contra el router
// de verdad, para que la sonda y el servicio no vuelvan a separarse sin que se vea.
func TestLaSondaDeVerifyLlegaAlColector(t *testing.T) {
	t.Parallel()
	guion, err := os.ReadFile("../../../../deploy/helm/bin/verify.sh")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`check_url Analytics "https://analytics\.\$DOMAIN(/\S*)" '?(\d+)'?\n`).FindSubmatch(guion)
	if m == nil {
		t.Fatal("verify.sh ya no comprueba Analytics con una ruta y UN código fijos")
	}
	ruta, espera := string(m[1]), string(m[2])
	req := httptest.NewRequest(http.MethodGet, "http://analytics.ejemplo.com"+ruta, http.NoBody)
	rec := httptest.NewRecorder()
	nuevoColector(t, colectorOpts{}).ServeHTTP(rec, req)
	if strconv.Itoa(rec.Code) != espera {
		t.Errorf("GET %s desde un host no medido = %d; verify.sh espera %s", ruta, rec.Code, espera)
	}
	// Un 404 no prueba nada: es lo que responde Traefik cuando falta el ingress.
	if rec.Code == http.StatusNotFound {
		t.Errorf("GET %s = 404: no distingue al colector de un ingress que no existe", ruta)
	}
}

// COM-13. Una IP que ya ha agotado su cubo no gasta el del sitio: si lo gastara, una
// sola IP dejaba al sitio sin medición para todos los demás visitantes.
func TestUnaIPLimitadaNoAgotaElCuboDelSitio(t *testing.T) {
	t.Parallel()
	h := nuevoColector(t, colectorOpts{rate: 0.01, burst: 2, burstSitio: 4})
	desde := func(peer string) int {
		req := httptest.NewRequest(http.MethodPost, "http://"+hostSitio+"/_sytue/e", strings.NewReader(`{"n":"pageview","p":"/"}`))
		req.RemoteAddr = peer
		req.Header.Set("User-Agent", uaHumano)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for range 10 {
		desde("198.51.100.7:40000")
	}
	if c := desde("203.0.113.99:40000"); c == http.StatusTooManyRequests {
		t.Fatal("otro visitante recibe 429: la IP limitada agotó el cubo del sitio")
	}
}
