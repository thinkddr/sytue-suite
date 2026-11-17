// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// piezasDeCarga cuenta los bloques de esqueleto sueltos, las filas de carga
// (`ui.FilasEsqueleto`) y los bloques de un grupo (`ui.BloquesEsqueleto`), y las ruedas
// de lo que se pinta.
func piezasDeCarga(o fyne.CanvasObject) (esqueletos []*ui.Esqueleto, filas, ruedas int) {
	recorrerInterno(o, func(x fyne.CanvasObject) {
		switch y := x.(type) {
		case *ui.Esqueleto:
			esqueletos = append(esqueletos, y)
		case *ui.FilasEsqueleto:
			filas += y.Filas
		case *ui.BloquesEsqueleto:
			filas += len(y.Bloques())
		case *ui.Rueda:
			ruedas++
		}
	})
	return esqueletos, filas, ruedas
}

// TestCargarUnCuadernoEnseñaSuFormaYNoUnaRueda es NOTA-03: mientras llegan las notas, el
// lienzo pinta la geometría de la disposición del cuaderno —filas en lista, tarjetas en
// columnas y en el lienzo libre, la hoja en el árbol— y ninguna rueda.
//
// VISTO EN ROJO: con la rueda centrada de antes, todos los casos fallan con «0
// esqueletos y 1 rueda».
func TestCargarUnCuadernoEnseñaSuFormaYNoUnaRueda(t *testing.T) {
	test.NewTempApp(t)
	casos := []struct {
		nombre string
		vista  string
		llenar bool
		forma  FormaEsqueleto
	}{
		{"lista", "list", false, EsqueletoFilas},
		{"diario", "list", false, EsqueletoFilas},
		{"columnas", "columns", false, EsqueletoTarjetas},
		{"libre", "free", false, EsqueletoTarjetas},
		// En árbol el panel principal es la hoja de UNA nota, que llega como lista.
		{"arbol", "list", true, EsqueletoHoja},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			l := NuevoLienzo(trInterno(t))
			nb := api.Notebook{ID: "c1", Layout: c.vista}
			if c.nombre == "diario" {
				nb.DailyDate = "2026-11-09"
			}
			l.Cuaderno, l.Fase, l.Llenar = DeCuaderno(nb), Cargando, c.llenar
			if c.llenar {
				// Abierta desde un enlace: la nota elegida aún no ha llegado.
				l.Seleccionada = "n1"
			}
			if f := l.forma(); f != c.forma {
				t.Fatalf("la forma es %d y tiene que ser %d", f, c.forma)
			}
			o := l.Objeto(tema.Claro)
			esqueletos, filas, ruedas := piezasDeCarga(o)
			// Y todo en piezas comunes: un solo latido y un solo «Cargando». VISTO EN
			// ROJO con la hoja y las tarjetas de antes: 6 y 5 esqueletos sueltos.
			if len(esqueletos) != 0 || filas < 3 || ruedas != 0 {
				t.Fatalf("cargando pinta %d esqueletos sueltos, %d bloques de carga y %d ruedas: la forma del cuaderno en piezas comunes, sin rueda", len(esqueletos), filas, ruedas)
			}
		})
	}
}

// TestLasFilasDeCargaOcupanLaColumnaDeLectura: la línea de cada fila se queda con el
// ancho que deja la marca, y pinta su fracción; así las filas no miden todas lo mismo.
func TestLasFilasDeCargaOcupanLaColumnaDeLectura(t *testing.T) {
	test.NewTempApp(t)
	fila := filasEsqueleto(1, fyne.NewSize(1.25*tema.Rem, 0.375*tema.Rem), []float32{1}, nil, 0)
	fila.Resize(fyne.NewSize(400, AltoFilaEsqueleto))
	if fila.MinSize().Height != AltoFilaEsqueleto {
		t.Fatalf("la fila mide %g de alto y la de la lista mide %g", fila.MinSize().Height, float32(AltoFilaEsqueleto))
	}
	objs := test.WidgetRenderer(fila).Objects()
	if len(objs) != 2 {
		t.Fatalf("la fila tiene %d bloques y son la marca y la línea", len(objs))
	}
	marca, linea := objs[0], objs[1]
	if x := marca.Position().X; x != RellenoFilaX {
		t.Errorf("la marca empieza en %g y tiene que caer en el relleno (%g)", x, float32(RellenoFilaX))
	}
	quiere := 400 - linea.Position().X - RellenoFilaX
	if w := linea.Size().Width; w != quiere {
		t.Errorf("la línea mide %g y tiene que llegar al relleno de la derecha (%g)", w, quiere)
	}
}

// TestElArbolYElMenuCarganConFilasGrises: el panel de notas del árbol enseña ramas sin
// el recuento falso de «0 notas», y el menú filas tintadas para el cromo donde antes no
// había nada (el árbol de cuadernos empujaba hacia abajo lo de debajo al llegar).
func TestElArbolYElMenuCarganConFilasGrises(t *testing.T) {
	test.NewTempApp(t)
	tr := trInterno(t)
	a := NuevoArbolNotas(tr, "es")
	a.Fase = Cargando
	o := a.Objeto(tema.Claro)
	_, filas, ruedas := piezasDeCarga(o)
	if filas < 3 || ruedas != 0 {
		t.Fatalf("el árbol cargando pinta %d filas de carga y %d ruedas", filas, ruedas)
	}
	cero := tr.T("notebook.notes", i18n.Vars{"count": 0})
	recorrerInterno(o, func(x fyne.CanvasObject) {
		if c, ok := x.(*fyne.Container); ok && esLinea(c, cero) {
			t.Errorf("cargando, la cabecera del árbol dice %q", cero)
		}
	})

	// Los cuadernos del menú son UNA `ui.FilasEsqueleto` de tres filas, tintada para el
	// cromo: un solo latido y un solo «Cargando». VISTO EN ROJO con los tres
	// `ui.Esqueleto` sueltos de antes: «0 filas de carga y 3 esqueletos sueltos».
	l := NuevoLateral(tr)
	l.Fase = Cargando
	var menu []*ui.FilasEsqueleto
	recorrerInterno(l.Objeto(tema.Claro), func(x fyne.CanvasObject) {
		if f, es := x.(*ui.FilasEsqueleto); es {
			menu = append(menu, f)
		}
	})
	sueltos, filas, ruedas := piezasDeCarga(l.Objeto(tema.Claro))
	if len(menu) != 1 || filas != 3 || len(sueltos) != 0 || ruedas != 0 {
		t.Fatalf("el menú cargando pinta %d filas de carga en %d grupos, %d esqueletos sueltos y %d ruedas",
			filas, len(menu), len(sueltos), ruedas)
	}
	if menu[0].Tinte != "sidebar-hover-bg" {
		t.Errorf("las filas del menú llevan el tinte %q: sobre el cromo es `sidebar-hover-bg`", menu[0].Tinte)
	}
	// La geometría de antes, al píxel: tres filas de 2rem a todo el ancho, con el
	// `gap-0.5` entre ellas y el radio de un destino.
	cuadernos := esqueletoCuadernos()
	cuadernos.Resize(fyne.NewSize(240, cuadernos.MinSize().Height))
	if got := cuadernos.MinSize().Height; got != 3*2*tema.Rem+2*0.5*0.25*tema.Rem {
		t.Errorf("los cuadernos de carga miden %g de alto", got)
	}
	for i, b := range test.WidgetRenderer(cuadernos.(fyne.Widget)).Objects() {
		r := b.(*canvas.Rectangle)
		y := float32(i) * (2*tema.Rem + 0.5*0.25*tema.Rem)
		if r.Position() != fyne.NewPos(0, y) || r.Size() != fyne.NewSize(240, 2*tema.Rem) || r.CornerRadius != tema.Forma("radius-nav") {
			t.Errorf("cuaderno %d de carga en %v, %v, radio %g", i, r.Position(), r.Size(), r.CornerRadius)
		}
	}
}

// esLinea dice si el contenedor es una `linea` con ese texto.
func esLinea(c *fyne.Container, texto string) bool {
	l, ok := c.Layout.(*cajaLinea)
	return ok && l.s == texto
}

// TestTareasYCompartidasCarganConFilas es la paridad de `Tareas.tsx` y `Compartidas.tsx`
// (DS-14): mientras llegan, filas de carga comunes bajo el título de la página, y ninguna
// rueda.
//
// VISTO EN ROJO: con la rueda centrada de antes, las dos dicen «0 filas y 1 rueda».
func TestTareasYCompartidasCarganConFilas(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	p.tareas = estadoTareas{fase: Cargando, estado: api.TareaAbierta}
	p.comp.fase = Cargando
	for nombre, c := range map[string]struct {
		o             fyne.CanvasObject
		filas, lineas int
	}{
		"tareas":      {p.vistaTareas(tema.Claro), FilasEsqueletoTareas, 1},
		"compartidas": {p.vistaCompartidas(tema.Claro), FilasEsqueletoCompartidas, 2},
	} {
		var filas []*ui.FilasEsqueleto
		ruedas := 0
		recorrerInterno(c.o, func(x fyne.CanvasObject) {
			switch y := x.(type) {
			case *ui.FilasEsqueleto:
				filas = append(filas, y)
			case *ui.Rueda:
				ruedas++
			}
		})
		for _, f := range filas {
			f.Parar()
		}
		if len(filas) != 1 || ruedas != 0 {
			t.Errorf("%s cargando pinta %d listas de carga y %d ruedas: una lista y ninguna rueda", nombre, len(filas), ruedas)
			continue
		}
		if f := filas[0]; f.Filas != c.filas || f.Lineas != c.lineas {
			t.Errorf("%s carga %d filas de %d líneas y la web %d de %d", nombre, f.Filas, f.Lineas, c.filas, c.lineas)
		}
	}
}
