// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// avisosDe apunta los avisos del lienzo, con su tono.
func avisosDe(lz *Lienzo) *[]string {
	var out []string
	lz.AlAviso = func(tono ui.TonoAviso, titulo, _ string) {
		out = append(out, map[ui.TonoAviso]string{ui.AvisoInfo: "info", ui.AvisoAdvertencia: "warning"}[tono]+":"+titulo)
	}
	return &out
}

// TestLaProteccionSeLeeYSeEscribeComoEnLaWeb: la hoja y el libro protegidos ya no cierran
// el documento; se sanean como `sanitizeProtection` —solo lo que difiere del defecto— y
// vuelven a salir al guardar. Lo que no es un objeto no protege.
func TestLaProteccionSeLeeYSeEscribeComoEnLaWeb(t *testing.T) {
	lz := lienzoDePrueba()
	doc := `{"active":0,"protect":{"password":"","x":1},"sheets":[{"name":"H","cells":{"A1":"1"},` +
		`"protect":{"password":"k","selectCells":false,"formatCells":true,"sort":true,"insertRows":false,"otro":true}}]}`
	if !lz.Cargar(doc) {
		t.Fatal("un libro protegido tiene que abrirse para escribir")
	}
	p := lz.Libro.Proteccion()
	if p == nil || p.Password != "k" || p.SelectCells == nil || *p.SelectCells || !p.FormatCells || !p.Sort || p.InsertRows {
		t.Fatalf("la protección de la hoja se lee como %+v", p)
	}
	if lz.Libro.ProteccionLibro() == nil {
		t.Fatal("el libro protegido no se lee")
	}
	g := lz.Serializar()
	if !strings.Contains(g, `"protect":{"password":"k","selectCells":false,"formatCells":true,"sort":true}`) ||
		!strings.Contains(g, `"protect":{}`) {
		t.Errorf("la protección no vuelve a salir como en la web: %s", g)
	}
	lz.Cargar(`{"active":0,"protect":true,"sheets":[{"name":"H","cells":{},"protect":null}]}`)
	if lz.Libro.Proteccion() != nil || lz.Libro.ProteccionLibro() != nil {
		t.Error("lo que no es un objeto no protege (`typeof raw !== 'object'`)")
	}
}

// TestLaHojaProtegidaNoDejaEscribirEnLoBloqueado: con la hoja protegida solo se escribe
// en lo desbloqueado; el formato, solo con su permiso; lo que tiene permiso propio,
// solo concedido; y el aviso no se repite al teclear.
func TestLaHojaProtegidaNoDejaEscribirEnLoBloqueado(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"x","A2":"y"},"styles":{"B1":{"locked":false}},"protect":{}}]}`)
	avisos := avisosDe(lz)
	r := lz.Rejilla
	r.Editar("")
	if r.Editando() || r.Escribir("nuevo") || lz.Libro.Crudo(0, 0) != "x" {
		t.Error("A1 está bloqueada y se ha podido escribir")
	}
	if len(*avisos) != 1 || (*avisos)[0] != "info:sheet.msg.protectedSheet" {
		t.Errorf("los avisos son %v: uno solo (el antirrebote), informativo", *avisos)
	}
	r.Seleccionar(1, 0, false)
	if !r.Escribir("libre") || lz.Libro.Crudo(1, 0) != "libre" {
		t.Error("B1 está desbloqueada y no se ha podido escribir")
	}
	r.Seleccionar(0, 0, false)
	r.Seleccionar(1, 0, true)
	r.BorrarRango()
	if lz.Libro.Crudo(1, 0) != "libre" {
		t.Error("borrar un rango con una celda bloqueada no se hace")
	}
	r.Seleccionar(1, 0, false)
	for _, cmd := range []editor.Comando{editor.Negrita, CmdOrdenar, CmdPegar, CmdCombinar} {
		antes := lz.Serializar()
		lz.Mandar(cmd, "asc")
		if lz.Serializar() != antes {
			t.Errorf("%s pasa con la hoja protegida y sin permiso", cmd)
		}
	}
	// Con el permiso de formato, la negrita sí.
	lz.Mandar(CmdDesproteger, "")
	lz.Mandar(CmdProteger, `{"formatCells":true}`)
	lz.Mandar(editor.Negrita, "")
	if b, _, _ := lz.Libro.Formato(1, 0); !b {
		t.Error("con «Formato de celdas» concedido la negrita no llega")
	}
}

// TestLaFormulaOcultaNoSeVe: con la hoja protegida, la fórmula `hidden` no sale en la
// barra ni al copiar —sale su valor—, ni se puede editar aunque esté desbloqueada.
func TestLaFormulaOcultaNoSeVe(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"=1+1"},"styles":{"A1":{"locked":false,"hidden":true}},"protect":{}}]}`)
	lz.Rejilla.Seleccionar(0, 0, false)
	if got := lz.Barra.ed.Texto(); got != "" {
		t.Errorf("la barra enseña %q", got)
	}
	lz.Rejilla.Copiar(nil, false)
	if got := lz.Rejilla.copiado.celdas[0][0].html; got != "2" {
		t.Errorf("se copia %q y tiene que ser el valor", got)
	}
	if lz.Libro.Editable(0, 0) {
		t.Error("una fórmula oculta no se edita")
	}
}

// TestSinSeleccionarCeldasSoloSeSeleccionaLoDesbloqueado: las flechas saltan lo
// bloqueado hasta lo siguiente desbloqueado, y un clic en lo bloqueado no selecciona.
func TestSinSeleccionarCeldasSoloSeSeleccionaLoDesbloqueado(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{},"styles":{"B1":{"locked":false},"D1":{"locked":false}},` +
		`"protect":{"selectCells":false}}]}`)
	r := lz.Rejilla
	r.sel = Celda{C: 1}
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if r.sel != (Celda{C: 3}) {
		t.Errorf("la flecha lleva a %v y tiene que saltar hasta D1", r.sel)
	}
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if r.sel != (Celda{C: 3}) {
		t.Errorf("sin nada desbloqueado a la derecha no se mueve, y va a %v", r.sel)
	}
	r.pulsar(zonaCelda, Celda{C: 0, F: 4})
	if r.sel != (Celda{C: 3}) {
		t.Error("un clic en una celda bloqueada la ha seleccionado")
	}
}

// TestElLibroProtegidoNoCambiaSuEstructura: con el libro protegido no se añaden ni
// borran hojas, y se dice; la contraseña equivocada no lo desprotege; y deshacer
// devuelve la protección, que es parte del documento.
func TestElLibroProtegidoNoCambiaSuEstructura(t *testing.T) {
	lz := lienzoDePrueba()
	avisos := avisosDe(lz)
	lz.Mandar(CmdProtegerLibro, `{"password":"k"}`)
	lz.AnadirHoja()
	if len(lz.Libro.Hojas()) != 1 || len(*avisos) != 1 || (*avisos)[0] != "warning:sheet.msg.protectedBook" {
		t.Errorf("con el libro protegido: %d hojas y avisos %v", len(lz.Libro.Hojas()), *avisos)
	}
	lz.Mandar(CmdDesprotegerLibro, "mal")
	if lz.Libro.ProteccionLibro() == nil || (*avisos)[1] != "warning:sheet.msg.wrongPassword" {
		t.Error("la contraseña equivocada lo ha desprotegido, o no lo ha dicho")
	}
	lz.Mandar(CmdDesprotegerLibro, "k")
	if lz.Libro.ProteccionLibro() != nil {
		t.Fatal("la contraseña buena no lo desprotege")
	}
	lz.Deshacer(false)
	if lz.Libro.ProteccionLibro() == nil {
		t.Error("deshacer no devuelve la protección del libro")
	}
}

// TestElPanelProtegerHojaProtege: el escudo de Datos abre «Proteger hoja»; lo marcado
// en sus casillas y la contraseña llegan a la protección, y el escudo se enciende.
func TestElPanelProtegerHojaProtege(t *testing.T) {
	lz := lienzoDePrueba()
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
	cuerpo := lz.cuerpoDatos(ctx)
	w := test.NewTempWindow(t, cuerpo)
	w.Resize(fyne.NewSize(1400, 500))
	escudo := botonDeCinta(cuerpo, "data.protectSheet")
	if escudo == nil {
		t.Fatal("Datos no tiene el escudo de «Proteger hoja»")
	}
	escudo.Tapped(nil)
	casillas := func() map[string]*casillaXS {
		out := map[string]*casillaXS{}
		for _, o := range todos(w.Canvas().Overlays().Top()) {
			if e, ok := o.(*casillaXS); ok {
				out[e.casilla.Etiqueta] = e
			}
		}
		return out
	}
	cs := casillas()
	if len(cs) != 11 || !cs["data.perm.selectCells"].Marcada() || cs["data.perm.sort"].Marcada() {
		t.Fatalf("el panel tiene %d casillas (ocultar, seleccionar, formato y los ocho permisos)", len(cs))
	}
	cs["data.perm.sort"].Tapped(nil)
	cs = casillas() // el panel se rehace con lo marcado
	if !cs["data.perm.sort"].Marcada() {
		t.Fatal("la casilla no se queda marcada")
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el panel abierto
	for _, o := range todos(w.Canvas().Overlays().Top()) {
		if e, ok := o.(*ui.Entrada); ok {
			e.PonerTexto("k")
		}
		if b, ok := o.(*editor.BotonPequeno); ok && b.Texto == "data.protectSheet" {
			b.Tapped(nil)
		}
	}
	p := lz.Libro.Proteccion()
	if p == nil || !p.Sort || p.Password != "k" || p.FormatCells || !lz.Estado().Protegida {
		t.Errorf("el panel protege con %+v", p)
	}
}

// botonDeCinta busca un botón de la cinta por su rótulo.
func botonDeCinta(o fyne.CanvasObject, rotulo string) *editor.BotonCinta {
	for _, x := range todos(o) {
		if b, ok := x.(*editor.BotonCinta); ok && b.Rotulo == rotulo {
			return b
		}
	}
	return nil
}

// todos es el árbol entero que cuelga de un objeto, widgets por dentro incluidos.
func todos(o fyne.CanvasObject) []fyne.CanvasObject {
	out := []fyne.CanvasObject{o}
	switch v := o.(type) {
	case *fyne.Container:
		for _, h := range v.Objects {
			out = append(out, todos(h)...)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(v).Objects() {
			out = append(out, todos(h)...)
		}
	}
	return out
}
