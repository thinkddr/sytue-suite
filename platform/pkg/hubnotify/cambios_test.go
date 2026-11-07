// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// conRelojFalso deja el cliente sin red: lo que se enviaría queda en `enviados`, y las
// ventanas abiertas en `cierres`, para cerrarlas a mano.
type relojFalso struct {
	mu       sync.Mutex
	enviados []claveCambio
	cierres  []func()
}

func conRelojFalso(c *Client) *relojFalso {
	f := &relojFalso{}
	c.cambios.enviar = func(k claveCambio) {
		f.mu.Lock()
		f.enviados = append(f.enviados, k)
		f.mu.Unlock()
	}
	c.cambios.tras = func(_ time.Duration, cerrar func()) {
		f.mu.Lock()
		f.cierres = append(f.cierres, cerrar)
		f.mu.Unlock()
	}
	return f
}

// cerrarVentanas cierra las ventanas abiertas hasta ahora (no las que abran al cerrarse).
func (f *relojFalso) cerrarVentanas() {
	f.mu.Lock()
	cs := f.cierres
	f.cierres = nil
	f.mu.Unlock()
	for _, c := range cs {
		c()
	}
}

func (f *relojFalso) vaciar() []claveCambio {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.enviados
	f.enviados = nil
	return out
}

// Una ráfaga dentro de la ventana manda la primera en el acto y UNA más al cerrarla, y
// la siguiente ventana, sin nada dentro, no manda nada. Visto en rojo enviando cada
// AvisarCambio (cinco señales) y sin la de cierre (lo último de la ráfaga se perdía).
func TestLaSenalSeAgrupaPorVentana(t *testing.T) {
	c := New("http://hub.invalid", "s")
	f := conRelojFalso(c)
	ctx := context.Background()
	for range 5 {
		c.AvisarCambio(ctx, "acme", "calendar", "ana")
	}
	c.AvisarCambio(ctx, "acme", "mail", "ana")
	c.AvisarCambio(ctx, "acme", "calendar", "luis")
	if got := f.vaciar(); len(got) != 3 {
		t.Fatalf("dentro de la ventana salen %d señales, quiero 3 (una por persona y producto): %v", len(got), got)
	}
	f.cerrarVentanas()
	if got := f.vaciar(); len(got) != 1 || got[0] != (claveCambio{"acme", "ana", "calendar"}) {
		t.Fatalf("al cerrar la ventana sale %v, quiero solo la de ana/calendar, que tuvo más dentro", got)
	}
	f.cerrarVentanas()
	if got := f.vaciar(); len(got) != 0 {
		t.Fatalf("una ventana sin nada dentro manda %v", got)
	}
	c.cambios.mu.Lock()
	quedan := len(c.cambios.abiertas)
	c.cambios.mu.Unlock()
	if quedan != 0 {
		t.Fatalf("quedan %d ventanas abiertas sin nada que mandar", quedan)
	}
	c.AvisarCambio(ctx, "acme", "calendar", "ana")
	if got := f.vaciar(); len(got) != 1 {
		t.Fatalf("pasada la ventana, la siguiente sale en el acto: %v", got)
	}
}

// Lista cerrada de productos, sin organización no hay a quién, y cada persona una vez.
func TestLaSenalSoloLlevaProductosDeLaLista(t *testing.T) {
	c := New("http://hub.invalid", "s")
	f := conRelojFalso(c)
	ctx := context.Background()
	c.AvisarCambio(ctx, "acme", "avatar", "ana")
	c.AvisarCambio(ctx, "acme", "", "ana")
	c.AvisarCambio(ctx, " ", "mail", "ana")
	c.AvisarCambio(ctx, "acme", "drive", "", " ", PorCorreo(""), "ana", "ana", PorCorreo("Eva@X.es"))
	got := f.vaciar()
	quiero := []claveCambio{{"acme", "ana", "drive"}, {"acme", "email:eva@x.es", "drive"}}
	if !slices.Equal(got, quiero) {
		t.Fatalf("salen %v, quiero %v", got, quiero)
	}
	var nada *Client
	nada.AvisarCambio(ctx, "acme", "drive", "ana") // sin Hub, no-op
}

// TrasEscribir avisa a quien escribe, y a quienes añade el handler, solo tras un 2xx de
// una escritura que no se salta. Visto en rojo quitando la comprobación del código (un
// 400 avisaba) y la del método (un GET avisaba).
func TestTrasEscribirAvisaSoloDeLoQueSeEscribio(t *testing.T) {
	c := New("http://hub.invalid", "s")
	f := conRelojFalso(c)
	saltar := func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/drafts") }
	h := c.TrasEscribir(saltar, "calendar")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("compartido") != "" {
			AvisarTambien(r.Context(), "dueña", PorCorreo("eva@x.es"))
		}
		if p := r.URL.Query().Get("producto"); p != "" {
			CambioEn(r.Context(), p)
		}
		if r.URL.Query().Get("mal") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	pedir := func(metodo, ruta string) []claveCambio {
		f.cerrarVentanas()
		f.cerrarVentanas()
		_ = f.vaciar()
		r := httptest.NewRequest(metodo, ruta, http.NoBody)
		r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "ana", Tenant: "acme"}))
		h.ServeHTTP(httptest.NewRecorder(), r)
		return f.vaciar()
	}
	if got := pedir(http.MethodPost, "/events"); !slices.Equal(got, []claveCambio{{"acme", "ana", "calendar"}}) {
		t.Errorf("POST 201: %v", got)
	}
	if got := pedir(http.MethodGet, "/events"); len(got) != 0 {
		t.Errorf("un GET avisa: %v", got)
	}
	if got := pedir(http.MethodPost, "/events?mal=1&compartido=1"); len(got) != 0 {
		t.Errorf("un 400 avisa: %v", got)
	}
	if got := pedir(http.MethodPut, "/drafts/1"); len(got) != 0 {
		t.Errorf("una ruta que se salta avisa: %v", got)
	}
	quiero := []claveCambio{{"acme", "ana", "calendar"}, {"acme", "dueña", "calendar"}, {"acme", "email:eva@x.es", "calendar"}}
	if got := pedir(http.MethodDelete, "/events/1?compartido=1"); !slices.Equal(got, quiero) {
		t.Errorf("con AvisarTambien: %v, quiero %v", got, quiero)
	}
	if got := pedir(http.MethodPatch, "/doc?producto=sheets"); !slices.Equal(got, []claveCambio{{"acme", "ana", "sheets"}}) {
		t.Errorf("con CambioEn: %v", got)
	}
	// Sin claims (una ruta que no pasó por RequireAuth) no hay a quién.
	r := httptest.NewRequest(http.MethodPost, "/events", http.NoBody)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got := f.vaciar(); len(got) != 0 {
		t.Errorf("sin claims avisa: %v", got)
	}
}

// Lo que llega al Hub es `datos` con {"producto"} y nada más.
func TestLaSenalLlegaAlHubSinNadaMas(t *testing.T) {
	llega := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		if r.URL.Path != "/internal/events" || json.NewDecoder(r.Body).Decode(&cuerpo) != nil {
			t.Errorf("%s: cuerpo ilegible", r.URL.Path)
		}
		llega <- cuerpo
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	New(srv.URL, "s").AvisarCambio(context.Background(), "acme", "photos", "ana")
	select {
	case c := <-llega:
		b, _ := json.Marshal(c)
		if string(b) != `{"data":{"producto":"photos"},"event":"datos","subject":"ana","tenant":"acme"}` {
			t.Fatalf("llega %s", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la señal no llegó al Hub")
	}
}

// La lista cerrada es la de las apps nativas con widget: cada `sytue-<producto>` de un
// FyneApp.toml, salvo la demo y One (el Hub, que escucha los de mail y calendar).
// Visto en rojo quitando "pdf" de la lista: el widget de CertiPDF no se enteraba.
func TestLosProductosDeLaSenalSonLosDeLasApps(t *testing.T) {
	tomls, err := filepath.Glob(filepath.Join("..", "..", "..", "apps", "fyne", "cmd", "*", "FyneApp.toml"))
	if err != nil || len(tomls) < 10 {
		t.Fatalf("no encuentro las apps nativas: %v (%d)", err, len(tomls))
	}
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([a-z-]+)"`)
	var apps []string
	for _, p := range tomls {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m := esquema.FindSubmatch(b)
		if m == nil {
			t.Fatalf("%s sin esquema", p)
		}
		if a := string(m[1]); a != "fyne-demo" && a != "one" {
			apps = append(apps, a)
		}
	}
	slices.Sort(apps)
	if !slices.Equal(apps, ProductosDeDatos()) {
		t.Fatalf("la lista de la señal es %v y las apps con widget son %v", ProductosDeDatos(), apps)
	}
}
