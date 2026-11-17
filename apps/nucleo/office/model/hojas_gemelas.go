// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"fmt"

	"git.sytue.eu/sytue/one/apps/nucleo/office/nombrehoja"
)

// Hojas gemelas: dos hojas de un libro que solo difieren en mayúsculas («Ventas» y
// «VENTAS»). Excel y LibreOffice no las dejan tener, una referencia `VENTAS!A1` confunde
// a quien la lee y la web y Calcus nativo ya no dejan crearlas ni renombrar a ellas. El
// servidor rechaza el guardado que las CREA —una hoja nueva o un renombre a un nombre
// gemelo—, comparando con el libro que había; el que ya las tenía (un importado de antes
// de que el importador las reparara, un libro de antes de esta regla) se sigue guardando
// sin tocarlas. La regla es nombrehoja.GemelaNueva.

// ErrHojaGemela: el guardado crea una hoja, o renombra una, con el nombre de otra salvo
// mayúsculas. Es una operación inválida para quien no lo distinga.
var ErrHojaGemela = fmt.Errorf("%w: el nombre de una hoja solo difiere en mayúsculas del de otra", ErrInvalidOp)

// gemelas aplica la regla al modelo entrante que gana al registro, ya aceptado por la
// protección. `prev` es el modelo vigente que guarda el estado de protección, que solo
// está con alguna protección puesta; sin ella, si el modelo entrante trae gemelas, se lee
// el registro guardado (Proteccion.Vigente). Sin modelo previo no hay con qué comparar:
// es el contenido con el que el libro empieza, y se acepta.
func (c *evaluador) gemelas(est EstadoProteccion, prev, nuevo *libro) error {
	if nuevo == nil {
		return nil
	}
	ahora := nuevo.nombres
	defer func() { c.hojas, c.hayHojas = ahora, true }()
	if c.p.Restaurando || !nombrehoja.HayGemelas(ahora) {
		return nil
	}
	var antes []string
	switch {
	case c.hayHojas:
		antes = c.hojas
	case prev != nil:
		antes = prev.nombres
	case est.Modelo == "" && est.HayRegistro && c.p.Vigente != nil:
		raw, err := c.p.Vigente()
		if err != nil {
			return err
		}
		l, ok := leerLibro(raw)
		if !ok || l == nil {
			return nil // lo guardado no es un modelo: no hay hojas con las que comparar
		}
		antes = l.nombres
	default:
		return nil
	}
	if n := nombrehoja.GemelaNueva(antes, ahora); n != "" {
		return fmt.Errorf("%w: «%s»", ErrHojaGemela, n)
	}
	return nil
}

// desgemelarHojas repara al importar un libro con hojas gemelas, que trae un fichero hecho
// a mano o por otro programa: la segunda de cada grupo pasa a «VENTAS (2)», como al
// reparar Excel un libro (nombrehoja.Desgemelar). Va al final de la lectura, con las
// dinámicas y los nombres definidos ya puestos, y les cambia la hoja que citaban por su
// nombre exacto: el ámbito de los nombres y el origen de las dinámicas. Las fórmulas de las
// celdas que la citaban no se reescriben: con dos hojas que Excel daba por la misma ya
// eran ambiguas, y renombrar una hoja en el editor tampoco las reescribe.
func desgemelarHojas(m *sheetModel) {
	if m == nil {
		return
	}
	viejos := make([]string, len(m.Sheets))
	for i := range m.Sheets {
		viejos[i] = m.Sheets[i].Name
	}
	nuevos := nombrehoja.Desgemelar(viejos)
	cambio := map[string]string{}
	for i := range m.Sheets {
		if nuevos[i] == viejos[i] {
			continue
		}
		m.Sheets[i].Name = nuevos[i]
		// Dos hojas con el mismo nombre exacto no se distinguen por él: lo que lo cita se
		// queda con la primera, que no cambia.
		if repeticiones(viejos, viejos[i]) == 1 {
			cambio[viejos[i]] = nuevos[i]
		}
	}
	if len(cambio) == 0 {
		return
	}
	for i := range m.Names {
		if n, ok := cambio[m.Names[i].Sheet]; ok {
			m.Names[i].Sheet = n
		}
	}
	for i := range m.Sheets {
		for j := range m.Sheets[i].Pivots {
			if n, ok := cambio[m.Sheets[i].Pivots[j].SourceSheet]; ok {
				m.Sheets[i].Pivots[j].SourceSheet = n
			}
		}
	}
}
