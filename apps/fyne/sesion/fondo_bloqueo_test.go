// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// appSinEscena es una app cuyo driver es el de iOS lanzado sin escena (EscenaDePrueba):
// así se prueba Abrir entero, que es donde se decide el bloqueo.
type appSinEscena struct {
	widgetApp
	escena *EscenaDePrueba
}

func (a appSinEscena) Driver() fyne.Driver {
	return driverSinEscena{Driver: a.widgetApp.Driver(), EscenaDePrueba: a.escena}
}

type driverSinEscena struct {
	fyne.Driver
	*EscenaDePrueba
}

// verificaciones sustituye la verificación del dueño (Face ID) por `respuesta` y cuenta
// cuántas veces se pide.
func verificaciones(t *testing.T, respuesta error) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	antes := verificarDueno
	verificarDueno = func(context.Context, string) error {
		n.Add(1)
		return respuesta
	}
	t.Cleanup(func() { verificarDueno = antes })
	return &n
}

// ponerBloqueo deja el bloqueo de la suite puesto en disco, como lo deja la bandeja.
func ponerBloqueo(t *testing.T) {
	t.Helper()
	if err := marcarBloqueo(true); err != nil {
		t.Fatal(err)
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
}

// teléfonoBloqueado es lo que ve una app de iOS lanzada con el dispositivo bloqueado:
// el App Group (NSFileProtectionComplete) y el llavero (WhenUnlocked) no se dejan leer.
// Los ficheros se quitan —ajustes.Leer da lo de por defecto, como con EPERM— y el
// llavero falla. Al desbloquear, el llavero vuelve vacío: quien llama vuelve a guardar
// lo que tenía que haber.
func telefonoBloqueado(t *testing.T, e *EscenaDePrueba) (desbloquear func()) {
	t.Helper()
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ajustes.json", "sesion.marca", "sesion.epoca"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	keyring.MockInitWithError(errors.New("errSecInteractionNotAllowed"))
	e.Bloquear(true)
	return func() {
		keyring.MockInit()
		e.Bloquear(false)
	}
}

// esperarEscena espera a que corra lo que la app dejó para la escena.
func esperarEscena(t *testing.T, cargada *atomic.Bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for !cargada.Load() {
		if time.Now().After(limite) {
			t.Fatal("lo de CuandoHayaEscena no ha corrido al conectar la escena")
		}
		fyne.DoAndWait(func() {})
		time.Sleep(5 * time.Millisecond)
	}
}

// TestElBloqueoSeExigeAlConectarAunqueAbrirNoLoLeyera es el salto del bloqueo que abría
// el refresco de fondo: iOS lanza la app con el teléfono bloqueado, Abrir no puede leer
// ni el bloqueo ni la sesión, y al desbloquear el vigilante la adoptaba sin Face ID. Al
// tocar el icono, la escena conectaba con la sesión dentro.
//
// Ahora el vigilante no la adopta con el bloqueo puesto, y al conectar la escena se pide
// la verificación (aquí, sin verificador: la contraseña de Sytue) antes de las cargas.
//
// VISTO EN ROJO: sin retenerSiBloqueada en aplicar, la sesión queda dentro tras el
// vistazo; sin pasarBloqueoEnEscena, al conectar no se verifica nada.
func TestElBloqueoSeExigeAlConectarAunqueAbrirNoLoLeyera(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	guardar(t, "ana", "r1")
	ponerBloqueo(t)
	pedidas := verificaciones(t, escritorio.ErrNoDisponible)
	escena := &EscenaDePrueba{}
	desbloquear := telefonoBloqueado(t, escena)

	s := Abrir(appSinEscena{widgetApp{app}, escena}, "sytue.eu")
	if s.Estado().Dentro() {
		t.Fatal("con el teléfono bloqueado no se puede haber leído ninguna sesión")
	}

	// La persona desbloquea: todo vuelve a leerse, y el vigilante lo ve.
	desbloquear()
	guardar(t, "ana", "r2")
	if err := s.almacen.Migrar(t.Context(), true); err != nil { // ver TestLanzadaBloqueadaRefrescaAlDesbloquear
		t.Fatal(err)
	}
	ponerBloqueo(t)
	s.aplicar(ajustes.Leer())
	if s.Estado().Dentro() {
		t.Fatal("el vigilante ha adoptado la sesión sin pasar el bloqueo")
	}
	if n := pedidas.Load(); n != 0 {
		t.Fatalf("sin escena se ha pedido la verificación %d veces", n)
	}

	var cargada, dentroAlCargar atomic.Bool
	s.CuandoHayaEscena(func() {
		dentroAlCargar.Store(s.Estado().Dentro())
		cargada.Store(true)
	})
	escena.Conectar()
	esperarEscena(t, &cargada)
	if n := pedidas.Load(); n != 1 {
		t.Fatalf("al conectar la escena se ha pedido la verificación %d veces, y es una", n)
	}
	if dentroAlCargar.Load() || s.Estado().Dentro() {
		t.Fatal("sin verificar, la app ha cargado con la sesión dentro")
	}
	if s.oidc.MaxAge <= 0 {
		t.Fatal("sin verificador, el login no pide la contraseña de Sytue")
	}
}

// TestLanzadaEnFondoNoVerificaHastaLaEscena: con el teléfono desbloqueado y el bloqueo
// puesto, el lanzamiento de fondo no pide Face ID (no hay interfaz: fallaría y la app
// saldría en cada refresco), no adopta la sesión y el refresco no lee nada. Al conectar
// la escena, verificado, la sesión se adopta ANTES de las cargas.
//
// VISTO EN ROJO: con pedirSytueIDAlAbrir en Abrir, la verificación se pide sin escena.
func TestLanzadaEnFondoNoVerificaHastaLaEscena(t *testing.T) {
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	app := test.NewApp()
	defer app.Quit()
	guardar(t, "ana", "r1")
	ponerBloqueo(t)
	pedidas := verificaciones(t, nil)
	escena := &EscenaDePrueba{}

	s := Abrir(appSinEscena{widgetApp{app}, escena}, "sytue.eu")
	if n := pedidas.Load(); n != 0 {
		t.Fatalf("sin escena se ha pedido la verificación %d veces", n)
	}
	if s.Estado().Dentro() {
		t.Fatal("con el bloqueo puesto y sin verificar, la sesión se ha adoptado")
	}
	var lecturas atomic.Int32
	s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
	s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		lecturas.Add(1)
		return conDocumentos(7), nil
	})
	if r := s.RefrescarWidgetEnFondo(t.Context()); r != SinCambios || lecturas.Load() != 0 {
		t.Fatalf("con el bloqueo puesto, el refresco: %v y %d lecturas", r, lecturas.Load())
	}

	var cargada, dentroAlCargar atomic.Bool
	s.CuandoHayaEscena(func() {
		dentroAlCargar.Store(s.Estado().Dentro())
		cargada.Store(true)
	})
	escena.Conectar()
	esperarEscena(t, &cargada)
	if n := pedidas.Load(); n != 1 {
		t.Fatalf("al conectar la escena se ha pedido la verificación %d veces, y es una", n)
	}
	if !dentroAlCargar.Load() {
		t.Fatal("verificado, las cargas de la escena han corrido sin la sesión")
	}
	if e := s.Estado(); e.Sujeto != "ana" {
		t.Fatalf("verificado, la sesión no se ha adoptado: %+v", e)
	}
}

// TestLanzadaBloqueadaRefrescaAlDesbloquear: lanzada con el teléfono bloqueado, el
// proceso se quedó sin sesión y con el widget en pausa. La tarea siguiente, ya
// desbloqueado, adopta lo que hay en disco y publica, sin esperar al vigilante.
//
// VISTO EN ROJO: sin el aplicar de RefrescarWidgetEnFondo, contesta SinCambios y no
// publica nada hasta el siguiente vistazo (que con la app suspendida no llega).
func TestLanzadaBloqueadaRefrescaAlDesbloquear(t *testing.T) {
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	app := test.NewApp()
	defer app.Quit()
	guardar(t, "ana", "r1")
	escena := &EscenaDePrueba{}
	desbloquear := telefonoBloqueado(t, escena)

	s := Abrir(appSinEscena{widgetApp{app}, escena}, "sytue.eu")
	var lecturas atomic.Int32
	s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
	s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		lecturas.Add(1)
		return conDocumentos(7), nil
	})
	s.Arrancar(t.Context())
	if r := s.RefrescarWidgetEnFondo(t.Context()); r != Aplazado || lecturas.Load() != 0 {
		t.Fatalf("bloqueado: %v y %d lecturas", r, lecturas.Load())
	}

	desbloquear()
	guardar(t, "ana", "r2")
	// En el teléfono la sesión ya estaba en el formato del almacén; aquí la migración de
	// Abrir no pudo hacerse con el llavero cerrado.
	if err := s.almacen.Migrar(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if r := s.RefrescarWidgetEnFondo(t.Context()); r != Nuevos {
		t.Fatalf("desbloqueado, el refresco contesta %v", r)
	}
	if n := publicado(t, "eu.sytue.docus"); n != 7 {
		t.Fatalf("desbloqueado, el widget tiene %d", n)
	}
}

// telefonoIlegible es telefonoBloqueado como lo ve de verdad la app: los ficheros
// protegidos ESTÁN, pero no se dejan leer (NSFileProtectionComplete da EPERM), y el
// llavero falla. Un directorio en su sitio, porque a root un chmod 000 no le impide
// leer. Con sinEpoca, `sesion.epoca` ni está: una sesión de una compilación anterior.
func telefonoIlegible(t *testing.T, e *EscenaDePrueba, sinEpoca bool) (desbloquear func()) {
	t.Helper()
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	guardados := map[string][]byte{}
	for _, f := range []string{"ajustes.json", "sesion.marca", "sesion.epoca"} {
		ruta := filepath.Join(dir, f)
		crudo, err := os.ReadFile(ruta)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if err == nil {
			guardados[f] = crudo
		}
		if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if f == "sesion.epoca" && sinEpoca {
			continue
		}
		if err := os.Mkdir(ruta, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	keyring.MockInitWithError(errors.New("errSecInteractionNotAllowed"))
	antes := cargarAlmacen
	cargarAlmacen = func(*auth.AlmacenSesion) (auth.Tokens, error) {
		return auth.Tokens{}, errors.New("errSecInteractionNotAllowed")
	}
	e.Bloquear(true)
	return func() {
		cargarAlmacen = antes
		for f, crudo := range guardados {
			ruta := filepath.Join(dir, f)
			if err := os.RemoveAll(ruta); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(ruta, crudo, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		keyring.MockInit()
		e.Bloquear(false)
	}
}

// TestLanzadaEnFrioBloqueadaNoBorraLosWidgets es el lanzamiento en frío que iOS hace
// para la tarea de refresco con el teléfono bloqueado toda la noche: Abrir no puede leer
// la sesión (el llavero es WhenUnlocked) y la app queda «fuera». El primer aviso del
// widget, en Arrancar, pedía ClearIf(""): la época ilegible salía vacía, coincidía, y se
// borraban los widgets de las quince apps. Por la mañana salían vacíos, no con el último
// dato y «Actualizado hace X». Ahora no se borra ni con la época ilegible (clearSnapshots)
// ni sin época (el primer aviso, sin sesión legible, no borra).
//
// VISTO EN ROJO: con las dos defensas quitadas, los dos casos borran los dos snapshots;
// con solo la de iniciarWidgets quitada, el caso «sin época» los borra.
func TestLanzadaEnFrioBloqueadaNoBorraLosWidgets(t *testing.T) {
	for _, caso := range []struct {
		nombre   string
		sinEpoca bool
	}{{"época ilegible", false}, {"sin época", true}} {
		t.Run(caso.nombre, func(t *testing.T) {
			aparte(t)
			t.Cleanup(widgets.SimularLector(true))
			app := test.NewApp()
			defer app.Quit()
			guardar(t, "ana", "r1")
			// Lo que publicaron ayer esta app y otra de la suite.
			ayer := widgets.Capture(ajustes.Leer().Epoca)
			snapshots := map[string]string{"eu.sytue.docus": "docs", "eu.sytue.mail": "mail"}
			for bundle, producto := range snapshots {
				if err := widgets.PublishDetail(t.Context(), bundle, producto, ayer, widgets.Detail{}, widgets.Vigencia); err != nil {
					t.Fatal(err)
				}
			}
			escena := &EscenaDePrueba{}
			desbloquear := telefonoIlegible(t, escena, caso.sinEpoca)
			defer desbloquear()

			s := Abrir(appSinEscena{widgetApp{app}, escena}, "sytue.eu")
			if s.Estado().Dentro() || !s.ilegibleAlAbrir {
				t.Fatal("la prueba no deja la sesión ilegible al abrir")
			}
			s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
			s.PublicadorWidget(func(context.Context) (widgets.Detail, error) { return conDocumentos(7), nil })
			ctx, parar := context.WithCancel(t.Context())
			defer parar()
			s.Arrancar(ctx)
			fyne.DoAndWait(func() {})
			time.Sleep(300 * time.Millisecond) // lo que tardaría la rutina del ClearIf
			if r := s.RefrescarWidgetEnFondo(ctx); r != Aplazado {
				t.Fatalf("bloqueado, el refresco contesta %v", r)
			}
			dir, err := ajustes.Carpeta()
			if err != nil {
				t.Fatal(err)
			}
			for bundle := range snapshots {
				if _, err := os.Stat(filepath.Join(dir, "widgets", bundle+".json")); err != nil {
					t.Errorf("el lanzamiento en frío bloqueado ha borrado el widget de %s: %v", bundle, err)
				}
			}
		})
	}
}

// TestElVigilanteNoVaciaLaSesionTrasFaceID: mientras se pide Face ID al conectar la
// escena, el vigilante aplica un cambio de sesión y la lee retenida (vacía). Si adoptaba
// esa lectura después de que pasarBloqueoEnEscena adoptara la buena, la app abría
// «fuera» tras verificar bien, y como `visto` ya coincidía no se recuperaba. Ahora las
// dos adoptan con `aplicando`: la del bloqueo espera a la del vigilante y gana.
//
// VISTO EN ROJO: sin `aplicando` en pasarBloqueoEnEscena, la sesión acaba vacía.
func TestElVigilanteNoVaciaLaSesionTrasFaceID(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	guardar(t, "ana", "r1")
	ponerBloqueo(t)
	verificaciones(t, nil) // Face ID dice que sí
	escena := &EscenaDePrueba{}
	s := Abrir(appSinEscena{widgetApp{app}, escena}, "sytue.eu")
	if s.Estado().Dentro() {
		t.Fatal("con el bloqueo puesto, Abrir ha adoptado la sesión sin escena")
	}

	leido, seguir := make(chan struct{}), make(chan struct{})
	var una atomic.Bool
	antes := trasLeerAlAplicar
	trasLeerAlAplicar = func() {
		if una.CompareAndSwap(false, true) {
			close(leido)
			<-seguir
		}
	}
	t.Cleanup(func() { trasLeerAlAplicar = antes })

	guardar(t, "ana", "r2") // otra app renueva la sesión mientras tanto
	vigilante := make(chan struct{})
	go func() { s.aplicar(ajustes.Leer()); close(vigilante) }()
	<-leido // el vigilante la ha leído retenida y aún no la ha adoptado

	var cargada atomic.Bool
	s.CuandoHayaEscena(func() { cargada.Store(true) })
	escena.Conectar()
	// Sin el cerrojo, el bloqueo adopta la buena aquí mismo; con él, espera al vigilante.
	limite := time.Now().Add(300 * time.Millisecond)
	for !cargada.Load() && time.Now().Before(limite) {
		fyne.DoAndWait(func() {})
		time.Sleep(5 * time.Millisecond)
	}
	close(seguir)
	<-vigilante
	esperarEscena(t, &cargada)
	if !s.Estado().Dentro() {
		t.Fatal("verificado Face ID, la app ha abierto fuera: ganó la lectura retenida del vigilante")
	}
}
