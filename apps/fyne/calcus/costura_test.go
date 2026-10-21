// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// elLibroDeServicioDocs es el documento de hoja de cálculo que usa de muestra el test
// de fidelidad de exportación de `services/docs`
// (`internal/domain/export_fidelity_test.go`, `sheetJSON`). No es un ejemplo
// inventado: es el que el servicio da por bueno.
//
// Trae formatos de número y una celda en negrita (`<b>10</b>`), y los dos se saben
// reescribir desde que el lienzo lee `formats`: su sitio es la prueba de ida y vuelta.
const elLibroDeServicioDocs = `{"active":0,"sheets":[` +
	`{"name":"Datos","cells":{"A1":"Nombre","B1":"<b>10</b>","A2":"Total","B2":"=B1*2"},` +
	`"formats":{"B1":{"id":"currency"},"B2":{"id":"number","decimals":1}}},` +
	`{"name":"Hoja2","cells":{"A1":"x"}}]}`

func lienzoDePrueba() *Lienzo {
	c := NuevoLienzo(NuevoLibro())
	c.Resize(fyne.NewSize(560, 300))
	return c
}

// TestNoSeAbreLoQueNoSeSabeReescribir es LA prueba de esta pieza, y la única regla que
// no se negocia: si el libro trae algo que no sabemos volver a escribir, `Cargar`
// devuelve false y el cromo lo enseña en solo lectura.
//
// Cada caso es una manera distinta de perder datos en silencio si esto dijera que sí.
func TestNoSeAbreLoQueNoSeSabeReescribir(t *testing.T) {
	for _, c := range []struct {
		nombre, doc string
	}{
		{"datos externos", `{"active":0,"sheets":[{"name":"H","cells":{},"externalTables":[{"name":"T","range":"A1:B2"}]}]}`},
		{"un formato fuera de la rejilla", `{"active":0,"sheets":[{"name":"H","cells":{},"formats":{"AA1":{"id":"number"}}}]}`},
		{"un estilo fuera de la rejilla", `{"active":0,"sheets":[{"name":"H","cells":{},"styles":{"A101":{"u":true}}}]}`},
		{"celda con un color en línea", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"<span style=\"color:red\">x</span>"}}]}`},
		{"celda subrayada con etiqueta", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"<u>x</u>"}}]}`},
		{"celda con un salto de línea", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x<br>y"}}]}`},
		{"celda con una entidad que stripHtml no decodifica", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"caf&eacute;"}}]}`},
		{"celda fuera de la rejilla A1:Z100", `{"active":0,"sheets":[{"name":"H","cells":{"AA1":"x"}}]}`},
		{"fila fuera de la rejilla", `{"active":0,"sheets":[{"name":"H","cells":{"A101":"x"}}]}`},
		{"no es JSON", `{rota`},
		{"sin hojas", `{"active":0,"sheets":[]}`},
	} {
		l := lienzoDePrueba()
		if l.Cargar(c.doc) {
			t.Errorf("%s: Cargar ha dicho que lo entiende ENTERO, y no es verdad:\n  %s", c.nombre, c.doc)
			continue
		}
		// Y el cinturón: el cromo apaga la cinta, pero las teclas de la rejilla las
		// apaga el lienzo. Sin esto se podría escribir en un documento que no
		// entendemos con solo teclear encima de una celda.
		l.Rejilla.Editar("escrito")
		l.Rejilla.Escribir("escrito")
		l.Rejilla.Pegar("pegado", false)
		if l.Rejilla.Editando() || l.Libro.Crudo(0, 0) == "escrito" || l.Libro.Crudo(0, 0) == "pegado" ||
			l.Libro.PuedeDeshacer() {
			t.Errorf("%s: el lienzo ha dejado escribir en un documento que no entiende", c.nombre)
		}
	}
}

// TestElViajeDeIdaYVueltaEsSinPerdida: un libro que SÍ se entiende tiene que volver a
// salir con exactamente los mismos datos. Es la otra mitad de la regla — negarse a
// abrir lo que no se entiende no vale de nada si lo que sí se entiende se estropea.
//
// Se compara el JSON DECODIFICADO y no la cadena: el orden de las claves de un objeto
// no es dato, y exigirlo haría fallar el test por algo que no le importa a nadie.
func TestElViajeDeIdaYVueltaEsSinPerdida(t *testing.T) {
	// El mismo libro de `services/docs`, quitando lo que hoy no se sabe reescribir:
	// los formatos de número y la celda con HTML.
	const doc = `{"active":1,"sheets":[` +
		`{"name":"Datos","cells":{"A1":"Nombre","B1":"10","A2":"Total","B2":"=B1*2"}},` +
		`{"name":"Hoja2","cells":{"A1":"x","Z100":"esquina"}}]}`

	c := lienzoDePrueba()
	if !c.Cargar(doc) {
		t.Fatal("Cargar no ha entendido un libro de celdas planas")
	}
	if igual(t, doc, c.Serializar()) != true {
		t.Error("el viaje de ida y vuelta ha cambiado el documento")
	}
	// Y con formato: el libro de muestra de `services/docs`, y un estilo y unos paneles
	// inmovilizados como los escribe la web.
	for _, conFormato := range []string{
		elLibroDeServicioDocs,
		`{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},` +
			`"formats":{"A1":{"id":"custom","mask":"0.00;[Rojo]-0.00"}},` +
			`"styles":{"A1":{"font":"Georgia","size":14,"color":"#DC2626","fill":"#FEF08A","u":true,` +
			`"valign":"middle","wrap":true,"border":{"t":true,"b":true},"locked":false}},` +
			`"freeze":{"cols":1,"rows":2}}]}`,
	} {
		d := lienzoDePrueba()
		if !d.Cargar(conFormato) {
			t.Fatalf("Cargar no ha entendido un libro con formato:\n  %s", conFormato)
		}
		if !igual(t, conFormato, d.Serializar()) {
			t.Errorf("el viaje de ida y vuelta ha cambiado el formato:\n  %s\n  %s", conFormato, d.Serializar())
		}
	}

	// Y sigue siendo sin pérdida DESPUÉS de editar: se escribe en la hoja activa
	// (Hoja2) y la otra tiene que salir intacta, fórmula incluida.
	c.Rejilla.Seleccionar(1, 0, false)
	c.Rejilla.Escribir("nuevo")
	var vuelta modelo
	if err := json.Unmarshal([]byte(c.Serializar()), &vuelta); err != nil {
		t.Fatal(err)
	}
	if got := vuelta.Sheets[0].Cells["B2"]; got != "=B1*2" {
		t.Errorf("la fórmula de la hoja que no se tocó ha salido como %q", got)
	}
	if got := vuelta.Sheets[1].Cells["B1"]; got != "nuevo" {
		t.Errorf("lo escrito en la hoja activa ha salido como %q", got)
	}
	if got := vuelta.Sheets[1].Cells["Z100"]; got != "esquina" {
		t.Errorf("la celda del último rincón de la rejilla ha salido como %q: el recorrido no llega al final", got)
	}
	if vuelta.Active != 1 {
		t.Errorf("la hoja activa ha salido como %d y era la 1", vuelta.Active)
	}
}

// TestLasFormulasVenLasOtrasHojas: al cargar se meten TODAS las hojas en el motor, no
// solo la que se está viendo. Con una sola, `=Hoja2!A1` daría #REF! y el usuario vería
// un error donde tiene un dato.
func TestLasFormulasVenLasOtrasHojas(t *testing.T) {
	const doc = `{"active":0,"sheets":[` +
		`{"name":"Datos","cells":{"A1":"=Hoja2!A1*2"}},` +
		`{"name":"Hoja2","cells":{"A1":"21"}}]}`
	c := lienzoDePrueba()
	if !c.Cargar(doc) {
		t.Fatal("Cargar no ha entendido el libro")
	}
	if got := c.Libro.Texto(0, 0); got != "42" {
		t.Errorf("`=Hoja2!A1*2` con 21 al otro lado da %q y tenía que dar 42", got)
	}
}

// TestLasEstadisticasSonHojas: en Calcus la unidad de la barra de estado es la hoja, y
// las palabras son 0 —como en la web, donde solo Docus las cuenta.
func TestLasEstadisticasSonHojas(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(`{"active":1,"sheets":[{"name":"A","cells":{}},{"name":"B","cells":{}}]}`) {
		t.Fatal("Cargar no ha entendido el libro")
	}
	if u, total, palabras := c.Estadisticas(); u != 2 || total != 2 || palabras != 0 {
		t.Errorf("las estadísticas dan (%d, %d, %d) y tenían que ser (2, 2, 0)", u, total, palabras)
	}
}

// TestLoQueNoSeEntiendeSeEnsenaCerrado: decir «no» es no dejar escribir, no dejar la
// rejilla en blanco. Un libro con una clave que este lienzo no conoce enseña igual sus
// celdas, CON su formato de número.
func TestLoQueNoSeEntiendeSeEnsenaCerrado(t *testing.T) {
	c := lienzoDePrueba()
	conFuturo := strings.Replace(elLibroDeServicioDocs, `"name":"Hoja2"`, `"name":"Hoja2","futuro":[{"id":"g"}]`, 1)
	if c.Cargar(conFuturo) {
		t.Fatal("el libro con una clave desconocida se ha dado por entendido")
	}
	if !c.Rejilla.SoloLectura {
		t.Error("el lienzo no se ha cerrado")
	}
	for _, celda := range []struct {
		c, f   int
		quiero string
		porQue string
	}{
		{0, 0, "Nombre", "un literal"},
		{1, 0, "10,00\u00a0€", "una celda en negrita con formato de moneda"},
		{1, 1, "20,0", "una fórmula que lee la celda en negrita, a un decimal"},
	} {
		if got := c.Libro.Texto(celda.c, celda.f); got != celda.quiero {
			t.Errorf("%s (%s) se ve %q y tenía que verse %q", Ref(celda.c, celda.f), celda.porQue, got, celda.quiero)
		}
	}
	if got := c.Libro.Hojas(); len(got) != 2 || got[1] != "Hoja2" {
		t.Errorf("las hojas del libro son %v", got)
	}
}

// TestLoQueNoSeHaTocadoSaleConLosMismosBytes: las celdas con negrita, alineación,
// entidades o una fórmula con comillas —que en la web se guardan como `&quot;`— se
// entienden, se pintan y vuelven al documento TAL CUAL. Y lo que se escribe aquí sale
// escapado, como lo escribe la web: una celda con «x<y» no puede llegar al documento
// como una etiqueta rota.
func TestLoQueNoSeHaTocadoSaleConLosMismosBytes(t *testing.T) {
	const doc = `{"active":0,"sheets":[{"name":"H","cells":{` +
		`"A1":"<b>10</b>",` +
		`"A2":"<div style=\"text-align: right;\"><i>a &amp; b</i></div>",` +
		`"A3":"=CONCAT(&quot;x&quot;;A1)",` +
		`"A4":"a&nbsp;b"}}]}`
	c := lienzoDePrueba()
	if !c.Cargar(doc) {
		t.Fatal("Cargar no ha entendido el envoltorio canónico de la web")
	}
	if !igual(t, doc, c.Serializar()) {
		t.Error("el viaje de ida y vuelta ha cambiado celdas que nadie tocó")
	}
	if got := c.Libro.Texto(0, 2); got != "x10" {
		t.Errorf("`=CONCAT(\"x\";A1)` con un 10 en negrita da %q", got)
	}
	if got := c.Libro.Crudo(0, 1); got != "a & b" {
		t.Errorf("la barra de fórmulas enseñaría %q y no «a & b»", got)
	}
	if n, cur, al := c.Libro.Formato(0, 1); n || !cur || al != "right" {
		t.Errorf("A2 es cursiva alineada a la derecha y el formato leído es negrita=%v cursiva=%v %q", n, cur, al)
	}

	// Escribir conserva el formato y escapa: la negrita de A1 sigue, y «<» sale `&lt;`.
	c.Rejilla.Seleccionar(0, 0, false)
	c.Rejilla.Escribir("x<y")
	var vuelta modelo
	if err := json.Unmarshal([]byte(c.Serializar()), &vuelta); err != nil {
		t.Fatal(err)
	}
	if got := vuelta.Sheets[0].Cells["A1"]; got != "<b>x&lt;y</b>" {
		t.Errorf("«x<y» escrito sobre una celda en negrita ha salido como %q", got)
	}
}

// igual compara dos documentos por su contenido, no por su texto: el orden de las
// claves de un objeto no es dato.
func igual(t *testing.T, a, b string) bool {
	t.Helper()
	var ma, mb any
	if err := json.Unmarshal([]byte(a), &ma); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(b), &mb); err != nil {
		t.Fatalf("lo serializado no es JSON válido: %v", err)
	}
	if !reflect.DeepEqual(ma, mb) {
		t.Logf("antes: %s\nahora: %s", a, b)
		return false
	}
	return true
}

// TestUnLibroConMedidasSeEditaYSaleIgual: con `colWidths` y `rowHeights` (anchos y altos que
// trae un .xlsx o un .ods importado) el libro no se abre en solo lectura, y al guardarlo las
// medidas salen como entraron aunque este lienzo no las pinte. Una medida absurda se descarta.
func TestUnLibroConMedidasSeEditaYSaleIgual(t *testing.T) {
	doc := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"colWidths":{"0":150,"3":64.5},"rowHeights":{"0":30}}]}`
	lz := NuevoLienzo(NuevoLibro())
	if !lz.Cargar(doc) {
		t.Fatal("un libro con medidas se abre en solo lectura")
	}
	var a, b any
	_ = json.Unmarshal([]byte(doc), &a)
	_ = json.Unmarshal([]byte(lz.Serializar()), &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("entra\n %s\ny sale\n %s", doc, lz.Serializar())
	}
	conCombinadas := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"merges":["A1:C1","B3:B4"]}]}`
	if !lz.Cargar(conCombinadas) || !strings.Contains(lz.Serializar(), `"merges":["A1:C1","B3:B4"]`) {
		t.Errorf("las celdas combinadas se conservan: %s", lz.Serializar())
	}
	malo := `{"active":0,"sheets":[{"name":"H","cells":{},"colWidths":{"x":150,"1":-3,"2":0}}]}`
	if !lz.Cargar(malo) || strings.Contains(lz.Serializar(), "colWidths") {
		t.Errorf("las medidas inválidas se descartan: %s", lz.Serializar())
	}
}

// El color de pestaña que trae un libro importado (`tabColor`) no cierra el libro a la
// edición, se conserva al guardar y se pinta en su pestaña.
func TestColorDePestanaSeConserva(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(`{"active":0,"sheets":[{"name":"Ventas","tabColor":"#00B050","cells":{"A1":"1"}},{"name":"Otra","cells":{}}]}`) {
		t.Fatal("un tabColor abre el libro en solo lectura")
	}
	if got := c.Libro.ColorPestana(0); got != "#00B050" {
		t.Errorf("ColorPestana = %q", got)
	}
	if out := c.Serializar(); !strings.Contains(out, `"tabColor":"#00B050"`) || strings.Count(out, "tabColor") != 1 {
		t.Errorf("al guardar: %s", out)
	}
}
