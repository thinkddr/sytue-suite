// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func textoCelda(s string) []Bloque {
	return []Bloque{{Parrafo: &Parrafo{Fragmentos: []Fragmento{{Texto: s}}}}}
}

func pngDePrueba(t *testing.T, alfa bool) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			a := uint8(255)
			if alfa && x > 60 {
				a = 90
			}
			im.SetNRGBA(x, y, color.NRGBA{uint8(2 * x), uint8(4 * y), 140, a})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegDePrueba(t *testing.T, gris bool) []byte {
	t.Helper()
	var im image.Image
	if gris {
		g := image.NewGray(image.Rect(0, 0, 80, 40))
		for i := range g.Pix {
			g.Pix[i] = uint8(i)
		}
		im = g
	} else {
		c := image.NewRGBA(image.Rect(0, 0, 80, 40))
		for i := range c.Pix {
			c.Pix[i] = uint8(i * 7)
		}
		im = c
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// conTablas es el documento de la pieza 3: tabla con cabecera combinada y celda con
// combinación vertical, tabla con cabecera de fila, tabla larga que pasa de página e
// imágenes con y sin texto alternativo.
func conTablas(t *testing.T) *Doc {
	t.Helper()
	d := Nuevo(Opciones{Titulo: "Tablas e imágenes", Idioma: "es"})
	d.Parrafo(Parrafo{Rol: "H1", Fragmentos: []Fragmento{{Texto: "Tablas e imágenes"}}, Tam: 22, Despues: 8})
	h := func(s string, cs, rs int) Celda {
		return Celda{Bloques: textoCelda(s), Encabezado: true, ColSpan: cs, RowSpan: rs}
	}
	c := func(s string) Celda { return Celda{Bloques: textoCelda(s)} }
	cub := Celda{Cubierta: true}
	d.Tabla(Tabla{Filas: [][]Celda{
		{h("Región", 1, 2), h("Ventas", 2, 1), cub, h("Notas", 1, 2)},
		{cub, h("2025", 1, 1), h("2026", 1, 1), cub},
		{c("Norte"), c("1.200 €"), c("1.450 €"), c("Crece un 20 % gracias al canal nuevo, que además reduce el coste de captación")},
		{c("Sur"), c("900 €"), c("880 €"), c("Estable")},
		{c("Este y oeste"), {Bloques: textoCelda("2.300 € en conjunto"), ColSpan: 2, RowSpan: 1}, cub, c("Se informan juntas")},
	}})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Una tabla de datos con la primera columna como cabecera de fila:"}}, Despues: 4})
	d.Tabla(Tabla{Filas: [][]Celda{
		{h("Mes", 1, 1), c("Enero"), c("Febrero")},
		{h("Visitas", 1, 1), c("1000"), c("1300")},
	}})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Con alt:"}}})
	if err := d.Imagen(Imagen{Datos: pngDePrueba(t, true), Alt: "Degradado de azul a violeta, con la mitad derecha translúcida", Ancho: 180, Alinear: Centro}); err != nil {
		t.Fatal(err)
	}
	if err := d.Imagen(Imagen{Datos: jpegDePrueba(t, false), Alt: "Ruido de colores"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Imagen(Imagen{Datos: jpegDePrueba(t, true), Alt: "Ruido en grises"}); err != nil {
		t.Fatal(err)
	}
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Sin alt (decorativa):"}}})
	if err := d.Imagen(Imagen{Datos: pngDePrueba(t, false), Ancho: 90}); err != nil {
		t.Fatal(err)
	}
	filas := [][]Celda{{h("N.º", 1, 1), h("Descripción", 1, 1)}}
	for i := 1; i <= 60; i++ {
		desc := textoCelda("Fila larga " + lorem + lorem)
		if i%20 == 0 {
			desc = append(desc, Bloque{Imagen: &Imagen{Datos: pngDePrueba(t, false), Ancho: 40, Alt: "Miniatura de la fila"}})
		}
		filas = append(filas, []Celda{c(strings.Repeat("9", 1+i%3)), {Bloques: desc}})
	}
	d.Tabla(Tabla{Filas: filas})
	// Una celda más alta que una página: se parte por líneas.
	gigante := strings.Repeat(lorem, 160)
	d.Tabla(Tabla{Filas: [][]Celda{{h("Texto largo", 1, 1)}, {{Bloques: textoCelda(gigante)}}, {c("Fin de la tabla")}}})
	return d
}

func TestTablasEImagenesEtiquetadas(t *testing.T) {
	d := conTablas(t)
	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("SYTUE_PDF_VOLCAR"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "pdfdoc-tablas.pdf"), b, 0o600)
	}
	if p := problemasDeMarcado(t, b); len(p) > 0 {
		t.Fatalf("marcado incorrecto:\n%s", strings.Join(p, "\n"))
	}
	todo := string(b)
	for _, quiero := range []string{
		"/S /Table ", "/S /TR ", "/S /TH ", "/S /TD ", "/Scope /Column", "/Scope /Row",
		"/ColSpan 2", "/RowSpan 2", "/S /Figure ", "/Alt (", "/SMask ", "/DCTDecode", "/DeviceGray", "/Subtype /Image",
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("falta %q", quiero)
		}
	}
	if d.SinAlt() != 1 {
		t.Errorf("SinAlt = %d, esperaba 1 (la imagen decorativa)", d.SinAlt())
	}
	if strings.Count(todo, "/Subtype /Image") != 5 { // PNG con alfa (+ su SMask), JPEG, JPEG gris y PNG opaco, cada uno una sola vez
		t.Errorf("imágenes escritas = %d", strings.Count(todo, "/Subtype /Image"))
	}
}

func TestImagenInvalidaSeRechaza(t *testing.T) {
	d := Nuevo(Opciones{})
	for _, datos := range [][]byte{nil, []byte("no es una imagen"), pngDePrueba(t, false)[:60]} {
		if err := d.Imagen(Imagen{Datos: datos}); err == nil {
			t.Errorf("imagen inválida admitida (%d bytes)", len(datos))
		}
	}
}
