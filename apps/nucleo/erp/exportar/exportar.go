// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package exportar convierte los registros del ERP en las filas del CSV que descarga la
// lista: las columnas de la vista con sus rótulos, el importe en unidades, la relación
// por su nombre y la selección o el estado con su rótulo. Lo usan services/syerp y el ERP
// de Sytue Office (apps/nucleo/local/erp), así que los dos sacan el mismo fichero.
package exportar

import (
	"encoding/json"
	"fmt"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Columna es una columna de la exportación con todo lo suyo ya resuelto: se calcula
// una vez por descarga y no una vez por fila.
type Columna struct {
	Clave  string
	Tipo   string
	Rotulo string
}

// Columnas traduce las columnas de la vista de lista al idioma. «ref» y «estado» no son
// campos del modelo: su rótulo viene del diccionario común.
func Columnas(cat *catalogo.Catalogo, v catalogo.ModeloVista, idioma string) []Columna {
	campos := make(map[string]catalogo.CampoVista, len(v.Campos))
	for _, c := range v.Campos {
		campos[c.Clave] = c
	}
	out := make([]Columna, 0, len(v.Lista))
	for _, clave := range v.Lista {
		col := Columna{Clave: clave, Rotulo: cat.Rotulo("campo."+clave, idioma)}
		if f, ok := campos[clave]; ok {
			col.Tipo, col.Rotulo = f.Tipo, f.Etiqueta
		}
		out = append(out, col)
	}
	return out
}

// Rotulos es la cabecera.
func Rotulos(cols []Columna) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.Rotulo)
	}
	return out
}

// RotulosDeValor recoge los rótulos ya traducidos de las selecciones y de los estados,
// para que el CSV diga «Ganada» y no «ganada».
func RotulosDeValor(v catalogo.ModeloVista) map[string]string {
	out := map[string]string{}
	for _, c := range v.Campos {
		for _, op := range c.Opciones {
			out[op.Valor] = op.Etiqueta
		}
	}
	for _, e := range v.Estados {
		out[e.Valor] = e.Etiqueta
	}
	return out
}

// Fila convierte un registro en su línea de CSV. Toda celda pasa por
// `validation.CSVSafe` salvo las que son un número y nada más (CWE-1236): un importe
// negativo escapado sería texto y la columna no se podría sumar, y una cadena que
// ParseFloat acepta entera no puede ser una fórmula.
func Fila(cols []Columna, ref, estado string, data map[string]any, etiquetas, valores map[string]string) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		v := c.valor(ref, estado, data, etiquetas, valores)
		if !numerico(v) {
			v = validation.CSVSafe(v)
		}
		out = append(out, v)
	}
	return out
}

func numerico(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func (c Columna) valor(ref, estado string, data map[string]any, etiquetas, valores map[string]string) string {
	switch c.Clave {
	case "ref":
		return ref
	case "estado":
		return Legible(valores, estado)
	}
	v := data[c.Clave]
	switch c.Tipo {
	case catalogo.TipoDinero:
		return Importe(v)
	case catalogo.TipoRelacion:
		id, _ := v.(string)
		return Legible(etiquetas, id)
	case catalogo.TipoSeleccion:
		s, _ := v.(string)
		return Legible(valores, s)
	}
	return Texto(v)
}

// Legible es el nombre traducido cuando se conoce y el valor crudo cuando no.
func Legible(nombres map[string]string, valor string) string {
	if s := nombres[valor]; s != "" {
		return s
	}
	return valor
}

// Texto pinta un valor de `data` sin exponente (un 20000 no sale como 2e+04).
func Texto(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// Importe pasa céntimos a UNIDADES con dos decimales y punto decimal (123456 →
// «1234.56»), en aritmética entera. Lo que no es un importe entero sale vacío.
func Importe(v any) string {
	c, ok := Centimos(v)
	if !ok {
		return ""
	}
	unidades, resto := c/100, c%100
	if resto < 0 {
		resto = -resto
	}
	signo := ""
	if c < 0 && unidades == 0 {
		signo = "-"
	}
	return fmt.Sprintf("%s%d.%02d", signo, unidades, resto)
}

// Centimos lee un importe de `data`: un entero, un float64 entero exacto (jsonb sin
// UseNumber) o un json.Number entero (SQLite con UseNumber).
func Centimos(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x != float64(int64(x)) {
			return 0, false
		}
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}
