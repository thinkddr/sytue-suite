// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// piezasDe recorre una banda de la cinta y devuelve sus controles, en orden.
func piezasDe(o fyne.CanvasObject) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *editor.Grupo:
			for _, h := range x.Controles {
				ir(h)
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		default:
			out = append(out, o)
		}
	}
	ir(o)
	return out
}

// TestInicioEsElDeLaWeb: los controles de «Inicio» en el orden de Inicio.tsx, y la cinta
// refleja el formato bajo el cursor (fuente, tamaño y el estilo pulsado).
func TestInicioEsElDeLaWeb(t *testing.T) {
	f := folioCon(t, `<h2><span style="font-family: Georgia; font-size: x-large;">Alcance</span></h2><p>x</p>`)
	in := nuevoInicio("es")
	f.mover(punto{0, 3}, false)
	piezas := piezasDe(in.cuerpo(f.ctx))
	var selectores []*editor.SelectorCinta
	var fichas []*fichaEstilo
	muestras := 0
	for _, p := range piezas {
		switch x := p.(type) {
		case *editor.SelectorCinta:
			selectores = append(selectores, x)
		case *fichaEstilo:
			fichas = append(fichas, x)
		case *editor.MuestraCinta:
			muestras++
		}
	}
	if len(selectores) != 3 || muestras != 2 || len(fichas) != 5 {
		t.Fatalf("Inicio: %d selectores, %d muestras, %d fichas de estilo", len(selectores), muestras, len(fichas))
	}
	if selectores[0].Valor != "Georgia" || selectores[1].Valor != "5" || selectores[2].Valor != "1.5" {
		t.Errorf("los selectores reflejan fuente %q, tamaño %q e interlineado %q", selectores[0].Valor, selectores[1].Valor, selectores[2].Valor)
	}
	for _, fe := range fichas {
		if fe.activa != (fe.e.valor == "h2") {
			t.Errorf("la ficha %q pulsada = %v con el cursor en un h2", fe.e.valor, fe.activa)
		}
	}
	// Pulsar «Título 1» cambia el bloque.
	fichas[1].Tapped(nil)
	if got := f.Serializar(); got != `<h1><span style="font-family: Georgia; font-size: x-large;">Alcance</span></h1><p>x</p>` {
		t.Errorf("la ficha «Título 1»: %s", got)
	}
	if m := new(fichaEstilo); m.MinSize() != fyne.NewSize(72, 44) {
		t.Errorf("la ficha mide %v y en la web 72 × 44", m.MinSize())
	}
}

// TestLaDisposicionCambiaElFolio: el margen, la orientación y el tamaño cambian el ancho
// del texto y vuelven a repartir las líneas; sin tocar el documento.
func TestLaDisposicionCambiaElFolio(t *testing.T) {
	// Largo de sobra para que 96 px más de ancho se noten en el número de líneas.
	largo := "<p>" + repetir("Migración del correo y el almacenamiento de la organización. ", 24) + "</p>"
	f := folioCon(t, largo)
	antes := len(f.cajas[0].lineas)
	if f.anchoTexto() != 602 {
		t.Fatalf("el A4 con margen Normal deja 602 de texto y aquí %v", f.anchoTexto())
	}
	p := f.Pagina()
	p.Margen = 48
	f.PonerPagina(p)
	if f.anchoTexto() != 698 || len(f.cajas[0].lineas) >= antes {
		t.Errorf("con margen estrecho: %v de ancho y %d líneas (antes %d)", f.anchoTexto(), len(f.cajas[0].lineas), antes)
	}
	p.Apaisada = true
	f.PonerPagina(p)
	if ancho, alto := f.Pagina().medidas(); ancho != AltoPagina || alto != AnchoPagina {
		t.Errorf("apaisado: %v × %v", ancho, alto)
	}
	p.Apaisada, p.Tamano = false, "carta"
	f.PonerPagina(p)
	if ancho, alto := f.Pagina().medidas(); ancho != 816 || alto != 1056 {
		t.Errorf("Carta: %v × %v", ancho, alto)
	}
	if got := f.Serializar(); got != largo {
		t.Error("la disposición es de la vista: no puede cambiar el documento")
	}
}

func repetir(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

// TestAjustarAlAnchoEsElDeLaWeb: `Math.round((avail - 48) / pageW * 20) * 5`.
func TestAjustarAlAnchoEsElDeLaWeb(t *testing.T) {
	f := folioCon(t, "<p>x</p>")
	f.scroll.Resize(fyne.NewSize(1200, 600))
	f.AjustarAlAncho()
	// (1200 − 48) / 794 = 1,4509 → ×20 = 29,02 → 29 × 5 = 145.
	if f.ctx.Zoom != 145 {
		t.Errorf("ajustar al ancho en 1200 da %d %% y la web 145 %%", f.ctx.Zoom)
	}
}

// TestElAjusteInicialSigueAlAncho: en un teléfono el folio baja hasta caber
// (`floor((360 − 32) / 794 · 100)` = 41), vuelve a 100 si la ventana crece —el primer
// Layout llega con la ventana en su tamaño mínimo— y se retira en cuanto la persona toca
// el zoom.
func TestElAjusteInicialSigueAlAncho(t *testing.T) {
	f := folioCon(t, "<p>x</p>")
	f.ctx.Refrescar = nil
	for _, c := range []struct {
		ancho  float32
		quiero int
	}{{360, 41}, {1280, 100}, {600, 71}} {
		f.ajusteInicial(c.ancho)
		if f.ctx.Zoom != c.quiero {
			t.Errorf("con %v de ancho el zoom es %d y la web pone %d", c.ancho, f.ctx.Zoom, c.quiero)
		}
	}
	f.ctx.PonerZoom(150) // la persona
	f.ajusteInicial(360)
	if f.ctx.Zoom != 150 {
		t.Errorf("tras tocar el zoom a mano el ajuste lo ha cambiado a %d", f.ctx.Zoom)
	}
}

// TestLaPestanaInsertarEsLaDeLaWeb: «Insertar» va entre «Inicio» y «Disposición», como
// en la web, con Tabla, Imagen, Formas, Gráfico, SmartArt, Enlace, Elementos (regla, salto,
// símbolo), Encabezado y pie, y Comentario; y
// con el cursor en una tabla, las herramientas de celda detrás de «Tabla».
func TestLaPestanaInsertarEsLaDeLaWeb(t *testing.T) {
	vis := registroDocus("es").Visibles(editor.Documento)
	ids := make([]string, 0, len(vis))
	for _, p := range vis {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, " ") != "inicio insertar diseno referencias revisar vista" {
		t.Errorf("las pestañas de Docus: %v", ids)
	}
	// Y son EXACTAMENTE las de la taxonomía común con la web, en su orden.
	comun := office.PestanasDe(office.Documento)
	taxonomia := make([]string, 0, len(comun))
	for _, p := range comun {
		taxonomia = append(taxonomia, p.ID)
	}
	if strings.Join(ids, " ") != strings.Join(taxonomia, " ") {
		t.Errorf("la cinta de Docus pinta %v y la taxonomía común dice %v", ids, taxonomia)
	}
	botones := func(f *Folio) []string {
		var out []string
		for _, p := range piezasDe(nuevoInicio("es").cuerpoInsertar(f.ctx)) {
			switch x := p.(type) {
			case *editor.BotonCinta:
				out = append(out, x.Rotulo)
			case *editor.MuestraCinta:
				out = append(out, x.Rotulo)
			case *editor.SelectorCinta:
				out = append(out, "selector")
			}
		}
		return out
	}
	f := folioCon(t, `<p>x</p><table><tbody><tr><td>a</td></tr></tbody></table>`)
	f.mover(punto{0, 0}, false)
	quiero := "insert.table insert.image blocks.shapes blocks.chart smartart.insertOrEdit insert.link insert.horizontalRule insert.pageBreak insert.symbol " +
		"insert.header insert.footer insert.pageNumber insert.differentFirstPage insert.comment"
	if got := strings.Join(botones(f), " "); got != quiero {
		t.Errorf("Insertar fuera de una tabla:\n  hay     %s\n  quería  %s", got, quiero)
	}
	f.mover(punto{1, 0}, false)
	quiero = "insert.table table.rowAbove table.rowBelow table.colLeft table.colRight sheet.merge table.splitCell " +
		"table.cellColor selector table.delete insert.image blocks.shapes blocks.chart smartart.insertOrEdit insert.link insert.horizontalRule insert.pageBreak insert.symbol " +
		"insert.header insert.footer insert.pageNumber insert.differentFirstPage insert.comment"
	if got := strings.Join(botones(f), " "); got != quiero {
		t.Errorf("Insertar dentro de una tabla:\n  hay     %s\n  quería  %s", got, quiero)
	}
}

// TestInsertarDesdeLaCinta: las órdenes de «Insertar» llegan al folio, dejan el cursor
// donde la web (en la primera celda de la tabla nueva, detrás del enlace) y se deshacen de
// una vez. Se vio en ROJO sin el `default` de Mandar: la cinta no hacía nada.
func TestInsertarDesdeLaCinta(t *testing.T) {
	f := folioCon(t, `<p>hola</p>`)
	f.mover(punto{0, 4}, false)
	f.Mandar(editor.InsertarTabla, "2x2h")
	if got := f.Serializar(); got != `<p>hola</p>`+rico.HTMLTabla(2, 2, true) {
		t.Fatalf("insertar tabla: %s", got)
	}
	if c := f.bloques[f.cur.b].Celda; c == nil || !c.Cabecera || c.Col != 0 {
		t.Errorf("el cursor no está en la primera celda: %v", f.cur)
	}
	f.escribir("A")
	f.Mandar(editor.TablaFilaDebajo, "")
	if got := f.Serializar(); !strings.Contains(got, `<tr><th>A</th><th><br></th></tr><tr><td><br></td><td><br></td></tr><tr><td><br></td>`) {
		t.Errorf("fila debajo: %s", got)
	}
	f.Mandar(editor.TablaBorrar, "")
	if got := f.Serializar(); got != `<p>hola</p><p><br></p>` {
		t.Errorf("borrar la tabla: %s", got)
	}
	f.mover(punto{0, 0}, false)
	f.Mandar(editor.InsertarEnlace, "https://sytue.eu\nSytue")
	if got := f.Serializar(); !strings.HasPrefix(got, `<p><a href="https://sytue.eu">Sytue</a>hola</p>`) || f.cur != (punto{0, 5}) {
		t.Errorf("insertar enlace: %s, cursor %v", got, f.cur)
	}
	f.Mandar(editor.InsertarTexto, "€")
	if got := f.Serializar(); !strings.HasPrefix(got, `<p><a href="https://sytue.eu">Sytue€</a>hola</p>`) {
		t.Errorf("insertar símbolo: %s", got)
	}
	for len(f.hist.atras) > 0 {
		f.deshacer()
	}
	if got := f.Serializar(); got != `<p>hola</p>` {
		t.Errorf("deshacer todo: %s", got)
	}
}

// TestLaPestanaReferencias: la tabla de contenidos, su actualización y la nota al pie
// llegan al folio desde la cinta; el índice se maqueta dentro de su borde discontinuo con
// `padding: 8px 12px`, y tras «Nota al pie» se escribe la nota encima del texto
// provisional. Se vio en ROJO sin el `referencias` de Mandar: la cinta no hacía nada.
func TestLaPestanaReferencias(t *testing.T) {
	f := folioCon(t, `<h1>Uno</h1><p>texto</p>`)
	var rotulos []string
	for _, p := range piezasDe(nuevoInicio("es").cuerpoReferencias(f.ctx)) {
		if b, ok := p.(*editor.BotonCinta); ok {
			rotulos = append(rotulos, b.Rotulo)
		}
	}
	if got := strings.Join(rotulos, " "); got != "ref.tocInsert ref.tocUpdate ref.indexMark ref.indexBuild ref.captionTable ref.captionFigure ref.crossReference.insert ref.captionsUpdate ref.footnote ref.endnote ref.endnotesUpdate bib.sources bib.cite bib.update insert.link comments.add extdata.source mail.mergeField.insert merge.preview mail.finishMerge" {
		t.Errorf("Referencias: %s", got)
	}
	f.mover(punto{1, 5}, false)
	f.Mandar(editor.InsertarIndice, "Índice")
	var dentro []caja
	for _, c := range f.cajas {
		if c.recuadro != nil {
			dentro = append(dentro, c)
		}
	}
	if len(dentro) != 2 || dentro[0].izq != 13 || abs(dentro[0].y-(dentro[0].recuadro.y+9)) > 0.01 {
		t.Fatalf("el índice no va en su recuadro: %d cajas, la primera en x %.1f", len(dentro), dentro[0].izq)
	}
	f.Mandar(editor.NotaPie, "Texto de la nota")
	f.escribir("Mi nota")
	if got := f.Serializar(); !strings.Contains(got, `<li id="fn-1" value="1">Mi nota</li>`) || !strings.Contains(got, `<sup data-footnote-ref="1"`) {
		t.Errorf("la nota al pie: %s", got)
	}
}

// TestLaLeyendaPuedeIrSinTexto es `permitirVacio` de Referencias.tsx: con el campo vacío,
// aceptar sigue encendido e Intro inserta la leyenda solo con su etiqueta («Tabla 1»). Se
// vio en ROJO con `ui.Pregunta`: el botón apagado e Intro sin insertar nada.
func TestLaLeyendaPuedeIrSinTexto(t *testing.T) {
	f := folioCon(t, `<p>x</p>`)
	b := nuevoInicio("es").botonLeyenda(f.ctx, iconos.EdLeyenda, "ref.captionTable", rico.LeyendaTabla, "dialog.caption.table")
	w := test.NewTempWindow(t, b)
	w.Resize(fyne.NewSize(1280, 800))
	b.AlPulsar()
	campo, ok := w.Canvas().Focused().(*ui.Entrada)
	if !ok {
		t.Fatalf("el foco ha caído en %T y tiene que caer en el campo", w.Canvas().Focused())
	}
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := f.Serializar(); !strings.Contains(got, `data-caption="tabla"`) || strings.Contains(got, "</span>. ") {
		t.Errorf("Intro con el campo vacío tenía que insertar la leyenda sin texto: %s", got)
	}
}
