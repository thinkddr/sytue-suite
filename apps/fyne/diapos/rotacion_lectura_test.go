// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El escenario puede tener área positiva y seguir siendo ilegible: la captura real
// de 568×320 lo dejaba en 171×96 y el cuerpo de 20 px acababa en 3,6 px. Se comprueba
// texto REAL de la referencia y su tamaño pintado, con la barra de suite montada.
func TestLaRotacionDeDiaposConPocaAlturaConservaLecturaReal(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, _, w := montar(t, string(crudo))
	// El aviso observado en producción sigue visible aunque su imagen esté en
	// otra diapositiva. No descargarla ni esconder la limitación para ganar alto.
	e.m.Diapos[2].Cajas = append(e.m.Diapos[2].Cajas, Caja{ID: "externa", Tipo: "image", X: 10, Y: 10, W: 10, H: 10, Src: "https://example.invalid/no-descargar.png"})
	if !e.Cargar(e.Serializar()) || e.aviso == "" {
		t.Fatal("la referencia no conserva el aviso de imagen externa")
	}
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	barra, parar := ui.BarraSuiteDePrueba(tr, iconos.Slides)
	t.Cleanup(parar)
	w.SetContent(barra.Montar(w.Content()))
	for _, tam := range []fyne.Size{{Width: 568, Height: 320}, {Width: 568, Height: 272}} {
		w.Resize(tam)
		e.deslizZona.ScrollToTop()
		e.desliz.ScrollToTop()
		if e.aviso == "" {
			t.Fatal("la disposición compacta ocultó la limitación de imagen externa")
		}
		if e.desliz.Size().Width < 500 || e.escenario.Size().Width < 576 {
			t.Fatalf("%v: el carril o el encaje se comen la escena: viewport=%v, escena=%v", tam, e.desliz.Size(), e.escenario.Size())
		}
		var ingreso *canvas.Text
		var buscar func(fyne.CanvasObject)
		buscar = func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *canvas.Text:
				if strings.Contains(x.Text, "Ingresos") {
					ingreso = x
				}
			case *fyne.Container:
				for _, h := range x.Objects {
					buscar(h)
				}
			case fyne.Widget:
				for _, h := range test.WidgetRenderer(x).Objects() {
					buscar(h)
				}
			}
		}
		buscar(e.escenario)
		if ingreso == nil || ingreso.TextSize < 12 {
			t.Fatalf("%v: falta texto real de ingresos a 12 dp: %v", tam, ingreso)
		}
		// El desplazamiento vertical deja la primera línea del cuerpo a la vista;
		// no basta que el renderer conserve texto fuera de la ventana.
		d := fyne.CurrentApp().Driver()
		test.Drag(w.Canvas(), d.AbsolutePositionForObject(ingreso).AddXY(10, 5), -40, -80)
		if e.desliz.Offset.Y <= 0 {
			t.Fatal("el escenario legible no permite desplazar su contenido")
		}
		p, origen := d.AbsolutePositionForObject(ingreso), d.AbsolutePositionForObject(e.desliz)
		if p.Y < origen.Y || p.Y+ingreso.Size().Height > origen.Y+e.desliz.Size().Height {
			t.Fatalf("%v: la primera línea de ingresos sigue fuera de la vista: %v/%v en %v/%v", tam, p, ingreso.Size(), origen, e.desliz.Size())
		}
		if e.notas.area.Texto() != "Presentar en dos minutos." {
			t.Fatal("girar perdió las notas del documento")
		}
		if dir := os.Getenv("SYTUE_ROTACION_CAPTURAS"); dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SYTUE_CAPTURAS", dir)
			guarda(t, fmt.Sprintf("native-rotacion-diapos-lectura-%gx%g.png", tam.Width, tam.Height), w.Canvas().Capture())
		}
	}
	e.EmpezarEscritura("t2")
	if e.Escribiendo() != "t2" {
		t.Fatal("el texto visible no se puede abrir para editar")
	}
	d := fyne.CurrentApp().Driver()
	antes := e.desliz.Offset.Y
	folio := e.Folio()
	p := d.AbsolutePositionForObject(folio).AddXY(20, 10)
	test.Drag(w.Canvas(), p, 0, -20)
	if e.desliz.Offset.Y <= antes || e.Folio() != folio || e.Escribiendo() != "t2" {
		t.Fatal("arrastrar texto en edición no desplaza su escenario o pierde el folio")
	}
	test.Type(enfocado(t, w), " Verificado")
	e.TerminarEscritura()
	if !strings.Contains(e.m.caja(0, "t2").HTML, "Verificado") {
		t.Fatal("la edición del texto visible no llegó al documento")
	}
}

func TestLaRotacionDeDiaposConPocaAlturaPermiteEditarTrasDesplazar(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	e, ctx, w := montar(t, conFormas)
	e.NuevaDiapositiva()
	e.Elegir(0)
	e.m.Diapos[0].Notas = "Borrador conservado"
	e.Refrescar()
	for _, tam := range []fyne.Size{{Width: 568, Height: 320}, {Width: 1024, Height: 768}, {Width: 568, Height: 272}} {
		w.Resize(tam)
		e.deslizZona.ScrollToTop()
		if tam.Height >= 400 {
			if !e.carril.Visible() || e.escenario.Escala != e.m.Encaje(e.desliz.Size(), HuecoEncaje, 100) {
				t.Fatal("el modo compacto cambió el carril o el encaje normal de iPad")
			}
			if e.sel != "f" || e.notas.area.Texto() != "Borrador conservado" {
				t.Fatal("girar a iPad perdió la selección o las notas")
			}
			continue
		}
		// El gesto pasa por el hit-test del canvas y por el mismo Scroll del editor.
		d := fyne.CurrentApp().Driver()
		origen := d.AbsolutePositionForObject(e.desliz)
		test.Scroll(w.Canvas(), origen.AddXY(20, 20), -40, -80)
		if e.desliz.Offset.Y <= 0 {
			t.Fatal("el gesto real no desplaza la escena")
		}
		m := marcoDe(t, e, "f")
		p := d.AbsolutePositionForObject(m).AddXY(m.Size().Width/2, m.Size().Height/2)
		if p.Y < origen.Y || p.Y >= origen.Y+e.desliz.Size().Height {
			t.Fatalf("la forma que se va a tocar no está en el viewport: %v en %v/%v", p, origen, e.desliz.Size())
		}
		test.TapCanvas(w.Canvas(), p)
		if e.sel != "f" {
			t.Fatal("el offset del scroll cambió la caja que recibe el toque")
		}
		antes := *e.m.caja(0, "f")
		k := e.escenario.Escala
		m = marcoDe(t, e, "f")
		p = d.AbsolutePositionForObject(m).AddXY(m.Size().Width/2, m.Size().Height/2)
		test.Drag(w.Canvas(), p, 9.6*k, 5.4*k)
		if c := e.m.caja(0, "f"); !casi(c.X, antes.X+1) || !casi(c.Y, antes.Y+1) || ctx.Guardado != editor.Sucio {
			t.Fatalf("tras desplazar/girar el arrastre no mueve 1 %% del modelo: antes=%+v, después=%+v", antes, c)
		}
		if e.sel != "f" || e.notas.area.Texto() != "Borrador conservado" {
			t.Fatal("mover/girar perdió la selección o las notas")
		}
	}
	// El selector cerrado no puede conservar el foco cuando el giro lo retira.
	retirado := botonCompacto(t, w, ctx.Traducir("slides.number", i18n.Vars{"n": 1}))
	w.Canvas().Focus(retirado)
	w.Resize(fyne.NewSize(1024, 768))
	if w.Canvas().Focused() == fyne.Focusable(retirado) {
		t.Fatal("girar deja el foco en el selector de diapositivas retirado")
	}
	if w.Canvas().Focused() == nil {
		t.Fatal("girar perdió el foco en vez de reubicarlo en un mando visible")
	}
	w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if e.menuDiapos != nil && e.menuDiapos.Abierto() {
		t.Fatal("Enter abre un selector que ya no está montado")
	}
	if w.Canvas().Overlays().Top() != nil && w.Canvas().Focused() != nil {
		// Enter puede abrir Archivo, el destino visible del foco reubicado.
		w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	}
	w.Resize(fyne.NewSize(568, 272))
	// Los mandos retirados de la segunda barra se usan desde la cinta real.
	notas := botonCompacto(t, w, e.t("slides.notes"))
	w.Canvas().Focus(notas)
	test.Tap(notas)
	if e.deslizZona.Offset.Y == 0 {
		t.Fatal("el botón de notas de la cinta no lleva al panel")
	}
	e.notas.area.PonerTexto("Notas editadas en horizontal")
	e.AlternarNotas()
	if e.deslizZona.Offset.Y != 0 || e.m.Diapos[0].Notas != "Notas editadas en horizontal" {
		t.Fatal("volver al escenario perdió la edición de notas")
	}
	elegir := botonCompacto(t, w, ctx.Traducir("slides.number", i18n.Vars{"n": 1}))
	w.Canvas().Focus(elegir)
	test.Tap(elegir)
	menu := e.menuDiapos
	if menu == nil {
		t.Fatal("la cinta no ofrece cambiar la diapositiva sin el carril")
	}
	for _, o := range menu.Elementos {
		if it, ok := o.(*ui.ElementoMenu); ok && it.MinSize().Height < 44 {
			t.Fatal("el selector de diapositivas reduce sus blancos táctiles")
		}
	}
	test.Tap(menu.Elementos[1].(*ui.ElementoMenu))
	if e.Activa() != 1 {
		t.Fatal("elegir otra diapositiva en horizontal no cambia la escena")
	}
	e.Elegir(0)
	if e.m.Diapos[0].Notas != "Notas editadas en horizontal" {
		t.Fatal("cambiar de diapositiva perdió las notas")
	}
}

func botonCompacto(t *testing.T, w fyne.Window, rotulo string) *editor.BotonCinta {
	t.Helper()
	var boton *editor.BotonCinta
	var buscar func(fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
		switch x := o.(type) {
		case *editor.BotonCinta:
			if x.Rotulo == rotulo {
				boton = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	buscar(w.Content())
	if boton == nil {
		t.Fatalf("falta el mando compacto %q en la cinta", rotulo)
	}
	if boton.Size().Width < 44 || boton.Size().Height < 44 {
		t.Fatalf("el mando compacto %q mide %v", rotulo, boton.Size())
	}
	return boton
}

func TestLaRotacionDeDiaposConservaMandosTactilesSinRecortarlos(t *testing.T) {
	for _, tactil := range []bool{false, true} {
		t.Run(fmt.Sprint(tactil), func(t *testing.T) {
			defer ui.TactilDePrueba(tactil)()
			e, _, w := montar(t, conFormas)
			w.Resize(fyne.NewSize(1024, 768))
			alto, mando := float32(36), float32(28)
			if tactil {
				alto, mando = 45, 44
			}
			if e.zona.Objects[0].Size().Height != alto || e.deslizBarra.Size().Height != alto-1 {
				t.Fatalf("mini-barra de alto %g no contiene la fila de %g: %v/%v", alto, mando, e.zona.Objects[0].Size(), e.deslizBarra.Size())
			}
			d := fyne.CurrentApp().Driver()
			origen := d.AbsolutePositionForObject(e.deslizBarra)
			for _, o := range e.miniBarra.Objects {
				if _, ok := o.(fyne.Focusable); !ok {
					continue
				}
				p := d.AbsolutePositionForObject(o)
				if o.Size().Height < mando || p.Y < origen.Y || p.Y+o.Size().Height > origen.Y+e.deslizBarra.Size().Height {
					t.Fatalf("mando recortado en mini-barra: %T %v/%v en %v/%v", o, p, o.Size(), origen, e.deslizBarra.Size())
				}
			}
			// El pie del carril también debe respetar los blancos táctiles: su
			// antiguo Resize(28,28) anulaba el mínimo44 declarado por BotonCinta.
			pie := d.AbsolutePositionForObject(e.carril)
			for _, o := range e.carril.pie {
				p := d.AbsolutePositionForObject(o)
				if o.Size().Width < mando || o.Size().Height < mando || p.Y < pie.Y || p.Y+o.Size().Height > pie.Y+e.carril.Size().Height {
					t.Fatalf("mando recortado en pie del carril: %v/%v", p, o.Size())
				}
			}
		})
	}
}
