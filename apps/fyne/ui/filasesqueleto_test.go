// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasFilasEsqueletoMidenLaFila es el espejo de «cada fila mide py-2 más lo más alto
// de sus hijos» de `Skeleton.test.tsx`: el esqueleto ocupa lo que ocuparán las filas de
// verdad y la lista no salta al llegar los datos.
//
// VISTO EN ROJO: con un alto fijo de una línea para todas las filas, la tesela de 36 de
// Drive desborda y el caso de la tesela dice «mide 36 y la fila de Drive 50».
func TestLasFilasEsqueletoMidenLaFila(t *testing.T) {
	for _, c := range []struct {
		d      DelanteroEsqueleto
		lineas int
		quiere float32
	}{
		{SinDelantero, 1, 2*AltoFila + tema.TextoBase.Linea},
		{SinDelantero, 2, 2*AltoFila + tema.TextoBase.Linea + tema.TextoSM.Linea},
		{DelanteroAvatar, 1, 2*AltoFila + 32},
		// La fila de carga de Drive mide lo que la fila de verdad.
		{DelanteroTesela, 1, AltoFilaDrive},
	} {
		if got := AltoFilaEsqueleto(c.d, c.lineas); got != c.quiere {
			t.Errorf("delantero %d con %d líneas: la fila mide %g y tiene que medir %g", c.d, c.lineas, got, c.quiere)
		}
		f := NuevasFilasEsqueleto(4, c.d, c.lineas)
		if h := f.MinSize().Height; h != 4*c.quiere {
			t.Errorf("cuatro filas (delantero %d, %d líneas) piden %g de alto y son %g", c.d, c.lineas, h, 4*c.quiere)
		}
	}
}

// TestLasFilasEsqueletoLatenAUna: una sola animación para toda la lista y todos los
// bloques del mismo tono en cada instante, como los `.sytue-skeleton` de la web, que
// arrancan juntos.
//
// VISTO EN ROJO: montando las filas con un `Esqueleto` por bloque —lo que hacían las
// recetas de Drive— el renderer tiene widgets y no rectángulos, y cada uno su animación.
func TestLasFilasEsqueletoLatenAUna(t *testing.T) {
	f := NuevasFilasEsqueleto(5, DelanteroTesela, 2)
	f.Meta = []MetaEsqueleto{{Ancho: 44}, {Ancho: 76}}
	f.Resize(fyne.NewSize(600, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	if f.anim == nil {
		t.Fatal("las filas no han arrancado su latido")
	}
	// Tesela, título, subtítulo y dos metas por fila: 25 bloques, todos de lienzo.
	if n := len(r.bloques); n != 25 {
		t.Errorf("cinco filas pintan %d bloques y tienen que pintar 25", n)
	}
	for _, o := range r.Objects() {
		if _, es := o.(*canvas.Rectangle); !es {
			t.Fatalf("un bloque de la lista es %T: cada widget late por su cuenta", o)
		}
	}
	f.mu.Lock()
	f.fase = 0.5
	f.mu.Unlock()
	r.tenir()
	primero := r.bloques[0].FillColor
	for i, b := range r.bloques {
		if b.FillColor != primero {
			t.Fatalf("el bloque %d pinta %v y el primero %v: no laten a una", i, b.FillColor, primero)
		}
	}
	if opaco := tintaEsqueleto("", 0); primero == opaco {
		t.Error("a mitad de latido los bloques siguen opacos: el latido no llega a los bloques")
	}
}

// TestLasFilasEsqueletoSeAnuncianUnaVez: un solo «Cargando» para toda la lista (§3.18),
// no uno por fila; con rótulo propio, el propio.
func TestLasFilasEsqueletoSeAnuncianUnaVez(t *testing.T) {
	f := NuevasFilasEsqueleto(6, DelanteroAvatar, 1)
	if got, quiere := f.AccessibilityLabel(), textoComun("ui.loading"); got != quiere || got == "" {
		t.Errorf("las filas se anuncian %q y tienen que decir %q", got, quiere)
	}
	f.Rotulo = "Cargando archivos…"
	if got := f.AccessibilityLabel(); got != f.Rotulo {
		t.Errorf("con rótulo propio se anuncian %q", got)
	}
	r := test.WidgetRenderer(f)
	defer f.Parar()
	for _, o := range r.Objects() {
		if _, es := o.(fyne.Accessible); es {
			t.Fatalf("un bloque (%T) llega al lector: la carga se diría una vez por fila", o)
		}
	}
}

// TestLasFilasEsqueletoColocanLaMeta: la meta va pegada a la derecha, la oculta no ocupa
// sitio y el título no se mete debajo de ella.
func TestLasFilasEsqueletoColocanLaMeta(t *testing.T) {
	f := NuevasFilasEsqueleto(1, SinDelantero, 1)
	f.Meta = []MetaEsqueleto{{Ancho: 44, Oculta: true}, {Ancho: 76}}
	f.Resize(fyne.NewSize(400, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	r.Layout(f.Size())
	fila := r.filas[0]
	if fila.meta[0].Visible() {
		t.Error("la columna oculta se pinta")
	}
	if fin := fila.meta[1].Position().X + fila.meta[1].Size().Width; fin != 400-MargenFila {
		t.Errorf("la meta acaba en %g y tiene que acabar en el px-3 de la fila (%g)", fin, 400-MargenFila)
	}
	if fin := fila.titulo.Position().X + fila.titulo.Size().Width; fin > fila.meta[1].Position().X {
		t.Errorf("el título acaba en %g, encima de la meta que empieza en %g", fin, fila.meta[1].Position().X)
	}
}

// TestLasFilasEsqueletoLlevanSuCabecera: la cabecera (el `header` de SkeletonRows) va
// encima de las filas, en el MISMO widget: sus rayas laten con las filas —el mismo
// tono en cada instante—, el rótulo a la izquierda y la meta a la derecha con su hueco
// (la oculta no ocupa sitio), centradas encima de su filete, y las filas bajan lo que
// mide.
//
// VISTO EN ROJO sin la cabecera en FilasEsqueleto: Drive la montaba en un
// `BloquesEsqueleto` aparte, con su propio latido y su propio «Cargando».
func TestLasFilasEsqueletoLlevanSuCabecera(t *testing.T) {
	f := NuevasFilasEsqueleto(2, DelanteroTesela, 1)
	f.Margen = 12
	f.Meta = []MetaEsqueleto{{Ancho: 44}, {Ancho: 76}}
	sin := f.MinSize().Height
	f = NuevasFilasEsqueleto(2, DelanteroTesela, 1).ConCabecera(CabeceraEsqueleto{
		Alto:      30.6,
		Filete:    true,
		Rotulo:    MetaEsqueleto{Ancho: 56, Alto: 9.6},
		Meta:      []MetaEsqueleto{{Ancho: 48, Alto: 9.6}, {Ancho: 72, Alto: 9.6, Oculta: true}},
		HuecoMeta: 32,
	})
	f.Margen = 12
	f.Meta = []MetaEsqueleto{{Ancho: 44}, {Ancho: 76}}
	if h := f.MinSize().Height; h != sin+30.6 {
		t.Errorf("con cabecera las filas piden %g y deberían pedir %g", h, sin+30.6)
	}
	f.Resize(fyne.NewSize(600, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	r.Layout(f.Size())
	rayas := f.RayasCabecera()
	if len(rayas) != 3 {
		t.Fatalf("la cabecera tiene %d rayas y deberían ser 3", len(rayas))
	}
	for _, c := range []struct {
		nombre string
		r      *canvas.Rectangle
		pos    fyne.Position
	}{
		{"el rótulo", rayas[0], fyne.NewPos(24, 10)},
		// Con la segunda oculta, la primera se pega al `px-3` de la derecha.
		{"la meta", rayas[1], fyne.NewPos(600-24-48, 10)},
	} {
		if c.r.Position() != c.pos || c.r.Size().Height != 9.6 {
			t.Errorf("%s cae en %v (alto %g) y debería caer en %v", c.nombre, c.r.Position(), c.r.Size().Height, c.pos)
		}
	}
	if rayas[2].Visible() {
		t.Error("la raya oculta de la cabecera se pinta")
	}
	if p, tam := r.fileteCabecera.Position(), r.fileteCabecera.Size(); p != fyne.NewPos(12, 29.6) || tam != fyne.NewSize(576, tema.Borde) {
		t.Errorf("el filete de la cabecera cae en %v y mide %v", p, tam)
	}
	if y := r.filas[0].delantero.Position().Y; y < 30.6 {
		t.Errorf("la primera fila empieza en %g, debajo de la cabecera de 30,6", y)
	}
	// Un solo latido: las rayas de la cabecera son bloques del renderer como las filas.
	enLatido := 0
	for _, b := range r.bloques {
		for _, q := range rayas {
			if b == q {
				enLatido++
			}
		}
	}
	if enLatido != len(rayas) {
		t.Errorf("%d de las %d rayas de la cabecera laten con las filas", enLatido, len(rayas))
	}
}

// TestLasFilasEsqueletoSeQuedanQuietasSinMovimiento: con `prefers-reduced-motion` no hay
// latido y los bloques se quedan opacos.
func TestLasFilasEsqueletoSeQuedanQuietasSinMovimiento(t *testing.T) {
	SinMovimiento = true
	defer func() { SinMovimiento = false }()
	f := NuevasFilasEsqueleto(3, SinDelantero, 1)
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	if f.anim != nil {
		t.Error("con SinMovimiento las filas han arrancado el latido")
	}
	if c := r.bloques[0].FillColor; c != tintaEsqueleto("", 0) {
		t.Errorf("quietos, los bloques pintan %v y tienen que ser opacos", c)
	}
}

// TestLasFilasEsqueletoSinUltimoFilete: con `last:border-b-0` la última fila no lleva
// filete, que dentro de una caja con borde sería una raya doble abajo.
func TestLasFilasEsqueletoSinUltimoFilete(t *testing.T) {
	f := NuevasFilasEsqueleto(3, SinDelantero, 2)
	f.Filete, f.SinUltimoFilete = true, true
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	for i, fila := range r.filas {
		if quiere := i < 2; (fila.filete != nil) != quiere {
			t.Errorf("la fila %d lleva filete: %v, y tiene que llevarlo: %v", i, fila.filete != nil, quiere)
		}
	}
}

// TestLasFilasConRecetaSiguenSuGeometria: una lista con fila propia (la bandeja de Mail,
// el historial de Rooms…) pone sus medidas en la receta y FilasEsqueleto las respeta:
// el alto, el aire entre filas y el de encima, el delantero con su medida, las rayas
// con su ancho fijo o su parte en ciclo, la cola en la línea de la primera y, con
// `items-start`, todo colgado del relleno de arriba.
//
// VISTO EN ROJO: sin receta, cada app montaba su receta con un `Esqueleto` por bloque.
func TestLasFilasConRecetaSiguenSuGeometria(t *testing.T) {
	cerca := func(a, b float32) bool { return a-b < 0.51 && b-a < 0.51 }
	t.Run("centrada", func(t *testing.T) {
		f := NuevasFilasEsqueleto(3, DelanteroAvatar, 2).ConReceta(RecetaEsqueleto{
			Alto: 60, Hueco: 4, Encima: 10, Izq: 12, Der: 8,
			Delantero: fyne.NewSize(40, 40), HuecoDelantero: 12,
			Rayas: []RayaEsqueleto{
				{Alto: 14, Partes: []float32{0.5}, Cola: fyne.NewSize(36, 12)},
				{Alto: 12, Partes: []float32{0.8, 0.6}},
			},
			HuecoRayas: 8,
		})
		f.Filete = true
		defer f.Parar()
		if f.Lineas != 2 || f.Meta != nil {
			t.Errorf("con receta las líneas son %d (quiero las rayas) y la meta %v", f.Lineas, f.Meta)
		}
		if h := f.MinSize().Height; h != 10+3*60+2*4 {
			t.Errorf("tres filas de 60 con 4 entre ellas y 10 encima piden %g", h)
		}
		f.Resize(fyne.NewSize(400, f.MinSize().Height))
		r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
		r.Layout(f.Size())
		for i, fila := range r.filas {
			y := 10 + float32(i)*64
			caja := 60 - tema.Borde
			if p := fila.filete.Position(); p.Y != y+caja {
				t.Errorf("fila %d: el filete está en %g y la fila acaba en %g", i, p.Y, y+60)
			}
			d := fila.delantero
			if d.Size() != fyne.NewSize(40, 40) || d.Position().X != 12 || !cerca(d.Position().Y, y+(caja-40)/2) || d.CornerRadius != 20 {
				t.Errorf("fila %d: el avatar está en %v con %v y radio %g", i, d.Position(), d.Size(), d.CornerRadius)
			}
			util := float32(400 - 8 - 64)
			l1, l2, cola := fila.rayas[0], fila.rayas[1], fila.colas[0]
			if l1.Position().X != 64 || !cerca(l1.Size().Width, util*0.5) {
				t.Errorf("fila %d: la primera raya está en %g y mide %g", i, l1.Position().X, l1.Size().Width)
			}
			if fin := cola.Position().X + cola.Size().Width; fin != 392 || !cerca(cola.Position().Y+6, l1.Position().Y+7) {
				t.Errorf("fila %d: la cola acaba en %g y se centra en %g; la raya, en %g", i, fin, cola.Position().Y+6, l1.Position().Y+7)
			}
			parte := []float32{0.8, 0.6}[i%2]
			if !cerca(l2.Size().Width, util*parte) || l2.Position().Y != l1.Position().Y+14+8 {
				t.Errorf("fila %d: la segunda raya mide %g en y=%g", i, l2.Size().Width, l2.Position().Y)
			}
			if !cerca(l1.Position().Y, y+(caja-34)/2) {
				t.Errorf("fila %d: las rayas no se centran en la fila (y=%g)", i, l1.Position().Y)
			}
		}
	})
	t.Run("arriba", func(t *testing.T) {
		f := NuevasFilasEsqueleto(2, DelanteroTesela, 2).ConReceta(RecetaEsqueleto{
			Alto: 48, Izq: 16, Delantero: fyne.NewSize(10, 10), HuecoDelantero: 12,
			Rayas:  []RayaEsqueleto{{Alto: 14, Ancho: 80}, {Alto: 16, Ancho: 1000}},
			Arriba: true, Relleno: 8, BajaDelantero: 6, BajaRayas: 2,
			Sangrias: []float32{0, 20},
		}).SobreCromo()
		defer f.Parar()
		f.Resize(fyne.NewSize(300, f.MinSize().Height))
		r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
		r.Layout(f.Size())
		for i, fila := range r.filas {
			y, sangria := float32(i)*48, []float32{0, 20}[i]
			if p := fila.delantero.Position(); p != fyne.NewPos(16+sangria, y+14) || fila.delantero.CornerRadius != tema.RadioMD {
				t.Errorf("fila %d: la tesela está en %v con radio %g", i, p, fila.delantero.CornerRadius)
			}
			if p := fila.rayas[0].Position(); p != fyne.NewPos(38+sangria, y+10) || fila.rayas[0].Size().Width != 80 {
				t.Errorf("fila %d: la raya de ancho fijo está en %v y mide %g", i, p, fila.rayas[0].Size().Width)
			}
			if w := fila.rayas[1].Size().Width; w != 300-38-sangria {
				t.Errorf("fila %d: la raya más ancha que la fila mide %g y no cabe", i, w)
			}
		}
		if c := r.bloques[0].FillColor; c != tintaEsqueleto("sidebar-hover-bg", f.fase) {
			t.Errorf("sobre el cromo los bloques pintan %v", c)
		}
	})
}

// TestNingunaAppMontaSusFilasDeCarga: las filas de carga de las listas de la suite son
// FilasEsqueleto con su receta, no un layout propio con un `Esqueleto` por bloque, que
// late cada uno a su aire y anuncia la carga por su cuenta (o no la anuncia).
//
// Mira todas las apps del módulo, no solo las que ya se habían pasado.
//
// VISTO EN ROJO con las recetas de Mail, Contactos, Rooms, Notes y Calendar.
func TestNingunaAppMontaSusFilasDeCarga(t *testing.T) {
	receta := regexp.MustCompile(`(?m)^type (filaEsqueleto|filaCarga|EsqueletoMensajes|EsqueletoCanales|Esqueleto)\b`)
	for _, dir := range []string{".."} {
		err := filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && fueraDelGuardian[filepath.ToSlash(ruta)] {
				return filepath.SkipDir
			}
			if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return err
			}
			fuente, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			if m := receta.Find(fuente); m != nil {
				t.Errorf("%s monta sus propias filas de carga (%s): son ui.NuevasFilasEsqueleto con su receta", ruta, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
