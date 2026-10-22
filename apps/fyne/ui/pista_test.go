// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElContratoDeLaPistaEsElDeTooltipTsx ata la pista nativa a `Tooltip.tsx`,
// que es la referencia: si la web cambia el retardo o el ancho máximo, este test
// se pone en rojo en vez de dejar que las dos se separen en silencio.
func TestElContratoDeLaPistaEsElDeTooltipTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Tooltip.tsx")
	for _, c := range []struct{ trozo, porque string }{
		{"setTimeout(() => setOpen(true), 300)", "el retardo del ratón"},
		{"max-w-[240px]", "el tope de ancho"},
		{"sytue-tooltip-surface px-2 py-1 text-xs font-medium", "la superficie invertida y el relleno"},
		{"rounded-md", "la esquina de 6"},
		{"bottom-[calc(100%+6px)]", "los 6 px de separación"},
		{"h-1.5 w-1.5 -translate-x-1/2 rotate-45", "el rombo de 5,25"},
		{"-bottom-[3px]", "los 3 px que la flecha asoma"},
		{"if (e.key === 'Escape') hide()", "que Escape la quita"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("Tooltip.tsx ya no dice %q (%s)", c.trozo, c.porque)
		}
	}
	if RetardoPista.Milliseconds() != 300 || SeparacionPista != 6 || AnchoMaxPista != 240 {
		t.Errorf("la pista dice %v/%v/%v y la web dice 300 ms, 6 y 240",
			RetardoPista, SeparacionPista, AnchoMaxPista)
	}
}

// TestLaBurbujaDePistaMide21: `py-1` dos veces más la caja de línea de `text-xs`, que
// con el rem de 14 son 3,5 + 14 + 3,5.
func TestLaBurbujaDePistaMide21(t *testing.T) {
	const quiero = 2*4 + 16
	if got := MedidaPista("Marcar como leído").Height; got != quiero {
		t.Errorf("la burbuja mide %v de alto y `py-1` con `text-xs` dan %v", got, float32(quiero))
	}
	// Y el tope de 240 recorta un rótulo largo.
	largo := strings.Repeat("Enviar una copia a la libreta compartida ", 3)
	if got := MedidaPista(largo).Width; got != AnchoMaxPista {
		t.Errorf("un rótulo largo da %v de ancho y el tope son %v", got, AnchoMaxPista)
	}
	// …y pasa de línea dentro de él, en vez de salirse de la burbuja. VISTO EN ROJO con
	// la burbuja de una sola línea: 21 de alto y el texto por fuera.
	if n := len(lineasPista(largo)); n < 2 || MedidaPista(largo).Height != float32(n)*16+2*4 {
		t.Errorf("el rótulo largo va en %d líneas y la burbuja mide %v de alto", n, MedidaPista(largo).Height)
	}
}

// TestLaPistaNoEmpujaAlControl: la burbuja va `absolute` en la web, así que no ocupa
// sitio en el flujo. Si lo ocupara, envolver un botón con su ayuda movería de sitio la
// barra entera.
func TestLaPistaNoEmpujaAlControl(t *testing.T) {
	boton := NuevoBoton("Archivar", nil)
	p := NuevaPista("Archivar la conversación", boton)
	if got, quiero := p.MinSize(), boton.MinSize(); got != quiero {
		t.Errorf("la pista mide %v y el control solo mide %v: la burbuja está empujando", got, quiero)
	}
}

// TestLaPistaSeAbreYSeCierraSinCapa: como los avisos, la pista NO monta capa, o
// bloquearía el puntero de toda la aplicación mientras se ve.
func TestLaPistaSeAbreYSeCierraSinCapa(t *testing.T) {
	pulsado := 0
	boton := NuevoBoton("Archivar", func() { pulsado++ })
	p := NuevaPista("Archivar la conversación", boton)
	w := test.NewTempWindow(t, container.NewWithoutLayout(p))
	w.Resize(fyne.NewSize(400, 200))
	p.Resize(p.MinSize())
	p.Move(fyne.NewPos(100, 100))

	if p.Abierta() {
		t.Error("la pista nace abierta")
	}
	p.Abrir() // es lo que hace el foco de teclado: al instante, sin retardo
	if !p.Abierta() {
		t.Fatal("Abrir no ha abierto la pista")
	}
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Errorf("la pista ha montado %d capas: con una, la aplicación se queda sin puntero", n)
	}
	// Y el control de debajo sigue respondiendo con la burbuja a la vista.
	test.Tap(boton)
	if pulsado != 1 {
		t.Errorf("con la pista abierta, el control ha recibido %d pulsaciones", pulsado)
	}

	p.Cerrar()
	if p.Abierta() {
		t.Error("Cerrar no ha cerrado la pista")
	}
}

// TestLaBurbujaSePintaFueraDelControl, que es lo que hace el `bottom-[calc(100%+6px)]`,
// y con la flecha asomando por debajo.
func TestLaBurbujaSePintaFueraDelControl(t *testing.T) {
	// El control es un rectángulo de `bg-subtle` y NO un botón: el relleno del botón
	// principal es la MISMA tinta que el texto (`primary` y `text` son los dos
	// accent-800), así que rastrear la tinta encontraría el botón además de la
	// burbuja y la medida saldría de donde no es.
	p := NuevaPista("Archivar", cuerpoDe(28))
	p.Abrir()
	tam := p.MinSize()
	p.Resize(tam)

	// Se pinta con hueco de sobra por arriba y se DESPLAZA el conjunto, porque la
	// burbuja vive en coordenadas negativas respecto al control.
	m := MedidaPista(p.Rotulo)
	hueco := m.Height + SeparacionPista + 12
	desplazado := container.NewWithoutLayout(p)
	desplazado.Resize(fyne.NewSize(tam.Width+40, tam.Height+hueco))
	p.Move(fyne.NewPos(20, hueco))
	img := pinta(t, tam.Width+40, tam.Height+hueco, desplazado)

	tinta := tema.Color("text", tema.Claro)
	_, _, y0, y1 := caja(img, tinta)
	// El borde de abajo de la burbuja tiene que quedar 6 px por encima del control,
	// y la flecha asomar por debajo de ese borde.
	arriba := float32(y0) / escala
	if quiero := hueco - m.Height - SeparacionPista; arriba < quiero-1 || arriba > quiero+1 {
		t.Errorf("la burbuja empieza en y=%v y `calc(100%%+6px)` la pone en %v", arriba, quiero)
	}
	if abajo := float32(y1) / escala; abajo <= hueco-SeparacionPista {
		t.Errorf("la flecha llega hasta y=%v y tiene que asomar por debajo de %v",
			abajo, hueco-SeparacionPista)
	}
}

// TestLaPistaSeAbreConElTeclado es el `onFocusCapture`/`onBlurCapture` de Tooltip.tsx y
// su Escape: con el foco en el control la ayuda sale al instante, se va al perderlo, y
// Escape la quita sin mover el foco. Y lleva su sombra `shadow-md`. VISTO EN ROJO antes:
// la pista solo sabía del ratón y no tenía sombra.
func TestLaPistaSeAbreConElTeclado(t *testing.T) {
	boton := NuevoBoton("Archivar", nil)
	p := NuevaPista("Archivar la conversación", boton)
	w := test.NewTempWindow(t, container.NewWithoutLayout(p))
	w.Resize(fyne.NewSize(400, 200))
	p.Resize(p.MinSize())
	p.Move(fyne.NewPos(100, 100))

	w.Canvas().Focus(boton)
	if !p.Abierta() {
		t.Fatal("con el foco en el control, la pista no se abre")
	}
	if !p.r.sombra.Objeto().Visible() {
		t.Error("la pista abierta no lleva su sombra")
	}
	boton.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if p.Abierta() || w.Canvas().Focused() != fyne.Focusable(boton) {
		t.Errorf("Escape: abierta=%v y el foco en %T; tiene que cerrarla sin mover el foco",
			p.Abierta(), w.Canvas().Focused())
	}
	w.Canvas().Unfocus()
	w.Canvas().Focus(boton)
	w.Canvas().Unfocus()
	if p.Abierta() || p.r.sombra.Objeto().Visible() {
		t.Error("al perder el foco la pista sigue abierta, o su sombra a la vista")
	}
}

// TestLaPistaSeAbreConElPunteroSobreElControl: el driver le da el `MouseIn` al objeto
// pasable MÁS PROFUNDO bajo el puntero, que es el botón y no la pista que lo envuelve.
// Se mira con `test.MoveMouse`, que busca igual que el driver de verdad. VISTO EN ROJO
// sin el aviso del botón: la burbuja no se abría nunca con el ratón.
func TestLaPistaSeAbreConElPunteroSobreElControl(t *testing.T) {
	boton := NuevoBoton("Archivar", nil)
	p := NuevaPista("Archivar la conversación", boton)
	w := test.NewTempWindow(t, container.NewWithoutLayout(p))
	w.Resize(fyne.NewSize(400, 200))
	p.Resize(p.MinSize())
	p.Move(fyne.NewPos(100, 100))
	test.MoveMouse(w.Canvas(), fyne.NewPos(110, 110))
	if !boton.encima {
		t.Fatal("el puntero no ha llegado al botón")
	}
	time.Sleep(RetardoPista + 150*time.Millisecond)
	fyne.DoAndWait(func() {}) // el `fyne.Do` del reloj corre aquí, en la rutina del test
	if !p.Abierta() {
		t.Error("con el puntero sobre el botón, la pista no se ha abierto a los 300 ms")
	}
	test.MoveMouse(w.Canvas(), fyne.NewPos(390, 190))
	if p.Abierta() {
		t.Error("al salir el puntero, la pista sigue abierta")
	}
}

// TestLaPistaAcabaDeEntrarConSuBurbujaALaVista: `sytue-fade-in` funde una imagen de la
// burbuja y, al acabar, la burbuja de verdad vuelve y la imagen se va. Si el final falla,
// la pista se queda abierta e invisible. VISTO EN ROJO sin el `rehacer()` del final.
func TestLaPistaAcabaDeEntrarConSuBurbujaALaVista(t *testing.T) {
	boton := NuevoBoton("Archivar", nil)
	p := NuevaPista("Archivar la conversación", boton)
	w := test.NewTempWindow(t, container.NewWithoutLayout(p))
	w.Resize(fyne.NewSize(400, 200))
	p.Resize(p.MinSize())
	p.Move(fyne.NewPos(100, 100))
	test.WidgetRenderer(p)
	p.Abrir()
	r := p.r
	if r.fundido != nil {
		t.Fatal("tras la entrada sigue la imagen de la burbuja")
	}
	for _, o := range r.burbujaYFlecha() {
		if !o.Visible() {
			t.Fatalf("tras la entrada la burbuja sigue escondida (%T)", o)
		}
	}
}

// TestConPortalLaBurbujaSalDeLoQueRecorta: con Portal, la burbuja no va en el envoltorio
// sino en el portal, colocada donde iría sin él —bajo el control, a 6 px—, y al cerrar
// el portal se queda como estaba, con lo que otros tuvieran puesto.
func TestConPortalLaBurbujaSalDeLoQueRecorta(t *testing.T) {
	boton := NuevoBoton("Archivar", nil)
	p := NuevaPista("Archivar la conversación", boton)
	p.Abajo = true
	ajeno := canvas.NewRectangle(color.Transparent)
	p.Portal = container.NewWithoutLayout(ajeno)
	w := test.NewTempWindow(t, container.NewStack(container.NewWithoutLayout(p), p.Portal))
	w.Resize(fyne.NewSize(400, 300))
	fyne.DoAndWait(func() {
		p.Resize(p.MinSize())
		p.Move(fyne.NewPos(100, 100))
		p.Abrir()
	})
	if len(p.Portal.Objects) < 2 {
		t.Fatalf("la burbuja no ha ido al portal: lleva %d objetos", len(p.Portal.Objects))
	}
	for _, o := range test.WidgetRenderer(p).Objects() {
		if o != boton && o.Visible() {
			t.Errorf("con portal el envoltorio sigue pintando %T", o)
		}
	}
	// Se mira el texto, que va dentro de la burbuja: la sombra se sale por arriba.
	var arriba float32 = 1e9
	for _, o := range p.Portal.Objects[1:] {
		if _, es := o.(*canvas.Text); es {
			arriba = min(arriba, o.Position().Y)
		}
	}
	if bajo := 100 + boton.MinSize().Height + SeparacionPista; arriba < bajo || arriba > bajo+tema.TextoXS.Linea {
		t.Errorf("el texto de la burbuja está en y=%g; va bajo el control, desde %g", arriba, bajo)
	}
	fyne.DoAndWait(p.Cerrar)
	if len(p.Portal.Objects) != 1 || p.Portal.Objects[0] != ajeno {
		t.Errorf("al cerrar el portal lleva %d objetos; tenía que quedar solo el ajeno", len(p.Portal.Objects))
	}
}

// TestLaPistaSaleEnUnBotonApagado: como el `title` de un `<button disabled>`, que es
// donde CertiPDF dice por qué una herramienta está bloqueada.
func TestLaPistaSaleEnUnBotonApagado(t *testing.T) {
	b := NuevoBotonIcono(iconos.Lapiz, "Dibujar", nil)
	b.Inerte = true
	p := NuevaPista("El documento no deja anotar", b)
	b.MouseIn(nil)
	if p.reloj == nil && !p.Abierta() {
		t.Error("al pasar por encima de un botón apagado la pista no se pone en marcha")
	}
	b.MouseOut()
}
