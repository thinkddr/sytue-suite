// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Exportar una tabla a CSV o JSON y nombrar un registro enlazado: lo mismo en el
// servicio (`services/bases/internal/http`, por tramos) y en Sytue Office.

// RegistroExportado es un registro en la exportación JSON: además de los ids de sus
// enlaces (`links`), el texto de cada registro enlazado (`link_values`, en el mismo orden).
type RegistroExportado struct {
	Registro
	LinkValues map[string][]string `json:"link_values"`
}

// ValoresDeEnlaces da, por campo de enlace, el texto de los registros enlazados.
func ValoresDeEnlaces(campos []Campo, rg Registro, etiquetas map[string]string) map[string][]string {
	out := map[string][]string{}
	for _, f := range campos {
		if f.Kind == TipoRelacion && len(rg.Links[f.ID]) > 0 {
			out[f.ID] = textosDe(rg.Links[f.ID], etiquetas)
		}
	}
	return out
}

func textosDe(ids []string, etiquetas map[string]string) []string {
	vs := make([]string, len(ids))
	for i, id := range ids {
		vs[i] = etiquetas[id]
	}
	return vs
}

// CabeceraExportada y FilaExportada son la fila de títulos y la de un registro del CSV.
// Un campo de enlace ocupa DOS columnas: la suya, con el texto de los registros enlazados
// (lo que una persona quiere leer), y «<nombre> (id)», con sus ids (lo que una máquina
// necesita para volver a casarlos).
func CabeceraExportada(campos []Campo) []string {
	cab := make([]string, 0, len(campos))
	for _, f := range campos {
		cab = append(cab, CeldaCSV(f.Name))
		if f.Kind == TipoRelacion {
			cab = append(cab, CeldaCSV(f.Name+" (id)"))
		}
	}
	return cab
}

// FilaExportada es la fila de un registro en el CSV (ver CabeceraExportada).
func FilaExportada(campos []Campo, rg Registro, etiquetas map[string]string) []string {
	fila := make([]string, 0, len(campos))
	for _, f := range campos {
		if f.Kind != TipoRelacion {
			fila = append(fila, CeldaExportada(f, rg))
			continue
		}
		// El texto de un registro enlazado lo escribió alguien: pasa por CeldaCSV como
		// cualquier otro texto. Los ids son UUID y no pueden ser una fórmula.
		fila = append(fila,
			CeldaCSV(strings.Join(textosDe(rg.Links[f.ID], etiquetas), "; ")),
			strings.Join(rg.Links[f.ID], ";"))
	}
	return fila
}

// CeldaExportada pasa el valor de un registro a texto de CSV. Solo lo que es texto
// del usuario pasa por CeldaCSV: un número negativo no es una fórmula.
func CeldaExportada(f Campo, rg Registro) string {
	if f.Kind == TipoAdjunto {
		// El nombre de un fichero lo puso quien lo subió: se neutraliza como cualquier texto.
		return CeldaCSV(NombresDeAdjuntos(rg.Attachments[f.ID]))
	}
	raw := rg.Data[f.ID]
	if len(raw) == 0 {
		return ""
	}
	switch f.Efectivo().Kind {
	case TipoNumero:
		var n float64
		if json.Unmarshal(raw, &n) == nil {
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
	case TipoCasilla:
		var b bool
		if json.Unmarshal(raw, &b) == nil {
			return strconv.FormatBool(b)
		}
	case TipoMultiple:
		var l []string
		if json.Unmarshal(raw, &l) == nil {
			return CeldaCSV(strings.Join(l, "; "))
		}
	default:
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return CeldaCSV(s)
		}
	}
	return ""
}

// NombresDeAdjuntos es la celda de un campo de adjuntos en la exportación: sus nombres.
func NombresDeAdjuntos(adjuntos []Adjunto) string {
	nombres := make([]string, len(adjuntos))
	for i, x := range adjuntos {
		nombres[i] = x.Name
	}
	return strings.Join(nombres, "; ")
}

// EtiquetaDe es el texto con el que se nombra un registro: su primer campo de texto,
// número, fecha o selección con valor; si no tiene, el principio de su id.
func EtiquetaDe(campos []Campo, data map[string]json.RawMessage, id string) string {
	for _, c := range campos {
		if c.Kind == TipoRelacion || c.Kind == TipoCasilla || c.Kind == TipoMultiple {
			continue
		}
		var v any
		if raw := data[c.ID]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil && v != nil {
			return fmt.Sprint(v)
		}
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
