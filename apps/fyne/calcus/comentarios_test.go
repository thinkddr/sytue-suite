// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// libroConIDs es un libro guardado por la web: con el id de cada hoja y una fila insertada
// encima (la lista `rowIds` materializada hasta ella y su base).
const libroConIDs = `{"active":0,"sheets":[` +
	`{"id":"hoja-a","rowIds":[{"id":"t0"},{"id":"n1a2b3c4d"},{"id":"t1"}],"rowBase":2,"name":"Datos","cells":{"A1":"x"}},` +
	`{"id":"hoja-b","name":"Hoja2","cells":{}}]}`

// TestLaIdentidadDeLasFilasEsLaDeLaWeb: los mismos nombres que `anclas.ts` para las
// posiciones materializadas y las implícitas, al insertar y al borrar.
func TestLaIdentidadDeLasFilasEsLaDeLaWeb(t *testing.T) {
	var e ejeAnclas
	if e.idDe(3) != "t3" {
		t.Errorf("sin lista, la fila 3 es %q", e.idDe(3))
	}
	e = e.mover(2, 1) // insertar en la 2: se materializan la 0 y la 1, y la nueva
	if len(e.ids) != 3 || e.ids[0] != "t0" || e.ids[1] != "t1" || !strings.HasPrefix(e.ids[2], "n") || e.base != 2 {
		t.Fatalf("tras insertar: %+v", e)
	}
	// La que era la 2 («t2») es ahora la 3, y sigue llamándose igual.
	if e.idDe(3) != "t2" {
		t.Errorf("la fila de debajo se llama %q", e.idDe(3))
	}
	if i, ok := e.indiceDe("t2"); !ok || i != 3 {
		t.Errorf("t2 está en %d %v", i, ok)
	}
	if i, ok := e.indiceDe(e.ids[2]); !ok || i != 2 {
		t.Errorf("la nueva está en %d %v", i, ok)
	}
	e = e.mover(0, -1) // borrar la 0
	if _, ok := e.indiceDe("t0"); ok {
		t.Error("la fila borrada sigue existiendo")
	}
	if i, ok := e.indiceDe("t2"); !ok || i != 2 {
		t.Errorf("tras borrar la de arriba, t2 está en %d %v", i, ok)
	}
	for _, malo := range []string{"", "t", "x1", "t12345678", "t-1"} {
		if _, ok := e.indiceDe(malo); ok {
			t.Errorf("%q no es una fila", malo)
		}
	}
}

// TestElLibroConservaLaIdentidadDeLaWeb: un libro con ids de hoja y de filas se entiende, se
// puede escribir, y sale con ellos tal cual; antes el `id` de la hoja lo cerraba entero.
func TestElLibroConservaLaIdentidadDeLaWeb(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(libroConIDs) {
		t.Fatal("un libro de la web con ids se abre en solo lectura")
	}
	if !igual(t, libroConIDs, c.Serializar()) {
		t.Errorf("entra\n %s\ny sale\n %s", libroConIDs, c.Serializar())
	}
	if ids := c.Libro.IDsHojas(); ids[0] != "hoja-a" || ids[1] != "hoja-b" {
		t.Errorf("ids: %v", ids)
	}
	// Una lista hostil se descarta, como `sanitizeAnclas`, sin cerrar el libro.
	if !c.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{},"rowIds":[{"id":"a b"}],"colIds":7}]}`) {
		t.Fatal("una lista de anclas mal formada cierra el libro")
	}
	if strings.Contains(c.Serializar(), "rowIds") || strings.Contains(c.Serializar(), "colIds") {
		t.Errorf("la lista mala sale al guardar: %s", c.Serializar())
	}
}

// TestElAnclaDeUnaCeldaSigueASuCelda: el ancla lleva la identidad de la fila y de la
// columna, así que insertar una fila encima la lleva a la de abajo y borrar la suya la deja
// sin celda; una hoja sin id ancla por el que la web deduce de su nombre, y renombrarla no lo
// pierde.
func TestElAnclaDeUnaCeldaSigueASuCelda(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(`{"active":0,"sheets":[{"name":"Datos","cells":{}},{"name":"Datos","cells":{}}]}`)
	if ids := c.Libro.IDsHojas(); ids[0] != "h:Datos" || ids[1] != "h:Datos#1" {
		t.Errorf("los ids deducidos son los de `conIdsDeHoja`: %v", ids)
	}
	c.Rejilla.Seleccionar(1, 3, false)
	a := c.AnclaActiva()
	if a != (api.AnclaCelda{Hoja: "h:Datos", Fila: "t3", Col: "t1", Text: "B4"}) {
		t.Fatalf("ancla de B4: %+v", a)
	}
	c.Libro.Estructura("insrow", "above", 0, 0, 0, 0)
	if c.Libro.RefDeAncla(a) != "B5" {
		t.Errorf("tras insertar una fila encima, el comentario está en %q", c.Libro.RefDeAncla(a))
	}
	c.Libro.Renombrar(0, "Cuentas")
	if c.Libro.RefDeAncla(a) != "B5" || c.Libro.IDsHojas()[0] != "h:Datos" {
		t.Errorf("renombrar la hoja pierde el ancla: %q %v", c.Libro.RefDeAncla(a), c.Libro.IDsHojas())
	}
	c.CambiarHoja(1)
	if c.Libro.RefDeAncla(a) != "Cuentas!B5" {
		t.Errorf("desde otra hoja se dice %q", c.Libro.RefDeAncla(a))
	}
	c.CambiarHoja(0)
	c.Libro.Estructura("delrow", "", 0, 4, 0, 4)
	if c.Libro.RefDeAncla(a) != "" {
		t.Errorf("con su fila borrada el ancla sigue en %q", c.Libro.RefDeAncla(a))
	}
	// De una combinada, su ancla: la celda que se ve.
	c.Libro.Combinar(2, 0, 3, 1, false)
	if got := c.Libro.AnclaDeCelda(3, 1); got.Text != "C1" {
		t.Errorf("en una combinada se ancla a %q", got.Text)
	}
}

// hiloEn es un comentario raíz anclado a una celda.
func hiloEn(id string, a api.AnclaCelda, resuelto bool, padre string) api.DocComment {
	b, _ := json.Marshal(a)
	return api.DocComment{ID: id, ParentID: padre, Body: "Revisa " + id, Resolved: resuelto, Anchor: b, CreatedAt: time.Now()}
}

// TestLaRejillaMarcaLasCeldasComentadas: el triángulo violeta en la esquina de arriba a la
// derecha de cada celda con hilos abiertos; ni los resueltos ni las respuestas marcan, y la
// marca sigue a su celda al insertar una fila encima.
func TestLaRejillaMarcaLasCeldasComentadas(t *testing.T) {
	r := rejillaDePrueba(500, 200)
	id := r.L.IDsHojas()[0]
	b2 := api.AnclaCelda{Hoja: id, Fila: "t1", Col: "t1", Text: "B2"}
	r.PonerComentarios([]api.DocComment{
		hiloEn("c1", b2, false, ""),
		hiloEn("c2", api.AnclaCelda{Hoja: id, Fila: "t3", Col: "t0", Text: "A4"}, true, ""),
		hiloEn("c3", api.AnclaCelda{Hoja: id, Fila: "t5", Col: "t0", Text: "A6"}, false, "c1"),
		hiloEn("c4", api.AnclaCelda{Hoja: "otra", Fila: "t0", Col: "t0", Text: "A1"}, false, ""),
	})
	r.Seleccionar(2, 5, false) // a la vista: la rejilla no se desplaza
	img := pinta(t, 500, 200, r)
	if got := r.r.marcas; len(got) != 1 || got[Celda{C: 1, F: 1}] != 1 {
		t.Fatalf("marcas: %v", got)
	}
	// La esquina de B2, por dentro de su línea derecha: violeta; la de abajo a la izquierda
	// del triángulo, papel.
	x, y := AnchoCabecera+2*AnchoCelda-2, AltoCelda+AltoCelda+1
	if px(img, x, y) != MarcaComentario {
		t.Errorf("la esquina de B2 es %v y la marca es %v", px(img, x, y), MarcaComentario)
	}
	if px(img, x-5, y+5) == MarcaComentario {
		t.Error("el triángulo pinta por debajo de su diagonal")
	}
	r.L.Estructura("insrow", "above", 0, 0, 0, 0)
	r.Refresh()
	pinta(t, 500, 200, r)
	if got := r.r.marcas; len(got) != 1 || got[Celda{C: 1, F: 2}] != 1 {
		t.Errorf("tras insertar una fila, las marcas: %v", got)
	}
	// Al pasar por su hilo, la celda se enmarca; fuera, no.
	r.ResaltarComentario(&b2)
	guarda(t, pinta(t, 500, 200, r))
	if !r.r.marcoComentario[0].Visible() || r.r.marcoComentario[0].Position() != fyne.NewPos(AnchoCabecera+AnchoCelda, AltoCelda+2*AltoCelda) {
		t.Errorf("el resalte no enmarca B3: %v", r.r.marcoComentario[0].Position())
	}
	r.ResaltarComentario(nil)
	pinta(t, 500, 200, r)
	if r.r.marcoComentario[0].Visible() {
		t.Error("el resalte sigue al salir del hilo")
	}
}

// TestElHiloDeUnaCeldaLlevaASuCelda: abierto el libro, la rejilla marca las celdas con los
// hilos del servicio sin abrir el panel; en el panel el hilo dice «Hoja2!B5» y su botón
// cambia de hoja y la selecciona.
func TestElHiloDeUnaCeldaLlevaASuCelda(t *testing.T) {
	s := &servicioHojas{rol: "commenter", hoja: libroConIDs, hilos: `{"comments":[` +
		`{"id":"c1","body":"Revisa","author_name":"Ana","created_at":"2026-01-02T10:00:00Z",` +
		`"anchor":{"hoja":"hoja-b","fila":"t4","col":"t1","text":"B5"}},` +
		`{"id":"c2","body":"Aquí","author_name":"Ana","created_at":"2026-01-02T10:00:00Z",` +
		`"anchor":{"hoja":"hoja-a","fila":"n1a2b3c4d","col":"t0","text":"A2"}}]}`}
	a := appDePrueba(t, s)
	a.AbrirID("h1", "")
	ab := a.abierto
	ab.lz.Rejilla.Resize(fyne.NewSize(500, 200))
	pinta(t, 500, 200, ab.lz.Rejilla)
	if got := ab.lz.Rejilla.r.marcas; len(got) != 1 || got[Celda{C: 0, F: 1}] != 1 {
		t.Errorf("con el panel cerrado, las marcas de Datos: %v", got)
	}
	ab.ctx.Paneles(editor.PanelComentarios)
	var boton *ui.Boton
	for _, o := range todos(ab.comentarios.Objeto()) {
		if b, ok := o.(*ui.Boton); ok && strings.Contains(b.Texto, "Hoja2!B5") {
			boton = b
		}
	}
	if boton == nil {
		t.Fatal("el hilo de la celda no dice «Celda Hoja2!B5»")
	}
	boton.Tapped(&fyne.PointEvent{})
	if ab.lz.Libro.Activa() != 1 || ab.lz.Rejilla.sel != (Celda{C: 1, F: 4}) {
		t.Errorf("el botón lleva a la hoja %d, celda %+v", ab.lz.Libro.Activa(), ab.lz.Rejilla.sel)
	}
}
