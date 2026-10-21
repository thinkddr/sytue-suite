// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElTituloDeUnaTareaHechaSeTachaEnCadaLinea: en solo lectura (sin conexión) el título
// de una tarea hecha parte en líneas como el de las demás, y va tachado.
func TestElTituloDeUnaTareaHechaSeTachaEnCadaLinea(t *testing.T) {
	n := Nota{
		ID: "t1", Tipo: "todo", Marcada: true,
		Titulo: "Llamar al del seguro para cambiar la póliza de la casa antes del viernes",
	}
	tj := NuevaTarjeta(trInterno(t), n)
	tj.SinAcciones = true
	p, ok := tj.titulo(tema.Claro).(*ui.Parrafo)
	if !ok || !p.Tachado {
		t.Fatalf("el título de una tarea hecha es %T y tiene que ser un párrafo tachado", tj.titulo(tema.Claro))
	}
	if len(ui.LineasDe(p, 200)) < 2 {
		t.Error("el título no parte en líneas")
	}
}

// TestUnaImagenQueNoLlegaEnseñaSuIcono: la nota de imagen cuya dirección no carga
// enseña el icono de imagen rota delante de su `alt`, que parte en lo que queda.
func TestUnaImagenQueNoLlegaEnseñaSuIcono(t *testing.T) {
	n := Nota{ID: "i1", Tipo: "image", Titulo: "Plano de la cocina con las medidas de los muebles", URL: "https://x.test/a.png"}
	tj := NuevaTarjeta(trInterno(t), n)
	tj.Imagen = func(string) imagenRemota { return imagenRemota{lista: true, err: errors.New("404")} }
	c, ok := tj.cuerpoImagen(tema.Claro).(*fyne.Container)
	if !ok || len(c.Objects) != 2 {
		t.Fatalf("el cuerpo es %T", tj.cuerpoImagen(tema.Claro))
	}
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(160, 200))
	c.Resize(fyne.NewSize(160, altoDe(c, 160)))
	icono, alt := c.Objects[0], c.Objects[1].(*ui.Parrafo)
	if icono.Size().Width != LadoImagenRota || alt.Position().X != LadoImagenRota {
		t.Errorf("el icono mide %v y el texto empieza en %v", icono.Size(), alt.Position())
	}
	if altoDe(c, 160) != ui.AltoParrafo(alt, 160-LadoImagenRota) || len(ui.LineasDe(alt, 160-LadoImagenRota)) < 2 {
		t.Errorf("el alto %v no es el del texto partido en lo que queda", altoDe(c, 160))
	}
}
