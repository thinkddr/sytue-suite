// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package uacheck

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfdoc"
)

func txt(s string) []pdfdoc.Fragmento { return []pdfdoc.Fragmento{{Texto: s}} }

func celda(s string) pdfdoc.Celda {
	return pdfdoc.Celda{Bloques: []pdfdoc.Bloque{{Parrafo: &pdfdoc.Parrafo{Fragmentos: txt(s)}}}}
}

func cabecera(s string, cs, rs int) pdfdoc.Celda {
	c := celda(s)
	c.Encabezado, c.ColSpan, c.RowSpan = true, cs, rs
	return c
}

func png8(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 16; x++ {
		im.SetNRGBA(x, 3, color.NRGBA{R: uint8(16 * x), G: 90, B: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// bateria son los documentos originales que deben ser PDF/UA-1 sin errores: títulos, listas
// anidadas, tablas con cabeceras combinadas, imágenes con y sin alt, idiomas mezclados y un
// texto largo de varias páginas.
func bateria(t *testing.T) map[string]*pdfdoc.Doc {
	t.Helper()
	nuevo := func(titulo, idioma string) *pdfdoc.Doc {
		return pdfdoc.Nuevo(pdfdoc.Opciones{Titulo: titulo, Idioma: idioma})
	}
	out := map[string]*pdfdoc.Doc{}

	d := nuevo("Títulos", "es")
	d.Parrafo(pdfdoc.Parrafo{Rol: "H1", Fragmentos: txt("Título"), Tam: 22})
	d.Parrafo(pdfdoc.Parrafo{Rol: "H3", Fragmentos: txt("Salta a H3 (se etiqueta H2)"), Tam: 14})
	d.Parrafo(pdfdoc.Parrafo{Rol: "H2", Fragmentos: txt("Vuelve a H2"), Tam: 16})
	d.Parrafo(pdfdoc.Parrafo{Rol: "H6", Fragmentos: txt("Salta a H6 (se etiqueta H3)"), Tam: 11})
	d.Parrafo(pdfdoc.Parrafo{Fragmentos: txt("Cuerpo.")})
	out["titulos"] = d

	d = nuevo("Titulos desde H3", "es")
	d.Parrafo(pdfdoc.Parrafo{Rol: "H3", Fragmentos: txt("El primer encabezado ya es H3"), Tam: 14}) // se etiqueta H1
	out["primer-encabezado-no-h1"] = d

	d = nuevo("Listas", "es")
	d.ItemLista(0, false, pdfdoc.Parrafo{Fragmentos: txt("Uno")})
	d.ItemLista(1, true, pdfdoc.Parrafo{Fragmentos: txt("Uno-uno")})
	d.ItemLista(2, false, pdfdoc.Parrafo{Fragmentos: txt("Uno-uno-uno")})
	d.ItemLista(0, false, pdfdoc.Parrafo{Fragmentos: txt("Dos")})
	d.ItemLista(0, true, pdfdoc.Parrafo{Fragmentos: txt("Otra lista, ordenada")})
	d.ItemLista(3, false, pdfdoc.Parrafo{Fragmentos: txt("Salto de tres niveles")})
	d.ItemLista(0, false, pdfdoc.Parrafo{})
	out["listas"] = d

	d = nuevo("Tablas", "es")
	d.Tabla(pdfdoc.Tabla{Filas: [][]pdfdoc.Celda{
		{cabecera("Región", 1, 2), cabecera("Ventas", 3, 1), {Cubierta: true}, {Cubierta: true}},
		{{Cubierta: true}, cabecera("T1", 1, 1), cabecera("T2", 1, 1), cabecera("T3", 1, 1)},
		{cabecera("Norte", 1, 1), celda("1"), celda("2"), celda("3")},
		{celda("Total"), {Bloques: celda("junto").Bloques, ColSpan: 2, RowSpan: 2}, {Cubierta: true}, celda("x")},
		{celda("Fin"), {Cubierta: true}, {Cubierta: true}, celda("y")},
	}})
	out["tablas"] = d

	d = nuevo("Imágenes", "es")
	_ = d.Imagen(pdfdoc.Imagen{Datos: png8(t), Ancho: 120, Alt: "Franja de azul a rojo"})
	_ = d.Imagen(pdfdoc.Imagen{Datos: png8(t), Ancho: 60})
	d.Tabla(pdfdoc.Tabla{Filas: [][]pdfdoc.Celda{{{Bloques: []pdfdoc.Bloque{{Imagen: &pdfdoc.Imagen{Datos: png8(t), Ancho: 40, Alt: "En una celda"}}}}}}})
	out["imagenes"] = d

	d = nuevo("Idiomas", "es")
	d.Parrafo(pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{
		{Texto: "Español, "},
		{Texto: "français", Idioma: "fr"},
		{Texto: ", "},
		{Texto: "Ελληνικά"},
		{Texto: " y "},
		{Texto: "web", Enlace: "https://sytue.eu", Idioma: "en"},
		{Texto: " "},
		{Texto: "correo", Enlace: "mailto:a@sytue.eu"},
	}})
	out["idiomas"] = d

	d = nuevo("Largo", "bg")
	for i := 0; i < 120; i++ {
		d.Parrafo(pdfdoc.Parrafo{Fragmentos: txt(strings.Repeat("Дълъг текст на няколко страници. ", 6)), Despues: 6})
	}
	out["largo"] = d
	return out
}

func TestLaBateriaEsConforme(t *testing.T) {
	for nombre, d := range bateria(t) {
		b, err := d.Bytes()
		if err != nil {
			t.Fatal(nombre, err)
		}
		if dir := os.Getenv("SYTUE_PDF_VOLCAR"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "bateria-"+nombre+".pdf"), b, 0o600)
		}
		if errs := Verificar(b); len(errs) > 0 {
			t.Errorf("%s no es conforme:\n  %s", nombre, strings.Join(errs, "\n  "))
		}
	}
}

// veraPDF es el control externo: la misma batería, validada por una implementación ajena de
// PDF/UA-1. Se salta si no está instalado (el verificador propio de arriba corre siempre).
func TestLaBateriaPasaVeraPDF(t *testing.T) {
	for nombre, d := range bateria(t) {
		b, _ := d.Bytes()
		fallos, err := VeraPDF(b)
		if errors.Is(err, ErrNoVeraPDF) {
			t.Skip("veraPDF no está instalado (SYTUE_VERAPDF o PATH)")
		}
		if err != nil {
			t.Fatal(nombre, err)
		}
		if len(fallos) > 0 {
			t.Errorf("%s: veraPDF PDF/UA-1:\n  %s", nombre, strings.Join(fallos, "\n  "))
		}
	}
}

// reescribe cambia un flujo de contenido (sin Flate) o un texto del fichero, para estropearlo.
func estropear(t *testing.T, pdf []byte, f func(s string) string) []byte {
	t.Helper()
	return []byte(f(string(pdf)))
}

func sinMarcas(t *testing.T, pdf []byte) []byte {
	t.Helper()
	return reObj.ReplaceAllFunc(pdf, func(o []byte) []byte {
		m := reObj.FindSubmatch(o)
		if !strings.Contains(string(m[2]), "/Filter /FlateDecode") {
			return o
		}
		cont, err := inflar(string(m[2]))
		if err != nil || !strings.Contains(cont, "BDC") {
			return o
		}
		cont = regexp.MustCompile(`(?m)^/\w+ (<<[^>]*>> )?BDC\n`).ReplaceAllString(cont, "")
		cont = strings.ReplaceAll(cont, "EMC\n", "")
		return []byte(string(m[1]) + " 0 obj\n<< /Length " + strconv.Itoa(len(cont)) + " >>\nstream\n" + cont + "\nendstream\nendobj\n")
	})
}

// Cada guardián se ve en rojo: se estropea a mano justo lo que vigila y se exige el motivo.
func TestElVerificadorSeQuejaDeCadaIncumplimiento(t *testing.T) {
	docs := bateria(t)
	base := func(n string) []byte {
		b, err := docs[n].Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cambiar := func(de, a string) func(string) string {
		return func(s string) string {
			if !strings.Contains(s, de) {
				t.Fatalf("la preparación no encuentra %q", de)
			}
			return strings.Replace(s, de, a, 1)
		}
	}
	for _, c := range []struct {
		nombre, doc, motivo string
		mut                 func(*testing.T, []byte) []byte
	}{
		{"sin marcado de contenido", "titulos", "sin marcar", sinMarcas},
		{"sin MarkInfo", "titulos", "MarkInfo", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/MarkInfo << /Marked true >>", ""))
		}},
		{"sin DisplayDocTitle", "titulos", "DisplayDocTitle", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/DisplayDocTitle true", "/DisplayDocTitle false"))
		}},
		{"sin pdfuaid", "titulos", "pdfuaid:part", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("<pdfuaid:part>1</pdfuaid:part>", "<pdfuaid:part>2</pdfuaid:part>"))
		}},
		{"encabezado que salta", "titulos", "salto de nivel", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/S /H2 ", "/S /H5 ")) }},
		{"figura sin alt", "imagenes", "Figure sin /Alt", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/Alt (", "/Alx (")) }},
		{"TH sin Scope", "tablas", "TH sin Scope", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/Scope /Column", "/Scopx /Column"))
		}},
		{"tabla irregular", "tablas", "no es regular", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/ColSpan 3", "/ColSpan 2")) }},
		{"fuente sin ToUnicode", "titulos", "ToUnicode", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/ToUnicode ", "/ToUnicodx ")) }},
		{"fuente sin incrustar", "titulos", "CIDToGIDMap", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/FontFile2 ", "/FontFilx "))
		}},
		{"idioma inválido", "idiomas", "Lang inválido", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/Lang (fr)", "/Lang (f r)")) }},
		{"enlace sin Contents", "idiomas", "/Contents", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/F 4 /Contents ", "/F 4 /Contentx "))
		}},
		{"enlace fuera de la estructura", "idiomas", "StructParent", func(t *testing.T, b []byte) []byte {
			return estropear(t, b, cambiar("/StructParent ", "/StructParenx "))
		}},
		{"tipo no estándar", "listas", "no estándar", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/S /LBody", "/S /Cuerpo")) }},
		{"anidación inválida", "listas", "no admite hijos", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/S /LI ", "/S /P ")) }},
		{"sin Tabs", "titulos", "/Tabs", func(t *testing.T, b []byte) []byte { return estropear(t, b, cambiar("/Tabs /S", "/Tabx /S")) }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			if errs := Verificar(base(c.doc)); len(errs) > 0 {
				t.Fatalf("el documento sano ya fallaba: %v", errs)
			}
			errs := Verificar(c.mut(t, base(c.doc)))
			if !strings.Contains(strings.Join(errs, "\n"), c.motivo) {
				t.Errorf("esperaba un error con %q y salió %v", c.motivo, errs)
			}
		})
	}
}

// veraPDF también se ve en rojo: el mismo documento sin marcado de contenido deja de ser PDF/UA.
func TestVeraPDFSeQuejaSinMarcado(t *testing.T) {
	b, _ := bateria(t)["titulos"].Bytes()
	fallos, err := VeraPDF(xrefNueva(t, sinMarcas(t, b)))
	if errors.Is(err, ErrNoVeraPDF) {
		t.Skip("veraPDF no está instalado (SYTUE_VERAPDF o PATH)")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(fallos, "\n"), "7.1 test 3") {
		t.Errorf("veraPDF debería fallar la regla 7.1-3 (contenido sin marcar ni artefacto): %v", fallos)
	}
}

// xrefNueva reescribe la tabla xref de un PDF cuyos objetos han cambiado de tamaño, para que
// otro lector lo abra y el fallo que vea sea el que se provocó y no un xref roto.
func xrefNueva(t *testing.T, pdf []byte) []byte {
	t.Helper()
	i := bytes.LastIndex(pdf, []byte("\nxref\n"))
	if i < 0 {
		t.Fatal("sin xref")
	}
	cuerpo := pdf[:i+1]
	_, trailer, _ := bytes.Cut(pdf[i:], []byte("trailer"))
	trailer, _, _ = bytes.Cut(append([]byte("trailer"), trailer...), []byte("startxref"))
	offs := map[int]int{}
	maxN := 0
	for _, m := range regexp.MustCompile(`(?m)^(\d+) 0 obj\n`).FindAllSubmatchIndex(cuerpo, -1) {
		n, _ := strconv.Atoi(string(cuerpo[m[2]:m[3]]))
		offs[n], maxN = m[0], max(maxN, n)
	}
	var b bytes.Buffer
	b.Write(cuerpo)
	xref := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(maxN+1) + "\n0000000000 65535 f \n")
	for n := 1; n <= maxN; n++ {
		b.WriteString(strings.Repeat("0", 10-len(strconv.Itoa(offs[n]))) + strconv.Itoa(offs[n]) + " 00000 n \n")
	}
	b.Write(trailer)
	b.WriteString("startxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")
	return b.Bytes()
}
