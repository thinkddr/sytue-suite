// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Límites de una importación CSV. El cuerpo lo acota además el servidor HTTP.
const (
	MaxBytesCSV    = 5 << 20
	MaxFilasCSV    = 10000
	MaxColumnasCSV = 50
)

// TablaCSV es el resultado de leer un CSV: campos inferidos y filas ya tipadas.
type TablaCSV struct {
	Campos []EntradaCampo
	Filas  [][]json.RawMessage // una por fila, una celda por campo (nil = vacío)
}

// LeerCSV lee un CSV hostil sin fiarse de nada: UTF-8 válido (con BOM opcional),
// sin NUL, separador coma o punto y coma, cabecera obligatoria, límites de filas,
// columnas y celdas, y tipos inferidos por columna. No ejecuta nada: un valor que
// empieza por `=` es solo texto (la protección es al EXPORTAR, ver CeldaCSV).
func LeerCSV(r io.Reader) (TablaCSV, error) {
	datos, err := io.ReadAll(io.LimitReader(r, MaxBytesCSV+1))
	if err != nil {
		return TablaCSV{}, err
	}
	if len(datos) > MaxBytesCSV {
		return TablaCSV{}, fmt.Errorf("%w: el CSV supera %d MiB", ErrInvalido, MaxBytesCSV>>20)
	}
	datos = bytes.TrimPrefix(datos, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(datos) || bytes.IndexByte(datos, 0) >= 0 {
		return TablaCSV{}, fmt.Errorf("%w: el CSV no es UTF-8 válido", ErrInvalido)
	}
	rd := csv.NewReader(bytes.NewReader(datos))
	rd.Comma = separadorProbable(datos)
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = false
	rd.ReuseRecord = false

	cab, err := rd.Read()
	if err != nil || len(cab) == 0 {
		return TablaCSV{}, fmt.Errorf("%w: falta la fila de cabecera", ErrInvalido)
	}
	if len(cab) > MaxColumnasCSV {
		return TablaCSV{}, fmt.Errorf("%w: más de %d columnas", ErrInvalido, MaxColumnasCSV)
	}
	nombres := nombresUnicos(cab)

	var celdas [][]string
	for {
		fila, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return TablaCSV{}, fmt.Errorf("%w: CSV mal formado", ErrInvalido)
		}
		if len(celdas) >= MaxFilasCSV {
			return TablaCSV{}, fmt.Errorf("%w: más de %d filas", ErrInvalido, MaxFilasCSV)
		}
		if len(fila) > len(nombres) {
			return TablaCSV{}, fmt.Errorf("%w: fila %d con más celdas que la cabecera", ErrInvalido, len(celdas)+2)
		}
		for len(fila) < len(nombres) {
			fila = append(fila, "")
		}
		for _, c := range fila {
			if utf8.RuneCountInString(c) > MaxLongTexto {
				return TablaCSV{}, fmt.Errorf("%w: celda demasiado larga en la fila %d", ErrInvalido, len(celdas)+2)
			}
		}
		celdas = append(celdas, fila)
	}

	out := TablaCSV{Campos: make([]EntradaCampo, len(nombres))}
	tipos := make([]Tipo, len(nombres))
	for j, n := range nombres {
		col := make([]string, len(celdas))
		for i := range celdas {
			col[i] = strings.TrimSpace(celdas[i][j])
		}
		tipos[j] = InferirTipo(col)
		out.Campos[j] = EntradaCampo{Name: n, Kind: tipos[j]}
	}
	for _, fila := range celdas {
		cel := make([]json.RawMessage, len(nombres))
		for j := range nombres {
			cel[j] = celdaTipada(tipos[j], strings.TrimSpace(fila[j]))
		}
		out.Filas = append(out.Filas, cel)
	}
	return out, nil
}

func separadorProbable(datos []byte) rune {
	linea := datos
	if i := bytes.IndexByte(datos, '\n'); i >= 0 {
		linea = datos[:i]
	}
	if bytes.Count(linea, []byte(";")) > bytes.Count(linea, []byte(",")) {
		return ';'
	}
	return ','
}

// nombresUnicos da nombre a las columnas vacías o repetidas (sin perder ninguna) y
// recorta a la longitud admitida.
func nombresUnicos(cab []string) []string {
	vistos := map[string]int{}
	out := make([]string, len(cab))
	for i, c := range cab {
		n := strings.Join(strings.Fields(c), " ")
		n = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, n)
		if r := []rune(n); len(r) > MaxLongNombre-8 {
			n = string(r[:MaxLongNombre-8])
		}
		if n == "" {
			n = "Columna " + strconv.Itoa(i+1)
		}
		clave := strings.ToLower(n)
		vistos[clave]++
		if vistos[clave] > 1 {
			n = n + " (" + strconv.Itoa(vistos[clave]) + ")"
		}
		out[i] = n
	}
	return out
}

// InferirTipo elige el tipo más estricto que cumplen TODAS las celdas no vacías de
// la columna: número, fecha ISO, casilla o, si no, texto.
func InferirTipo(col []string) Tipo {
	n := 0
	num, fecha, casilla := true, true, true
	for _, c := range col {
		if c == "" {
			continue
		}
		n++
		if _, ok := leerNumero(c); !ok {
			num = false
		}
		if _, err := time.Parse(time.DateOnly, c); err != nil {
			fecha = false
		}
		if _, ok := leerCasilla(c); !ok {
			casilla = false
		}
	}
	switch {
	case n == 0:
		return TipoTexto
	case num:
		return TipoNumero
	case fecha:
		return TipoFecha
	case casilla:
		return TipoCasilla
	}
	return TipoTexto
}

func leerNumero(s string) (float64, bool) {
	if strings.HasPrefix(s, "+") || strings.ContainsAny(s, "xX_") {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != f || f > 1e15 || f < -1e15 {
		return 0, false
	}
	return f, true
}

func leerCasilla(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "sí", "si", "yes", "1", "x":
		return true, true
	case "false", "no", "0":
		return false, true
	}
	return false, false
}

func celdaTipada(t Tipo, s string) json.RawMessage {
	if s == "" {
		return nil
	}
	switch t { //nolint:exhaustive // el resto de tipos se importa como texto
	case TipoNumero:
		f, _ := leerNumero(s)
		b, _ := json.Marshal(f)
		return b
	case TipoFecha:
		b, _ := json.Marshal(s)
		return b
	case TipoCasilla:
		v, _ := leerCasilla(s)
		b, _ := json.Marshal(v)
		return b
	}
	b, _ := json.Marshal(s)
	return b
}

// CeldaCSV prepara una celda para exportarla: neutraliza la inyección de fórmulas
// (OWASP «CSV injection»): una celda que empieza por = + - @ tab o CR la
// interpretaría una hoja de cálculo como fórmula, así que se antepone una comilla.
func CeldaCSV(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
