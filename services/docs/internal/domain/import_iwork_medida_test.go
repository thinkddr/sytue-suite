// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sonda de cuánto texto llega al abrir ficheros de iWork. No es una comprobación: escribe,
// por cada fichero del corpus, el texto que queda en el documento convertido, para
// compararlo con una extracción de referencia hecha aparte. Sin `SYTUE_IWORK_CORPUS` no
// hace nada.
//
//	SYTUE_IWORK_CORPUS=/corpus SYTUE_IWORK_SALIDA=/dir go test -run '^TestMedirIWork$' ./internal/domain
package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func TestMedirIWork(t *testing.T) {
	corpus, salida := os.Getenv("SYTUE_IWORK_CORPUS"), os.Getenv("SYTUE_IWORK_SALIDA")
	if corpus == "" || salida == "" {
		t.Skip("sin SYTUE_IWORK_CORPUS ni SYTUE_IWORK_SALIDA")
	}
	_ = filepath.WalkDir(corpus, func(ruta string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || extIWork(ruta) == fmtUnknown {
			return nil
		}
		data, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		res, err := ImportLocalized(filepath.Base(ruta), data, false, "en")
		texto, contenido := "", ""
		if err == nil {
			texto, contenido = textoDelContenido(res.Content), res.Content
		}
		rel, _ := filepath.Rel(corpus, ruta)
		destino := filepath.Join(salida, rel)
		_ = os.MkdirAll(filepath.Dir(destino), 0o755)
		_ = os.WriteFile(destino+".txt", []byte(texto), 0o644)
		_ = os.WriteFile(destino+".contenido", []byte(contenido), 0o644)
		t.Logf("%s\t%v\t%d", rel, err, len(texto))
		return nil
	})
}

// textoDelContenido es el texto que se ve de un contenido importado: el de un HTML, o
// todas las cadenas de un modelo JSON (hoja, presentación).
func textoDelContenido(c string) string {
	var b strings.Builder
	if strings.HasPrefix(c, "{") {
		var v any
		if json.Unmarshal([]byte(c), &v) == nil {
			var cadenas func(any)
			cadenas = func(v any) {
				switch x := v.(type) {
				case string:
					b.WriteString(x + "\n")
				case []any:
					for _, e := range x {
						cadenas(e)
					}
				case map[string]any:
					for _, e := range x {
						cadenas(e)
					}
				}
			}
			cadenas(v)
			return b.String()
		}
	}
	z := xhtml.NewTokenizer(strings.NewReader(c))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return b.String()
		}
		if tt == xhtml.TextToken {
			b.Write(z.Text())
			continue
		}
		// Un bloque o un salto separa palabras; un tramo en línea, no.
		switch nombre, _ := z.TagName(); string(nombre) {
		case "p", "li", "td", "th", "br", "div", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "sup":
			b.WriteByte('\n')
		}
	}
}
