// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// formularioWeb es lo que escribe `buildFormFieldHtml` con los tres tipos.
const formularioWeb = `<p>Nombre: <span data-formfield="text" data-name="Nombre" data-placeholder="Tu nombre"></span></p>` +
	`<p>Acepto <span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="0"></span> las bases</p>` +
	`<p>Talla <span data-formfield="select" contenteditable="false" data-name="Talla" data-options="S|M|L" data-value="" data-placeholder="Elegir…"></span></p>`

// folioFormulario es el folio con el formulario en una ventana: los paneles de los campos
// necesitan un lienzo donde abrirse.
func folioFormulario(t *testing.T) (*Folio, *int) {
	t.Helper()
	f := folioCon(t, formularioWeb)
	ediciones := 0
	f.AlEditar = func() { ediciones++ }
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(900, 500))
	return f, &ediciones
}

// enCampo es el punto de la página en el centro del campo i.
func enCampo(t *testing.T, f *Folio, i int) fyne.Position {
	t.Helper()
	fs := f.fichasCampo()
	if i >= len(fs) {
		t.Fatalf("hay %d campos y se pide el %d", len(fs), i)
	}
	z := f.escala()
	ox, oy := f.pag.origen()
	return fyne.NewPos(ox+(fs[i].x+fs[i].w/2)*z, oy+(fs[i].y+fs[i].h/2)*z)
}

func clic(f *Folio, pos fyne.Position) {
	f.pag.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: pos}, Button: desktop.MouseButtonPrimary})
	f.pag.MouseUp(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: pos}, Button: desktop.MouseButtonPrimary})
}

// TestLosCamposDeFormularioSePintanComoFichas: un documento con los tres tipos se abre
// editable, y cada campo es una ficha atómica con la medida de `editor.css`: el texto vacío
// con su marcador y al menos 2,5 em, la casilla de 1,1 em y el desplegable con su flecha.
func TestLosCamposDeFormularioSePintanComoFichas(t *testing.T) {
	f := folioCon(t, formularioWeb)
	if !f.editable() {
		t.Fatalf("un formulario de la web tiene que abrirse editable: %v", f.doc.NoEntendido())
	}
	fs := f.fichasCampo()
	if len(fs) != 3 {
		t.Fatalf("se han colocado %d campos y son 3", len(fs))
	}
	e := f.cajas[0].b.Base
	if fs[0].w < minFicha*e.Tamano || fs[0].w < medidor.ancho("Tu nombre", e)+10 {
		t.Errorf("el campo de texto mide %v: no cabe su marcador", fs[0].w)
	}
	if fs[1].w != ladoCasilla*e.Tamano || fs[1].h != ladoCasilla*e.Tamano {
		t.Errorf("la casilla mide %v × %v y es de 1,1 em", fs[1].w, fs[1].h)
	}
	if fs[2].w < medidor.ancho("Elegir…", e)+huecoFlecha+anchoFlecha*e.Tamano {
		t.Errorf("el desplegable mide %v: no cabe el marcador con la flecha", fs[2].w)
	}
	// Lo de después de la casilla empieza después de su caja, no encima.
	l := f.cajas[1].lineas[0]
	if fin := l.piezas[len(l.piezas)-1]; fin.x < fs[1].x-f.cajas[1].izq+fs[1].w {
		t.Errorf("«las bases» empieza en %v y la casilla acaba en %v", fin.x, fs[1].x+fs[1].w)
	}
	// Se pinta la caja tenue con el marcador en su tinta; marcada, la casilla lleva el tic.
	objs := f.pintarCajas(nil, f.cajas, 0, 0, 1, map[string]*canvas.Image{})
	cajas, marcador := 0, false
	for _, o := range objs {
		switch x := o.(type) {
		case *canvas.Rectangle:
			if x.FillColor == rico.FondoCampo && x.StrokeColor == rico.BordeCampo {
				cajas++
			}
		case *canvas.Text:
			if x.Text == "Tu nombre" && x.Color == rico.TintaMarcador {
				marcador = true
			}
		}
	}
	if cajas != 3 || !marcador {
		t.Errorf("cajas de campo %d (3) y marcador pintado %v", cajas, marcador)
	}
	if strings.Contains(textoPintado(objs), string(rico.ObjetoRuna)) {
		t.Error("la runa del objeto no se pinta: es la ficha")
	}
}

func textoPintado(objs []fyne.CanvasObject) string {
	var b strings.Builder
	for _, o := range objs {
		if x, ok := o.(*canvas.Text); ok {
			b.WriteString(x.Text)
		}
	}
	return b.String()
}

// TestRellenarFormularioConElRatonYConElTeclado: en el modo «Rellenar formulario» el folio
// no se escribe, pero la casilla se marca con un clic, el desplegable elige entre sus
// opciones y el de texto guarda lo escrito; con el teclado, Tab recorre los campos e Intro
// los activa. Todo vuelve al HTML del campo como en la web, avisa para guardar y se deshace.
func TestRellenarFormularioConElRatonYConElTeclado(t *testing.T) {
	f, ediciones := folioFormulario(t)
	// Sin el modo, un clic en la casilla no la marca: coloca el cursor, como en la web.
	clic(f, enCampo(t, f, 1))
	if strings.Contains(f.Serializar(), `data-checked="1"`) {
		t.Fatal("fuera del modo rellenar, la casilla no se marca")
	}
	f.AlternarRellenar()
	if !f.Rellenando() || f.editable() {
		t.Fatal("rellenando, el folio no se escribe")
	}
	f.pag.TypedRune('Q')
	if strings.Contains(f.Serializar(), "Q") {
		t.Fatal("rellenando, teclear no escribe en el folio")
	}

	clic(f, enCampo(t, f, 1))
	if !strings.Contains(f.Serializar(), `data-name="Acepto" data-checked="1"`) || *ediciones != 1 {
		t.Fatalf("el clic en la casilla la marca y avisa (%d ediciones):\n%s", *ediciones, f.Serializar())
	}
	if pintado := f.pintarCajas(nil, f.cajas, 0, 0, 1, map[string]*canvas.Image{}); !hayTic(pintado) {
		t.Error("marcada, la casilla lleva el tic")
	}
	f.Mandar(editor.Deshacer, "")
	if !strings.Contains(f.Serializar(), `data-checked="0"`) {
		t.Fatal("deshacer devuelve la casilla como estaba")
	}

	// Con el teclado: el clic dejó el foco en la casilla; Tab lleva al desplegable, y desde
	// el último Tab ya no es del folio (sale, como de la última ficha con `tabindex`).
	if f.campoFoco != 1 {
		t.Fatalf("el clic deja el foco en la casilla y está en %d", f.campoFoco)
	}
	if !f.pag.AcceptsTab() {
		t.Fatal("con otro campo detrás, Tab es del folio")
	}
	f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if f.campoFoco != 2 || f.pag.AcceptsTab() {
		t.Fatalf("Tab lleva al desplegable (foco en %d) y desde el último sale", f.campoFoco)
	}
	f.campoFoco = 2
	f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if f.panelCampo == nil || len(f.opcionesCampo) != 3 || f.opcionesCampo[1].Texto != "M" {
		t.Fatalf("Intro en el desplegable abre sus tres opciones: %v", f.opcionesCampo)
	}
	test.Tap(f.opcionesCampo[1])
	if !strings.Contains(f.Serializar(), `data-options="S|M|L" data-value="M"`) || f.panelCampo != nil {
		t.Fatalf("elegir «M» la guarda y cierra el menú:\n%s", f.Serializar())
	}
	if r := f.doc.Respuestas(); r[2].Valor != "M" {
		t.Fatalf("respuestas: %+v", r)
	}

	// El de texto abre su campo con el nombre; Intro guarda lo escrito, en una línea.
	clic(f, enCampo(t, f, 0))
	if f.entradaCampo == nil || f.panelCampo == nil {
		t.Fatal("el clic en el campo de texto abre donde escribir")
	}
	test.Type(f.entradaCampo, "Ana López")
	f.entradaCampo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !strings.Contains(f.Serializar(), `data-placeholder="Tu nombre">Ana López</span>`) {
		t.Fatalf("lo escrito vuelve al HTML del campo:\n%s", f.Serializar())
	}
	if got := rico.RespuestasATexto(f.doc.Respuestas(), "-"); got != "Nombre: Ana López\nAcepto: No\nTalla: M" {
		t.Fatalf("respuestas: %q", got)
	}
	// Ya con valor, el campo se pinta con él y no con el marcador.
	if txt := textoPintado(f.pintarCajas(nil, f.cajas, 0, 0, 1, map[string]*canvas.Image{})); !strings.Contains(txt, "Ana López") || strings.Contains(txt, "Tu nombre") {
		t.Errorf("pintado: %q", txt)
	}

	// Al salir del modo se vuelve a escribir en el folio.
	f.AlternarRellenar()
	if f.Rellenando() || !f.editable() {
		t.Fatal("fuera del modo, el folio se escribe")
	}
}

func hayTic(objs []fyne.CanvasObject) bool {
	for _, o := range objs {
		if l, ok := o.(*canvas.Line); ok && l.StrokeColor == rico.TintaMarca {
			return true
		}
	}
	return false
}

// TestLoEscritoNoSePierdeSiElDocumentoSeRehaceDebajo: con el panel del campo de texto
// abierto llega la coedición y vuelve a leer el documento; lo que se aplica va al campo del
// documento nuevo, no al leído al abrir el panel (que ya no está en él).
func TestLoEscritoNoSePierdeSiElDocumentoSeRehaceDebajo(t *testing.T) {
	f, _ := folioFormulario(t)
	f.AlternarRellenar()
	clic(f, enCampo(t, f, 0))
	if f.entradaCampo == nil {
		t.Fatal("el clic en el campo de texto abre donde escribir")
	}
	f.Fusionar(f.Serializar())
	test.Type(f.entradaCampo, "Ana")
	f.entradaCampo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !strings.Contains(f.Serializar(), `data-placeholder="Tu nombre">Ana</span>`) {
		t.Fatalf("lo escrito tras la fusión se ha perdido:\n%s", f.Serializar())
	}
	f.Fusionar(f.Serializar())
	clic(f, enCampo(t, f, 2))
	f.Fusionar(f.Serializar())
	test.Tap(f.opcionesCampo[0])
	if !strings.Contains(f.Serializar(), `data-value="S"`) {
		t.Fatalf("la opción elegida tras la fusión se ha perdido:\n%s", f.Serializar())
	}
}

// TestMayusTabDesdeElPrimerCampoSaleYElSiguienteTabEmpiezaPorElPrimero: salir hacia atrás
// olvida el campo enfocado, como salir hacia delante desde el último.
func TestMayusTabDesdeElPrimerCampoSaleYElSiguienteTabEmpiezaPorElPrimero(t *testing.T) {
	f, _ := folioFormulario(t)
	f.AlternarRellenar()
	f.campoFoco = 0
	f.mayus = true
	if f.pag.AcceptsTab() {
		t.Fatal("Mayús+Tab desde el primer campo sale del folio")
	}
	f.mayus = false
	if !f.pag.AcceptsTab() {
		t.Fatal("de vuelta, Tab es del folio")
	}
	f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if f.campoFoco != 0 {
		t.Fatalf("el Tab siguiente empieza por el primer campo y está en %d", f.campoFoco)
	}
}

// TestSoloLecturaNoRellena: sin permiso de edición el modo no cambia nada.
func TestSoloLecturaNoRellena(t *testing.T) {
	f, _ := folioFormulario(t)
	f.AlternarRellenar()
	f.ctx.SoloLectura = true
	clic(f, enCampo(t, f, 1))
	if strings.Contains(f.Serializar(), `data-checked="1"`) {
		t.Fatal("en solo lectura la casilla no se marca")
	}
}

// TestVerRespuestasListaLosCampos: «Ver respuestas» enseña el nombre y el valor de cada
// campo (— si está vacío) y «Copiar respuestas» copia «Nombre: valor» por línea; sin campos,
// lo dice. En la cinta, «Rellenar formulario» se queda pulsado mientras dura el modo.
func TestVerRespuestasListaLosCampos(t *testing.T) {
	f, _ := folioFormulario(t)
	c := f.ctx
	var avisos []string
	c.Notificar = func(_ ui.TonoAviso, titulo string) { avisos = append(avisos, titulo) }
	f.AlternarRellenar()
	f.rellenarCampo(f.fichasCampo()[1].campo, "1")
	textos, copiar := textosYBotones(panelRespuestas(c, f))
	quiero := "Nombre|—|Acepto|Sí|Talla|—"
	if strings.Join(textos, "|") != quiero {
		t.Fatalf("respuestas pintadas:\n  hay     %s\n  quería  %s", strings.Join(textos, "|"), quiero)
	}
	if copiar == nil {
		t.Fatal("falta «Copiar respuestas»")
	}
	test.Tap(copiar)
	if got := fyne.CurrentApp().Clipboard().Content(); got != "Nombre: \nAcepto: Sí\nTalla: " || len(avisos) != 1 || avisos[0] != "form.responses.copied" {
		t.Fatalf("copiado %q, avisos %v", got, avisos)
	}

	vacio := folioCon(t, `<p>sin campos</p>`)
	if textos, copiar := textosYBotones(panelRespuestas(vacio.ctx, vacio)); copiar != nil || len(textos) != 1 || textos[0] != "form.responses.empty" {
		t.Fatalf("sin campos: %v", textos)
	}

	g := grupoFormulario(c, f)
	for _, p := range piezasDe(g) {
		if b, ok := p.(interface{ AccessibilityLabel() string }); ok && b.AccessibilityLabel() == "form.fill" {
			return
		}
	}
	t.Error("la cinta no tiene «Rellenar formulario»")
}

// textosYBotones son los párrafos de un panel, en orden, y su botón (si lo tiene).
func textosYBotones(o fyne.CanvasObject) ([]string, *ui.Boton) {
	var textos []string
	var boton *ui.Boton
	var ir func(o fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Parrafo:
			textos = append(textos, x.Texto)
		case *ui.Boton:
			boton = x
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case *container.Scroll:
			ir(x.Content)
		}
	}
	ir(o)
	return textos, boton
}

// TestLasCapturasDelFormulario deja, con SYTUE_CAPTURAS=<dir>, el formulario pintado fuera
// y dentro del modo rellenar, con el foco del teclado en la casilla.
func TestLasCapturasDelFormulario(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("SYTUE_CAPTURAS no está puesta")
	}
	f, _ := folioFormulario(t)
	f.rellenarCampo(f.fichasCampo()[2].campo, "M") // no rellena: aún no está en el modo
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(700, 300))
	guardar := func(nombre string) {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(filepath.Join(dir, nombre))
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		if err := png.Encode(out, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	guardar("docus-formulario.png")
	f.AlternarRellenar()
	f.rellenarCampo(f.fichasCampo()[1].campo, "1")
	f.rellenarCampo(f.fichasCampo()[2].campo, "M")
	f.enfocado = true
	f.campoFoco = 1
	f.pag.Refresh()
	guardar("docus-formulario-rellenar.png")
}
