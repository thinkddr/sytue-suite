// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElContratoDelAvisoEsElDeToastTsx: las clases de las que salen las medidas siguen
// siendo las suyas.
func TestElContratoDelAvisoEsElDeToastTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Toast.tsx")
	for _, c := range []struct{ trozo, porque string }{
		{"fixed right-[max(1rem,var(--sytue-safe-right,0px))] bottom-[max(1rem,var(--sytue-safe-bottom,0px))]", "la esquina con un mínimo de 1rem y respetando el área segura"},
		{"w-[calc(100%_-_max(1rem,var(--sytue-safe-left,0px))_-_max(1rem,var(--sytue-safe-right,0px)))] max-w-sm", "el ancho disponible entre ambos márgenes, su tope y la separación"},
		{"pointer-events-none", "que la pila no se coma el puntero de lo de debajo"},
		{"flex items-start gap-3 overflow-hidden rounded-lg border", "la tarjeta"},
		{"bg-surface-raised pr-3 shadow-[var(--shadow-lg)]", "su superficie, su margen derecho y su elevación"},
		{`cx('w-1 self-stretch', cfg.bar)`, "la banda de tono, de 4 y a toda la altura"},
		{`cx('py-3', cfg.fg)`, "el `py-3` del glifo"},
		{`Icon name={cfg.icon} size={18}`, "el glifo de 18"},
		{"text-text-muted mt-0.5 text-sm", "la descripción"},
		{"mt-1.5 rounded-sm text-sm font-semibold", "la acción"},
		{"mt-1 inline-flex h-7 w-7 shrink-0", "el aspa de 28"},
		{"duration ?? (persistent ? 0 : 5000)", "los 5 s, y que advertencia y peligro persisten"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("Toast.tsx ya no dice %q (%s)", c.trozo, c.porque)
		}
	}
	if AnchoPilaAvisos != 384 || MargenPilaAvisos != 16 || HuecoPilaAvisos != 8 {
		t.Errorf("la pila mide %v/%v/%v y `max-w-sm`, el margen mínimo de 1rem y `gap-2` dan 384/16/8",
			float32(AnchoPilaAvisos), float32(MargenPilaAvisos), float32(HuecoPilaAvisos))
	}
}

// TestElAvisoMideLoQueSuPiezaMasAlta: con `items-start` y sin relleno vertical propio,
// la tarjeta mide lo que su pieza más alta más sus dos bordes. Las cifras se escriben
// enteras para que se lea de dónde sale cada sumando.
func TestElAvisoMideLoQueSuPiezaMasAlta(t *testing.T) {
	solo := NuevoAviso(AvisoExito, "Documento guardado", "")
	// py-3 ×2 + la caja de línea de text-base, contra los 42 del glifo con su py-3.
	quiero := max(2*12+22, 2*12+18) + 2*tema.Borde
	if got := MedidaAviso(solo, AnchoPilaAvisos).Height; got != quiero {
		t.Errorf("un aviso de solo título mide %v y la cuenta de la web da %v", got, quiero)
	}

	con := NuevoAviso(AvisoPeligro, "No se ha podido enviar", "El servidor de correo no responde.")
	// …y con descripción: + mt-0.5 + la caja de línea de text-sm.
	quiero = 2*12 + 22 + 2 + 18 + 2*tema.Borde
	if got := MedidaAviso(con, AnchoPilaAvisos).Height; got != quiero {
		t.Errorf("un aviso con descripción mide %v y la cuenta de la web da %v", got, quiero)
	}

	// Y el aspa nunca manda: 4 de `mt-1` más 28 son 32, por debajo de los 42 del
	// glifo. Si alguien la agranda, esta cuenta cambia y hay que volver a mirarla.
	if arribaCerrar+ladoCerrarAviso >= 2*margenYAviso+iconos.TallaMD {
		t.Error("el aspa ha pasado a ser la pieza más alta del aviso: revisar MedidaAviso")
	}
}

// TestLosAvisosQueImportanNoSeVanSolos: un error que se autodescarta puede perderse sin
// ser leído, así que advertencia y peligro persisten.
func TestLosAvisosQueImportanNoSeVanSolos(t *testing.T) {
	for _, c := range []struct {
		tono   TonoAviso
		quiero time.Duration
	}{
		{AvisoInfo, 5 * time.Second},
		{AvisoExito, 5 * time.Second},
		{AvisoAdvertencia, 0},
		{AvisoPeligro, 0},
	} {
		if got := DuracionDe(c.tono); got != c.quiero {
			t.Errorf("el tono %d dura %v y la web le da %v", c.tono, got, c.quiero)
		}
	}
}

// pilaQueSeVacia es una pila que descarta sus avisos al acabar el test. Sin eso, un
// aviso de éxito deja un reloj de 5 s vivo que dispara en mitad de OTRO test: su
// `Refresh` escribe la caché global de renderers de Fyne mientras el otro pinta, y
// `-race` tumba a ese otro (run 2593, `TestCargandoRelevaAlIconoYAtenuaMenos`).
func pilaQueSeVacia(t *testing.T) *PilaAvisos {
	p := NuevaPilaAvisos()
	t.Cleanup(func() {
		for _, a := range p.Avisos() {
			p.Descartar(a)
		}
	})
	return p
}

// TestLaPilaSeQuedaEnLaEsquinaInferiorDerecha, con el más nuevo abajo.
func TestLaPilaSeQuedaEnLaEsquinaInferiorDerecha(t *testing.T) {
	p := pilaQueSeVacia(t)
	viejo := NuevoAviso(AvisoInfo, "Subiendo 3 ficheros", "")
	nuevo := NuevoAviso(AvisoExito, "Listo", "")
	p.Notificar(viejo)
	p.Notificar(nuevo)

	ventana := fyne.NewSize(1280, 800)
	p.Resize(ventana)

	alto := MedidaAviso(nuevo, AnchoPilaAvisos).Height
	if got := nuevo.Position(); got.X != 1280-16-AnchoPilaAvisos || got.Y != 800-16-alto {
		t.Errorf("el aviso nuevo está en %v y el margen de 1rem con 384 de ancho lo pone en (%v, %v)",
			got, 1280-16-float32(AnchoPilaAvisos), 800-16-alto)
	}
	if got, quiero := nuevo.Position().Y-viejo.Position().Y,
		MedidaAviso(viejo, AnchoPilaAvisos).Height+HuecoPilaAvisos; got != quiero {
		t.Errorf("entre dos avisos hay %v y el `gap-2` más la tarjeta dan %v", got, quiero)
	}
	if nuevo.Size().Width != AnchoPilaAvisos {
		t.Errorf("el aviso mide %v de ancho y `max-w-sm` son %v", nuevo.Size().Width, float32(AnchoPilaAvisos))
	}

	// En móvil los dos márgenes se descuentan del ancho: la tarjeta entera queda
	// dentro del área interactiva, sin duplicar los insets que reserva el driver.
	p.Resize(fyne.NewSize(320, 640))
	if got := nuevo.Size().Width; got != 288 {
		t.Errorf("a 320 el aviso mide %v, quiero 288 entre dos márgenes de 16", got)
	}
	if got := nuevo.Position(); got.X != 16 || got.X+nuevo.Size().Width != 304 || got.Y+nuevo.Size().Height != 624 {
		t.Errorf("a 320×640 el aviso está en %v con %v: debe dejar 16 a ambos lados y abajo", got, nuevo.Size())
	}
}

// TestUnAvisoNoDejaLaAplicacionSinPoderPulsar es el guardián que justifica que la pila
// NO sea una capa, y está escrito invirtiéndolo.
//
// En Fyne, `FindObjectAtPositionMatching` hace `if overlay != nil { … } else { roots }`:
// con UNA capa puesta el puntero no visita el contenido. Una pila de avisos montada
// como capa deja la aplicación entera muerta mientras haya un aviso, y eso no se ve en
// ninguna captura. Aquí se comprueba pulsando de verdad por debajo.
func TestUnAvisoNoDejaLaAplicacionSinPoderPulsar(t *testing.T) {
	pulsado := 0
	boton := NuevoBoton("Guardar", func() { pulsado++ })
	pila := pilaQueSeVacia(t)
	w := test.NewTempWindow(t, container.NewStack(boton, pila))
	w.Resize(fyne.NewSize(1280, 800))

	pila.Notificar(NuevoAviso(AvisoExito, "Documento guardado", ""))
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Fatalf("la pila de avisos ha montado %d capas: con una sola, la aplicación se queda sin puntero", n)
	}

	// El botón ocupa la esquina superior izquierda; el aviso, la inferior derecha.
	test.TapCanvas(w.Canvas(), fyne.NewPos(10, 10))
	if pulsado != 1 {
		t.Errorf("con un aviso en pantalla, pulsar el botón de debajo ha llegado %d veces", pulsado)
	}

	// Y el aspa del aviso SÍ recoge su clic: el aviso no es transparente al puntero.
	test.Tap(pila.Avisos()[0].aspa)
	if n := len(pila.Avisos()); n != 0 {
		t.Errorf("tras pulsar el aspa quedan %d avisos", n)
	}
	if pulsado != 1 {
		t.Error("el clic en el aspa del aviso ha llegado además al botón de debajo")
	}
}

// TestLaPilaDeAvisosNoCorreCarreraConSuReloj es el guardián de la carrera que ponía
// `main` en flaky: el reloj de un aviso dispara en su propia rutina, y el driver de
// pruebas de Fyne ejecutaba su `fyne.Do` AHÍ. Se veía como un fallo de OTRO test: el
// aviso sobrevivía al suyo y tumbaba al que el `-shuffle` pusiera detrás.
//
// Desde el fork v2.8.1-sytue.4 el driver lo encola y lo ejecuta la rutina del test
// (`TestElFyneDoDeOtraRutinaEsperaAlTest`), así que aquí se le deja pasar con
// `fyne.DoAndWait` y lo que queda por comprobar es que los relojes descartan.
func TestLaPilaDeAvisosNoCorreCarreraConSuReloj(t *testing.T) {
	// SIN `test.NewTempApp`: TestMain ya montó la aplicación CON el tema de la suite, y
	// `NewTempApp` la sustituye por una de serie —y su limpieza monta otra—, así que
	// deja sin letra Figtree a todos los tests que vengan detrás. Se ve como
	// `TestLaLetraEsFigtree` fallando en otro fichero.
	p := pilaQueSeVacia(t)
	p.Resize(fyne.NewSize(600, 400))
	for range 40 {
		a := NuevoAviso(AvisoInfo, "Guardado", "")
		a.Duracion = time.Millisecond
		p.Notificar(a)
	}

	plazo := time.Now().Add(2 * time.Second)
	for time.Now().Before(plazo) {
		fyne.DoAndWait(func() {}) // los Descartar que los relojes han encolado
		if len(p.Avisos()) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("quedan %d avisos: los relojes no los han descartado", len(p.Avisos()))
}

// TestElAvisoLargoPasaDeLineaYSuAccionSePulsa: el título y la descripción son `<p>` y
// pasan de línea, y la acción es un `<button>` que hace lo suyo y cierra el aviso.
// VISTO EN ROJO con el título en un `canvas.Text` de una línea y la acción pintada
// como texto suelto: el aviso medía lo mismo y «Deshacer» no hacía nada.
func TestElAvisoLargoPasaDeLineaYSuAccionSePulsa(t *testing.T) {
	corto := MedidaAviso(NuevoAviso(AvisoPeligro, "No se ha podido mover", ""), AnchoPilaAvisos).Height
	largo := NuevoAviso(AvisoPeligro,
		"No se ha podido mover «Presupuesto 2027 revisado y definitivo» a la carpeta compartida", "")
	if got := MedidaAviso(largo, AnchoPilaAvisos).Height; got < corto+tema.TextoBase.Linea {
		t.Errorf("el aviso largo mide %v de alto y el corto %v: el título no ha pasado de línea", got, corto)
	}

	pila := pilaQueSeVacia(t)
	hecho := false
	a := NuevoAviso(AvisoInfo, "Movido a la papelera", "")
	a.RotuloAccion, a.AlPulsarAccion = "Deshacer", func() { hecho = true }
	pila.Notificar(a)
	r := test.WidgetRenderer(a).(*avisoRenderer)
	if r.accion == nil {
		t.Fatal("el aviso con acción no tiene botón de acción")
	}
	r.accion.Tapped(nil)
	if !hecho || len(pila.Avisos()) != 0 {
		t.Errorf("al pulsar la acción: hecha=%v y quedan %d avisos; la web la hace y cierra", hecho, len(pila.Avisos()))
	}
}

// El aviso entra en el flujo del diálogo y vuelve a la esquina al cerrarlo.
func TestElAvisoSeInsertaEnElDialogo(t *testing.T) {
	pila := pilaQueSeVacia(t)
	w := test.NewTempWindow(t, container.NewStack(canvas.NewRectangle(tema.Color("bg", tema.Claro)), pila))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	d := NuevoDialogo("Reglas de correo", cuerpoDe(60))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	a := NuevoAviso(AvisoPeligro, "No se ha podido guardar", "")
	pila.Notificar(a)
	if !slices.Contains(test.WidgetRenderer(d.r.bloqueAvisos).Objects(), fyne.CanvasObject(a)) {
		t.Fatal("el aviso no se inserta dentro del diálogo")
	}
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if slices.Contains(test.WidgetRenderer(cp).Objects(), fyne.CanvasObject(a)) {
		t.Error("el aviso sigue superpuesto al panel")
	}
	if slices.Contains(test.WidgetRenderer(pila).Objects(), fyne.CanvasObject(a)) {
		t.Error("el aviso se pinta también en la esquina")
	}
	test.WidgetRenderer(a)
	test.Tap(a.aspa)
	if len(pila.Avisos()) != 0 {
		t.Error("el aspa no cierra el aviso dentro del diálogo")
	}
	b := NuevoAviso(AvisoPeligro, "Otra vez", "")
	pila.Notificar(b)
	d.Cerrar()
	if !slices.Contains(test.WidgetRenderer(pila).Objects(), fyne.CanvasObject(b)) {
		t.Error("al cerrar el diálogo el aviso no vuelve a la pila")
	}
	alto := MedidaAviso(b, AnchoPilaAvisos).Height
	if pos, quiero := b.Position(), fyne.NewPos(1280-16-AnchoPilaAvisos, 800-16-alto); pos != quiero {
		t.Errorf("al cerrar vuelve a %v, esperaba %v", pos, quiero)
	}
}

// El aviso en el flujo del diálogo es accesible también con Tab, sin salir de él.
func TestElAvisoParticipaEnElFocoDelDialogo(t *testing.T) {
	pila := pilaQueSeVacia(t)
	w := test.NewTempWindow(t, container.NewStack(canvas.NewRectangle(tema.Color("bg", tema.Claro)), pila))
	w.Resize(fyne.NewSize(1280, 800))
	a := NuevoAviso(AvisoPeligro, "No se ha podido guardar", "")
	a.RotuloAccion = "Reintentar"
	pila.Notificar(a)
	r := test.WidgetRenderer(a).(*avisoRenderer)
	delAviso := func(f fyne.Focusable) bool {
		return f == fyne.Focusable(a.aspa) || f == fyne.Focusable(r.accion)
	}

	vistos := 0
	for range 4 {
		w.Canvas().FocusNext()
		if delAviso(w.Canvas().Focused()) {
			vistos++
		}
	}
	if vistos == 0 {
		t.Fatal("sin diálogo, el aspa y la acción del aviso tienen que ser paradas del Tab")
	}

	d := NuevoDialogo("Reglas", container.New(Columna{}, NuevoBoton("Uno", nil), NuevoBoton("Dos", nil)))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	vistos = 0
	for range 12 {
		w.Canvas().FocusNext()
		if delAviso(w.Canvas().Focused()) {
			vistos++
		}
	}
	if vistos == 0 {
		t.Fatal("el aviso insertado en el diálogo no se puede recorrer con Tab")
	}
	test.Tap(a.aspa)
	if len(pila.Avisos()) != 0 {
		t.Error("fuera del Tab, el aspa del aviso tiene que seguir cerrándolo con el ratón")
	}
}

// TestLaPilaDeAvisosNoRetieneSuVentana: el registro de pilas por ventana no le alarga la
// vida a la ventana. Con un mapa normal, cada ventana que tuvo avisos se quedaba viva
// para siempre con su contenido y sus capas: +280 MB por pasada en los tests de Mail, y
// en la app, cada ventana cerrada. VISTO EN ROJO con el `sync.Map` de lienzo a pila.
func TestLaPilaDeAvisosNoRetieneSuVentana(t *testing.T) {
	lienzo, _ := func() (weak.Pointer[byte], bool) {
		p := NuevaPilaAvisos()
		w := test.NewWindow(p)
		p.registrar()
		if pilaDe(w.Canvas()) != p {
			t.Fatal("la pila no se ha registrado en su ventana")
		}
		defer w.Close()
		return claveLienzo(w.Canvas())
	}()
	for range 3 {
		runtime.GC()
	}
	if lienzo.Value() != nil {
		t.Error("la ventana cerrada sigue viva: algo la retiene")
	}
}
