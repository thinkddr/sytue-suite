// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	temafyne "fyne.io/fyne/v2/theme"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// aparte deja la carpeta compartida y el llavero en un sitio de mentira, para no tocar
// los del usuario que ejecuta las pruebas. `keyring.MockInit` es un mapa en memoria del
// proceso: dos Suites en esta prueba comparten UN llavero, que es justo lo que en la
// máquina de alguien comparten las trece apps.
func aparte(t *testing.T) {
	t.Helper()
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(casa, ".cache")) // cerrar sesión vacía la caché
	keyring.MockInit()
	tema.Elegir("")
}

// Dos apps de la suite, UNA sesión: la segunda encuentra lo que dejó la primera sin
// volver a pasar por el navegador.
func TestLaSesionEsUnaParaLasTrece(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	// Sytue Mail entra.
	mail := Abrir(a, "sytue.eu")
	if mail.Estado().Dentro() {
		t.Fatal("sin nada guardado no puede haber sesión")
	}
	mail.Sesion.Adoptar(auth.Tokens{Access: "a", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})

	// Sytue Drive abre después y ya está dentro: no ha hablado con el IdP.
	drive := Abrir(a, "sytue.eu")
	if e := drive.Estado(); !e.Dentro() || e.Sujeto != "ada" {
		t.Fatalf("Drive tendría que abrir con la sesión de Mail: %+v", e)
	}
}

// Cerrar sesión en una la cierra en las trece: Drive se entera por la marca, sin que
// nadie le diga nada, y en menos de lo que tarda en mirar.
func TestCerrarEnUnaCierraEnLaOtra(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	mail := Abrir(a, "sytue.eu")
	mail.Sesion.Adoptar(auth.Tokens{Access: "a", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})
	drive := Abrir(a, "sytue.eu")
	if !drive.Estado().Dentro() {
		t.Fatal("Drive tendría que estar dentro")
	}

	mail.Salir()

	// Lo que hace Drive es lo mismo que hace su vigilante al ver la marca cambiada.
	drive.aplicar(ajustes.Leer())
	if e := drive.Estado(); e.Dentro() {
		t.Errorf("Drive sigue dentro tras cerrar en Mail: %+v", e)
	}
	if tok, err := auth.Load(); err != nil || tok.Refresh != "" {
		t.Errorf("el llavero conserva %+v (%v)", tok, err)
	}
}

// El tema es transversal y se ve al momento: lo que elige una app lo aplica la otra sin
// reabrirla. Y manda sobre el del sistema, que es lo que Fyne no deja hacer por su
// cuenta (`settings.ThemeVariant()` devuelve el del sistema y `SetTheme` no lo toca).
func TestElTemaSeCambiaEnUnaYLaOtraLoAplica(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	mail, drive := Abrir(a, "sytue.eu"), Abrir(a, "sytue.eu")
	// Ojo al orden de Fyne, que es al revés de lo que uno espera:
	// `theme.VariantDark` es 0 y `VariantLight`, 1.
	if v := tema.De(temafyne.VariantDark); v != tema.Claro {
		t.Fatalf("sin elegir nada quiero el tema claro, tengo %v", v)
	}
	if err := mail.Poner(func(aj *ajustes.Ajustes) { aj.Tema = "oscuro" }); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	// Se le sigue pasando la variante CLARA del sistema: si la elección no mandara,
	// saldría Claro.
	if v := tema.De(temafyne.VariantLight); v != tema.Oscuro {
		t.Errorf("la elección no manda sobre el sistema: %v", v)
	}
	drive.aplicar(ajustes.Leer())
	if e := drive.Estado(); e.Tema != "oscuro" {
		t.Errorf("Drive no se ha enterado: %+v", e)
	}
}

func TestLaEleccionGuardadaDelSistemaMandaAlAbrir(t *testing.T) {
	aparte(t)
	if err := ajustes.Cambiar(func(aj *ajustes.Ajustes) { aj.Tema = "" }); err != nil {
		t.Fatal(err)
	}
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	if e := s.Estado(); e.Tema != "" {
		t.Fatalf("se perdió el tema del sistema: %+v", e)
	}
	if v := tema.De(temafyne.VariantDark); v != tema.Oscuro {
		t.Errorf("la elección del sistema no sigue su variante oscura: %v", v)
	}
}

// El idioma también es transversal, pero se aplica al SIGUIENTE arranque: las pantallas
// reciben su traductor al construirse. Lo que se comprueba aquí es que se guarda y que
// la app que abre después nace con él.
func TestElIdiomaLoRecogeLaQueAbreDespues(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	mail := Abrir(a, "sytue.eu")
	if err := mail.Poner(func(aj *ajustes.Ajustes) { aj.Idioma = "is" }); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	if l := Abrir(a, "sytue.eu").Idioma(); string(l) != "is" {
		t.Errorf("la que abre después nace en %q", l)
	}
}

// Abrir no habla con la red ni abre un navegador: una app tiene que poder arrancar sin
// conexión. Si esto se rompe, las trece tardan en abrir lo que tarde el IdP en contestar.
func TestAbrirNoTocaLaRed(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	if s.oidc.Open == nil {
		t.Fatal("sin forma de abrir el navegador no se podría entrar")
	}
	// Con un ctx ya muerto, Entrar tiene que fallar SIN llegar a abrir nada: es la
	// prueba de que abrir el navegador es cosa de Entrar y no de Abrir.
	ctx, parar := context.WithCancel(context.Background())
	parar()
	abierto := false
	s.oidc.Open = func(string) error { abierto = true; return nil }
	if err := s.Entrar(ctx); err == nil {
		t.Error("Entrar con el contexto muerto tendría que fallar")
	}
	if abierto {
		t.Error("ha abierto el navegador con el contexto ya cancelado")
	}
}

func TestURLDeProductoLlegaAQuienLaRegistro(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	var got string
	s.AlAbrirURL(func(u *url.URL) { got = u.String() })
	// La URL se construye con url.Parse, que es como la entrega el SISTEMA, y no a
	// mano: Parse marca OmitHost y la escribe «sytue-drive:/files/42»; un literal
	// sin ese campo se escribe «sytue-drive:///files/42». Construida a mano, este
	// test probaba una URL que el sistema nunca entrega.
	entrante, err := url.Parse("sytue-drive:/files/42")
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(entrante)
	if got != "sytue-drive:/files/42" {
		t.Fatalf("URL de producto = %q", got)
	}
}

// TestAbrirEnLaAplicacionDeLaWebLlegaAlReceptor: lo que emite «Abrir en la aplicación»
// en la web (`openInApp.ts`) llega al receptor de la app como la PÁGINA que se miraba,
// no como «ábrete». Antes el `url=` no lo leía nadie y la hoja se quedaba en la portada
// de Calcus. VISTO EN ROJO quitando la traducción de `recibirURL`.
func TestAbrirEnLaAplicacionDeLaWebLlegaAlReceptor(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	var got []string
	s.AlAbrirURL(func(u *url.URL) { got = append(got, u.String()) })
	for _, crudo := range []string{
		"sytue-sheets://open?url=https%3A%2F%2Fcalcus.sytue.eu%2Fcalcus%2Fedit%3Fid%3Dh1",
		// Una página que no es de la suite no se traduce: sigue siendo «ábrete».
		"sytue-sheets://open?url=https%3A%2F%2Fsytue.eu.evil.com%2Fcalcus%2Fedit%3Fid%3Dh1",
	} {
		u, err := url.Parse(crudo)
		if err != nil {
			t.Fatal(err)
		}
		s.recibirURL(u)
	}
	if len(got) != 2 || got[0] != "sytue-sheets:/calcus/edit?id=h1" || !esRaiz(enlace(t, got[1])) {
		t.Fatalf("recibido = %q", got)
	}
}

// appEspia apunta a dónde manda la app al sistema. El `test.App` de Fyne se traga
// `OpenURL` sin dejar rastro, así que sin esto el respaldo web no se puede ver.
//
// Con cerrojo y con canal: el respaldo de una carga que falla llega desde la goroutine
// de la carga, así que el test ESPERA a que llegue en vez de mirar una lista a destiempo.
type appEspia struct {
	fyne.App
	mu    sync.Mutex
	urls  []string
	llega chan string
}

func nuevaEspia(a fyne.App) *appEspia { return &appEspia{App: a, llega: make(chan string, 8)} }

func (a *appEspia) OpenURL(u *url.URL) error {
	a.mu.Lock()
	a.urls = append(a.urls, u.String())
	a.mu.Unlock()
	a.llega <- u.String()
	return nil
}

func (a *appEspia) abiertas() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.urls)
}

// nadaEnLaWeb comprueba que en un rato prudencial no se ha abierto el navegador. Solo
// puede dar un falso VERDE, nunca un falso rojo: si algo llega, llega.
func (a *appEspia) nadaEnLaWeb(t *testing.T) {
	t.Helper()
	if u, ok := recibir(a.llega, 200*time.Millisecond); ok {
		t.Errorf("ha abierto %q en el navegador y no debía", u)
	}
}

// esperar devuelve lo que llegue por el canal, o falla a los dos segundos.
func esperar[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	v, ok := recibir(c, 2*time.Second)
	if !ok {
		t.Fatal("no ha llegado en dos segundos")
	}
	return v
}

// recibir espera lo que llegue por c durante un plazo. Mientras, deja pasar lo que las
// rutinas de fondo encolan con `fyne.Do`: el driver de pruebas del fork lo ejecuta en
// la rutina del test, así que sin el `DoAndWait` no llegaría nunca, y un «nada» daría
// un falso verde.
func recibir[T any](c <-chan T, plazo time.Duration) (T, bool) {
	fin := time.Now().Add(plazo)
	for {
		fyne.DoAndWait(func() {})
		select {
		case v := <-c:
			return v, true
		case <-time.After(5 * time.Millisecond):
		}
		if time.Now().After(fin) {
			var cero T
			return cero, false
		}
	}
}

// TestElFicheroEntranteLlegaDesarmado: lo que Drive emite —y que este test construye con
// el emisor de verdad, no con una cadena a mano— llega a la app dueña del formato con la
// referencia ya separada, y NO se toca el navegador.
func TestElFicheroEntranteLlegaDesarmado(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	vistas := make(chan api.Apertura, 1)
	s.AlAbrirFichero("docs", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })

	entrante, err := url.Parse("sytue-docs:/open-from-drive?file=abc-123&volver=%2FDocs")
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(entrante)
	if visto := esperar(t, vistas); visto.Ref.ID != "abc-123" || visto.Volver != "/Docs" {
		t.Errorf("apertura = %+v", visto)
	}
	espia.nadaEnLaWeb(t)
}

// TestSiNoSePuedeCargarSigueEnLaWeb: la app sabe cargar, pero esta vez no ha podido —sin
// sesión, el fichero ya no está, un formato que no entiende—. El enlace no se queda en
// una ventana vacía: va a la misma página en el navegador.
func TestSiNoSePuedeCargarSigueEnLaWeb(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia
	s.AlAbrirFichero("slides", func(context.Context, api.Apertura) error {
		return errors.New("401: no hay sesión")
	})

	entrante, err := url.Parse("sytue-slides:/open-from-drive?file=p-1&volver=%2FEquipo")
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(entrante)
	quiero := "https://diapos.sytue.eu/diapos/open-from-drive?file=p-1&volver=%2FEquipo"
	if got := esperar(t, espia.llega); got != quiero {
		t.Errorf("respaldo = %q, quiero %q", got, quiero)
	}
}

// TestSiLaAppNoSabeCargarloSigueEnLaWeb: el enlace no se pierde. Es el caso de HOY en las
// cinco apps destino, que todavía no piden el documento al servicio.
func TestSiLaAppNoSabeCargarloSigueEnLaWeb(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia
	s.AlAbrirFichero("pdf", nil)

	entrante, err := url.Parse("sytue-pdf:/pdf?path=%2FDocs%2Facta.pdf&volver=%2FDocs")
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(entrante)
	quiero := "https://certipdf.sytue.eu/certipdf/pdf?path=%2FDocs%2Facta.pdf&volver=%2FDocs"
	if got := espia.abiertas(); len(got) != 1 || got[0] != quiero {
		t.Errorf("respaldo = %v, quiero %q", got, quiero)
	}
	<-espia.llega

	// Un enlace que no es de apertura —la vuelta del login, un argumento cualquiera—
	// no manda a nadie al navegador.
	otro, _ := url.Parse("sytue-pdf://auth/callback?code=x")
	s.recibirURL(otro)
	espia.nadaEnLaWeb(t)
}

// TestElEnlaceVaALaInstanciaViva: en Linux y en Windows abrir un esquema EJECUTA el
// programa otra vez. La segunda copia tiene que entregar la URL a la que ya corre y no
// abrir una segunda ventana.
func TestElEnlaceVaALaInstanciaViva(t *testing.T) {
	aparte(t)
	ctx, parar := context.WithCancel(context.Background())
	defer parar()

	llegadas := make(chan string, 2)
	go escuchar(ctx, "eu.sytue.pruebas", func(u *url.URL) { llegadas <- u.String() })
	// El socket se crea en otra goroutine: se espera a que conteste, sin dormir a ojo.
	var vivo bool
	for i := 0; i < 100 && !vivo; i++ {
		vivo = reenviar("eu.sytue.pruebas", []string{"sytue-docs:/open-from-drive?file=abc"})
		if !vivo {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !vivo {
		t.Fatal("la instancia viva no ha cogido el enlace")
	}
	select {
	case got := <-llegadas:
		if got != "sytue-docs:/open-from-drive?file=abc" {
			t.Errorf("ha llegado %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("el enlace no ha llegado a la instancia viva")
	}

	// Sin URL en los argumentos no hay nada que reenviar: arrancar la app a mano
	// segunda vez NO debe matar el proceso nuevo por sorpresa.
	// Una ruta de Windows se parsea CON esquema (`c:`) y no es un enlace: si colara,
	// el documento que se pidió no se abriría y el proceso se iría en silencio.
	if reenviar("eu.sytue.pruebas", []string{"--depurar", "/tmp/acta.odt", `C:\Docs\acta.json`}) {
		t.Error("sin enlace no se reenvía nada")
	}
	// Abierta es la misma pregunta sin mandar nada: el Hub la hace antes de avisar por
	// otra app. Se ha visto en rojo devolviendo siempre true.
	if !Abierta("eu.sytue.pruebas") || Abierta("eu.sytue.nadie") {
		t.Error("Abierta no distingue la app viva de la que no está")
	}
	// Y sin nadie escuchando, quien arranca es la aplicación.
	if reenviar("eu.sytue.nadie", []string{"sytue-docs:/open-from-drive?file=abc"}) {
		t.Error("sin instancia viva no hay a quién reenviar")
	}
}

// TestElEnlaceDelArranqueNoSePierde: con la app CERRADA, pulsar un documento arranca el
// programa con la URL en la línea de órdenes, y Fyne la despacha dentro de `Abrir` —antes
// de arrancar, en el hilo principal, la ejecuta en el acto—. En ese momento el `main`
// todavía no ha registrado su receptor. Se vio leyendo `runOnMainWithWait` del driver:
// sin guardarla, el caso más normal de todos abría la app y perdía el documento.
func TestElEnlaceDelArranqueNoSePierde(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	entrante, err := url.Parse("sytue-slides:/open-from-drive?file=pres-7&volver=%2FEquipo")
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(entrante) // lo que hace Fyne dentro de Abrir, antes del receptor

	vistas := make(chan api.Apertura, 1)
	s.AlAbrirFichero("slides", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })
	if visto := esperar(t, vistas); visto.Ref.ID != "pres-7" || visto.Volver != "/Equipo" {
		t.Fatalf("el enlace del arranque no ha llegado: %+v", visto)
	}

	// Le llega también al SEGUNDO receptor que se apunte antes de arrancar: Diapos,
	// Calcus, Contactos y Photos registran dos, y cada uno mira si el enlace es suyo.
	// Dárselo solo al primero lo perdía cuando el primero no era el suyo.
	otra := 0
	s.AlAbrirURL(func(*url.URL) { otra++ })
	if otra != 1 {
		t.Errorf("el segundo receptor ha recibido el enlace del arranque %d veces", otra)
	}

	// Y una vez arrancada, no se repite: quien se apunte después no lo recibe, y como
	// tenía receptores no es un destino que falte.
	if s.finArranque() {
		t.Error("un enlace que ya recogieron se ha tomado por uno sin dueño")
	}
	tarde := 0
	s.AlAbrirURL(func(*url.URL) { tarde++ })
	if tarde != 0 {
		t.Errorf("el enlace del arranque se ha entregado %d veces después de arrancar", tarde)
	}
}

// TestElAvisoPulsadoConLaAppCerradaLlevaASuDestino: el enlace de un aviso cuyo proceso ya
// no existe —el toast que se queda en el centro de actividades, el aviso de Android tras
// matar la app— hace lo mismo que la campana: marca el aviso leído en el Hub y abre su
// página en la app de su producto, con el host de la suite. VISTO EN ROJO sin el
// `escritorio.AlSeguir` de Abrir: el enlace se tragaba y no pasaba nada.
func TestElAvisoPulsadoConLaAppCerradaLlevaASuDestino(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	leidos := make(chan string, 2)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			leidos <- r.URL.Path
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	s := Abrir(a, "sytue.eu")
	s.Sesion.Adoptar(auth.Tokens{Access: "a", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})
	s.Hub = api.NewHub(hub.URL, s.Sesion, nil)
	espia := nuevaEspia(a)
	s.app = espia

	d := escritorio.DestinoDe("n-7", "mail", "https://mail.sytue.eu/mail/inbox?m=42")
	u, err := url.Parse(escritorio.EnlaceDeAviso("sytue-mail", "de-un-proceso-muerto", d))
	if err != nil {
		t.Fatal(err)
	}
	s.recibirURL(u)
	if got := esperar(t, espia.llega); got != "sytue-mail:/mail/inbox?m=42" {
		t.Errorf("ha abierto %q", got)
	}
	if got := esperar(t, leidos); got != "/api/v1/notifications/n-7/read" {
		t.Errorf("ha marcado %q", got)
	}
}

// Ocho de las trece apps lanzan Entrar en una goroutine y tiran el error. Si Sytue ID
// rechaza el canje, lo único que queda es lo que Entrar deje escrito: sin ello, Mail se
// quedaba en «Cargando…» sin rastro (visto en una ventana real el 2026-09-27).
func TestEntrarDejaEscritoPorQueNoEntra(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "caído", http.StatusBadGateway)
	}))
	defer idp.Close()

	s := Abrir(a, "sytue.eu")
	s.oidc.Issuer = idp.URL
	var escrito strings.Builder
	antes := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&escrito, nil)))
	defer slog.SetDefault(antes)

	if err := s.Entrar(context.Background()); err == nil {
		t.Fatal("con Sytue ID caído, Entrar tendría que fallar")
	}
	if !strings.Contains(escrito.String(), "no se ha podido entrar") || !strings.Contains(escrito.String(), "502") {
		t.Fatalf("Entrar no ha dejado escrito el porqué: %q", escrito.String())
	}
}

// Con un Entrar esperando al navegador, pulsar «Iniciar sesión» otra vez vuelve a abrir
// la MISMA autorización: un segundo login no podría escuchar en el puerto de vuelta, que
// es fijo, y fallaba en silencio.
func TestEntrarDosVecesReabreLaMismaAutorizacion(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	var abiertas []string
	s.oidc.Open = func(d string) error { abiertas = append(abiertas, d); return nil }
	s.mu.Lock()
	s.entrando, s.autorizando = true, "https://id.sytue.eu/oauth2/authorize?state=uno"
	s.mu.Unlock()

	if err := s.Entrar(context.Background()); !errors.Is(err, ErrYaEntrando) {
		t.Fatalf("Entrar con otro en marcha = %v, quiero ErrYaEntrando", err)
	}
	if !slices.Equal(abiertas, []string{"https://id.sytue.eu/oauth2/authorize?state=uno"}) {
		t.Fatalf("ha abierto %q, quiero la autorización que ya estaba en marcha", abiertas)
	}
	s.mu.Lock()
	sigue := s.entrando
	s.mu.Unlock()
	if !sigue {
		t.Error("el segundo Entrar no puede dar por terminado el primero")
	}
}

// hubDePresencia es un Hub que dice `estado` de quien pregunta y apunta lo que se le
// publica.
type hubDePresencia struct {
	mu        sync.Mutex
	estado    api.EstadoPresencia
	publicado []api.EstadoPresencia
}

func (h *hubDePresencia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Method == http.MethodPut {
		var cuerpo struct {
			State api.EstadoPresencia `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		h.publicado = append(h.publicado, cuerpo.State)
		h.estado = cuerpo.State
		_, _ = w.Write([]byte(`{"heartbeat_seconds":3600}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"presence": []map[string]string{{"subject": "ada", "state": string(h.estado)}}})
}

func (h *hubDePresencia) primero(t *testing.T) api.EstadoPresencia {
	t.Helper()
	for limite := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		h.mu.Lock()
		if len(h.publicado) > 0 {
			p := h.publicado[0]
			h.mu.Unlock()
			return p
		}
		h.mu.Unlock()
		if time.Now().After(limite) {
			t.Fatal("no se ha latido la presencia")
		}
	}
}

// El Hub dice «desconectada» también de quien lleva un rato sin latir. Abrir una app es
// estar en línea: la primera no puede adoptar ese «desconectada» y volverlo a publicar
// (visto en una ventana real el 2026-09-27: Rooms salía «Desconectado» con la persona
// trabajando). Solo si la persona lo ELIGIÓ en este equipo se mantiene.
//
// VISTO EN ROJO sin la condición de `ajustes.Leer().Invisible` en latir: publica offline.
func TestAbrirUnaAppEsEstarEnLinea(t *testing.T) {
	for _, c := range []struct {
		invisible bool
		quiero    api.EstadoPresencia
	}{{false, api.EnLinea}, {true, api.Desconectado}} {
		aparte(t)
		if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Invisible = c.invisible }); err != nil {
			t.Fatal(err)
		}
		a := test.NewApp()
		hub := &hubDePresencia{estado: api.Desconectado}
		srv := httptest.NewServer(hub)
		s := Abrir(a, "sytue.eu")
		s.Sesion.Adoptar(auth.Tokens{Access: "a", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})
		s.Hub = api.NewHub(srv.URL, s.Sesion, nil)
		ctx, parar := context.WithCancel(context.Background())
		go s.latir(ctx)
		if got := hub.primero(t); got != c.quiero {
			t.Errorf("elegida invisible=%v: se ha publicado %q, quiero %q", c.invisible, got, c.quiero)
		}
		parar()
		srv.Close()
		a.Quit()
	}
}

// TestUnAvisoDeUnProductoQueNoExisteNoSeSigue: el destino de un aviso pulsado con la app
// ya cerrada llega por un ENLACE (`sytue-mail:/_aviso?origen=…&ruta=…`), y un enlace lo
// escribe cualquiera. Con un origen que no es un producto de la suite, `api.ServiceURL`
// entraba en pánico y el enlace cerraba la app (NAT-06).
func TestUnAvisoDeUnProductoQueNoExisteNoSeSigue(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s := &Suite{app: a, apex: "sytue.eu"}
	for _, origen := range []string{"zzz", "", "mail.otro"} {
		s.seguirAviso(escritorio.Destino{Origen: origen, Ruta: "/x"})
	}
}

// TestCerrarSesionRevocaElTokenYVaciaLaCache: «Cerrar sesión» borraba el refresh token
// de este equipo y nada más. En Sytue ID seguía valiendo para quien tuviera una copia
// (NAT-07), y la caché de trabajar sin conexión —la libreta, las notas, los listados— se
// quedaba en el disco (NAT-08). Lo que NO se borra es lo que aún no ha subido.
func TestCerrarSesionRevocaElTokenYVaciaLaCache(t *testing.T) {
	aparte(t)
	revocado := make(chan url.Values, 1)
	var idp *httptest.Server
	idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": idp.URL, "authorization_endpoint": idp.URL + "/authorize",
				"token_endpoint": idp.URL + "/token", "revocation_endpoint": idp.URL + "/revoke",
			})
		case "/revoke":
			_ = r.ParseForm()
			select {
			case revocado <- r.PostForm:
			default:
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()
	if err := auth.Save(auth.Tokens{Refresh: "r-1", Subject: "u1"}); err != nil {
		t.Fatal(err)
	}
	alm, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}
	const dueno, pagina = "u1/acme", "https://contacts.sytue.eu/api/v1/contacts"
	if err := errors.Join(alm.Write(dueno, "http:contacts", pagina, "la libreta"),
		alm.Write(dueno, "notes-borradores", "b1", "un borrador sin subir")); err != nil {
		t.Fatal(err)
	}

	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	s.oidc.Issuer = idp.URL
	s.Salir()

	select {
	case f := <-revocado:
		if f.Get("token") != "r-1" || f.Get("client_id") != auth.ClientID {
			t.Errorf("a Sytue ID se le ha pedido revocar %v: quiero el refresh token guardado, del cliente de escritorio", f)
		}
	case <-time.After(5 * time.Second):
		t.Error("cerrar sesión no ha revocado el refresh token en Sytue ID")
	}
	var v string
	if hay, _ := alm.Read(dueno, "http:contacts", pagina, &v); hay {
		t.Error("tras cerrar sesión, la caché de respuestas sigue en el disco")
	}
	// NAT-08, «avisar y luego borrar» (2026-10-16): del borrador ya se avisó antes de
	// salir (PendientesNotepads); salir no lo deja en claro en el disco.
	if hay, _ := alm.Read(dueno, "notes-borradores", "b1", &v); hay {
		t.Error("tras cerrar sesión, un borrador de Notepads sigue en claro en el disco")
	}
}

// TestCerrarSesionOlvidaLasClavesDeLosCuadernos (FYS-09): la clave recordada de un
// cuaderno cifrado no sobrevive al cierre de sesión; la marca del bloqueo del equipo,
// que no es de la sesión, sí.
func TestCerrarSesionOlvidaLasClavesDeLosCuadernos(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	if err := errors.Join(auth.GuardarSecreto("notes-c1", []byte("clave")), auth.GuardarSecreto("rooms-matrix-master-test", []byte("clave Matrix")), marcarBloqueo(true)); err != nil {
		t.Fatal(err)
	}
	s.Salir()
	if clave, err := auth.LeerSecreto("notes-c1"); err != nil || len(clave) != 0 {
		t.Fatalf("la clave del cuaderno sigue tras salir: %q %v", clave, err)
	}
	if clave, err := auth.LeerSecreto("rooms-matrix-master-test"); err != nil || len(clave) != 0 {
		t.Fatal("la clave Matrix sobrevivió al cierre de sesión")
	}
	if marca, err := auth.LeerSecreto(cuentaBloqueo); err != nil || len(marca) == 0 {
		t.Fatalf("salir quitó el bloqueo del equipo: %v", err)
	}
}

func TestGeneracionCompartidaPerteneceALaIdentidadAdoptada(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	primera := Abrir(a, "sytue.eu")
	segunda := Abrir(a, "sytue.eu")
	primera.Sesion.Adoptar(auth.Tokens{Refresh: "r-ada", Subject: "ada"})
	primera.aplicar(ajustes.Leer())
	antes := primera.Estado()
	if antes.Sujeto != "ada" || antes.GeneracionSesion == "" {
		t.Fatal("la primera identidad no tiene una generación adoptada")
	}
	segunda.Sesion.Adoptar(auth.Tokens{Refresh: "r-grace", Subject: "grace"})
	marcaNueva := ajustes.Leer().Sesion
	// Leer el estado mientras otra app ya guardó una cuenta no mezcla su marca
	// nueva con el access/identidad que esta app todavía conserva.
	aun := primera.Estado()
	if aun.Sujeto != "ada" || aun.GeneracionSesion != antes.GeneracionSesion || aun.GeneracionSesion == marcaNueva {
		t.Fatal("se atribuyó una generación nueva a la identidad anterior")
	}
	primera.aplicar(ajustes.Leer())
	despues := primera.Estado()
	if despues.Sujeto != "grace" || despues.GeneracionSesion != marcaNueva {
		t.Fatal("la identidad y su generación no se adoptaron juntas")
	}
	segunda.Sesion.Logout()
	primera.aplicar(ajustes.Leer())
	marcaCierre := ajustes.Leer().Sesion
	if primera.Estado().Dentro() || primera.Estado().GeneracionSesion != marcaCierre {
		t.Fatal("el cierre no se propagó con su generación")
	}
	primera.aplicar(ajustes.Leer())
	if ajustes.Leer().Sesion != marcaCierre {
		t.Fatal("una app volvió a publicar el cierre compartido")
	}
}

func TestCerrarSesionNoReadoptaAlmacenQueNoPudoBorrar(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	tokens := auth.Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}
	if err := s.almacen.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	s.Sesion = auth.NewSession(tokens, nil, func(auth.Tokens) error { return errors.New("almacen no escribible") })
	if err := s.SalirConError(); err == nil {
		t.Fatal("ocultó el fallo de cierre compartido")
	}
	if s.Estado().Dentro() {
		t.Fatal("el cierre fallido volvió a adoptar el refresh que no pudo borrar")
	}
	s.aplicar(ajustes.Leer())
	if s.Estado().Dentro() {
		t.Fatal("la vigilancia readoptó una copia no borrada")
	}
}

func TestHabilitarDriveNoAutorizaSesionObsoletaEnMemoria(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	// Representa otra app que aún conserva la cuenta anterior tras el cierre
	// compartido. No ha recibido una nueva notificación de ajustes.
	s.Sesion.Sincronizar(auth.Tokens{Refresh: "copia-obsoleta", Subject: "ana", Organizacion: "acme", Issuer: "https://id.sytue.eu", Cliente: auth.ClientID})
	if err := s.PonerDriveEnSistema(true); err != nil {
		t.Fatal(err)
	}
	acceso, _ := ajustes.LeerAccesoDrive()
	if acceso.Activo {
		t.Fatal("un ajuste publicó autorización desde memoria obsoleta")
	}
}

func TestHabilitarDriveRespetaBloqueoYFaltaDeSesionLocal(t *testing.T) {
	for _, caso := range []string{"sin_sesion_local", "reidentificacion"} {
		t.Run(caso, func(t *testing.T) {
			aparte(t)
			a := test.NewApp()
			defer a.Quit()
			s := Abrir(a, "sytue.eu")
			s.Sesion.Adoptar(auth.Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"})
			s.aplicar(ajustes.Leer()) // la app ya recibió la generación guardada
			if caso == "reidentificacion" {
				s.reidentificando.Store(true)
			} else {
				s.Sesion.Sincronizar(auth.Tokens{})
			}
			if err := s.PonerDriveEnSistema(true); err != nil {
				t.Fatal(err)
			}
			acceso, _ := ajustes.LeerAccesoDrive()
			if acceso.Activo {
				t.Fatal("habilitar Drive eludió la sesión local o su bloqueo")
			}
		})
	}
}
