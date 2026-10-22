// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// TestCorpusDocx mide el importador de .docx contra una carpeta de ficheros reales: cuántos
// fallan, cuántos pierden texto y qué propiedades se leen sin traducirse, por tipo y ordenadas
// por el número de ficheros que las llevan. No es una prueba que pueda fallar: es la medida con
// la que se decide qué arreglar primero. El corpus no vive en el repositorio (pesa y no es
// nuestro); se apunta con la variable, en el frontal:
//
//	SYTUE_CORPUS_DOCX=/ruta/al/corpus go test ./services/docs/internal/domain -run TestCorpusDocx -v
//
// Con SYTUE_CORPUS_HTML=/ruta además deja el HTML de cada fichero, para mirarlo o pintarlo.
func TestCorpusDocx(t *testing.T) {
	dir := os.Getenv("SYTUE_CORPUS_DOCX")
	if dir == "" {
		t.Skip("sin SYTUE_CORPUS_DOCX")
	}
	var rutas []string
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && (strings.HasSuffix(strings.ToLower(p), ".docx") || strings.HasSuffix(strings.ToLower(p), ".docm")) {
			rutas = append(rutas, p)
		}
		return nil
	})
	slices.Sort(rutas)
	type cuenta struct{ ficheros, veces int }
	porTipo := map[string]*cuenta{}
	var fallos, pierden []string
	for _, p := range rutas {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		docxDescartes = map[string]int{}
		res, err := importarSinPanico(p, data)
		vistos := docxDescartes
		docxDescartes = nil
		if err != nil {
			fallos = append(fallos, fmt.Sprintf("%s: %v", rel(dir, p), err))
			continue
		}
		for k, n := range vistos {
			c := porTipo[k]
			if c == nil {
				c = &cuenta{}
				porTipo[k] = c
			}
			c.ficheros++
			c.veces += n
		}
		if perdido, total := textoPerdido(data, res.Content); total > 0 && perdido*100 > total {
			pierden = append(pierden, fmt.Sprintf("%s: %d de %d caracteres", rel(dir, p), perdido, total))
		}
		if out := os.Getenv("SYTUE_CORPUS_HTML"); out != "" {
			nombre := strings.NewReplacer("/", "__", " ", "_").Replace(rel(dir, p)) + ".html"
			_ = os.WriteFile(filepath.Join(out, nombre), []byte(res.Content), 0o600)
		}
	}
	tipos := make([]string, 0, len(porTipo))
	for k := range porTipo {
		tipos = append(tipos, k)
	}
	slices.SortFunc(tipos, func(a, b string) int {
		if d := porTipo[b].ficheros - porTipo[a].ficheros; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "ficheros: %d · fallan: %d · pierden texto (>1 %%): %d\n", len(rutas), len(fallos), len(pierden))
	for _, f := range fallos {
		fmt.Fprintf(&b, "  FALLA %s\n", f)
	}
	for _, f := range pierden {
		fmt.Fprintf(&b, "  PIERDE %s\n", f)
	}
	fmt.Fprintf(&b, "\n| Se lee y no se traduce | Ficheros | Veces |\n| --- | --- | --- |\n")
	for _, k := range tipos {
		fmt.Fprintf(&b, "| %s | %d | %d |\n", k, porTipo[k].ficheros, porTipo[k].veces)
	}
	t.Log("\n" + b.String())
}

// TestCorpusDocxIdaYVuelta mide Docus -> .docx -> Docus contra el mismo corpus: importa cada
// fichero, lo exporta y vuelve a importar lo exportado. Cuenta cuántos vuelven idénticos, en
// cuántos cambia el texto y qué declaraciones de CSS se pierden, por ficheros: una declaración se
// pierde si sale menos veces a la vuelta que a la ida (que vaya en otro elemento no cuenta: el
// color de un párrafo vuelve en sus tramos y se ve igual).
//
//	SYTUE_CORPUS_DOCX=/ruta/al/corpus go test ./services/docs/internal/domain -run TestCorpusDocxIdaYVuelta -v
func TestCorpusDocxIdaYVuelta(t *testing.T) {
	dir := os.Getenv("SYTUE_CORPUS_DOCX")
	if dir == "" {
		t.Skip("sin SYTUE_CORPUS_DOCX")
	}
	var rutas []string
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(strings.ToLower(p), ".docx") {
			rutas = append(rutas, p)
		}
		return nil
	})
	slices.Sort(rutas)
	var total, iguales int
	var textoDistinto []string
	pierde := map[string]int{}
	for _, p := range rutas {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		ida, err := importarSinPanico(p, data)
		if err != nil {
			continue
		}
		salida, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", ida.Content), FormatOOXML)
		if err != nil {
			t.Errorf("%s: la exportación falla: %v", rel(dir, p), err)
			continue
		}
		vuelta, err := importarSinPanico("vuelta.docx", salida)
		if err != nil {
			t.Errorf("%s: lo exportado no se vuelve a importar: %v", rel(dir, p), err)
			continue
		}
		total++
		if ida.Content == vuelta.Content {
			iguales++
			continue
		}
		if palabrasDe(ida.Content) != palabrasDe(vuelta.Content) {
			textoDistinto = append(textoDistinto, rel(dir, p))
		}
		a, b := declaracionesDe(ida.Content), declaracionesDe(vuelta.Content)
		perdidas := map[string]bool{}
		for d, n := range a {
			if b[d] < n {
				prop, _, _ := strings.Cut(d, ":")
				perdidas[prop] = true
			}
		}
		for prop := range perdidas {
			pierde[prop]++
		}
	}
	props := make([]string, 0, len(pierde))
	for p := range pierde {
		props = append(props, p)
	}
	slices.SortFunc(props, func(x, y string) int {
		if d := pierde[y] - pierde[x]; d != 0 {
			return d
		}
		return strings.Compare(x, y)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "ficheros: %d · vuelven idénticos: %d · cambia el texto: %d\n", total, iguales, len(textoDistinto))
	for _, f := range textoDistinto {
		fmt.Fprintf(&b, "  TEXTO %s\n", f)
	}
	fmt.Fprintf(&b, "\n| Se pierde en la ida y vuelta | Ficheros |\n| --- | --- |\n")
	for _, p := range props {
		fmt.Fprintf(&b, "| %s | %d |\n", p, pierde[p])
	}
	t.Log("\n" + b.String())
}

// palabrasDe son las palabras de un HTML, sin etiquetas.
func palabrasDe(h string) string {
	return strings.Join(strings.Fields(html.UnescapeString(reEtiqueta.ReplaceAllString(h, " "))), " ")
}

func rel(dir, p string) string {
	r, err := filepath.Rel(dir, p)
	if err != nil {
		return p
	}
	return r
}

// importarSinPanico importa y convierte un pánico en error: un fichero que tumba el importador
// cuenta como un fallo más, no corta la medida.
func importarSinPanico(nombre string, data []byte) (res *ImportResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pánico: %v", r)
		}
	}()
	return Import(filepath.Base(nombre), data)
}

var reEtiqueta = regexp.MustCompile(`<[^>]*>`)

// textoPerdido compara las letras del cuerpo (los w:t y w:delText de document.xml, sin el
// respaldo de mc:AlternateContent) con las del HTML, sin espacios, como multiconjuntos: cuenta
// las que el HTML no tiene. El HTML puede tener de más (encabezado, pie, notas), así que solo se
// mira lo que falta.
func textoPerdido(data []byte, salida string) (perdido, total int) {
	files, err := readZip(data)
	if err != nil {
		return 0, 0
	}
	dec := xml.NewDecoder(bytes.NewReader(files["word/document.xml"]))
	quedan := map[rune]int{}
	enTexto := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "Fallback" {
				_ = dec.Skip() // la representación de respaldo repite la de mc:Choice (§M.2.5 de la Parte 3)
				continue
			}
			enTexto = e.Name.Local == "t" && e.Name.Space != "http://schemas.openxmlformats.org/drawingml/2006/main" || e.Name.Local == "delText"
		case xml.EndElement:
			enTexto = false
		case xml.CharData:
			if enTexto {
				for _, r := range string(e) {
					if !unicode.IsSpace(r) {
						quedan[r]++
						total++
					}
				}
			}
		}
	}
	for _, r := range html.UnescapeString(reEtiqueta.ReplaceAllString(salida, " ")) {
		if quedan[r] > 0 {
			quedan[r]--
		}
	}
	for _, n := range quedan {
		perdido += n
	}
	return perdido, total
}
