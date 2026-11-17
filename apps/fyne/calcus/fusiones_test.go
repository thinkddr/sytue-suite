// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// vectoresFusiones es la sección `fusiones` de los vectores compartidos con la web.
type vectoresFusiones struct {
	Parse []struct {
		Merges []string
		Sale   []Fusion
	}
	FusionEn struct {
		Merges []string
		Casos  []struct {
			C, R int
			Sale *Fusion
		}
	} `json:"fusionEn"` //nolint:tagliatelle // el nombre lo fija la web
	Ventana struct {
		Merges []string
		Casos  []struct {
			C1, R1, C2, R2 int
			Tope           *int
			Sale           []Fusion
		}
	}
	Mover struct {
		Merges []string
		Casos  []struct {
			Base   struct{ C, R int }
			Dc, Dr int
			Sale   struct{ C, R int }
		}
	}
	Rango []struct {
		F    Fusion
		Sale string
	}
	Combinar []struct {
		Merges []string
		Rect   Fusion
		Sale   *struct {
			Merges []string
			Fusion Fusion
		}
	}
	Separar []struct {
		Merges []string
		Rect   Fusion
		Sale   []string
	}
	Hojas []struct {
		Cells     map[string]string
		Merges    []string
		Rect      Fusion
		Centrar   bool
		Cubiertas []string
		Pierde    bool
		Sale      *struct {
			Cells  map[string]string
			Merges []string
			Fusion Fusion
		}
	}
}

func leerVectoresFusiones(t *testing.T) vectoresFusiones {
	t.Helper()
	b, err := os.ReadFile(rutaVectores)
	if err != nil {
		t.Fatalf("no se leen los vectores de la web: %v", err)
	}
	var v struct{ Fusiones vectoresFusiones }
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("los vectores no se entienden: %v", err)
	}
	if len(v.Fusiones.Parse) == 0 || len(v.Fusiones.Hojas) == 0 {
		t.Fatal("los vectores no traen la sección de combinadas")
	}
	return v.Fusiones
}

// celdasDe pasa el mapa A1 de los vectores a coordenadas, y al revés.
func celdasDe(t *testing.T, m map[string]string) map[Celda]string {
	t.Helper()
	out := map[Celda]string{}
	for ref, html := range m {
		r, ok := formula.ParseRangeRef(ref)
		if !ok || r.C1 != r.C2 || r.R1 != r.R2 {
			t.Fatalf("referencia rota en los vectores: %q", ref)
		}
		out[Celda{C: r.C1, F: r.R1}] = html
	}
	return out
}

func refsDe(m map[Celda]string) map[string]string {
	out := map[string]string{}
	for k, html := range m {
		out[Ref(k.C, k.F)] = html
	}
	return out
}

// TestVectoresDeFusiones: el puerto de `sheets/fusiones.ts` da lo mismo que la web en
// cada caso: leer las combinadas, cuál cubre una celda, cuáles tocan la ventana, a
// dónde lleva una flecha, combinar, separar y lo que combinar le hace a una hoja.
func TestVectoresDeFusiones(t *testing.T) {
	v := leerVectoresFusiones(t)
	for _, c := range v.Parse {
		if got := parseFusiones(c.Merges); !reflect.DeepEqual(comoJSON(t, orVacia(got)), comoJSON(t, orVacia(c.Sale))) {
			t.Errorf("parseFusiones(%q) = %v; la web da %v", c.Merges, got, c.Sale)
		}
	}
	base := parseFusiones(v.FusionEn.Merges)
	for _, c := range v.FusionEn.Casos {
		got, ok := fusionEn(base, c.C, c.R)
		if ok != (c.Sale != nil) || (ok && got != *c.Sale) {
			t.Errorf("fusionEn(%d, %d) = %v, %v; la web da %v", c.C, c.R, got, ok, c.Sale)
		}
	}
	base = parseFusiones(v.Ventana.Merges)
	for _, c := range v.Ventana.Casos {
		tope := TopeFusionesVentana
		if c.Tope != nil {
			tope = *c.Tope
		}
		if got := fusionesEnVentana(base, c.C1, c.R1, c.C2, c.R2, tope); !slices.Equal(orVacia(got), orVacia(c.Sale)) {
			t.Errorf("fusionesEnVentana(%d,%d,%d,%d) = %v; la web da %v", c.C1, c.R1, c.C2, c.R2, got, c.Sale)
		}
	}
	base = parseFusiones(v.Mover.Merges)
	for _, c := range v.Mover.Casos {
		got := moverPorFusiones(base, Celda{C: c.Base.C, F: c.Base.R}, c.Dc, c.Dr, 25, 99)
		if got != (Celda{C: c.Sale.C, F: c.Sale.R}) {
			t.Errorf("moverPorFusiones(%v, %d, %d) = %v; la web da %v", c.Base, c.Dc, c.Dr, got, c.Sale)
		}
	}
	for _, c := range v.Rango {
		if got := fusionARango(c.F); got != c.Sale {
			t.Errorf("fusionARango(%v) = %q; la web da %q", c.F, got, c.Sale)
		}
	}
	for _, c := range v.Combinar {
		merges, u, ok := combinar(c.Merges, c.Rect)
		if ok != (c.Sale != nil) || (ok && (!slices.Equal(merges, c.Sale.Merges) || u != c.Sale.Fusion)) {
			t.Errorf("combinar(%q, %v) = %q, %v, %v; la web da %+v", c.Merges, c.Rect, merges, u, ok, c.Sale)
		}
	}
	for _, c := range v.Separar {
		quedan, ok := separar(c.Merges, c.Rect)
		if ok != (c.Sale != nil) || (ok && !slices.Equal(quedan, c.Sale)) {
			t.Errorf("separar(%q, %v) = %q, %v; la web da %q", c.Merges, c.Rect, quedan, ok, c.Sale)
		}
	}
	for _, c := range v.Hojas {
		celdas := celdasDe(t, c.Cells)
		var cubiertas []string
		for _, k := range celdasCubiertas(celdas, c.Rect) {
			cubiertas = append(cubiertas, Ref(k.C, k.F))
		}
		slices.Sort(cubiertas)
		if !slices.Equal(orVacia(cubiertas), orVacia(c.Cubiertas)) {
			t.Errorf("celdasCubiertas(%v, %v) = %q; la web da %q", c.Cells, c.Rect, cubiertas, c.Cubiertas)
		}
		if got := combinarPierdeDatos(celdas, c.Rect); got != c.Pierde {
			t.Errorf("combinarPierdeDatos(%v, %v) = %v; la web da %v", c.Cells, c.Rect, got, c.Pierde)
		}
		out, merges, u, ok := combinarHoja(celdas, c.Merges, c.Rect, c.Centrar)
		if ok != (c.Sale != nil) {
			t.Errorf("combinarHoja(%v, %v) combina=%v; la web da %+v", c.Cells, c.Rect, ok, c.Sale)
			continue
		}
		if ok && (!reflect.DeepEqual(refsDe(out), c.Sale.Cells) || !slices.Equal(merges, c.Sale.Merges) || u != c.Sale.Fusion) {
			t.Errorf("combinarHoja(%v, %v, %v) = %v, %q, %v; la web da %+v", c.Cells, c.Rect, c.Centrar, refsDe(out), merges, u, c.Sale)
		}
	}
}

// orVacia iguala la lista nula y la vacía: la web siempre devuelve `[]`.
func orVacia[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// libroCabecera es el caso del plan: un .xlsx con la cabecera A1:D1 combinada, con
// relleno y centrada, y en C1 lo que dejó otra aplicación debajo (texto y relleno rojo),
// que no se ve porque está cubierto.
const libroCabecera = `{"active":0,"sheets":[{"name":"H","cells":{"A1":"<div style=\"text-align: center;\">Ventas</div>","C1":"oculto","A2":"1"},` +
	`"styles":{"A1":{"fill":"#FEF08A"},"C1":{"fill":"#FECACA"}},"merges":["A1:D1"]}]}`

// TestLaCabeceraCombinadaSePintaEntera: el ancla ocupa A1:D1 entera —su relleno llega
// hasta D—, sin la línea de la cuadrícula entre columnas, y la cubierta C1 no se pinta.
//
// Visto en rojo antes del cambio: el amarillo acababa en x=136 y en C1 salía el rojo.
func TestLaCabeceraCombinadaSePintaEntera(t *testing.T) {
	lz := lienzoDePrueba()
	if !lz.Cargar(libroCabecera) {
		t.Fatal("no se abre el libro")
	}
	r := lz.Rejilla
	r.Move(fyne.NewPos(0, 0))
	r.Resize(fyne.NewSize(500, 200))
	r.Seleccionar(1, 4, false) // lejos de la cabecera
	img := pinta(t, 500, 200, r)
	guarda(t, img)
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 0xff}
	rojo := color.NRGBA{R: 0xFE, G: 0xCA, B: 0xCA, A: 0xff}
	y := AltoCelda + AltoCelda/2 - 6 // por encima del texto, en la fila 1
	for _, x := range []float32{60, 136, 232, 250, 380, 420} {
		if got := px(img, x, y); !cerca(got, amarillo) {
			t.Errorf("la combinada A1:D1 va de 40 a 424 con el relleno del ancla; en (%v, %v) hay %v", x, y, got)
		}
		if got := px(img, x, y); cerca(got, rojo) {
			t.Errorf("C1 está cubierta y su relleno no se pinta; en (%v, %v) sale", x, y)
		}
	}
	if got := px(img, 440, y); cerca(got, amarillo) {
		t.Error("E1 no es de la combinada y sale amarilla")
	}

	// Con A fuera de la vista, el ancla se sigue pintando: la cabecera no sale en blanco.
	r.desX = AnchoCelda
	img = pinta(t, 500, 200, r)
	if got := px(img, 200, y); !cerca(got, amarillo) {
		t.Errorf("desplazada una columna, B1:D1 sigue siendo la combinada; en (200, %v) hay %v", y, got)
	}
}

// TestLaCubiertaVaAlAncla: seleccionar una cubierta selecciona el ancla, escribir en ella
// escribe en el ancla, y las flechas saltan la combinada entera.
func TestLaCubiertaVaAlAncla(t *testing.T) {
	lz := lienzoDePrueba()
	if !lz.Cargar(libroCabecera) {
		t.Fatal("no se abre el libro")
	}
	r := lz.Rejilla
	r.Seleccionar(2, 0, false)
	if r.Seleccion() != (Celda{}) {
		t.Fatalf("pulsar C1 (cubierta) deja la selección en %v y no en A1", r.Seleccion())
	}
	if !r.Escribir("Ingresos") || lz.Libro.Crudo(0, 0) != "Ingresos" || lz.Libro.Crudo(2, 0) != "oculto" {
		t.Errorf("escribir sobre la combinada va al ancla: A1=%q, C1=%q", lz.Libro.Crudo(0, 0), lz.Libro.Crudo(2, 0))
	}
	r.mover(r.Seleccion(), 1, 0, false)
	if r.Seleccion() != (Celda{C: 4}) {
		t.Errorf("→ desde A1:D1 va a E1, no a %v", r.Seleccion())
	}
	r.mover(r.Seleccion(), -1, 0, false)
	if r.Seleccion() != (Celda{}) {
		t.Errorf("← desde E1 cae en la combinada y queda en su ancla, no en %v", r.Seleccion())
	}
	r.mover(Celda{C: 3, F: 1}, 0, -1, false)
	if r.Seleccion() != (Celda{}) {
		t.Errorf("↑ desde D2 cae en D1, cubierta, y queda en A1, no en %v", r.Seleccion())
	}
}

// TestCombinarSepararYDeshacer: «Combinar y centrar» sin datos que perder combina a la
// primera y centra el ancla; «Separar» la quita; deshacer vuelve atrás cada paso, y lo
// guardado lleva `merges` como la web.
func TestCombinarSepararYDeshacer(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"B2":"Total"}}]}`)
	r := lz.Rejilla
	r.Seleccionar(1, 1, false)
	r.Seleccionar(3, 2, true)
	lz.Mandar(CmdCombinar, "center")
	if u, ok := lz.Libro.FusionEn(3, 2); !ok || u != (Fusion{C1: 1, F1: 1, C2: 3, F2: 2}) {
		t.Fatalf("«Combinar y centrar» en B2:D3 no deja la combinada: %v, %v", u, ok)
	}
	if got := lz.Libro.HTML(1, 1); got != `<div style="text-align: center;">Total</div>` {
		t.Errorf("el ancla se centra: %q", got)
	}
	if r.Seleccion() != (Celda{C: 1, F: 1}) || r.ext != nil {
		t.Errorf("tras combinar la selección es el ancla sola: %v, %v", r.Seleccion(), r.ext)
	}
	if !strings.Contains(lz.Serializar(), `"merges":["B2:D3"]`) {
		t.Errorf("lo guardado no lleva la combinada: %s", lz.Serializar())
	}

	lz.Mandar(CmdCombinar, "unmerge")
	if _, ok := lz.Libro.FusionEn(3, 2); ok || strings.Contains(lz.Serializar(), `"merges"`) {
		t.Errorf("«Separar» no quita la combinada: %s", lz.Serializar())
	}
	lz.Deshacer(false)
	if _, ok := lz.Libro.FusionEn(3, 2); !ok {
		t.Error("deshacer «Separar» no vuelve a combinar")
	}
	lz.Deshacer(false)
	if _, ok := lz.Libro.FusionEn(3, 2); ok || lz.Libro.HTML(1, 1) != "Total" {
		t.Errorf("deshacer «Combinar» no deja la hoja como estaba: %q", lz.Libro.HTML(1, 1))
	}

	// «Combinar celdas» no centra, y «Combinar contenido» es la de siempre, sin combinada.
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 1, true)
	lz.Mandar(CmdCombinar, "merge")
	if _, ok := lz.Libro.FusionEn(2, 1); !ok || lz.Libro.HTML(1, 1) != "Total" {
		t.Errorf("«Combinar celdas» combina sin centrar: %q", lz.Libro.HTML(1, 1))
	}
	lz.Deshacer(false)
	lz.Libro.Escribir(2, 1, "neto")
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 1, true)
	lz.Mandar(CmdCombinar, "content")
	if _, ok := lz.Libro.FusionEn(2, 1); ok || lz.Libro.Crudo(1, 1) != "Total neto" || lz.Libro.Crudo(2, 1) != "" {
		t.Errorf("«Combinar contenido» une los textos sin combinar: %q, %q", lz.Libro.Crudo(1, 1), lz.Libro.Crudo(2, 1))
	}
}

// TestCombinarConDatosPregunta: si combinar vaciaría celdas escritas, se pregunta antes
// con el diálogo de peligro; cancelar no toca nada y confirmar combina en un solo paso.
func TestCombinarConDatosPregunta(t *testing.T) {
	lz := lienzoEnVentana(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"uno","B1":"dos"}}]}`)
	w := fyne.CurrentApp().Driver().CanvasForObject(lz)
	r := lz.Rejilla
	r.Seleccionar(0, 0, false)
	r.Seleccionar(1, 0, true)
	antes := lz.Serializar()
	lz.Mandar(CmdCombinar, "")
	if lz.Serializar() != antes || lz.combinarPendiente == nil {
		t.Fatal("combinar A1:B1 con B1 escrito tiene que preguntar antes de vaciarla")
	}
	if len(w.Overlays().List()) == 0 {
		t.Fatal("no se enseña la pregunta")
	}
	w.Overlays().Remove(w.Overlays().Top()) // cerrar sin confirmar
	if lz.Serializar() != antes {
		t.Error("cerrar la pregunta ha cambiado la hoja")
	}
	lz.combinarPendiente()
	if _, ok := lz.Libro.FusionEn(1, 0); !ok || lz.Libro.Crudo(1, 0) != "" || lz.Libro.Crudo(0, 0) != "uno" {
		t.Errorf("confirmar combina y vacía B1: A1=%q B1=%q", lz.Libro.Crudo(0, 0), lz.Libro.Crudo(1, 0))
	}
	lz.Deshacer(false)
	if lz.Serializar() != antes {
		t.Errorf("un solo deshacer vuelve a la hoja de antes: %s", lz.Serializar())
	}
}

// TestLasOpcionesDeCombinar: el «▾» trae las cuatro opciones de la web en su orden, y
// «Separar» basta con el ancla seleccionada.
func TestLasOpcionesDeCombinar(t *testing.T) {
	lz := lienzoEnVentana(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"merges":["A1:B2"]}]}`)
	got := make([]string, 0, len(opcionesCombinar))
	for _, o := range opcionesCombinar {
		got = append(got, o.valor)
	}
	if strings.Join(got, ",") != "center,merge,content,unmerge" {
		t.Errorf("las opciones del «▾» son %v", got)
	}
	lz.Mandar(CmdCombinar, "unmerge")
	if _, ok := lz.Libro.FusionEn(1, 1); ok {
		t.Error("«Separar celdas» con el ancla seleccionada no separa")
	}
}

// TestElAnclaDeFueraNoPintaSuCabecera: el ancla de una combinada que se ha desplazado
// por debajo de lo inmovilizado se sigue pintando, pero su cabecera de fila no: caería
// encima de la de la fila inmovilizada, que es la que se ve ahí.
//
// Visto en rojo antes del arreglo: la cabecera «2» salía en y = 24, encima de la «1».
func TestElAnclaDeFueraNoPintaSuCabecera(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A2":"Bloque"},"merges":["A2:A8"]}]}`) {
		t.Fatal("no se abre el libro")
	}
	r := c.Rejilla
	c.Libro.Fijar(0, 1)
	pinta(t, 400, 300, c)
	r.Desplazar(0, AltoCelda) // la fila 2, el ancla, pasa entera por debajo de la 1
	pinta(t, 400, 300, c)
	_, finY := r.vista().finFijas()
	for _, cb := range r.r.cabFilas {
		if cb.rotulo.Visible() && cb.rotulo.Text == "2" && cb.fondo.Position().Y < finY {
			t.Errorf("la cabecera de la fila 2 asoma sobre la inmovilizada (y = %v, fin = %v)", cb.fondo.Position().Y, finY)
		}
	}
}
