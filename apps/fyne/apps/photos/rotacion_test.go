// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var rotacionesPhotos = []fyne.Size{
	fyne.NewSize(320, 700), fyne.NewSize(568, 320), fyne.NewSize(320, 700),
	fyne.NewSize(390, 844), fyne.NewSize(844, 390), fyne.NewSize(390, 844),
	fyne.NewSize(768, 1024), fyne.NewSize(1024, 768), fyne.NewSize(768, 1024),
}

// La preferencia alta de retrato no puede ocupar toda la foto al girar un SE,
// y una ficha lateral ancha tampoco debe comerse la foto en una tableta.
func TestElVisorConFichaConservaLaFotoAlGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	fotos := fotosDePrueba(1)
	v := vistaDePrueba(t, tecaCargada(fotos))
	pregunta := ui.NuevaEntrada(v.Traduce("viewer.askPlaceholder"))
	pregunta.PonerTexto("¿Dónde está la barca?")
	d := Visor{
		Foto: fotos[0], ConFicha: true, AltoFicha: AltoFichaMax, AnchoFicha: AnchoFichaMax,
		Descripcion: "Una cala con dos barcas y una casa al fondo.", Pregunta: pregunta,
	}
	for i, tam := range rotacionesPhotos {
		v.Ventana = tam.Width
		p := PantallaVisor(v, d)
		p.Resize(tam)
		foto, asa, ficha := p.Objects[3], p.Objects[4], p.Objects[5]
		for _, o := range []fyne.CanvasObject{foto, asa, ficha} {
			dentroDePhotos(t, tam, o)
		}
		if foto.Size().Width <= 0 || foto.Size().Height <= 0 {
			t.Fatalf("a %v la ficha deja la foto sin superficie: %v", tam, foto.Size())
		}
		if asa.Position().Y < foto.Position().Y || ficha.Position().Y < foto.Position().Y {
			t.Errorf("a %v la ficha invade la cabecera", tam)
		}
		s := ficha.(*fyne.Container).Objects[1].(*container.Scroll)
		comprobarFinalPanelPhotos(t, s)
		if pregunta.Texto() != "¿Dónde está la barca?" || d.AltoFicha != AltoFichaMax || d.AnchoFicha != AnchoFichaMax {
			t.Fatal("girar perdió lo escrito o modificó la preferencia de la ficha")
		}
		if dir := directorioRotacionPhotos(); dir != "" && tam.Width > tam.Height && !conRaza {
			guardarEn(t, dir, fmt.Sprintf("native-rotacion-photos-visor-%d-%gx%g", i, tam.Width, tam.Height), pintaPantallaA(t, v.Teca, p, tam.Width, tam.Height))
		}
	}
}

// La app conservaba editorLayout.movil del día en que se abrió el editor: pintar
// por un cambio de ancho sólo rehacía el visor. Se mantiene la edición y sus campos,
// pero la ficha pasa de abajo a la derecha y vuelve con la ventana.
func TestElEditorSeAdaptaAlGirarSinPerderLaEdicion(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	v := vistaDePrueba(t, nil)
	v.Ventana = 320
	a := Nueva(nil, v, nil)
	a.AbrirEditor(fotosDePrueba(1)[0], "")
	t.Cleanup(a.cerrarEditor)
	e := a.ed
	e.cargando, e.original = false, bandas(80, 60)
	e.receta = conValor(e.receta, api.PasoContraste, 0.3)
	e.peticion.PonerTexto("Un poco más de luz")
	peticion, lienzo := e.peticion, e.lienzo
	for i, tam := range rotacionesPhotos {
		a.v.Ventana = tam.Width
		a.pintar()
		e.capa.Resize(tam)
		p := e.capa.dentro.(*fyne.Container)
		l := p.Layout.(*editorLayout)
		if l.movil != movil(tam.Width) {
			t.Fatalf("a %v conserva el reparto anterior del editor", tam)
		}
		for _, o := range p.Objects[3:] {
			dentroDePhotos(t, tam, o)
		}
		if hueco := p.Objects[6].Size(); hueco.Width <= 2*RellenoEditor || hueco.Height <= 2*RellenoEditor {
			t.Errorf("a %v el panel deja el lienzo sin superficie: %v", tam, hueco)
		}
		comprobarFinalPanelPhotos(t, l.panel)
		if a.ed != e || e.peticion != peticion || e.lienzo != lienzo || e.peticion.Texto() != "Un poco más de luz" || valorDe(e.receta, api.PasoContraste) != 0.3 {
			t.Fatal("girar perdió la edición, la petición de IA o el lienzo vivo")
		}
		if dir := directorioRotacionPhotos(); dir != "" && tam.Width > tam.Height && !conRaza {
			guardarEn(t, dir, fmt.Sprintf("native-rotacion-photos-editor-%d-%gx%g", i, tam.Width, tam.Height), pintaPantallaA(t, nil, p, tam.Width, tam.Height))
		}
	}
}

func directorioRotacionPhotos() string {
	if dir := os.Getenv("SYTUE_ROTACION_CAPTURAS"); dir != "" {
		return dir
	}
	return os.Getenv("SYTUE_CAPTURA")
}

func dentroDePhotos(t *testing.T, tam fyne.Size, o fyne.CanvasObject) {
	t.Helper()
	p, s := o.Position(), o.Size()
	if p.X < 0 || p.Y < 0 || s.Width < 0 || s.Height < 0 || p.X+s.Width > tam.Width+0.01 || p.Y+s.Height > tam.Height+0.01 {
		t.Errorf("%T queda fuera de %v: %v/%v", o, tam, p, s)
	}
}

func comprobarFinalPanelPhotos(t *testing.T, s *container.Scroll) {
	t.Helper()
	if s.Size().Width <= 0 || s.Size().Height <= 0 {
		t.Fatalf("el panel no tiene superficie para sus controles: %v", s.Size())
	}
	s.ScrollToBottom()
	// El contenido debe medir su alto al ancho que recibió. Un mínimo calculado
	// con el ancho de retrato dejaba las últimas filas fuera del alcance del scroll.
	o, y := s.Content, float32(0)
	for {
		c, ok := o.(*fyne.Container)
		if !ok || len(c.Objects) == 0 {
			break
		}
		if len(c.Objects) != 1 {
			ultimo := c.Objects[len(c.Objects)-1]
			if fin := y + ultimo.Position().Y + ultimo.Size().Height; fin > s.Content.Size().Height+0.01 {
				t.Errorf("el último bloque queda fuera del contenido desplazable: fin=%g, contenido=%v", fin, s.Content.Size())
			}
			break
		}
		o = c.Objects[0]
		y += o.Position().Y
	}
	if fin := s.Content.Size().Height - s.Offset.Y; fin > s.Size().Height+0.01 {
		t.Errorf("el último control no se alcanza: fin=%g, panel=%v, offset=%v", fin, s.Size(), s.Offset)
	}
	s.ScrollToTop()
}

// Volver y Guardar deben seguir accesibles incluso con el nombre y los textos de
// estado: el scroll horizontal evita que el último botón quede fuera en retrato.
func TestLosMandosDelEditorSeAlcanzanTrasGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	v := vistaDePrueba(t, nil)
	vueltas, guardados := 0, 0
	for _, tam := range rotacionesPhotos {
		v.Ventana = tam.Width
		cab := cabeceraEditor(v, Editor{
			Foto: fotosDePrueba(1)[0], Sucia: true,
			AlVolver: func() { vueltas++ }, AlGuardar: func() { guardados++ },
		})
		cab.Resize(fyne.NewSize(tam.Width-2*RellenoVisorX, ui.SueloTactil(LadoMandoVisor)))
		var objetos []fyne.CanvasObject
		tira, desliza := cab.(*ui.TiraDeslizable)
		if desliza {
			objetos = tira.Objetos
		} else {
			objetos = cab.(*fyne.Container).Objects
		}
		guardar := objetos[len(objetos)-1].(*ui.Boton)
		if desliza {
			tira.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-10000, 0)})
		}
		if x := guardar.Position().X; x < 0 || x+guardar.Size().Width > cab.Size().Width+0.01 {
			t.Errorf("a %v Guardar no se alcanza: %v/%v", tam, guardar.Position(), guardar.Size())
		}
		test.Tap(guardar)
		volver := objetos[0].(*ui.Boton)
		volver.FocusGained()
		if volver.Position().X < 0 || volver.Position().X+volver.Size().Width > cab.Size().Width+0.01 {
			t.Errorf("a %v Volver no se alcanza", tam)
		}
		test.Tap(volver)
		guardar.FocusGained()
		if x := guardar.Position().X; x < 0 || x+guardar.Size().Width > cab.Size().Width+0.01 {
			t.Errorf("a %v enfocar Guardar lo deja fuera", tam)
		}
	}
	if vueltas != len(rotacionesPhotos) || guardados != len(rotacionesPhotos) {
		t.Errorf("faltan acciones tras girar: Volver=%d, Guardar=%d", vueltas, guardados)
	}
}

type lienzoSeguroPhotos struct {
	fyne.Canvas
	pos fyne.Position
	tam fyne.Size
}

func (c lienzoSeguroPhotos) InteractiveArea() (fyne.Position, fyne.Size) { return c.pos, c.tam }

// Abrir con el canvas entero sobrescribía el tamaño seguro que ya había puesto
// el driver al añadir la capa. El notch lateral tapaba los mandos del visor/editor.
func TestLasCapasDePhotosAbrenDentroDelAreaSegura(t *testing.T) {
	for _, c := range []struct {
		ventana, segura fyne.Size
		pos             fyne.Position
	}{
		{fyne.NewSize(390, 844), fyne.NewSize(390, 763), fyne.NewPos(0, 47)},
		{fyne.NewSize(844, 390), fyne.NewSize(756, 369), fyne.NewPos(44, 0)},
		{fyne.NewSize(390, 844), fyne.NewSize(390, 440), fyne.NewPos(0, 47)},
	} {
		w := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{}))
		w.Resize(c.ventana)
		lienzo := lienzoSeguroPhotos{Canvas: w.Canvas(), pos: c.pos, tam: c.segura}
		capa := nuevaCapaVisor(func() {})
		capa.poner(canvas.NewRectangle(color.NRGBA{}))
		capa.mostrar(lienzo)
		if capa.Position() != c.pos || capa.Size() != c.segura || capa.dentro.Size() != c.segura {
			t.Errorf("apertura %v: capa=%v/%v, cuerpo=%v; área segura=%v/%v", c.ventana, capa.Position(), capa.Size(), capa.dentro.Size(), c.pos, c.segura)
		}
	}
}
