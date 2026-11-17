// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// botonConPista monta un BotonIcono suelto en una ventana de 400 × 200, en `pos`, con el
// retardo de grupo a cero para que ninguna pista cerrada por otro test lo abra al vuelo.
func botonConPista(t *testing.T, pos fyne.Position) (*BotonIcono, fyne.Window) {
	t.Helper()
	ultimoCierrePista = time.Time{}
	b := NuevoBotonIcono(iconos.Lapiz, "Archivar la conversación", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(b))
	w.Resize(fyne.NewSize(400, 200))
	b.Resize(b.MinSize())
	b.Move(pos)
	return b, w
}

// esperarReloj deja correr `d` y el `fyne.Do` del reloj de la pista, que se ejecuta en la
// rutina del test.
func esperarReloj(d time.Duration) {
	time.Sleep(d)
	fyne.DoAndWait(func() {})
}

// textoDeLaPista es el rectángulo de la primera línea de la burbuja, en coordenadas de
// la ventana.
func textoDeLaPista(p *Pista) (fyne.Position, fyne.Size) {
	l := p.r.lineas[0]
	return absDe(p, l), l.Size()
}

// absDe es dónde cae en la ventana una pieza de la burbuja: la pista está en el árbol,
// pero la pieza puede estar escondida mientras la burbuja entra con su fundido, y el
// driver solo encuentra lo visible.
func absDe(p *Pista, o fyne.CanvasObject) fyne.Position {
	if p.Portal != nil {
		return fyne.CurrentApp().Driver().AbsolutePositionForObject(p.Portal).Add(o.Position())
	}
	return fyne.CurrentApp().Driver().AbsolutePositionForObject(p).Add(o.Position())
}

// TestElBotonIconoEnseñaSuPistaDeSerie es el `showTitle` de IconButton.tsx, que viene
// encendido: con el puntero encima la burbuja sale a los 300 ms y no antes, se va al
// salir, y no monta capa ni le quita el `:hover` al botón. VISTO EN ROJO sin la pista
// de serie: el botón no decía nada al pasar.
func TestElBotonIconoEnseñaSuPistaDeSerie(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(100, 100))
	arriba := fyne.CurrentApp().Driver().AbsolutePositionForObject(b) // la ventana de pruebas tiene su relleno
	test.MoveMouse(w.Canvas(), arriba.AddXY(10, 10))
	if !b.encima {
		t.Fatal("el puntero no ha llegado al botón: la pista se lo ha quitado")
	}
	p := b.ayudaAbierta()
	if p == nil {
		t.Fatal("el botón no ha creado su pista al pasar el puntero")
	}
	if p.Rotulo != b.Rotulo {
		t.Errorf("la pista dice %q y el rótulo del botón es %q", p.Rotulo, b.Rotulo)
	}
	esperarReloj(RetardoPista / 2)
	if p.Abierta() {
		t.Error("la pista se ha abierto antes de los 300 ms")
	}
	esperarReloj(RetardoPista/2 + 150*time.Millisecond)
	if !p.Abierta() {
		t.Fatal("con el puntero encima, la pista no se ha abierto a los 300 ms")
	}
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Errorf("la pista ha montado %d capas", n)
	}
	// La burbuja va ENCIMA del botón y centrada en él.
	pos, tam := textoDeLaPista(p)
	if pos.Y+tam.Height > arriba.Y-SeparacionPista {
		t.Errorf("el texto de la burbuja acaba en y=%v; va encima del botón, antes de %v", pos.Y+tam.Height, arriba.Y-SeparacionPista)
	}
	if centro := pos.X + tam.Width/2; centro < arriba.X || centro > arriba.X+b.Size().Width {
		t.Errorf("el texto de la burbuja tiene su centro en x=%v, fuera del botón", centro)
	}
	test.MoveMouse(w.Canvas(), fyne.NewPos(390, 190))
	if p.Abierta() {
		t.Error("al salir el puntero, la pista sigue abierta")
	}
}

// TestLaPistaDelBotonIconoSaleAlInstanteConElFoco: quien navega con Tab ya ha decidido
// mirar ahí. Y Escape la quita sin mover el foco (WCAG 1.4.13).
func TestLaPistaDelBotonIconoSaleAlInstanteConElFoco(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(100, 100))
	w.Canvas().Focus(b)
	p := b.ayudaAbierta()
	if p == nil || !p.Abierta() {
		t.Fatal("con el foco en el botón, la pista no sale al instante")
	}
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if p.Abierta() || w.Canvas().Focused() != fyne.Focusable(b) {
		t.Errorf("Escape: abierta=%v y el foco en %T; tiene que cerrarla sin mover el foco", p.Abierta(), w.Canvas().Focused())
	}
	w.Canvas().Unfocus()
	w.Canvas().Focus(b)
	if !p.Abierta() {
		t.Error("al volver a enfocarlo la pista no sale")
	}
	w.Canvas().Unfocus()
	if p.Abierta() {
		t.Error("al perder el foco la pista sigue abierta")
	}
}

// TestSinPistaYConPistaExplicitaNoHayPistaDeSerie: `showTitle={false}` la apaga, y la que
// lo envuelve manda sobre la de serie (dos burbujas a la vez serían una de más).
func TestSinPistaYConPistaExplicitaNoHayPistaDeSerie(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(100, 100))
	b.SinPista = true
	w.Canvas().Focus(b)
	if b.propia != nil {
		t.Error("con SinPista el botón ha creado su pista")
	}
	w.Canvas().Unfocus()

	ultimoCierrePista = time.Time{}
	c := NuevoBotonIcono(iconos.Lapiz, "Archivar", nil)
	explicita := NuevaPista("Archivar y pasar a la siguiente", c)
	w2 := test.NewTempWindow(t, container.NewWithoutLayout(explicita))
	w2.Resize(fyne.NewSize(400, 200))
	explicita.Resize(explicita.MinSize())
	explicita.Move(fyne.NewPos(100, 100))
	w2.Canvas().Focus(c)
	if c.propia != nil {
		t.Error("envuelto con NuevaPista, el botón ha creado además la suya")
	}
	if !explicita.Abierta() {
		t.Error("la pista explícita no se abre con el foco")
	}
}

// TestUnBotonIconoInerteNoTienePistaYUnoApagadoSi: el `disabled` es `pointer-events-none`
// (ni `mouseenter` ni foco), pero el `aria-disabled` está para decir por qué.
func TestUnBotonIconoInerteNoTienePistaYUnoApagadoSi(t *testing.T) {
	b, _ := botonConPista(t, fyne.NewPos(100, 100))
	b.Inerte = true
	b.MouseIn(nil)
	if b.propia != nil {
		t.Error("un botón inerte ha creado su pista")
	}
	b.MouseOut()
	b.Inerte, b.Apagado = false, true
	b.FocusGained()
	if p := b.ayudaAbierta(); p == nil || !p.Abierta() {
		t.Error("un botón apagado no enseña su pista, que es donde dice por qué")
	}
}

// TestPulsarElBotonIconoCierraSuPista es el `onPointerDown` de Tooltip.tsx: ya se ha
// elegido, y si la pulsación cambia de pantalla la burbuja se quedaría huérfana.
func TestPulsarElBotonIconoCierraSuPista(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(100, 100))
	pulsado := 0
	b.AlPulsar = func() { pulsado++ }
	w.Canvas().Focus(b)
	test.Tap(b)
	if pulsado != 1 {
		t.Fatalf("el botón ha recibido %d pulsaciones con la pista abierta", pulsado)
	}
	if b.ayudaAbierta().Abierta() {
		t.Error("tras pulsar el botón su pista sigue abierta")
	}
}

// TestLaPistaDelBotonIconoSigueAlRotulo: el rótulo de un conmutador cambia con su estado
// («Fijar» / «Dejar de fijar»), y la burbuja tiene que decir el de ahora.
func TestLaPistaDelBotonIconoSigueAlRotulo(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(100, 100))
	w.Canvas().Focus(b)
	b.Rotulo = "Dejar de archivar"
	b.Refresh()
	p := b.ayudaAbierta()
	if p.Rotulo != "Dejar de archivar" || p.r.lineas[0].Text != "Dejar de archivar" {
		t.Errorf("la pista dice %q tras cambiar el rótulo a «Dejar de archivar»", p.r.lineas[0].Text)
	}
}

// TestLaPistaSeVolteaYSeAcotaALaVentana es `colocarTooltip`: un botón pegado arriba la
// tiene debajo, y uno pegado a la izquierda la tiene dentro de la ventana con 8 px de
// aire, con la flecha apuntando todavía al botón. VISTO EN ROJO sin el volteo: la burbuja
// salía por encima de la ventana.
func TestLaPistaSeVolteaYSeAcotaALaVentana(t *testing.T) {
	b, w := botonConPista(t, fyne.NewPos(0, 2))
	w.Canvas().Focus(b)
	p := b.ayudaAbierta()
	pos, _ := textoDeLaPista(p)
	ab := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	if pos.Y < ab.Y+b.Size().Height+SeparacionPista {
		t.Errorf("con el botón pegado arriba el texto está en y=%v; tenía que voltearse debajo, desde %v",
			pos.Y, ab.Y+b.Size().Height+SeparacionPista)
	}
	if x := absDe(p, p.r.burbuja.Objetos()[0]).X; x < MargenPista-0.5 {
		t.Errorf("la burbuja empieza en x=%v y tiene que dejar %v de aire con la ventana", x, MargenPista)
	}
	f := absDe(p, p.r.flecha)
	if centro := f.X + p.r.flecha.Size().Width/2; centro < ab.X || centro > ab.X+b.Size().Width {
		t.Errorf("la flecha apunta a x=%v, fuera del botón (%v–%v)", centro, ab.X, ab.X+b.Size().Width)
	}
	if f.Y > pos.Y {
		t.Errorf("volteada debajo, la flecha (y=%v) tiene que asomar por ARRIBA de la burbuja", f.Y)
	}
}

// TestColocarPistaEsColocarTooltip ata la cuenta a la de la web: preferido, volteo al
// opuesto y acotado, y la derecha que pasa a la izquierda si no cabe.
func TestColocarPistaEsColocarTooltip(t *testing.T) {
	ancla, m, ventana := fyne.NewSize(28, 28), fyne.NewSize(100, 24), fyne.NewSize(400, 300)
	casos := []struct {
		nombre string
		abs    fyne.Position
		pide   ladoPista
		lado   ladoPista
	}{
		{"arriba con sitio", fyne.NewPos(150, 150), pistaArriba, pistaArriba},
		{"arriba sin sitio", fyne.NewPos(150, 10), pistaArriba, pistaAbajo},
		{"abajo sin sitio", fyne.NewPos(150, 260), pistaAbajo, pistaArriba},
		{"derecha con sitio", fyne.NewPos(10, 150), pistaDerecha, pistaDerecha},
		{"derecha sin sitio", fyne.NewPos(360, 150), pistaDerecha, pistaIzquierda},
	}
	for _, c := range casos {
		pos, lado, flecha := colocarPista(ancla, m, c.abs, ventana, c.pide)
		if lado != c.lado {
			t.Errorf("%s: lado %v, quiero %v", c.nombre, lado, c.lado)
		}
		absX, absY := c.abs.X+pos.X, c.abs.Y+pos.Y
		if absX < MargenPista || absX+m.Width > ventana.Width-MargenPista || absY < MargenPista-0.5 && lado != pistaArriba {
			t.Errorf("%s: la burbuja en (%v, %v) se sale de la ventana", c.nombre, absX, absY)
		}
		if flecha < 6 {
			t.Errorf("%s: la flecha a %v del borde; el mínimo son 6", c.nombre, flecha)
		}
	}
	// Sin ventana no hay límites: arriba y centrada, como el `absolute` de la v1.
	pos, lado, flecha := colocarPista(ancla, m, fyne.Position{}, fyne.Size{}, pistaArriba)
	if lado != pistaArriba || pos.X != (28-100)/2 || pos.Y != -24-SeparacionPista || flecha != 50 {
		t.Errorf("sin ventana: %v %v %v", pos, lado, flecha)
	}
}

// TestElRetardoDeGrupoAbreLaSiguienteAlInstante: recorrer una barra de iconos con el
// ratón no cuesta 300 ms por icono.
func TestElRetardoDeGrupoAbreLaSiguienteAlInstante(t *testing.T) {
	a, w := botonConPista(t, fyne.NewPos(100, 100))
	b := NuevoBotonIcono(iconos.Papelera, "Borrar", nil)
	w.SetContent(container.NewWithoutLayout(a, b))
	w.Resize(fyne.NewSize(400, 200))
	a.Resize(a.MinSize())
	a.Move(fyne.NewPos(100, 100))
	b.Resize(b.MinSize())
	b.Move(fyne.NewPos(200, 100))
	w.Canvas().Focus(a)
	w.Canvas().Unfocus() // se cierra la de `a`: arranca la ventana de grupo
	test.MoveMouse(w.Canvas(), fyne.CurrentApp().Driver().AbsolutePositionForObject(b).AddXY(5, 5))
	if !b.encima {
		t.Fatal("el puntero no ha llegado al segundo botón")
	}
	if p := b.ayudaAbierta(); p == nil || !p.Abierta() {
		t.Errorf("justo después de cerrarse otra pista, la siguiente no abre al instante (%v)", time.Since(ultimoCierrePista))
	}
}

// TestLaPistaDelBotonIconoVaAlPortalDeLaPantalla: con portal, la burbuja sale de lo que
// recorta y se pinta encima de todo; con una capa arriba se queda junto al botón, porque
// el portal va debajo de las capas.
func TestLaPistaDelBotonIconoVaAlPortalDeLaPantalla(t *testing.T) {
	ultimoCierrePista = time.Time{}
	b := NuevoBotonIcono(iconos.Lapiz, "Archivar", nil)
	portal := container.NewWithoutLayout()
	w := test.NewTempWindow(t, container.NewStack(container.NewWithoutLayout(b), portal))
	w.Resize(fyne.NewSize(400, 200))
	b.Resize(b.MinSize())
	b.Move(fyne.NewPos(100, 100))
	portalesPista.poner(w.Canvas(), portal)
	t.Cleanup(func() { portalesPista.quitar(w.Canvas()) })

	w.Canvas().Focus(b)
	if len(portal.Objects) == 0 {
		t.Fatal("con portal en la pantalla, la burbuja no ha ido a él")
	}
	var texto *canvas.Text
	for _, o := range portal.Objects {
		if tx, es := o.(*canvas.Text); es {
			texto = tx
		}
	}
	if arriba := fyne.CurrentApp().Driver().AbsolutePositionForObject(b).Y; texto == nil || absDe(b.propia, texto).Y > arriba-SeparacionPista {
		t.Errorf("en el portal el texto no va encima del botón: %v", texto)
	}
	w.Canvas().Unfocus()
	if len(portal.Objects) != 0 {
		t.Errorf("al cerrarse, la pista ha dejado %d objetos en el portal", len(portal.Objects))
	}

	// Con una capa encima, la pista va junto al botón.
	capa := canvas.NewRectangle(nil)
	w.Canvas().Overlays().Add(capa)
	ultimoCierrePista = time.Time{}
	b.FocusGained()
	if len(portal.Objects) != 0 || b.propia.Portal != nil {
		t.Error("con una capa arriba, la burbuja ha ido al portal, que está debajo de ella")
	}
	b.FocusLost()
	w.Canvas().Overlays().Remove(capa)
}
