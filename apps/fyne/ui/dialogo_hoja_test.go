// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// telefono pone el «¿con el dedo?» de los guardianes mientras dura el test.
func telefono(t *testing.T) {
	t.Helper()
	si := true
	antes := tactilDePrueba
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = antes })
}

// TestEnElTelefonoElDialogoEsUnaHojaConAsa: por debajo de 768 todo diálogo —centrado o
// lateral— es la hoja de Modal.tsx: a todo el ancho, apoyada abajo, redonda solo arriba
// y con el asa si se puede cerrar con un clic fuera. VISTO EN ROJO con la tarjeta
// centrada de antes (x=16, y a media pantalla).
func TestEnElTelefonoElDialogoEsUnaHojaConAsa(t *testing.T) {
	telefono(t)
	for _, lateral := range []bool{false, true} {
		w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
		w.Resize(fyne.NewSize(390, 844))
		d := NuevoDialogo("Mover a la papelera", cuerpoDe(40))
		if lateral {
			d.ALaDerecha()
		}
		d.Mostrar(w.Canvas())
		if d.Position().X != 0 || d.Size().Width != 390 {
			t.Errorf("lateral=%t: la hoja va de %v a %v y es a todo el ancho", lateral, d.Position().X, d.Size().Width)
		}
		if fin := d.Position().Y + d.Size().Height; fin != 844 {
			t.Errorf("lateral=%t: la hoja acaba en %v y se apoya abajo (844)", lateral, fin)
		}
		r := test.WidgetRenderer(d).(*dialogoRenderer)
		if !contiene(r.Objects(), r.asa) || r.asa.Size() != fyne.NewSize(36, 4) {
			t.Errorf("lateral=%t: la hoja no lleva el asa de 36×4 (%v)", lateral, r.asa.Size())
		}
		for _, o := range r.caja.Objetos() {
			rect := o.(*fcanvas.Rectangle)
			if rect.BottomLeftCornerRadius > 0.01 || rect.BottomRightCornerRadius > 0.01 || rect.CornerRadius == 0 {
				t.Errorf("lateral=%t: la hoja redondea abajo (%v/%v) o no arriba (%v)", lateral,
					rect.BottomLeftCornerRadius, rect.BottomRightCornerRadius, rect.CornerRadius)
			}
		}
		if s := d.capa.sombra; s != nil && s.Objeto().Visible() {
			t.Errorf("lateral=%t: la hoja lleva la sombra de la tarjeta", lateral)
		}
		d.Cerrar()
	}
	// Sin cierre por clic fuera, sin asa: un asa que no arrastra promete algo que no hace.
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(390, 844))
	d := NuevoDialogo("Subiendo 240 ficheros", cuerpoDe(40))
	d.NoDescartable = true
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	if r := test.WidgetRenderer(d).(*dialogoRenderer); contiene(r.Objects(), r.asa) {
		t.Error("un diálogo que no se cierra con un clic fuera lleva asa")
	}
}

// TestElPieDelTelefonoVaATodoElAncho: el `PIE_TELEFONO` de Modal.tsx. En la hoja, dos
// acciones a partes iguales, 44 de alto y el principal a la derecha; con tres, la primera
// sola arriba a todo el ancho; un rótulo que no cabe en media fila baja a la suya. En
// escritorio no cambia: a la derecha y cada una con su ancho, y la de `Izquierda`, al otro
// lado. Medido en Chromium a 390×844: 167 + 8 + 167 en los 342 del pie.
func TestElPieDelTelefonoVaATodoElAncho(t *testing.T) {
	boton := func(s string) *Boton { return NuevoBoton(s, nil).Con(Secundario, TallaMD) }
	abrir := func(ancho float32, pie fyne.CanvasObject) *Dialogo {
		w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
		w.Resize(fyne.NewSize(ancho, 844))
		d := NuevoDialogo("Editar evento", cuerpoDe(40)).Con(DialogoMD, pie)
		d.Mostrar(w.Canvas())
		t.Cleanup(d.Cerrar)
		return d
	}
	fila := func(bs ...fyne.CanvasObject) fyne.CanvasObject {
		return container.New(Fila{Hueco: HuecoPieDialogo}, bs...)
	}
	util := float32(390 - 2*24) // la hoja no lleva filetes a los lados
	mitad := (util - HuecoPieDialogo) / 2

	cancelar, guardar := boton("Cancelar"), boton("Guardar")
	abrir(390, fila(cancelar, guardar))
	if cancelar.Size() != fyne.NewSize(mitad, 44) || guardar.Size() != fyne.NewSize(mitad, 44) {
		t.Errorf("dos acciones miden %v y %v; quiero %v×44 cada una", cancelar.Size(), guardar.Size(), mitad)
	}
	if guardar.Position().X != mitad+HuecoPieDialogo || guardar.Position().Y != cancelar.Position().Y {
		t.Errorf("el principal está en %v y va a la derecha de «Cancelar»", guardar.Position())
	}

	eliminar, c2, g2 := boton("Eliminar"), boton("Cancelar"), boton("Guardar")
	abrir(390, fila(eliminar, c2, g2))
	if eliminar.Size().Width != util || eliminar.Position().Y >= c2.Position().Y {
		t.Errorf("con tres, «Eliminar» mide %v en y=%v y va sola arriba (%v de ancho)", eliminar.Size().Width, eliminar.Position().Y, util)
	}
	if c2.Size().Width != mitad || g2.Size().Width != mitad || c2.Position().Y != g2.Position().Y {
		t.Errorf("con tres, la pareja de abajo mide %v y %v; quiero %v cada una", c2.Size().Width, g2.Size().Width, mitad)
	}

	c3, largo := boton("Cancelar"), boton("Eliminar definitivamente las conversaciones")
	abrir(390, fila(c3, largo))
	if largo.MinSize().Width <= mitad {
		t.Fatalf("el rótulo largo mide %v y no prueba nada en media fila (%v)", largo.MinSize().Width, mitad)
	}
	if c3.Size().Width != util || largo.Size().Width != util || largo.Position().Y <= c3.Position().Y {
		t.Errorf("un rótulo que no cabe: %v y %v en y=%v/%v; quiero cada una en su fila a %v",
			c3.Size().Width, largo.Size().Width, c3.Position().Y, largo.Position().Y, util)
	}

	borrar, c4, g4 := boton("Eliminar"), boton("Cancelar"), boton("Guardar")
	d := abrir(1280, container.New(PieAcciones{Izquierda: 1}, borrar, c4, g4))
	dentro := d.Size().Width - 2*tema.Borde - 2*24
	if c4.Size() != c4.MinSize() || g4.Size() != g4.MinSize() {
		t.Errorf("en escritorio las acciones cambian de medida: %v y %v", c4.Size(), g4.Size())
	}
	if fin := g4.Position().X + g4.Size().Width; fin != dentro || borrar.Position().X != 0 {
		t.Errorf("en escritorio el principal acaba en %v (quiero %v) y «Eliminar» empieza en %v (quiero 0)", fin, dentro, borrar.Position().X)
	}

	solo := boton("Cerrar")
	abrir(390, solo)
	if solo.Size() != fyne.NewSize(util, 44) {
		t.Errorf("un botón suelto en el pie del teléfono mide %v; quiero %v×44", solo.Size(), util)
	}
	soloE := boton("Cerrar")
	abrir(1280, soloE)
	if soloE.Size() != soloE.MinSize() {
		t.Errorf("un botón suelto en escritorio cambia de medida: %v", soloE.Size())
	}
}

// TestBajarLaHojaPorLaCabeceraLaCierra: más de 80 px cierra, menos la devuelve a su
// sitio, y un arrastre que empieza en el cuerpo no es de la hoja. VISTO EN ROJO sin
// `Dragged`: el asa no hacía nada.
func TestBajarLaHojaPorLaCabeceraLaCierra(t *testing.T) {
	telefono(t)
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(390, 844))
	cierres := 0
	d := NuevoDialogo("Compartir", cuerpoDe(200))
	d.AlCerrar = func() { cierres++ }
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	y0 := d.Position().Y
	tirar := func(desde, cuanto float32) {
		d.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, desde+cuanto)}, Dragged: fyne.Delta{DY: cuanto}})
	}
	tirar(10, 40)
	if d.Position().Y != y0+40 {
		t.Errorf("la hoja no sigue al dedo: y=%v y tocaba %v", d.Position().Y, y0+40)
	}
	d.inicioArrastre = time.Now().Add(-time.Second) // despacio: no es un tirón
	d.DragEnd()
	if cierres != 0 || d.Position().Y != y0 {
		t.Errorf("un tirón de 40 ha cerrado (%d) o no ha devuelto la hoja (y=%v)", cierres, d.Position().Y)
	}
	tirar(10, 120)
	d.DragEnd()
	if cierres != 1 {
		t.Errorf("bajar 120 por la cabecera ha cerrado %d veces y es una", cierres)
	}
	// Empezado en el cuerpo, el gesto es del desplazamiento.
	tirar(150, 120)
	d.DragEnd()
	if cierres != 1 {
		t.Error("un arrastre que empieza en el cuerpo ha cerrado la hoja")
	}
	// Un TIRÓN: 40 px en 40 ms (1 px/ms) cierra aunque no llegue a 80, como las hojas
	// del sistema. VISTO EN ROJO con la regla de solo distancia.
	tirar(10, 40)
	d.inicioArrastre = time.Now().Add(-40 * time.Millisecond)
	d.DragEnd()
	if cierres != 2 {
		t.Errorf("un tirón rápido de 40 px ha cerrado %d veces en total y son dos", cierres)
	}
}

// TestLaReglaDelTironEsLaDeModalTsx: distancia y velocidad, leídas del fuente de la web.
func TestLaReglaDelTironEsLaDeModalTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Modal.tsx")
	for _, c := range []struct {
		patron string
		valor  float32
	}{
		{`const CIERRE_ARRASTRE = (\d+(?:\.\d+)?);`, CierreArrastre},
		{`const VELOCIDAD_CIERRE = (\d+(?:\.\d+)?);`, VelocidadCierre},
		{`const TIRON_MINIMO = (\d+(?:\.\d+)?);`, TironMinimo},
	} {
		m := regexp.MustCompile(c.patron).FindStringSubmatch(fuente)
		if m == nil {
			t.Errorf("Modal.tsx no declara %s", c.patron)
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 32)
		if float32(v) != c.valor {
			t.Errorf("%s: la web dice %v y Fyne %v", c.patron, v, c.valor)
		}
	}
	for _, c := range []struct {
		dy   float32
		dur  time.Duration
		cier bool
	}{
		{81, time.Second, true},           // lejos, aunque despacio
		{40, 40 * time.Millisecond, true}, // tirón
		{40, time.Second, false},          // despacio y corto
		{20, 5 * time.Millisecond, false}, // tiembla
		{40, 0, false},                    // sin tiempo medido
	} {
		if got := cierraArrastre(c.dy, c.dur); got != c.cier {
			t.Errorf("dy=%v en %v cierra=%v y tocaba %v", c.dy, c.dur, got, c.cier)
		}
	}
}

// TestLaHojaSeApoyaSobreElTeclado: con el teclado fuera, la hoja acaba ENCIMA de él y
// el campo enfocado queda a la vista, como en la web (la capa mide el viewport visual).
// Al irse, vuelve abajo. VISTO EN ROJO sin `tapadoPorTeclado`: la hoja seguía apoyada en
// el borde de la pantalla y el campo, a 700, debajo de un teclado que empieza en 544.
func TestLaHojaSeApoyaSobreElTeclado(t *testing.T) {
	telefono(t)
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(390, 844))
	campo := NuevaEntrada("Asunto")
	cuerpo := container.New(Columna{Hueco: 8}, cuerpoDe(600), campo)
	guardar := NuevoBoton("Guardar", nil)
	d := NuevoDialogo("Nuevo evento", cuerpo).Con(DialogoMD, guardar)
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	w.Canvas().Focus(campo.editor)

	const teclado = 300
	d.ponerTeclado(w.Canvas(), teclado)
	arribaTeclado := float32(844 - teclado)
	if fin := d.Position().Y + d.Size().Height; fin != arribaTeclado {
		t.Errorf("con el teclado la hoja acaba en %v y tiene que apoyarse en él (%v)", fin, arribaTeclado)
	}
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
	if fin := abs(campo).Y + campo.Size().Height; fin > arribaTeclado {
		t.Errorf("el campo enfocado acaba en %v, debajo del teclado (%v)", fin, arribaTeclado)
	}
	if fin := abs(guardar).Y + guardar.Size().Height; fin > arribaTeclado {
		t.Errorf("«Guardar» acaba en %v, debajo del teclado (%v)", fin, arribaTeclado)
	}
	d.ponerTeclado(w.Canvas(), 0)
	if fin := d.Position().Y + d.Size().Height; fin != 844 {
		t.Errorf("sin teclado la hoja acaba en %v y vuelve abajo (844)", fin)
	}
}

// TestConElDedoAbrirNoEnfocaElCampo: el teclado no tiene que tapar la hoja antes de que
// nadie decida escribir. VISTO EN ROJO con el foco en el primer campo, que es lo que
// sigue haciendo con el ratón (TestElFocoInicialCaeEnElPrimerCampo).
func TestConElDedoAbrirNoEnfocaElCampo(t *testing.T) {
	telefono(t)
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(390, 844))
	campo := NuevaEntrada("Título")
	d := NuevoDialogo("Nuevo evento", campo)
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	if f := w.Canvas().Focused(); f == fyne.Focusable(campo) {
		t.Error("con el dedo, abrir el diálogo ha enfocado el campo y sacaría el teclado")
	}
}

// TestLaHojaSubeComoEnLaHoja: `sytue-hoja-in` de styles.css, 220 ms desde
// `translateY(100%)`, solo por debajo de 48rem.
func TestLaHojaSubeComoEnLaHoja(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/ui/src/styles.css")
	if !regexp.MustCompile(`@media \(width < 48rem\)\s*\{\s*\.sytue-hoja-movil\s*\{\s*animation: sytue-hoja-in 220ms var\(--ease-standard\) backwards;`).MatchString(hoja) ||
		DuracionHoja.Milliseconds() != 220 {
		t.Error("styles.css ya no sube la hoja en 220 ms por debajo de 48rem, o DuracionHoja no lo dice")
	}
	if !regexp.MustCompile(`sytue-hoja-in\s*\{\s*from\s*\{\s*transform: translateY\(100%\)`).MatchString(hoja) {
		t.Error("`sytue-hoja-in` ya no entra desde translateY(100%)")
	}
	if Estrecho != 48*16 {
		t.Errorf("la hoja de la web empieza por debajo de 48rem (768) y aquí por debajo de %v", Estrecho)
	}
}

// TestUnBotonDelPieQueCambiaDeRotuloCrece: «Eliminar» → «Confirmar eliminación» con
// Recolocar. VISTO EN ROJO sin repartir el pie en Recolocar: el pie medía lo mismo, no
// se recolocaba y el botón seguía con el ancho de «Eliminar» («Confirmar» a secas).
func TestUnBotonDelPieQueCambiaDeRotuloCrece(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	borrar := NuevoBoton("Eliminar", nil).Con(Peligro, TallaMD)
	cancelar := NuevoBoton("Cancelar", nil).Con(Secundario, TallaMD)
	d := NuevoDialogo("Editar contacto", cuerpoDe(40)).Con(DialogoMD,
		container.New(PieAcciones{Izquierda: 1}, borrar, cancelar))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	borrar.Texto = "Confirmar eliminación"
	borrar.Refresh()
	d.Recolocar()
	if borrar.Size().Width < borrar.MinSize().Width {
		t.Errorf("el botón mide %v y su rótulo pide %v", borrar.Size().Width, borrar.MinSize().Width)
	}
}

// TestUnRefreshDelDialogoVuelveAMedirlo: muchas pantallas cambian el cuerpo y llaman a
// d.Refresh(). VISTO EN ROJO sin volver a colocar la capa en el Refresh del renderer:
// el panel se quedaba con el alto del cuerpo de antes (Notepads, «Etiquetas»: 106 px).
func TestUnRefreshDelDialogoVuelveAMedirlo(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	cuerpo := container.New(Columna{}, cuerpoDe(40))
	d := NuevoDialogo("Etiquetas", cuerpo).Con(DialogoMD, nil)
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	antes := d.Size().Height
	cuerpo.Add(cuerpoDe(200))
	d.Refresh()
	if got := d.Size().Height; got < antes+200 {
		t.Errorf("con 200 más de cuerpo el panel mide %v (antes %v)", got, antes)
	}
}
