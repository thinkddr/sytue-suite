// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"math"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// pantallaConPanel monta la pantalla a 1280×800 con el documento de la medida y abre el
// panel pedido, como hace quien pulsa su pestaña.
func pantallaConPanel(t *testing.T, a PanelActivo) (*Pantalla, fyne.Window) {
	t.Helper()
	return pantallaConDocumento(t, documentoDePrueba(), a)
}

// pantallaConDocumento es pantallaConPanel con otro documento.
func pantallaConDocumento(t *testing.T, c *Contenido, a PanelActivo) (*Pantalla, fyne.Window) {
	t.Helper()
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.PonerContenido(c)
	p.panel.Abrir(a)
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	return p, w
}

func cerca(t *testing.T, que string, got, quiero float32) {
	t.Helper()
	if math.Abs(float64(got-quiero)) > 0.05 {
		t.Errorf("%s: %g; la web, %g", que, got, quiero)
	}
}

// TestLosComentariosSeColocanComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz de 16 px, con Ada y una réplica de Grace sin texto.
// Filtro en 8,155 de 271×26; casilla en 185; ficha en 207 de 103 de alto;
// selector en 183,256 de 87×21, pegado a 270; «Aplanar» en 8,318 de 144,60938×30.
func TestLosComentariosSeColocanComoEnLaWeb(t *testing.T) {
	p, w := pantallaConPanel(t, PanelComentarios)
	defer w.Close()
	c := p.panel.comentariosP
	if c == nil {
		t.Fatal("el panel de comentarios no se ha montado")
	}
	f := absoluta(c.filtro)
	cerca(t, "el filtro x", f.X, 8)
	cerca(t, "el filtro y", f.Y, 155)
	cerca(t, "el alto del filtro", c.filtro.Size().Height, 26)

	var casilla *casillaNativa
	var fichas []*recuadro
	var selector *ui.Selector
	var aplanar *Accion
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *casillaNativa:
			casilla = x
		case *recuadro:
			fichas = append(fichas, x)
		case *ui.Selector:
			selector = x
		case *Accion:
			if x.Texto == "Aplanar las anotaciones" {
				aplanar = x
			}
		}
	})
	if casilla == nil || len(fichas) != 1 || selector == nil || aplanar == nil {
		t.Fatalf("faltan piezas: casilla %v, %d fichas, selector %v, aplanar %v", casilla != nil, len(fichas), selector != nil, aplanar != nil)
	}
	cerca(t, "«solo sin resolver» y", absoluta(casilla).Y, 185)
	cerca(t, "la ficha y", absoluta(fichas[0]).Y, 207)
	cerca(t, "el alto de la ficha", fichas[0].Size().Height, 103)
	s := absoluta(selector)
	cerca(t, "el select y", s.Y, 256)
	// El ancho sale de medir «Descartado», y un rótulo queda a ≤0,17 del navegador
	// (REGLAS §3). El DOM medido da 87 px y el extremo derecho en 270.
	if d := math.Abs(float64(selector.Size().Width - 87)); d > 0.17 || math.Abs(float64(s.X+selector.Size().Width-270)) > 0.05 {
		t.Errorf("el select mide %g y acaba en %g; la web: 87, pegado a 270", selector.Size().Width, s.X+selector.Size().Width)
	}
	a := absoluta(aplanar)
	cerca(t, "«Aplanar» y", a.Y, 318)
	cerca(t, "el alto de «Aplanar»", aplanar.Size().Height, 30)
	if math.Abs(float64(aplanar.Size().Width-144.60938)) > 0.6 {
		t.Errorf("«Aplanar» mide %g de ancho; la web, 144,60938", aplanar.Size().Width)
	}
}

// TestBuscarSeColocaComoEnLaWeb: Paneles.tsx real en Chromium a rem16, Figtree cargada.
// Con «Contrato» y Reemplazar abierto: campo en 155 de 30; details en 193 de 104
// (border p-2, resumen16, gap-2, campo30, gap-1.5 y botón26); resultado en 329 de 44.
func TestBuscarSeColocaComoEnLaWeb(t *testing.T) {
	p, w := pantallaConPanel(t, PanelBuscar)
	defer w.Close()
	p.textos = [][]Fragmento{{{Texto: "Contrato", X: 60, Y: 700, Tamano: 14}}, {}, {}}
	b := p.panel.buscarP
	b.campo.PonerTexto("Contrato")
	b.abierto = true
	b.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))

	cerca(t, "el campo y", absoluta(b.campo).Y, 155)
	cerca(t, "el alto del campo", b.campo.Size().Height, 30)
	var caja *recuadro
	var resumen *detalles
	var fila *filaResultado
	var revisar *Accion
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *recuadro:
			caja = x
		case *detalles:
			resumen = x
		case *filaResultado:
			fila = x
		case *Accion:
			if x.Texto == "Ver qué cambiaría" {
				revisar = x
			}
		}
	})
	if caja == nil || resumen == nil || fila == nil || revisar == nil {
		t.Fatalf("faltan piezas: caja %v, resumen %v, fila %v, revisar %v", caja != nil, resumen != nil, fila != nil, revisar != nil)
	}
	cerca(t, "el details y", absoluta(caja).Y, 193)
	cerca(t, "el alto del details abierto", caja.Size().Height, 104)
	cerca(t, "el resumen y", absoluta(resumen).Y, 202)
	cerca(t, "el reemplazo x", absoluta(b.reemplazo).X, 17)
	cerca(t, "el reemplazo y", absoluta(b.reemplazo).Y, 226)
	cerca(t, "el ancho del reemplazo", b.reemplazo.Size().Width, 253)
	cerca(t, "«Ver qué cambiaría» y", absoluta(revisar).Y, 262)
	cerca(t, "el alto de «Ver qué cambiaría»", revisar.Size().Height, 26)
	cerca(t, "el resultado y", absoluta(fila).Y, 329)
	cerca(t, "el alto del resultado", fila.Size().Height, 44)
}

// TestBuscarEsElDeLaWeb: sin distinguir mayúsculas, sobre todas las páginas, con su
// número 1-based.
func TestBuscarEsElDeLaWeb(t *testing.T) {
	t.Parallel()
	pags := [][]Fragmento{{{Texto: "Contrato de obra"}}, nil, {{Texto: "el CONTRATO"}, {Texto: "otra cosa"}}}
	got := Buscar(pags, "contrato")
	if len(got) != 2 || got[0].pagina != 1 || got[1].pagina != 3 || got[1].texto != "el CONTRATO" {
		t.Errorf("Buscar = %+v", got)
	}
}

// TestLosHilosSonLosDeLaWeb: una réplica no abre hilo; buscar su texto enseña el hilo
// donde se dijo; «solo sin resolver» esconde los aceptados y los hechos.
func TestLosHilosSonLosDeLaWeb(t *testing.T) {
	t.Parallel()
	cero := 0
	a := []AnotacionLeida{
		{Texto: "revisar precio", Autor: "Ada"},
		{Texto: "de acuerdo con el IVA", Autor: "Grace", RespuestaA: &cero},
		{Texto: "firma", Autor: "Ada", Estado: "Accepted"},
	}
	if v, n := HilosVisibles(a, "", false); !slices.Equal(v, []int{0, 2}) || n != 2 {
		t.Errorf("sin filtro: %v de %d", v, n)
	}
	if v, _ := HilosVisibles(a, "iva", false); !slices.Equal(v, []int{0}) {
		t.Errorf("buscar el texto de una réplica tiene que enseñar su hilo: %v", v)
	}
	if v, _ := HilosVisibles(a, "", true); !slices.Equal(v, []int{0}) {
		t.Errorf("solo sin resolver: %v", v)
	}
}

// TestElBorradorDelIndiceSigueLasReglasDeLaWeb: el primero no se sangra, nadie baja más
// de un nivel bajo su anterior y lo que no tiene título no se guarda.
func TestElBorradorDelIndiceSigueLasReglasDeLaWeb(t *testing.T) {
	t.Parallel()
	b := []Marcador{{Titulo: "A"}, {Titulo: "B", Nivel: 1}, {Titulo: " "}}
	if PuedeSangrar(b, 0) || PuedeSangrar(b, 1) || !PuedeSangrar(b, 2) {
		t.Errorf("sangrar: %v %v %v", PuedeSangrar(b, 0), PuedeSangrar(b, 1), PuedeSangrar(b, 2))
	}
	if got := LimpiarBorrador(b); len(got) != 2 {
		t.Errorf("LimpiarBorrador = %+v", got)
	}
}

// TestElIndiceEnEdicionEsElDeLaWeb: Paneles.tsx real en Chromium, rem16 y Figtree.
// Primera fila en 12,159: título128,3125, página48 y botones24,15625/24,15625/22,375.
// Sangría0,6rem (9,6 px); Añadir263×30 en 249; Guardar193,65625 y Cancelar65,34375 en 283.
// Las flechas y el aspa usan la fuente de sustitución del navegador, con avances distintos.
func TestElIndiceEnEdicionEsElDeLaWeb(t *testing.T) {
	p, w := pantallaConPanel(t, PanelIndice)
	defer w.Close()
	p.panel.editarIndice()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))

	var campos []*ui.Entrada
	var acciones []*Accion
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Entrada:
			campos = append(campos, x)
		case *Accion:
			acciones = append(acciones, x)
		}
	})
	if len(campos) != 6 {
		t.Fatalf("hay %d campos; tres marcadores son seis", len(campos))
	}
	t0, p0, t1 := absoluta(campos[0]), absoluta(campos[1]), absoluta(campos[2])
	cerca(t, "el título x", t0.X, 12)
	cerca(t, "el título y", t0.Y, 159)
	cerca(t, "el ancho del título", campos[0].Size().Width, 128.3125)
	cerca(t, "la página x", p0.X, 144.3125)
	cerca(t, "el ancho de la página", campos[1].Size().Width, 48)
	cerca(t, "la sangría del segundo", t1.X, 21.59375)
	cerca(t, "el segundo y", t1.Y, 189)
	por := map[string]*Accion{}
	for _, a := range acciones {
		por[a.Texto+a.Nombre] = a
	}
	fuera := por["Reducir sangría"]
	if fuera == nil {
		for _, a := range acciones {
			if a.Texto == "" {
				fuera = a
				break
			}
		}
	}
	cerca(t, "el primer botón x", absoluta(fuera).X, 196.3125)
	cerca(t, "el primer botón y", absoluta(fuera).Y, 160)
	cerca(t, "el ancho del botón", fuera.Size().Width, 24.15625)
	var anadir, guardar, cancelar *Accion
	for _, a := range acciones {
		switch {
		case strings.HasPrefix(a.Texto, "Añadir"):
			anadir = a
		case a.Texto == "Guardar índice":
			guardar = a
		case a.Texto == "Cancelar":
			cancelar = a
		}
	}
	if anadir == nil || guardar == nil || cancelar == nil {
		t.Fatal("faltan los botones del pie")
	}
	cerca(t, "«Añadir» y", absoluta(anadir).Y, 249)
	cerca(t, "el ancho de «Añadir»", anadir.Size().Width, 263)
	cerca(t, "«Guardar» y", absoluta(guardar).Y, 283)
	if math.Abs(float64(guardar.Size().Width-193.65625)) > 0.2 || math.Abs(float64(cancelar.Size().Width-65.34375)) > 0.2 {
		t.Errorf("«Guardar» mide %g y «Cancelar» %g; la web, 193,65625 y 65,34375", guardar.Size().Width, cancelar.Size().Width)
	}
}

// TestLaFichaDeAdjuntoLlevaGuardar: Paneles.tsx real en Chromium a rem16.
// Ficha en 8,155 de 271×98; «Guardar el fichero» en 17,218 de 26 px de alto.
func TestLaFichaDeAdjuntoLlevaGuardar(t *testing.T) {
	_, w := pantallaConPanel(t, PanelAdjuntos)
	defer w.Close()
	var ficha *recuadro
	var guardar *Accion
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *recuadro:
			ficha = x
		case *Accion:
			if x.Texto == "Guardar el fichero" {
				guardar = x
			}
		}
	})
	if ficha == nil || guardar == nil {
		t.Fatal("la ficha o su «Guardar» no están")
	}
	cerca(t, "la ficha y", absoluta(ficha).Y, 155)
	cerca(t, "el alto de la ficha", ficha.Size().Height, 98)
	g := absoluta(guardar)
	cerca(t, "«Guardar» x", g.X, 17)
	cerca(t, "«Guardar» y", g.Y, 218)
	cerca(t, "el alto de «Guardar»", guardar.Size().Height, 26)
}

// TestLosPanelesEnElCajonSonLosDeLaWeb: a 360×740 el cajón mide 288 de ancho,
// con cabecera45 y tira35; p-2 deja el cuerpo en 8,88. Los mismos controles de
// escritorio conservan sus cajas de línea con la raíz de 16 px.
func TestLosPanelesEnElCajonSonLosDeLaWeb(t *testing.T) {
	// PanelLateral móvil: h-11 y border-b (45), tira de pestañas (35) y p-2 (8).
	// El campo añade su etiqueta text-xs (16); el asistente, dos líneas text-xs,
	// dos gap-2 y una fila de Button sm (32): 88 + 32 + 16 + 32 = 168.
	for _, c := range []struct {
		panel PanelActivo
		que   string
		y     float32
	}{
		{PanelComentarios, "el filtro", 88},
		{PanelCampos, "el primer campo", 104},
		{PanelAsistente, "el campo de pregunta", 168},
	} {
		p := NuevaPantalla(traductorES(t))
		w := ventanaSinRelleno(p.Objeto())
		w.Resize(fyne.NewSize(360, 740))
		p.PonerContenido(documentoConCampos())
		p.panel.Abrir(c.panel)
		p.PanelAbierto = true
		p.Recomponer()
		w.Resize(fyne.NewSize(360, 741))
		w.Resize(fyne.NewSize(360, 740))
		var e *ui.Entrada
		todos(w.Content(), func(o fyne.CanvasObject) {
			if x, ok := o.(*ui.Entrada); ok && e == nil {
				e = x
			}
		})
		if e == nil {
			t.Errorf("%s: no se ve en el cajón", c.que)
			w.Close()
			continue
		}
		cerca(t, c.que+" x", absoluta(e).X, 8)
		cerca(t, c.que+" y", absoluta(e).Y, c.y)
		w.Close()
	}
}
