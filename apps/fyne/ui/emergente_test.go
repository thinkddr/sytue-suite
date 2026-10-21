// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElEmergenteCuelgaDeSuDisparador es `absolute top-full left-0 mt-1`: el panel
// empieza en el borde izquierdo del disparador y 3,5 px por debajo de él, mide su
// contenido más el relleno y el borde, y el clic de fuera lo recoge.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con el `mt-1` sumado dos veces el panel
// caía en y = 42 y el test lo decía. Deshecho.
func TestElEmergenteCuelgaDeSuDisparador(t *testing.T) {
	disparador := NuevoBoton("Tema", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(fyne.NewSize(100, 35))
	disparador.Move(fyne.NewPos(40, 0))

	contenido := fcanvas.NewRectangle(tema.Color("primary", tema.Claro))
	contenido.SetMinSize(fyne.NewSize(150, 60))
	cerrado := 0
	e := MostrarEmergente(disparador, contenido, tema.Espacio(2))
	e.AlRecogerse = func() { cerrado++ }

	quiere := fyne.NewSize(150+2*(1+8), 60+2*(1+8))
	if got := MedidaEmergente(contenido, tema.Espacio(2)); got != quiere {
		t.Errorf("mide %v y son el contenido más `p-2` y el borde: %v", got, quiere)
	}
	if got := e.Position(); got != fyne.NewPos(40, 35+4) {
		t.Errorf("el panel está en %v y va en el borde izquierdo del disparador, 3,5 debajo (40, 38,5)", got)
	}
	if !Descartar(w.Canvas()) || cerrado != 1 || e.Abierto() {
		t.Error("Escape / el clic de fuera tiene que recoger el panel y avisar una vez")
	}
}

// TestElEmergenteNoSeSaleDeLaVentana: pegado al borde derecho se aparta 8 px, como el
// menú; si no, lo que se sale no se alcanza.
func TestElEmergenteNoSeSaleDeLaVentana(t *testing.T) {
	got := ColocarEmergente(fyne.NewPos(700, 10), fyne.NewSize(28, 28), fyne.NewSize(200, 100), fyne.NewSize(800, 600))
	if got.X != 800-200-8 {
		t.Errorf("x = %v: pegado al borde derecho tiene que apartarse 8 px (592)", got.X)
	}
}

// TestElPopoverSeVolteaComoPopoverTsx: debajo y a la izquierda; ARRIBA si abajo no cabe
// y arriba sí; anclado a la DERECHA si a la izquierda se sale por la derecha. VISTO EN
// ROJO con la colocación de la carcasa del editor, que nunca se voltea.
func TestElPopoverSeVolteaComoPopoverTsx(t *testing.T) {
	ventana, disp, panel := fyne.NewSize(800, 600), fyne.NewSize(28, 28), fyne.NewSize(240, 200)
	if p := ColocarPopover(fyne.NewPos(100, 100), disp, panel, ventana); p != fyne.NewPos(100, 134) {
		t.Errorf("con sitio, el popover va en %v y es debajo a 6: (100, 134)", p)
	}
	if p := ColocarPopover(fyne.NewPos(100, 500), disp, panel, ventana); p.Y != 500-6-200 {
		t.Errorf("sin sitio abajo, el popover va en y=%v y se voltea arriba: %v", p.Y, 500-6-200)
	}
	if p := ColocarPopover(fyne.NewPos(700, 100), disp, panel, ventana); p.X != 700+28-240 {
		t.Errorf("saliéndose por la derecha, el popover va en x=%v y se ancla a la derecha: %v", p.X, 700+28-240)
	}
}

func TestElPopoverLargoCabeYPermiteAlcanzarSuUltimaAccion(t *testing.T) {
	disparador := NuevoBoton("Abrir", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(320, 180))
	disparador.Move(fyne.NewPos(10, 10))
	disparador.Resize(disparador.MinSize())
	botones := make([]fyne.CanvasObject, 20)
	for i := range botones {
		botones[i] = NuevoBoton("Una acción del panel", nil)
	}
	cuerpo := container.New(Columna{Hueco: 8}, botones...)
	e := MostrarPopover(disparador, cuerpo)
	t.Cleanup(e.Cerrar)
	if e.Position().X < 8 || e.Position().Y < 8 || e.Position().X+e.Size().Width > 312 || e.Position().Y+e.Size().Height > 172 {
		t.Fatalf("el popover se sale de 320×180: %v/%v", e.Position(), e.Size())
	}
	if !e.recortado || e.desplaza == nil {
		t.Fatal("el panel largo se recorta sin desplazamiento interior")
	}
	e.desplaza.ScrollToBottom()
	ultimo := botones[len(botones)-1]
	fin := e.fluido.Position().Y + cuerpo.Position().Y + ultimo.Position().Y + ultimo.Size().Height
	if e.desplaza.Offset.Y <= 0 || fin <= 0 || fin > e.desplaza.Size().Height+0.01 {
		t.Errorf("la última acción no se alcanza: fin=%v, scroll=%v, hueco=%v", fin, e.desplaza.Offset, e.desplaza.Size())
	}
}

// TestElPopoverSeLlevaElFocoYLoDevuelve: al abrir, el foco va al primer control (o al
// panel, que no es parada de Tab); Escape lo cierra y devuelve el foco a quien lo tenía;
// el clic de fuera lo cierra SIN devolverlo. Y va en `menu-surface` con `p-4`.
func TestElPopoverSeLlevaElFocoYLoDevuelve(t *testing.T) {
	disparador := NuevoBoton("Color", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	w.Canvas().Focus(disparador)

	dentro := NuevoBoton("Rojo", nil)
	e := MostrarPopover(disparador, container.New(Columna{}, dentro))
	if w.Canvas().Focused() != fyne.Focusable(dentro) {
		t.Errorf("al abrir el foco está en %T y va al primer control", w.Canvas().Focused())
	}
	if e.fondo() != "menu-surface" || e.relleno != tema.Espacio(4) {
		t.Errorf("el popover va en %q con %v de relleno", e.fondo(), e.relleno)
	}
	dentro.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if e.Abierto() || w.Canvas().Focused() != fyne.Focusable(disparador) {
		t.Errorf("Escape: abierto=%v, foco en %T; tiene que cerrar y volver al disparador",
			e.Abierto(), w.Canvas().Focused())
	}

	sin := MostrarPopover(disparador, cuerpoDe(20))
	if w.Canvas().Focused() != fyne.Focusable(sin) {
		t.Errorf("sin controles el foco va al panel y está en %T", w.Canvas().Focused())
	}
	sin.capa.fondo.Tapped(nil)
	if sin.Abierto() || w.Canvas().Focused() != nil {
		t.Errorf("clic fuera: abierto=%v y foco en %T; cierra sin devolverlo", sin.Abierto(), w.Canvas().Focused())
	}
}

// TestElEmergenteSeRecoloca: cuando su contenido crece con el panel abierto, Recolocar
// lo vuelve a medir. VISTO EN ROJO sin él: el panel se quedaba con el alto de al abrir.
func TestElEmergenteSeRecoloca(t *testing.T) {
	disparador := NuevoBoton("Enlace", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	lista := fcanvas.NewRectangle(tema.Color("bg", tema.Claro))
	lista.SetMinSize(fyne.NewSize(200, 40))
	e := MostrarEmergente(disparador, lista, 7)
	t.Cleanup(e.Cerrar)
	antes := e.Size().Height
	lista.SetMinSize(fyne.NewSize(200, 140))
	e.Recolocar()
	if got := e.Size().Height; got != antes+100 {
		t.Errorf("tras crecer 100 el contenido, el panel mide %v y medía %v", got, antes)
	}
}

// TestElEmergenteVaDondeDigaColocaYConSuFondo: la misma capa, en el sitio y con el
// tamaño que diga `coloca` —que recibe lo que pide el panel— y en `menu-surface`, que
// es el fondo de la vista rápida de un evento de Calendar.
func TestElEmergenteVaDondeDigaColocaYConSuFondo(t *testing.T) {
	w := test.NewTempWindow(t, container.NewWithoutLayout())
	w.SetPadded(false)
	w.Resize(fyne.NewSize(800, 600))
	contenido := fcanvas.NewRectangle(tema.Color("primary", tema.Claro))
	contenido.SetMinSize(fyne.NewSize(100, 40))
	var pedido fyne.Size
	e := MostrarEmergenteEn(w.Canvas(), contenido, tema.Espacio(3.5),
		func(_, tam fyne.Size) (fyne.Position, fyne.Size) {
			pedido = tam
			return fyne.NewPos(300, 200), fyne.NewSize(252, tam.Height)
		})
	if quiere := MedidaEmergente(contenido, tema.Espacio(3.5)); pedido != quiere {
		t.Errorf("`coloca` recibe %v y el panel pide %v", pedido, quiere)
	}
	if e.Position() != fyne.NewPos(300, 200) || e.Size().Width != 252 {
		t.Errorf("el panel está en %v con %v, y no donde dijo `coloca`", e.Position(), e.Size())
	}
	if e.fondo() != "menu-surface" {
		t.Errorf("el fondo es %q", e.fondo())
	}
	if !Descartar(w.Canvas()) || e.Abierto() {
		t.Error("Escape tiene que recogerlo")
	}
}

// TestElEmergenteSeAnunciaConSuRotulo: el `aria-label` del `role="dialog"` de
// Popover.tsx. VISTO EN ROJO sin él: el panel no tenía nombre.
func TestElEmergenteSeAnunciaConSuRotulo(t *testing.T) {
	var e fyne.CanvasObject = &Emergente{Rotulo: "Color del texto"}
	a, es := e.(fyne.Accessible)
	if !es || a.AccessibilityLabel() != "Color del texto" {
		t.Error("el emergente no se anuncia con su rótulo")
	}
}

// TestNingunDesplazamientoPintaSombraDeBorde: Fyne pinta una sombra en el borde de todo
// `container.Scroll` por donde queda contenido, y la web no (un `overflow-auto` a secas).
// El tema de la suite la apaga para TODAS las piezas, no una a una. Se pinta un
// desplazamiento blanco con contenido de sobra y se mira su último píxel de abajo, que es
// donde Fyne la ponía. VISTO EN ROJO con la sombra del tema en `overlay`.
func TestNingunDesplazamientoPintaSombraDeBorde(t *testing.T) {
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	dentro := fcanvas.NewRectangle(blanco)
	dentro.SetMinSize(fyne.NewSize(100, 600))
	d := container.NewVScroll(dentro)
	d.Resize(fyne.NewSize(100, 100))
	img := pinta(t, 100, 100, d)
	for _, y := range []int{int(99 * escala), int(95 * escala)} {
		if p := px(img, int(50*escala), y); !cerca(p, blanco) {
			t.Fatalf("en y=%d el desplazamiento pinta %v sobre blanco: queda la sombra de borde de Fyne", y, p)
		}
	}
}

// TestLaListaDelSelectorLlevaLaSombraDelMenu: la lista abierta copia el panel de
// Dropdown.tsx, y su sombra es la `shadow-md` de `ui.Sombra`, no la del `PopUp` de Fyne,
// que era la del tema y cambiaba con el DPI. VISTO EN ROJO con `widget.NewPopUp`.
func TestLaListaDelSelectorLlevaLaSombraDelMenu(t *testing.T) {
	s := NuevoSelector([]string{"Uno", "Dos"}, nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(s))
	w.Resize(fyne.NewSize(400, 300))
	s.Resize(MedidaSelector(CampoMD, 200))
	s.Alternar()
	t.Cleanup(func() {
		if s.abierto != nil {
			s.abierto.Hide()
		}
	})
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil || cp.sombra == nil {
		t.Fatalf("la lista abierta va en %T y sin la sombra de la suite", w.Canvas().Overlays().Top())
	}
	if !cp.TapThrough() {
		t.Error("el clic de fuera de la lista se la come: tiene que cerrar y seguir, como un menú")
	}
}

// TestLaEntradaFundeElPanelEnteroYLoEscala es la opacidad de GRUPO de `sytue-modal-in`:
// el panel se pinta una vez a una imagen, que es la que se funde y se escala desde
// `scale(.98)` alrededor de su centro, bajando de 10 px; el panel de verdad, escondido
// hasta el final. A la mitad: alfa 0,5, escala 0,99 y 5 px abajo. VISTO EN ROJO con la
// entrada de antes, que solo desplazaba.
func TestLaEntradaFundeElPanelEnteroYLoEscala(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(800, 600))
	d := NuevoDialogo("Reglas", cuerpoDe(60))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	if d.capa.fundido != nil || !d.Visible() {
		t.Fatal("tras la entrada queda la imagen del fundido, o el panel escondido")
	}
	pos, tam := d.Position(), d.Size()
	f := nuevoFundido(w.Canvas(), d, d.capa.sombra, DesplazamientoDialogo, EscalaEntrada)
	if d.Visible() {
		t.Error("mientras se funde, el panel de verdad tiene que estar escondido")
	}
	if b := f.img.Image.Bounds(); float32(b.Dx()) < tam.Width || float32(b.Dx()) > tam.Width+1 ||
		float32(b.Dy()) < tam.Height || float32(b.Dy()) > tam.Height+1 {
		t.Errorf("la imagen del panel mide %v y el panel %v", b, tam)
	}
	f.fotograma(0.5)
	if f.img.Translucency != 0.5 {
		t.Errorf("a la mitad la imagen tiene %v de transparencia y va a 0,5", f.img.Translucency)
	}
	quiero := fyne.NewSize(tam.Width*0.99, tam.Height*0.99)
	if got := f.img.Size(); got != quiero {
		t.Errorf("a la mitad la imagen mide %v y va a escala 0,99: %v", got, quiero)
	}
	centro := func(p fyne.Position, s fyne.Size) fyne.Position { return p.Add(fyne.NewPos(s.Width/2, s.Height/2)) }
	if got, q := centro(f.img.Position(), f.img.Size()), centro(pos, tam).Add(fyne.NewPos(0, 5)); got != q {
		t.Errorf("a la mitad el centro de la imagen está en %v y va 5 px bajo el del panel: %v", got, q)
	}
	f.acabar()
	if !d.Visible() || d.Position() != pos || d.Size() != tam {
		t.Error("al acabar el panel no vuelve, o no vuelve a su sitio")
	}
}
