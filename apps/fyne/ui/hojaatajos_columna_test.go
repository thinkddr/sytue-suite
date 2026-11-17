// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// UNA COLUMNA DE TECLAS PARA TODA LA HOJA (`ShortcutsModal.tsx`, rejilla con `subgrid`).
//
// Visto en ROJO: con la fila de antes —la descripción justo tras SUS teclas— las cuatro
// descripciones de la muestra empezaban a 36, 75, 128 y 162 px del grupo, y la larga,
// que no se partía, acababa 23 px fuera de la hoja ya en el escritorio de 1280.

var gruposDeMuestra = []GrupoDeAtajos{
	{Titulo: "General", Atajos: []AtajoDeTeclado{
		{Teclas: []string{"?"}, Descripcion: "Ver los atajos"},
		{Teclas: []string{"Ctrl", "K"}, Descripcion: "Buscar"},
	}},
	{Titulo: "Edición", Atajos: []AtajoDeTeclado{
		{Teclas: []string{"Ctrl", "Mayús", "Z"}, Descripcion: "Rehacer"},
		{Teclas: []string{"Ctrl", "Alt", "Mayús", "V"}, Descripcion: "Pegar sin formato, conservando los saltos de párrafo del original"},
	}},
}

// abrirHojaDeMuestra abre la hoja en una ventana de ese tamaño y devuelve sus párrafos
// (las descripciones) y sus teclas, en orden.
func abrirHojaDeMuestra(t *testing.T, tam fyne.Size) (*Dialogo, []*Parrafo, []*Tecla) {
	t.Helper()
	w := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{}))
	w.SetPadded(false)
	w.Resize(tam)
	var d *Dialogo
	fyne.DoAndWait(func() { d = MostrarHojaAtajos(w.Canvas(), "Atajos", "Cerrar", gruposDeMuestra) })
	w.Canvas().Capture()
	var parrafos []*Parrafo
	var teclas []*Tecla
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *Parrafo:
			parrafos = append(parrafos, x)
			return
		case *Tecla:
			teclas = append(teclas, x)
			return
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	for _, s := range seccionesDe(d) {
		ir(s)
	}
	return d, parrafos, teclas
}

// seccionesDe son los grupos de la hoja: solo los suyos, no el título ni el pie, que
// también llevan texto.
func seccionesDe(d *Dialogo) []*seccionAtajos {
	var out []*seccionAtajos
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *seccionAtajos:
			out = append(out, x)
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	ir(d)
	return out
}

func absX(o fyne.CanvasObject) float32 {
	return fyne.CurrentApp().Driver().AbsolutePositionForObject(o).X
}

func TestLasDescripcionesDeLaHojaCaenEnLaMismaVertical(t *testing.T) {
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(390, 844), fyne.NewSize(320, 640)} {
		d, parrafos, _ := abrirHojaDeMuestra(t, tam)
		if len(seccionesDe(d)) != 2 || len(parrafos) != 4 {
			t.Fatalf("%v: la hoja tiene %d grupos y %d descripciones, no 2 y 4", tam, len(seccionesDe(d)), len(parrafos))
		}
		x0 := absX(parrafos[0])
		for _, p := range parrafos[1:] {
			if x := absX(p); x != x0 {
				t.Errorf("%v: «%s» empieza en %.1f y «%s» en %.1f: la columna de teclas no es de la hoja",
					tam, parrafos[0].Texto, x0, p.Texto, x)
			}
		}
		d.Cerrar()
	}
}

// En el teléfono la columna no pasa de media hoja: la combinación más larga baja de
// línea y la descripción se parte en vez de salirse por el canto.
func TestEnLaHojaDelTelefonoNadaSeSalePorElCanto(t *testing.T) {
	d, parrafos, teclas := abrirHojaDeMuestra(t, fyne.NewSize(320, 640))
	defer d.Cerrar()
	s := seccionesDe(d)[1]
	if s.columna > s.ancho/2 {
		t.Errorf("la columna de teclas mide %.1f en una hoja de %.1f: pasa de la mitad", s.columna, s.ancho)
	}
	derecha := fyne.CurrentApp().Driver().AbsolutePositionForObject(s).X + s.Size().Width
	for _, p := range parrafos {
		if fin := absX(p) + p.Size().Width; fin > derecha+0.5 {
			t.Errorf("«%s» acaba en %.1f, fuera de la hoja (%.1f)", p.Texto, fin, derecha)
		}
		for _, linea := range LineasDe(p, p.Size().Width) {
			if w := Medir(linea, p.Escalon, p.Peso).Width; w > p.Size().Width+0.5 {
				t.Errorf("la línea «%s» mide %.1f en un hueco de %.1f", linea, w, p.Size().Width)
			}
		}
	}
	if n := len(LineasDe(parrafos[3], parrafos[3].Size().Width)); n < 2 {
		t.Errorf("la descripción larga sale en %d línea: no se parte", n)
	}
	// «Ctrl Alt Mayús V»: la última tecla baja a la línea siguiente.
	v, ctrl := teclas[len(teclas)-1], teclas[len(teclas)-4]
	pv := fyne.CurrentApp().Driver().AbsolutePositionForObject(v)
	pc := fyne.CurrentApp().Driver().AbsolutePositionForObject(ctrl)
	if pv.Y <= pc.Y {
		t.Errorf("«V» va en la misma línea que «Ctrl» (y %.1f y %.1f) con la columna llena", pv.Y, pc.Y)
	}
	for _, k := range teclas {
		if fin := absX(k) + k.Size().Width; fin > absX(parrafos[0])+0.5 {
			t.Errorf("la tecla «%s» acaba en %.1f, encima de las descripciones (%.1f)", k.Texto, fin, absX(parrafos[0]))
		}
	}
	// La sección declara el alto que de verdad ocupa: con las líneas de más, nada se
	// monta sobre el grupo siguiente.
	if _, ultimo := s.MinSize(), parrafos[3]; fyne.CurrentApp().Driver().AbsolutePositionForObject(ultimo).Y+ultimo.Size().Height >
		fyne.CurrentApp().Driver().AbsolutePositionForObject(s).Y+s.MinSize().Height+0.5 {
		t.Errorf("la última descripción se sale del alto que declara su grupo")
	}
}

func TestColumnaTeclasAtajosEsFitContent(t *testing.T) {
	ancho := float32(560)
	una := ColumnaTeclasAtajos(gruposDeMuestra[:1], ancho)
	dos := ColumnaTeclasAtajos(gruposDeMuestra, ancho)
	if !(una < dos) {
		t.Errorf("con un grupo más ancho la columna no crece: %.1f y %.1f", una, dos)
	}
	if estrecha := ColumnaTeclasAtajos(gruposDeMuestra, 100); estrecha != 50 {
		t.Errorf("a 100 la columna mide %.1f, no la mitad", estrecha)
	}
	// Nunca menos que la tecla más ancha, que no se parte.
	mayus := MedidaTecla(TeclaSuperficie("Mayús")).Width
	if minima := ColumnaTeclasAtajos(gruposDeMuestra, 20); minima != mayus {
		t.Errorf("a 20 la columna mide %.1f, no lo que «Mayús» (%.1f)", minima, mayus)
	}
}

// TestCapturasDeLaHojaDeAtajos deja en SYTUE_CAPTURA_DIALOGOS la hoja de muestra en
// escritorio y en el teléfono, clara y oscura, para ponerla al lado del muestrario web
// (`?abrir=dialogos:atajos`). Sin la variable no hace nada: es la cámara, no un guardián.
func TestCapturasDeLaHojaDeAtajos(t *testing.T) {
	if os.Getenv("SYTUE_CAPTURA_DIALOGOS") == "" {
		t.Skip("sin SYTUE_CAPTURA_DIALOGOS")
	}
	t.Cleanup(func() { tema.Elegir("") })
	for _, tam := range []struct {
		nombre  string
		ventana fyne.Size
		tactil  bool
	}{
		{"escritorio", fyne.NewSize(1280, 800), false},
		{"movil", fyne.NewSize(320, 640), true},
	} {
		for _, tono := range []string{"claro", "oscuro"} {
			tema.Elegir(tono)
			tactil, antes := tam.tactil, tactilDePrueba
			tactilDePrueba = &tactil
			w := test.NewTempWindow(t, canvas.NewRectangle(tema.Color("bg", tema.De(0))))
			w.SetPadded(false)
			w.Canvas().(interface{ SetScale(float32) }).SetScale(2)
			w.Resize(tam.ventana)
			d := MostrarHojaAtajos(w.Canvas(), "Atajos de muestra", "Cerrar", gruposDeMuestra)
			guardarDialogoMovil(t, w.Canvas(), fmt.Sprintf("atajos-%s-%s", tam.nombre, tono))
			d.Cerrar()
			tactilDePrueba = antes
		}
	}
}
