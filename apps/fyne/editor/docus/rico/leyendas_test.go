// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

const (
	leyendaTabla  = `<p data-caption="tabla" id="cap-1" style="` + estiloLeyenda + `"><span data-caption-label="" style="font-weight:600">Tabla 1</span>. Ventas</p>`
	leyendaFigura = `<p data-caption="figura" id="cap-2" style="` + estiloLeyenda + `"><span data-caption-label="" style="font-weight:600">Figura 1</span></p>`
)

// TestLasLeyendasDeLaWeb: la de tabla va debajo de la tabla del cursor y la de figura
// debajo de su bloque, numeradas por clase; se pintan como su estilo (más pequeñas, en
// gris, centradas y con sus márgenes en em); la referencia cruzada enlaza con la etiqueta, y
// «Actualizar» renumera y pone al día las referencias. Se vio en ROJO numerando todas las
// leyendas juntas: la figura salía «Figura 2».
func TestLasLeyendasDeLaWeb(t *testing.T) {
	d := Parse(`<table><tbody><tr><td>a</td></tr></tbody></table><p>b</p>`)
	if _, ok := d.InsertarLeyenda(d.Bloques(), 0, LeyendaTabla, "Tabla", "Ventas"); !ok {
		t.Fatal("no se ha insertado la leyenda de tabla")
	}
	d.InsertarLeyenda(d.Bloques(), 2, LeyendaFigura, "Figura", "  ")
	tabla := `<table><tbody><tr><td>a</td></tr></tbody></table>`
	despues(t, "insertar", d, tabla+leyendaTabla+`<p>b</p>`+leyendaFigura)
	b := d.Bloques()[1]
	if b.Base.Tamano != 14.4 || b.Base.Alineacion != Centro || b.Base.Color != tintaCita || b.Arriba != 3.6 || abs32(b.Abajo-10.8) > 0.001 {
		t.Errorf("la leyenda se pinta %+v, arriba %g, abajo %g", b.Base, b.Arriba, b.Abajo)
	}
	if ls := d.Leyendas(); len(ls) != 2 || ls[0] != (Leyenda{"cap-1", "Tabla 1", "Ventas"}) || ls[1] != (Leyenda{"cap-2", "Figura 1", ""}) {
		t.Errorf("las leyendas: %+v", ls)
	}
	d = Parse(`<p>ver .</p>` + leyendaFigura + leyendaTabla)
	abrir(t, d, 0).InsertarReferencia(4, 4, d.Leyendas()[1])
	despues(t, "la referencia", d, `<p>ver <a data-xref="cap-1" href="#cap-1">Tabla 1</a>.</p>`+leyendaFigura+leyendaTabla)
	abrir(t, d, 2).Sustituir(0, 7, "Cuadro 9")
	if !d.ActualizarLeyendas("Tabla", "Figura") {
		t.Fatal("actualizar dice si había algo")
	}
	despues(t, "actualizar", d, `<p>ver <a data-xref="cap-1" href="#cap-1">Tabla 1</a>.</p>`+leyendaFigura+leyendaTabla)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
