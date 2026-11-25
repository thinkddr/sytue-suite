// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// DefectoHoy es el valor por defecto de una fecha que no es un día fijo sino el de
// cada alta: «Fecha de entrada = hoy». No choca con un valor real porque una fecha
// guardada siempre es AAAA-MM-DD.
const DefectoHoy = "today"

// ValidarDefecto comprueba el valor por defecto de un campo y devuelve su forma
// canónica; nil quita el que hubiera. Vale lo mismo que en una celda (ValidarValor),
// más DefectoHoy en las fechas. Los enlaces, las fórmulas y los adjuntos no lo
// admiten: un enlace por defecto ataría cada alta a un registro concreto, una fórmula
// se calcula sola y un fichero no se copia.
func ValidarDefecto(c Campo, raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	switch c.Kind { //nolint:exhaustive // el resto admite cualquier valor de su tipo
	case TipoRelacion, TipoFormula, TipoAdjunto:
		return nil, fmt.Errorf("%w: el campo %q no admite valor por defecto", ErrInvalido, c.Name)
	case TipoFecha:
		var s string
		if json.Unmarshal(raw, &s) == nil && s == DefectoHoy {
			return json.Marshal(DefectoHoy)
		}
	}
	return ValidarValor(c, raw)
}

// ConDefectos añade a los datos de un registro NUEVO el valor por defecto de cada campo
// que la petición no trae. Lo que sí trae —también un null, que es «déjalo vacío» a
// propósito— no se toca: el defecto es para lo que nadie ha escrito. `entrada` son las
// claves que envió el cliente; `datos`, lo ya validado.
//
// Un defecto se vuelve a validar al aplicarlo: entre que se fijó y el alta pueden haber
// retirado la opción que nombraba, y entonces el registro nace sin él en vez de fallar
// por algo que quien da el alta no ha escrito. `hoy` es la fecha de DefectoHoy.
func ConDefectos(campos []Campo, entrada, datos map[string]json.RawMessage, hoy time.Time) map[string]json.RawMessage {
	for _, c := range campos {
		if len(c.Default) == 0 {
			continue
		}
		if _, escrito := entrada[c.ID]; escrito {
			continue
		}
		raw := c.Default
		var s string
		if c.Kind == TipoFecha && json.Unmarshal(raw, &s) == nil && s == DefectoHoy {
			raw, _ = json.Marshal(hoy.Format(time.DateOnly))
		}
		v, err := ValidarValor(c, raw)
		if err != nil || v == nil {
			continue
		}
		if datos == nil {
			datos = map[string]json.RawMessage{}
		}
		datos[c.ID] = v
	}
	return datos
}
