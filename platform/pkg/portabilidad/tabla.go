// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package portabilidad

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// EscribirTabla escribe las filas en el formato pedido.
//
// Cada fila pasa primero por encoding/json: así las tres salidas dicen lo mismo
// que la API del producto —mismos nombres de campo, mismas fechas RFC 3339, lo
// que un `json:"-"` esconde tampoco sale— y no hay tres serializadores que
// mantener de acuerdo.
func EscribirTabla[T any](w io.Writer, f Formato, nombre, fila string, filas iter.Seq2[T, error]) error {
	bw := bufio.NewWriter(w)
	var err error
	switch f {
	case CSV:
		var cols []string
		if cols, err = columnas(reflect.TypeFor[T]()); err == nil {
			err = tablaCSV(bw, cols, filas)
		}
	case XML:
		err = tablaXML(bw, nombreXML(nombre), nombreXML(fila), filas)
	case JSON:
		err = tablaJSON(bw, filas)
	default:
		err = ErrFormato
	}
	if err != nil {
		return err
	}
	return bw.Flush()
}

func tablaJSON[T any](w *bufio.Writer, filas iter.Seq2[T, error]) error {
	put(w, "[")
	primera := true
	for v, err := range filas {
		if err != nil {
			return err
		}
		b, err := marshal(v)
		if err != nil {
			return err
		}
		if !primera {
			put(w, ",")
		}
		primera = false
		put(w, "\n")
		put(w, string(b))
	}
	put(w, "\n]\n")
	return nil
}

// tablaCSV lleva BOM porque Excel, sin él, abre el UTF-8 como si fuera Latin-1 y
// cualquier tilde sale rota; el resto de herramientas lo ignoran. Un valor que no
// es escalar (una lista de correos, una dirección) va como JSON en su celda: es
// lo único que se puede volver a leer sin perder nada.
func tablaCSV[T any](w *bufio.Writer, cols []string, filas iter.Seq2[T, error]) error {
	put(w, "\ufeff")
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	celdas := make([]string, len(cols))
	for v, err := range filas {
		if err != nil {
			return err
		}
		b, err := marshal(v)
		if err != nil {
			return err
		}
		var campos map[string]json.RawMessage
		if err := json.Unmarshal(b, &campos); err != nil {
			return fmt.Errorf("portabilidad: una fila CSV debe ser un objeto: %w", err)
		}
		for i, c := range cols {
			celdas[i] = celda(campos[c])
		}
		if err := cw.Write(celdas); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func celda(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Los textos salen neutralizados: «Tus datos» se abre en una hoja de cálculo,
	// y el nombre de un evento que otra persona escribió como =HYPERLINK(...) se
	// ejecutaba al abrirlo (NEG-18). Los números y los objetos JSON no pueden
	// empezar por una fórmula y salen tal cual.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return validation.CSVSafe(s)
	}
	// Una lista de textos se lee mejor separada que como JSON: «a@x; b@y».
	var lista []string
	if json.Unmarshal(raw, &lista) == nil {
		return validation.CSVSafe(strings.Join(lista, "; "))
	}
	return string(raw)
}

func tablaXML[T any](w *bufio.Writer, raiz, elemento string, filas iter.Seq2[T, error]) error {
	put(w, xml.Header)
	put(w, "<"+raiz+">\n")
	for v, err := range filas {
		if err != nil {
			return err
		}
		b, err := marshal(v)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		put(w, "  ")
		if err := valorXML(w, dec, elemento); err != nil {
			return err
		}
		put(w, "\n")
	}
	put(w, "</"+raiz+">\n")
	return nil
}

// valorXML escribe el siguiente valor JSON del decodificador como <nombre>…</nombre>.
// Un objeto da un hijo por campo y una lista, un <item> por elemento.
func valorXML(w *bufio.Writer, dec *json.Decoder, nombre string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		put(w, "<"+nombre+">")
		if t == '{' {
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				clave, _ := k.(string)
				if err := valorXML(w, dec, nombreXML(clave)); err != nil {
					return err
				}
			}
		} else {
			for dec.More() {
				if err := valorXML(w, dec, "item"); err != nil {
					return err
				}
			}
		}
		if _, err := dec.Token(); err != nil { // cierre } o ]
			return err
		}
		put(w, "</"+nombre+">")
	case nil:
		put(w, "<"+nombre+"/>")
	default:
		put(w, "<"+nombre+">")
		if err := xml.EscapeText(w, []byte(fmt.Sprint(t))); err != nil {
			return err
		}
		put(w, "</"+nombre+">")
	}
	return nil
}

// nombreXML convierte una clave en un nombre de elemento válido: lo que no es
// letra, dígito, «_», «-» o «.» pasa a «_», y no puede empezar por dígito.
func nombreXML(s string) string {
	var b strings.Builder
	for i, r := range s {
		valido := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && (r == '-' || r == '.' || (r >= '0' && r <= '9')))
		if !valido {
			r = '_'
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

// put escribe sin mirar el error: los de bufio.Writer se quedan pegados y el
// primero vuelve en Flush, que sí se comprueba.
func put(w *bufio.Writer, s ...string) {
	for _, x := range s {
		_, _ = w.WriteString(x)
	}
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // «<» y «&» se leen tal cual en un fichero para personas.
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("portabilidad: serializar fila: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// columnas son las cabeceras del CSV: los nombres JSON de los campos del tipo,
// incluidos los omitempty (una columna no puede aparecer a mitad de fichero).
func columnas(t reflect.Type) ([]string, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, errors.New("portabilidad: las filas de una tabla deben ser structs")
	}
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		nombre, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if nombre == "-" {
			continue
		}
		// Un struct embebido sin nombre aporta sus campos, como hace encoding/json.
		if f.Anonymous && nombre == "" {
			if sub, err := columnas(f.Type); err == nil {
				out = append(out, sub...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if nombre == "" {
			nombre = f.Name
		}
		out = append(out, nombre)
	}
	return out, nil
}
