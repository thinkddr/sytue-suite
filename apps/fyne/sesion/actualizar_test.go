// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// TestSoloSeInstalaLoPosteriorYConSuSuma: el instalador que toca es el de ESTA app,
// posterior a la que corre y con suma; lo bajado se queda solo si su SHA-256 es la
// publicada, y si no, no queda nada que alguien pueda lanzar. VISTO EN ROJO sin
// comparar la suma (el instalador cambiado se quedaba) y sin exigirla en la elección.
func TestSoloSeInstalaLoPosteriorYConSuSuma(t *testing.T) {
	lista := []api.Instalador{
		{App: "mail", Version: "9.0.0", SHA256: "aa", Arquitectura: "universal"},
		{App: "hub", Version: "3.7.0", Arquitectura: "universal"}, // sin suma
		{App: "hub", Version: "3.6.1", SHA256: "bb", Arquitectura: "universal"},
	}
	if i, ok := elegirActualizacion(lista, "arm64", "hub", "3.6.0"); !ok || i.Version != "3.6.1" {
		t.Errorf("elige %+v, %v", i, ok)
	}
	if _, ok := elegirActualizacion(lista, "arm64", "hub", "3.6.1"); ok {
		t.Error("la misma versión no es una actualización")
	}

	// Con hermanas: se pone la que va por detrás (por la versión que dice, o por la de
	// esta si no dice ninguna), no la que no trae suma ni la que ya está al día, y esta
	// app la ÚLTIMA, que es la que se cierra. VISTO EN ROJO poniendo esta la primera.
	publicado := []api.Instalador{
		{App: "hub", Version: "3.6.0", SHA256: "a", Arquitectura: "universal"},
		{App: "mail", Version: "3.6.0", SHA256: "b", Arquitectura: "universal"},
		{App: "notes", Version: "3.6.0", SHA256: "c", Arquitectura: "universal"},
		{App: "drive", Version: "3.6.0", Arquitectura: "universal"},
		{App: "docus", Version: "3.6.0", SHA256: "d", Arquitectura: "universal"},
	}
	hermanas := []escritorio.Hermana{
		{App: "mail", Destino: "/A/Sytue Mail.app", Version: "3.4.0"},
		{App: "notes", Destino: "/A/Sytue Notes.app"},
		{App: "drive", Destino: "/A/Sytue Drive.app"},
		{App: "docus", Destino: "/A/Sytue Docus.app", Version: "3.6.0"},
	}
	pendientes := pendientesDe(publicado, "arm64", "hub", "3.5.0", hermanas)
	orden := make([]string, 0, len(pendientes))
	for _, p := range pendientes {
		orden = append(orden, p.App+"@"+p.Destino)
	}
	if strings.Join(orden, " ") != "mail@/A/Sytue Mail.app notes@/A/Sytue Notes.app hub@" {
		t.Errorf("se pondrían %q", orden)
	}

	contenido := "el DMG publicado"
	h := sha256.Sum256([]byte(contenido))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, contenido)
	}))
	defer srv.Close()
	hub := api.NewHub(srv.URL, nil, nil)
	dir := t.TempDir()
	inst := api.Instalador{Fichero: "sytue-hub-macos-universal.dmg", SHA256: hex.EncodeToString(h[:]), URL: srv.URL + "/x"}

	ruta, err := bajarComprobado(context.Background(), hub, inst, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(ruta); string(b) != contenido || ruta != filepath.Join(dir, inst.Fichero) {
		t.Errorf("ha quedado %q en %s", b, ruta)
	}

	inst.SHA256 = hex.EncodeToString(make([]byte, 32))
	inst.Fichero = "sytue-hub-windows-amd64-installer.exe"
	if _, err := bajarComprobado(context.Background(), hub, inst, dir); !errors.Is(err, errSuma) {
		t.Errorf("con otra suma: %v", err)
	}
	quedan, _ := filepath.Glob(filepath.Join(dir, "sytue-hub-windows-*"))
	if len(quedan) != 0 {
		t.Errorf("lo que no casa con la suma se ha quedado: %v", quedan)
	}
}

// TestElBundleSeBajaUnaVezParaTodasSusApps: en escritorio se publica un instalador por
// bundle, y el catálogo pone ese mismo fichero en cada app que lleva. Poner al día dos
// hermanas del mismo bundle lo baja UNA vez, aunque ponerlas falle. VISTO EN ROJO
// bajando por cada pendiente (dos peticiones).
func TestElBundleSeBajaUnaVezParaTodasSusApps(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	contenido := "el bundle publicado"
	h := sha256.Sum256([]byte(contenido))
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		peticiones.Add(1)
		_, _ = io.WriteString(w, contenido)
	}))
	defer srv.Close()
	bundle := api.Instalador{Fichero: "sytue-one-linux-amd64.tar.xz", Version: "3.7.0", SHA256: hex.EncodeToString(h[:]), URL: srv.URL + "/b"}
	mail, notes := bundle, bundle
	mail.App, notes.App = "mail", "notes"
	s := &Suite{Hub: api.NewHub(srv.URL, nil, nil)}
	// Los destinos no existen: ponerlas falla, que aquí da igual; lo que se mide es la bajada.
	_ = s.Actualizar(context.Background(), []Pendiente{
		{Instalador: mail, Destino: filepath.Join(cache, "no", "sytue-mail")},
		{Instalador: notes, Destino: filepath.Join(cache, "no", "sytue-notes")},
	})
	if n := peticiones.Load(); n != 1 {
		t.Errorf("el mismo bundle se ha bajado %d veces", n)
	}
}

// Una versión nueva se deja también en la campana, UNA vez por versión aunque la app
// se reinicie (va en las preferencias), y si el Hub falla se vuelve a intentar.
func TestLaActualizacionSeDejaEnLaCampanaUnaVez(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	var avisos atomic.Int32
	falla := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notifications" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"type":"one.update.available"`) || !strings.Contains(string(b), `"version":"3.7.0"`) {
			t.Errorf("aviso sin su tipo o su versión: %s", b)
		}
		if falla.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		avisos.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	s := &Suite{app: a, apex: "sytue.eu", Hub: api.NewHub(srv.URL, nil, nil)}

	falla.Store(true)
	s.avisarEnLaCampana("Sytue Drive", "3.7.0")
	falla.Store(false)
	s.avisarEnLaCampana("Sytue Drive", "3.7.0")
	s.avisarEnLaCampana("Sytue Drive", "3.7.0")
	if n := avisos.Load(); n != 1 {
		t.Fatalf("la misma versión se deja %d veces en la campana, quiero 1", n)
	}
}

// TestLaActualizacionEsLaDeEsteProcesador: se baja el de `runtime.GOARCH` o un universal,
// y si falta el de este procesador NO se cae a otro: para esta máquina no hay
// actualización. Tampoco con uno que no dice su procesador (un Hub anterior). VISTO EN
// ROJO eligiendo por app y versión sin mirar la arquitectura.
func TestLaActualizacionEsLaDeEsteProcesador(t *testing.T) {
	lista := []api.Instalador{
		{App: "one", Fichero: "sytue-one-linux-amd64.tar.xz", Arquitectura: "amd64", Version: "3.7.0", SHA256: "a"},
		{App: "one", Fichero: "sytue-one-linux-arm64.tar.xz", Arquitectura: "arm64", Version: "3.7.0", SHA256: "b"},
		{App: "drive", Fichero: "sytue-drive-linux-amd64.tar.xz", Arquitectura: "amd64", Version: "3.7.0", SHA256: "c"},
		{App: "notes", Fichero: "sytue-notes-linux.tar.xz", Version: "3.7.0", SHA256: "d"},
	}
	for _, c := range []struct{ arq, app, quiero string }{
		{"amd64", "one", "sytue-one-linux-amd64.tar.xz"},
		{"arm64", "one", "sytue-one-linux-arm64.tar.xz"},
		{"amd64", "drive", "sytue-drive-linux-amd64.tar.xz"},
		{"arm64", "drive", ""}, // falta el de ARM: no se cae al de x86_64
		{"amd64", "notes", ""}, // no dice su procesador
	} {
		i, ok := elegirActualizacion(lista, c.arq, c.app, "3.6.0")
		if got := map[bool]string{true: i.Fichero}[ok]; got != c.quiero {
			t.Errorf("%s en %s elige %q; quiero %q", c.app, c.arq, got, c.quiero)
		}
	}
	mac := []api.Instalador{{App: "one", Fichero: "sytue-one-macos-universal.dmg", Arquitectura: "universal", Version: "3.7.0", SHA256: "e"}}
	for _, arq := range []string{"amd64", "arm64"} {
		if _, ok := elegirActualizacion(mac, arq, "one", "3.6.0"); !ok {
			t.Errorf("el universal no le sirve a un Mac %s", arq)
		}
	}
}
