// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package banca

import (
	"slices"
	"strings"
	"time"
	"unicode"
)

// Pendiente es una factura aún sin cobrar o sin pagar, con lo que hace falta
// para reconocer su movimiento en el banco.
type Pendiente struct {
	ID  string
	Ref string
	// Tercero es el nombre del cliente o del proveedor; NIF, su identificador.
	Tercero, NIF       string
	Fecha, Vencimiento time.Time
	// Centimos es el total, siempre positivo.
	Centimos int64
}

// Propuesta es lo que se sabe de un movimiento frente a las facturas pendientes.
type Propuesta struct {
	// Automatica es la factura cuando no hay duda: se concilia sin preguntar.
	Automatica *Pendiente
	// Candidatas son las posibles, de más a menos probable (tres como mucho).
	Candidatas []Pendiente
}

// Emparejar busca la factura de un movimiento entre las pendientes de su mismo
// sentido: un ingreso, entre las emitidas sin cobrar; un cargo, entre las
// recibidas sin pagar.
//
// EL CRITERIO ES EL DE UN CONTABLE PRUDENTE. El importe tiene que ser el mismo
// al céntimo y la fecha, razonable (desde una semana antes de la factura hasta
// cuatro meses después de su vencimiento). Con eso solo, se PROPONE. Se concilia
// solo cuando además hay algo que lo confirma —el número de la factura, el NIF o
// el nombre del cliente en el concepto— y no hay otra igual de probable: dos
// recibos de 50 € del mismo mes no se casan a ciegas, porque un cobro mal
// asignado es peor que una pregunta.
func Emparejar(m Movimiento, pendientes []Pendiente) Propuesta {
	texto := normalizar(strings.Join([]string{m.Descripcion, m.Referencia1, m.Referencia2, m.Documento}, " "))
	importe := m.Centimos
	if importe < 0 {
		importe = -importe
	}
	type candidata struct {
		f      Pendiente
		puntos int
		dias   float64
	}
	var cs []candidata
	for _, f := range pendientes {
		ultimo := f.Vencimiento
		if ultimo.Before(f.Fecha) {
			ultimo = f.Fecha
		}
		if f.Centimos != importe || m.Fecha.Before(f.Fecha.AddDate(0, 0, -7)) || m.Fecha.After(ultimo.AddDate(0, 4, 0)) {
			continue
		}
		cs = append(cs, candidata{f: f, puntos: evidencia(texto, f), dias: abs(m.Fecha.Sub(f.Fecha).Hours())})
	}
	slices.SortStableFunc(cs, func(a, b candidata) int {
		if a.puntos != b.puntos {
			return b.puntos - a.puntos
		}
		switch {
		case a.dias < b.dias:
			return -1
		case a.dias > b.dias:
			return 1
		}
		return 0
	})
	var p Propuesta
	for i, c := range cs {
		if i == 3 {
			break
		}
		p.Candidatas = append(p.Candidatas, c.f)
	}
	if len(cs) > 0 && cs[0].puntos > 0 && (len(cs) == 1 || cs[0].puntos > cs[1].puntos) {
		p.Automatica = &p.Candidatas[0]
	}
	return p
}

// evidencia puntúa lo que el concepto dice de la factura: su número vale más que
// el NIF, y el NIF más que una palabra del nombre.
func evidencia(texto string, f Pendiente) int {
	puntos := 0
	if ref := compacto(f.Ref); len(ref) >= 4 && strings.Contains(compacto(texto), ref) {
		puntos += 4
	}
	if nif := compacto(f.NIF); len(nif) >= 8 && strings.Contains(compacto(texto), nif) {
		puntos += 2
	}
	palabras := strings.Fields(texto)
	for _, w := range strings.Fields(normalizar(f.Tercero)) {
		if len([]rune(w)) >= 4 && !genericas[w] && slices.Contains(palabras, w) {
			puntos++
			break
		}
	}
	return puntos
}

// genericas son las palabras de un nombre de empresa que no la distinguen.
var genericas = map[string]bool{
	"SOCIEDAD": true, "LIMITADA": true, "ANONIMA": true, "COOPERATIVA": true, "HERMANOS": true,
	"SERVICIOS": true, "GRUPO": true, "COMERCIAL": true, "ESPANA": true, "IBERICA": true,
}

// normalizar deja el texto en mayúsculas, sin tildes y con solo letras, números
// y espacios: «Suministros Acme, S.L.» y «SUMINISTROS ACME SL» dicen lo mismo.
func normalizar(s string) string {
	var b strings.Builder
	for _, r := range sinTildes.Replace(strings.ToUpper(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// sinTildes basta para los nombres de un extracto español, sin tirar de una tabla
// Unicode entera.
var sinTildes = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ä", "A", "É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I", "Ó", "O", "Ò", "O", "Ô", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U", "Ñ", "N", "Ç", "C",
)

// compacto quita también los espacios: «FV-2026-00042» y «FV 2026 00042» casan.
func compacto(s string) string {
	return strings.ReplaceAll(normalizar(s), " ", "")
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
