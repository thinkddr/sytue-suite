// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// TestBuscarRecorreLaHojaFilaAFila: el orden es el de leer la hoja —B1 antes que A2—,
// se busca en el texto crudo (la fórmula también), y la coincidencia activa es la celda
// seleccionada, a la vista.
func TestBuscarRecorreLaHojaFilaAFila(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A2":"hola","B1":"<b>Hola hola</b>","C3":"=LEN(\"hola\")","A30":"x"}}]}`)
	b := &busqueda{lz: lz}
	if n := b.Buscar("hola", false, false); n != 4 {
		t.Fatalf("hay cuatro «hola» (dos en B1 y uno en la fórmula de C3) y salen %d", n)
	}
	celdas := make([]string, 0, len(b.hallazgos))
	for _, h := range b.hallazgos {
		celdas = append(celdas, Ref(h.c, h.f))
	}
	if got := celdas; got[0] != "B1" || got[1] != "B1" || got[2] != "A2" || got[3] != "C3" {
		t.Errorf("el orden es fila a fila: %v", got)
	}
	b.IrACoincidencia(-1) // con vuelta: la última
	if lz.Rejilla.Seleccion() != (Celda{C: 2, F: 2}) || b.Activa() != 3 {
		t.Errorf("ir a la última selecciona %v (activa %d)", lz.Rejilla.Seleccion(), b.Activa())
	}
	if b.Buscar("Hola", true, false) != 1 || b.Buscar("hola", false, true) != 4 {
		t.Error("las opciones de mayúsculas y palabra completa no son las de `matchIndices`")
	}
}

// TestReemplazarEscribeComoTeclear: reemplazar una conserva la negrita de la celda y pasa
// a la siguiente; «reemplazar todo» es UN paso de deshacer; y lo bloqueado se salta.
func TestReemplazarEscribeComoTeclear(t *testing.T) {
	lz := lienzoDePrueba()
	const doc = `{"active":0,"sheets":[{"name":"H","cells":{"A1":"<b>gato y gato</b>","A2":"gato","B2":"gato"},` +
		`"styles":{"A1":{"locked":false},"A2":{"locked":false}},"protect":{}}]}`
	lz.Cargar(doc)
	avisos := avisosDe(lz)
	b := &busqueda{lz: lz}
	b.Buscar("gato", false, false)
	b.IrACoincidencia(0)
	b.Reemplazar("perro", "gato", false, false)
	if got := lz.Libro.HTML(0, 0); got != "<b>perro y gato</b>" {
		t.Errorf("reemplazar la primera deja %q", got)
	}
	if b.Coincidencias() != 3 || b.hallazgos[b.Activa()] != (hallazgo{0, 0, 8, 12}) {
		t.Errorf("tras reemplazar, la activa es la siguiente de la misma celda: %+v", b.hallazgos)
	}
	antes := lz.Libro.Crudo(0, 0)
	b.ReemplazarTodo("pez", "gato", false, false)
	if lz.Libro.Crudo(0, 0) != "perro y pez" || lz.Libro.Crudo(0, 1) != "pez" || lz.Libro.Crudo(1, 1) != "gato" {
		t.Errorf("reemplazar todo: A1 %q, A2 %q, B2 (bloqueada) %q",
			lz.Libro.Crudo(0, 0), lz.Libro.Crudo(0, 1), lz.Libro.Crudo(1, 1))
	}
	if len(*avisos) != 1 {
		t.Errorf("lo bloqueado se avisa una vez: %v", *avisos)
	}
	if b.Coincidencias() != 1 {
		t.Errorf("queda la de la celda bloqueada y el contador dice %d", b.Coincidencias())
	}
	lz.Deshacer(false)
	if lz.Libro.Crudo(0, 0) != antes || lz.Libro.Crudo(0, 1) != "gato" {
		t.Errorf("deshacer «reemplazar todo» de UN paso: A1 %q, A2 %q", lz.Libro.Crudo(0, 0), lz.Libro.Crudo(0, 1))
	}
}

// TestReemplazarRespetaLaValidacionYLaSoloLectura: una validación estricta no deja
// escribir lo que no admite, y en solo lectura no se reemplaza nada.
func TestReemplazarRespetaLaValidacionYLaSoloLectura(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"Sí","B1":"Sí"},` +
		`"validations":[{"range":"A1","values":["Sí","No"],"strict":true}]}]}`)
	b := &busqueda{lz: lz}
	b.Buscar("Sí", true, false)
	b.ReemplazarTodo("Quizá", "Sí", true, false)
	if lz.Libro.Crudo(0, 0) != "Sí" || lz.Libro.Crudo(1, 0) != "Quizá" {
		t.Errorf("la estricta deja %q en A1 y B1 queda %q", lz.Libro.Crudo(0, 0), lz.Libro.Crudo(1, 0))
	}
	lz.SoloLectura(true)
	b.Buscar("Sí", true, false)
	b.Reemplazar("No", "Sí", true, false)
	b.ReemplazarTodo("No", "Sí", true, false)
	if lz.Libro.Crudo(0, 0) != "Sí" || b.Editable() {
		t.Error("en solo lectura se ha reemplazado")
	}
}

// TestLaBusquedaViejaNoEscribeEnOtraHoja: si la hoja activa cambia después de buscar,
// reemplazar vuelve a buscar en vez de escribir en la celda que apuntaba la otra.
func TestLaBusquedaViejaNoEscribeEnOtraHoja(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"A","cells":{"C5":"uno"}},{"name":"B","cells":{"A1":"otro"}}]}`)
	b := &busqueda{lz: lz}
	b.Buscar("uno", false, false)
	lz.Libro.Activar(1)
	b.Reemplazar("dos", "uno", false, false)
	if lz.Libro.Crudo(2, 4) != "" || b.Coincidencias() != 0 {
		t.Errorf("se ha escrito %q en C5 de la hoja B con una búsqueda de la hoja A", lz.Libro.Crudo(2, 4))
	}
}

// TestElPanelBuscarDeCalcusSePinta: el marco de Calcus con «Buscar» abierto enseña el
// panel de verdad —no el campo suelto de reserva—, siempre el mismo objeto.
func TestElPanelBuscarDeCalcusSePinta(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(elLibroDeServicioDocs)
	ctx := &editor.Contexto{Tipo: editor.Hoja, Estado: editor.Listo, Zoom: 100, Lienzo: lz, Panel: editor.PanelBuscar}
	ctx.Paneles = panelesDe(lz, nil)
	if panel := ctx.Paneles(editor.PanelBuscar); panel == nil || panel != ctx.Paneles(editor.PanelBuscar) {
		t.Fatal("el panel tiene que ser SIEMPRE el mismo objeto, o lo tecleado se pierde")
	}
	if ctx.Paneles(editor.PanelEsquema) != nil {
		t.Error("los demás paneles son los de reserva")
	}
	m := editor.NuevoMarco(ctx, Registro(lz), nil)
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(m)
	c.Resize(fyne.NewSize(1280, 800))
	img := c.Capture()
	if img.Bounds().Dx() == 0 {
		t.Fatal("no se ha pintado nada")
	}
	guarda(t, img)
}

// TestBuscarPintaLasCeldasQueCasan: como en la web, las celdas con coincidencias salen en
// amarillo (#fef08a), la actual —la seleccionada— en melocotón (#fed7aa), y al buscar
// nada vuelven al papel.
func TestBuscarPintaLasCeldasQueCasan(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A2":"hola","B1":"hola","C1":"nada"}}]}`)
	en := func(c, f int) (float32, float32) { // a la derecha del texto, en mitad de la fila
		return AnchoCabecera + float32(c+1)*AnchoCelda - 10, AltoBarra + AltoCelda*float32(f+1) + AltoCelda/2
	}
	b := &busqueda{lz: lz}
	b.Buscar("hola", false, false)
	b.IrACoincidencia(1) // A2
	img := pinta(t, 560, 300, lz)
	for _, caso := range []struct {
		c, f int
		want color.NRGBA
	}{{1, 0, editor.ResalteBuscar}, {0, 1, editor.ResalteActivo}, {2, 0, Papel}} {
		x, y := en(caso.c, caso.f)
		if got := px(img, x, y); !cerca(got, caso.want) {
			t.Errorf("%s pinta %v y debería ser %v", Ref(caso.c, caso.f), got, caso.want)
		}
	}
	b.Buscar("", false, false)
	img = pinta(t, 560, 300, lz)
	if x, y := en(1, 0); !cerca(px(img, x, y), Papel) {
		t.Errorf("sin búsqueda B1 sigue pintando %v", px(img, x, y))
	}
}

// TestCerrarBuscarQuitaElAmarillo: como en la web, al cerrar el panel se quita el
// resaltado de las celdas; al volver a abrirlo, con lo tecleado, se repinta.
func TestCerrarBuscarQuitaElAmarillo(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A2":"hola","B1":"hola"}}]}`)
	ctx := &editor.Contexto{Tipo: editor.Hoja, Estado: editor.Listo, Zoom: 100, Lienzo: lz}
	ctx.Paneles = panelesDe(lz, nil)
	editor.NuevoMarco(ctx, Registro(lz), nil)
	ctx.AbrirPanel(editor.PanelBuscar)
	lz.busqueda.Buscar("hola", false, false)
	if len(lz.Rejilla.resaltes) != 2 {
		t.Fatalf("con el panel abierto hay %d celdas pintadas", len(lz.Rejilla.resaltes))
	}
	ctx.AbrirPanel(editor.PanelBuscar) // el mismo botón lo cierra
	if len(lz.Rejilla.resaltes) != 0 {
		t.Error("al cerrar el panel las celdas siguen en amarillo")
	}
	ctx.AbrirPanel(editor.PanelBuscar)
	if len(lz.Rejilla.resaltes) != 2 {
		t.Error("al volver a abrirlo no se repinta lo buscado")
	}
}
