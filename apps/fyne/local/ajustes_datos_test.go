// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traslado"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/discocifrado"
)

// TestAppsDeLaEdicionSonLasDeCmdLocal: cada app de `cmd-local` está en AppsDeLaEdicion
// con el producto de su esquema, y no sobra ninguna: si faltara una, mover la carpeta de
// datos no miraría si está abierta.
func TestAppsDeLaEdicionSonLasDeCmdLocal(t *testing.T) {
	tomls, err := filepath.Glob(filepath.Join("..", "cmd-local", "*", "FyneApp.toml"))
	if err != nil || len(tomls) == 0 {
		t.Fatal(tomls, err)
	}
	vistas := map[string]bool{}
	for _, f := range tomls {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		app := filepath.Base(filepath.Dir(f))
		vistas[app] = true
		esquema := ""
		for l := range strings.SplitSeq(string(raw), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.TrimSpace(k) == "esquema" {
				esquema = strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
		if quiero := espacio.Esquema(AppsDeLaEdicion[app]); esquema != quiero {
			t.Errorf("cmd-local/%s: esquema %q y AppsDeLaEdicion dice %q", app, esquema, quiero)
		}
	}
	for app := range AppsDeLaEdicion {
		if !vistas[app] {
			t.Errorf("AppsDeLaEdicion tiene %q, que no está en cmd-local", app)
		}
	}
	if NombreDeApp("calcus") != "Calcus" || NombreDeApp("notes") == "notes" {
		t.Errorf("nombres: %q %q", NombreDeApp("calcus"), NombreDeApp("notes"))
	}
}

// appConDatosDePrueba es una app de la edición con su carpeta de ajustes y de datos de prueba.
func appConDatosDePrueba(t *testing.T) (*App, fyne.Window) {
	t.Helper()
	a := test.NewTempApp(t)
	base := t.TempDir()
	e := &espacio.Espacio{Rutas: espacio.Rutas{Ajustes: filepath.Join(base, "ajustes"), Datos: filepath.Join(base, "Sytue Office"), Cache: filepath.Join(base, "cache")}}
	for _, d := range []string{e.Rutas.Ajustes, e.Rutas.Datos} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	l := &App{A: a, Producto: "docs", Espacio: e, Secretos: secretos.SoloFichero(filepath.Join(base, "secretos"))}
	w := a.NewWindow("prueba")
	w.Resize(fyne.NewSize(900, 700))
	w.SetContent(widget.NewLabel(""))
	return l, w
}

// textosEn son los textos de las etiquetas de la ventana y de lo que tiene encima.
func textosEn(w fyne.Window) string {
	var b strings.Builder
	fyne.DoAndWait(func() {
		raices := append([]fyne.CanvasObject{w.Content()}, w.Canvas().Overlays().List()...)
		for _, r := range raices {
			for _, o := range test.LaidOutObjects(r) {
				switch x := o.(type) {
				case *widget.Label:
					b.WriteString(x.Text + "\n")
				case *widget.Button:
					b.WriteString("[" + x.Text + "]\n")
				}
			}
		}
	})
	return b.String()
}

// esperar espera a que la ventana diga algo.
func esperar(t *testing.T, w fyne.Window, algo string) string {
	t.Helper()
	fin := time.Now().Add(15 * time.Second)
	for {
		texto := textosEn(w)
		if strings.Contains(texto, algo) {
			return texto
		}
		if time.Now().After(fin) {
			t.Fatalf("la ventana no dice %q:\n%s", algo, texto)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// El cifrado del disco sale en sus tres estados, con el aviso de X-10 cuando no está
// cifrado o no se sabe, y en el idioma de la app.
func TestElCifradoDelDiscoSeDice(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	for _, caso := range []struct {
		r      discocifrado.Resultado
		quiero string
	}{
		{discocifrado.Resultado{Estado: discocifrado.Cifrado, Como: "LUKS"}, "está cifrado (LUKS)"},
		{discocifrado.Resultado{Estado: discocifrado.SinCifrar}, "no está cifrado: tus datos van en claro y quien acceda al equipo los lee"},
		{discocifrado.Resultado{Estado: discocifrado.NoSeSabe}, "No se puede saber si el disco de esta carpeta está cifrado. Si no lo está, tus datos van en claro y quien acceda al equipo los lee"},
	} {
		if got := l.textoCifrado(caso.r); !strings.Contains(got, caso.quiero) {
			t.Errorf("%+v: %q", caso.r, got)
		}
	}
	// En la pestaña: «mirando…» y, en cuanto se sabe, lo que es.
	listo := make(chan struct{})
	mirarCifrado = func(_ context.Context, ruta string) discocifrado.Resultado {
		<-listo
		if ruta != l.Espacio.Rutas.Datos {
			t.Errorf("mira %s", ruta)
		}
		return discocifrado.Resultado{Estado: discocifrado.SinCifrar}
	}
	t.Cleanup(func() { mirarCifrado = discocifrado.De })
	fyne.DoAndWait(func() { w.SetContent(l.ajustesDatos(w)) })
	esperar(t, w, "Mirando si el disco está cifrado")
	close(listo)
	esperar(t, w, "no está cifrado")
	// En otro idioma, en el suyo.
	t.Setenv("SYTUE_IDIOMA", "de")
	if got := l.textoCifrado(discocifrado.Resultado{Estado: discocifrado.Cifrado, Como: "BitLocker"}); !strings.Contains(got, "verschlüsselt (BitLocker)") {
		t.Errorf("de: %q", got)
	}
}

// Con otra app de la edición abierta no se mueve nada: se dice cuál cerrar. Y una carpeta
// que no vale se dice antes de nada.
func TestMoverDatosPideCerrarLasDemas(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	var preguntadas []string
	antes := abiertasDeLaEdicion
	abiertasDeLaEdicion = func(cs []string) ([]string, bool) {
		preguntadas = cs
		return []string{"notes", "photos"}, true
	}
	t.Cleanup(func() { abiertasDeLaEdicion = antes })
	destino := filepath.Join(t.TempDir(), "nueva")
	fyne.DoAndWait(func() { l.moverDatos(w, destino) })
	esperar(t, w, "cierra estas apps de Sytue Office: Notepads, Photos")
	if !slices.Contains(preguntadas, "docus") || !slices.Contains(preguntadas, "hub") {
		t.Errorf("no pregunta por todas: %v", preguntadas)
	}
	if _, err := os.Stat(destino); !errors.Is(err, os.ErrNotExist) {
		t.Error("ha creado la carpeta nueva")
	}

	l2, w2 := appConDatosDePrueba(t)
	fyne.DoAndWait(func() { l2.moverDatos(w2, l2.Espacio.Rutas.Datos) })
	esperar(t, w2, "Esa ya es tu carpeta de datos")
}

// Mover de verdad: progreso, copia comprobada, perfil apuntado a la nueva, la vieja
// entera y el ofrecimiento de borrarla; borrarla la quita.
func TestMoverLaCarpetaDeDatos(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	origen := l.Espacio.Rutas.Datos
	if err := os.MkdirAll(filepath.Join(origen, "Drive"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origen, "Drive", "acta.txt"), []byte("acta"), 0o600); err != nil {
		t.Fatal(err)
	}
	cerrado := false
	l.AlCerrar(func() { cerrado = true })
	destino := filepath.Join(t.TempDir(), "nueva")
	var fases []traslado.Fase
	moverCarpeta = func(ctx context.Context, o traslado.Opciones) (traslado.Informe, error) {
		progreso := o.Progreso
		o.Progreso = func(p traslado.Progreso) {
			if len(fases) == 0 || fases[len(fases)-1] != p.Fase {
				fases = append(fases, p.Fase)
			}
			progreso(p)
		}
		return traslado.Mover(ctx, o)
	}
	t.Cleanup(func() { moverCarpeta = traslado.Mover })
	fyne.DoAndWait(func() { l.hacerTraslado(w, origen, destino) })
	texto := esperar(t, w, "Tus datos ya están en la carpeta nueva")
	if !strings.Contains(texto, "[Borrar la carpeta de antes]") || !strings.Contains(texto, "[Cerrar la app]") {
		t.Errorf("falta ofrecer borrar o cerrar:\n%s", texto)
	}
	if !cerrado {
		t.Error("no ha cerrado lo que la app tenía abierto antes de mover")
	}
	var p espacio.Perfil
	if err := espacio.LeerJSON(filepath.Join(l.Espacio.Rutas.Ajustes, "perfil.json"), &p); err != nil || p.Datos != destino {
		t.Errorf("perfil: %+v, %v", p, err)
	}
	if raw, err := os.ReadFile(filepath.Join(destino, "Drive", "acta.txt")); err != nil || string(raw) != "acta" {
		t.Errorf("la copia: %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(origen, "Drive", "acta.txt")); err != nil {
		t.Errorf("la vieja se ha tocado sola: %v", err)
	}
	if !slices.Contains(fases, traslado.Copiando) || !slices.Contains(fases, traslado.Comprobando) {
		t.Errorf("fases: %v", fases)
	}
}

// Si falla, nada cambia: el perfil sigue en la de antes y se dice.
func TestMoverLaCarpetaDeDatosSiFalla(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	moverCarpeta = func(context.Context, traslado.Opciones) (traslado.Informe, error) {
		return traslado.Informe{}, fmt.Errorf("%w: Drive/acta.txt", traslado.ErrCopiaMala)
	}
	t.Cleanup(func() { moverCarpeta = traslado.Mover })
	fyne.DoAndWait(func() { l.hacerTraslado(w, l.Espacio.Rutas.Datos, filepath.Join(t.TempDir(), "nueva")) })
	texto := esperar(t, w, "No se ha cambiado nada")
	if !strings.Contains(texto, "La copia no ha salido igual que el original") {
		t.Errorf("no dice por qué:\n%s", texto)
	}
	if _, err := os.Stat(filepath.Join(l.Espacio.Rutas.Ajustes, "perfil.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("ha apuntado algo en el perfil")
	}
}

// En el móvil, el alta OAuth vuelve por el esquema de la app: el enlace que llega por
// SetOnOpenURL va a la vuelta y no a AlAbrirEnlace, y el alta se canjea con PKCE.
func TestLaVueltaDeOAuthNoLlegaALaApp(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	l := &App{A: a, Producto: "drive"}
	l.prepararVuelta(true)
	t.Cleanup(func() { nube.UsarVuelta(nil) })
	if nube.VueltaDeLaApp() == nil || l.vuelta.Redireccion != "sytueoffice-drive:/oauth" {
		t.Fatalf("vuelta: %+v", l.vuelta)
	}
	var enlaces []string
	l.AlAbrirEnlace(func(u *url.URL) { enlaces = append(enlaces, u.String()) })
	l.alArrancar()

	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("redirect_uri") != "sytueoffice-drive:/oauth" || r.Form.Get("code") != "c0d1g0" || r.Form.Get("code_verifier") == "" {
			t.Errorf("canje: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "refresh_token": "r"})
	}))
	defer tokens.Close()
	o := nube.OAuth{Autorizar: "https://login.ejemplo/authorize", Token: tokens.URL, Cliente: "cli"}
	navegador := func(destino string) error {
		d, _ := url.Parse(destino)
		if d.Query().Get("redirect_uri") != "sytueoffice-drive:/oauth" {
			t.Errorf("redirect_uri %q", d.Query().Get("redirect_uri"))
		}
		// El sistema reabre la app con el enlace de vuelta.
		u, _ := url.Parse("sytueoffice-drive:/oauth?code=c0d1g0&state=" + url.QueryEscape(d.Query().Get("state")))
		go l.recibir(u)
		return nil
	}
	got, err := o.Conectar(t.Context(), tokens.Client(), navegador)
	if err != nil || got.Refresco != "r" {
		t.Fatalf("%+v, %v", got, err)
	}
	// Otra vuelta suelta tampoco llega a la app; un enlace de la app, sí.
	for _, s := range []string{"sytueoffice-drive:/oauth?code=otro&state=x", "sytueoffice-drive:/abrir-carpeta?ruta=/"} {
		u, _ := url.Parse(s)
		l.recibir(u)
	}
	fyne.DoAndWait(func() {})
	if len(enlaces) != 1 || enlaces[0] != "sytueoffice-drive:/abrir-carpeta?ruta=/" {
		t.Errorf("enlaces que llegan a la app: %q", enlaces)
	}
}

// En el escritorio no hay vuelta por esquema: sigue 127.0.0.1.
func TestEnElEscritorioNoHayVuelta(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	l := &App{A: a, Producto: "drive"}
	l.prepararVuelta(false)
	if l.vuelta != nil || nube.VueltaDeLaApp() != nil {
		t.Fatal("vuelta en el escritorio")
	}
}
