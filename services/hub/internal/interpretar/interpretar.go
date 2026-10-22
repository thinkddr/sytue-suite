// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package interpretar traduce una frase a la consulta que la búsqueda federada
// YA sabe ejecutar.
//
// POR QUÉ ESTO Y NO UN ÍNDICE SEMÁNTICO. El buscador del Hub ya filtra por
// origen, por tipo y por rango de fechas: `sources=drive&kinds=file&from=…`.
// Lo que no sabe es que «las facturas de Acme del mes pasado en Drive» ES esa
// consulta. Aquí no se añade capacidad de búsqueda, se le enseña a leer la
// frase — y eso vale para todo lo que el buscador aprenda a filtrar mañana,
// mientras que un índice de vectores sería un sistema paralelo que mantener.
//
// EL MODELO NO BUSCA NADA. Devuelve parámetros; los resultados los sigue
// trayendo la misma búsqueda federada de siempre, con los permisos de siempre.
// Un modelo que alucina aquí, como mucho, devuelve una consulta rara —y la
// interfaz enseña qué ha entendido para poder quitarlo—; no puede enseñar un
// documento que no te toca.
//
// LAS FECHAS RELATIVAS LAS RESUELVE EL MODELO, con `ahora` puesto por nosotros.
// «El mes pasado» no significa nada sin saber qué día es hoy, y calcularlo aquí
// a mano querría decir escribir un analizador de fechas en cinco idiomas.
package interpretar

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// MaxConsulta acota la frase que se acepta interpretar, en runas. Quien busca
// escribe una línea; lo que llegue por encima es un pegote y no una consulta.
const MaxConsulta = 500

// Interpretacion es lo que se ha entendido de la frase.
//
// Lleva la consulta ya normalizada Y el desglose de lo que se ha aplicado,
// porque la interfaz tiene que PODER ENSEÑARLO: una búsqueda que filtra por un
// rango de fechas que el usuario no ha visto es una búsqueda que devuelve cero
// resultados sin explicar por qué.
type Interpretacion struct {
	Consulta domain.SearchQuery
	// Texto es lo que queda como término literal tras sacar los filtros.
	Texto string
	// Fuentes, Tipos y el rango, tal cual se han aplicado. Redundante con
	// Consulta a propósito: es lo que se serializa hacia la interfaz.
	Fuentes []domain.Source
	Tipos   []string
	Desde   *time.Time
	Hasta   *time.Time
}

// Tipos de entidad que los conectores saben filtrar. Lista CERRADA: un `kind`
// inventado por el modelo no filtra nada en unos conectores y lo filtra todo en
// otros, así que se descarta aquí en vez de dejar que cada uno improvise.
var tiposValidos = map[string]bool{
	"file": true, "message": true, "event": true, "contact": true, "note": true, "channel": true,
}

// Instruccion es lo que se le pide al modelo.
//
// PIDE JSON Y NADA MÁS, y enumera los valores admitidos: un modelo al que se le
// deja elegir el nombre del origen escribe «Sytue Drive», «drive_files» y
// «Drive» en tres peticiones seguidas, y las tres se caerían al validar.
//
// Y le dice que ANTE LA DUDA NO FILTRE. Es la regla que más cambia el
// resultado: un filtro de más devuelve cero resultados y parece que no hay
// nada; un filtro de menos devuelve algún resultado sobrante, que se ve y se
// descarta de un vistazo.
func Instruccion(ahora time.Time) string {
	return strings.Join([]string{
		"Traduce la consulta de búsqueda a parámetros. Devuelve SOLO un objeto JSON con las claves",
		`{"texto","fuentes","tipos","desde","hasta"}.`,
		`"texto" son las palabras que hay que buscar literalmente, sin las que ya expresas como filtro.`,
		`"fuentes" es una lista con valores de: drive, mail, calendar, contacts, chat, notes, integrations.`,
		`"tipos" es una lista con valores de: file, message, event, contact, note, channel.`,
		`"desde" y "hasta" son instantes ISO 8601.`,
		"Omite las claves que la consulta no exprese. ANTE LA DUDA NO FILTRES: es preferible un resultado de más que ninguno.",
		fmt.Sprintf("Ahora son las %s.", ahora.Format(time.RFC3339)),
	}, " ")
}

// respuesta es la forma que se espera del modelo. Todo opcional.
type respuesta struct {
	Texto   string   `json:"texto"`
	Fuentes []string `json:"fuentes"`
	Tipos   []string `json:"tipos"`
	Desde   string   `json:"desde"`
	Hasta   string   `json:"hasta"`
}

// De valida lo que respondió el modelo y lo convierte en una consulta.
//
// `original` es lo que escribió la persona, y se usa de RED: si el modelo no
// deja término literal —pasa cuando la frase es toda filtros, «correos de la
// semana pasada»— se conserva vacío, pero si además no ha entendido ni un
// filtro, se devuelve la consulta original sin tocar. Nunca se busca algo que
// el usuario no escribió.
func De(bruto, original string, ahora time.Time) (Interpretacion, error) {
	var r respuesta
	if err := json.Unmarshal([]byte(asistente.JSON(bruto)), &r); err != nil {
		return Interpretacion{}, fmt.Errorf("%w: la respuesta no es JSON", domain.ErrInvalidInput)
	}

	in := Interpretacion{Texto: strings.TrimSpace(r.Texto)}
	for _, f := range r.Fuentes {
		s := domain.Source(strings.ToLower(strings.TrimSpace(f)))
		if domain.ValidSource(s) && !contiene(in.Fuentes, s) {
			in.Fuentes = append(in.Fuentes, s)
		}
	}
	for _, k := range r.Tipos {
		k = strings.ToLower(strings.TrimSpace(k))
		if tiposValidos[k] && !contieneStr(in.Tipos, k) {
			in.Tipos = append(in.Tipos, k)
		}
	}
	in.Desde = instante(r.Desde)
	in.Hasta = instante(r.Hasta)
	// Un rango al revés no acota nada: acotaría a la nada. Se tira el extremo
	// que sobra en vez de devolver una búsqueda que no puede tener resultados.
	if in.Desde != nil && in.Hasta != nil && in.Hasta.Before(*in.Desde) {
		in.Hasta = nil
	}

	// Si no ha entendido NADA, la consulta se queda como estaba: interpretar no
	// puede empeorar lo que ya funcionaba.
	if in.Texto == "" && len(in.Fuentes) == 0 && len(in.Tipos) == 0 && in.Desde == nil && in.Hasta == nil {
		in.Texto = strings.TrimSpace(original)
	}

	in.Consulta = domain.SearchQuery{
		Text:    in.Texto,
		Sources: in.Fuentes,
		Kinds:   in.Tipos,
		From:    in.Desde,
		To:      in.Hasta,
	}
	return in, nil
}

// instante acepta SOLO ISO 8601 con forma completa.
//
// Se comprueba con `time.Parse` de RFC 3339, que es estricto — al revés que el
// parser de fechas del navegador, donde `new Date('el jueves')` devuelve una
// fecha válida. Aquí una frase sin resolver tiene que quedarse en «sin filtro»,
// no convertirse en un rango inventado que esconde resultados.
func instante(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

func contiene(xs []domain.Source, x domain.Source) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func contieneStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// MereceInterpretarse decide si una consulta da para preguntarle al modelo.
//
// EL FILTRO ES DE COSTE, no de calidad: cada interpretación es una llamada a un
// modelo, y disparar una por cada tecla de «fact» mientras alguien escribe
// «factura» sería pagar seis veces por nada. Una palabra suelta tampoco tiene
// nada que interpretar: es ya el término literal que hay que buscar.
func MereceInterpretarse(q string) bool {
	n := len(strings.Fields(q))
	return n >= 3 && len([]rune(q)) <= MaxConsulta
}
