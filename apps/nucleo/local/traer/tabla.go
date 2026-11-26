// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// # Las tablas de «Tus datos»
//
// Cada producto entrega sus tablas (la lista de fotos, de álbumes…) en el formato que
// eligió la persona: JSON, CSV o XML. Las escribe `platform/pkg/portabilidad`
// (`EscribirTabla`), y las tres dicen lo mismo porque cada fila pasa antes por
// encoding/json:
//
//   - JSON: una lista de objetos;
//   - CSV: con BOM, una cabecera con los nombres JSON de los campos y una fila por
//     elemento; un texto que empieza como una fórmula (`=`, `+`, `-`, `@`, tabulador o
//     retorno) lleva delante un apóstrofo, una lista de textos va separada por «; », lo
//     que no es texto va como su JSON y null va vacío;
//   - XML: `<tabla><fila><campo>valor</campo>…</fila>…</tabla>`, una lista como un
//     `<item>` por elemento, y null como `<campo/>`.
//
// [leerTabla] las lee de vuelta, cada campo como texto (o como lista de textos): quien
// la usa sabe de qué tipo es cada uno.

// fila es una fila de una tabla: el valor de cada campo.
type fila map[string]valor

// valor es un campo: su texto o, si es una lista, sus elementos.
type valor struct {
	texto   string
	lista   []string
	esLista bool
}

// ErrTabla: la tabla no se puede leer.
var ErrTabla = errors.New("traer: la tabla no se puede leer")

// leerTabla lee una tabla en su formato («json», «csv» o «xml»: la extensión del fichero).
func leerTabla(datos []byte, formato string) ([]fila, error) {
	switch strings.ToLower(strings.TrimPrefix(formato, ".")) {
	case "json":
		return tablaJSON(datos)
	case "csv":
		return tablaCSV(datos)
	case "xml":
		return tablaXML(datos)
	}
	return nil, ErrTabla
}

func tablaJSON(datos []byte) ([]fila, error) {
	var objetos []map[string]json.RawMessage
	if err := json.Unmarshal(datos, &objetos); err != nil {
		return nil, ErrTabla
	}
	out := make([]fila, 0, len(objetos))
	for _, o := range objetos {
		f := fila{}
		for k, raw := range o {
			f[k] = valorJSON(raw)
		}
		out = append(out, f)
	}
	return out, nil
}

// valorJSON es el texto de un valor JSON: un texto tal cual, una lista de valores como
// lista, null vacío y lo demás (números, verdadero o falso, objetos) como su JSON.
func valorJSON(raw json.RawMessage) valor {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return valor{}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return valor{texto: s}
	}
	var lista []json.RawMessage
	if json.Unmarshal(raw, &lista) == nil {
		v := valor{esLista: true}
		for _, e := range lista {
			v.lista = append(v.lista, valorJSON(e).texto)
		}
		return v
	}
	return valor{texto: string(raw)}
}

func tablaCSV(datos []byte) ([]fila, error) {
	datos = bytes.TrimPrefix(datos, []byte("\xef\xbb\xbf"))
	r := csv.NewReader(bytes.NewReader(datos))
	cabecera, err := r.Read()
	if err != nil {
		return nil, ErrTabla
	}
	var out []fila
	for {
		celdas, err := r.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, ErrTabla
		}
		f := fila{}
		for i, c := range cabecera {
			if i < len(celdas) {
				f[c] = valor{texto: sinApostrofo(celdas[i])}
			}
		}
		out = append(out, f)
	}
}

// sinApostrofo deshace la neutralización de un texto que empezaba como una fórmula.
func sinApostrofo(s string) string {
	if len(s) > 1 && s[0] == '\'' && strings.ContainsRune("=+-@\t\r", rune(s[1])) {
		return s[1:]
	}
	return s
}

func tablaXML(datos []byte) ([]fila, error) {
	d := xml.NewDecoder(bytes.NewReader(datos))
	// profundidad: 1 la tabla, 2 la fila, 3 el campo, 4 un elemento de una lista.
	var (
		out         []fila
		actual      fila
		campo       string
		v           valor
		texto       strings.Builder
		profundidad int
	)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ErrTabla
		}
		switch t := tok.(type) {
		case xml.StartElement:
			profundidad++
			switch profundidad {
			case 2:
				actual = fila{}
			case 3:
				campo, v = t.Name.Local, valor{}
				texto.Reset()
			case 4:
				v.esLista = true
				texto.Reset()
			}
		case xml.CharData:
			if profundidad == 3 || profundidad == 4 {
				texto.Write(t)
			}
		case xml.EndElement:
			switch profundidad {
			case 4:
				v.lista = append(v.lista, texto.String())
				texto.Reset()
			case 3:
				if !v.esLista {
					v.texto = texto.String()
				}
				actual[campo] = v
			case 2:
				out = append(out, actual)
			}
			profundidad--
		}
	}
	if profundidad != 0 {
		return nil, ErrTabla
	}
	return out, nil
}

// texto es el campo como texto.
func (f fila) texto(k string) string { return strings.TrimSpace(f[k].texto) }

// lista es el campo como lista de textos (en CSV, separados por «; »).
func (f fila) lista(k string) []string {
	v := f[k]
	if v.esLista {
		return v.lista
	}
	s := strings.TrimSpace(v.texto)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") { // una lista que no era de textos, como su JSON
		if w := valorJSON(json.RawMessage(s)); w.esLista {
			return w.lista
		}
	}
	return strings.Split(s, "; ")
}

// si es el campo como verdadero o falso.
func (f fila) si(k string) bool { return f.texto(k) == "true" }

// entero es el campo como número entero; 0 si no lo es.
func (f fila) entero(k string) int64 {
	n, _ := strconv.ParseInt(f.texto(k), 10, 64)
	return n
}

// hora es el campo como fecha (RFC 3339, como la escribe encoding/json); nil si no lo es.
func (f fila) hora(k string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, f.texto(k))
	if err != nil || t.IsZero() {
		return nil
	}
	return &t
}
