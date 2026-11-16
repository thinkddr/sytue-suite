// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// Cumpleaños SIN año: el «--0215» de vCard 4 (RFC 6350 §4.3.1, fecha truncada), el
// «--02-15» que escriben algunos clientes de vCard 3 y el año ficticio que Contactos de
// Apple marca con `X-APPLE-OMIT-YEAR=1604`. La proyección lo da en `birthday_no_year`
// como «--MM-DD» (la forma extendida de ISO 8601:2000), aparte de `birthday`, que
// sigue siendo una fecha completa: así un cliente que solo entiende fechas no recibe
// algo que no sabe leer, y la app nativa puede contar y enseñar los dos.

// paramOmitirAno es el parámetro con el que Contactos de Apple dice que el año del BDAY
// es de relleno (vale el propio año, normalmente 1604).
const paramOmitirAno = "X-APPLE-OMIT-YEAR"

// NormalizeBirthdayNoYear valida un cumpleaños sin año («--MM-DD» o «--MMDD») y lo
// devuelve como «--MM-DD». Vale el 29 de febrero —quien nació ese día lo celebra—,
// pero no el 31 de abril.
func NormalizeBirthdayNoYear(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "--") {
		return "", fmt.Errorf("%w: cumpleaños sin año no válido %q", ErrInvalidContact, v)
	}
	r := strings.Replace(v[2:], "-", "", 1)
	if len(r) != 4 {
		return "", fmt.Errorf("%w: cumpleaños sin año no válido %q", ErrInvalidContact, v)
	}
	mes, err1 := strconv.Atoi(r[:2])
	dia, err2 := strconv.Atoi(r[2:])
	if err1 != nil || err2 != nil || !mesYDiaValidos(mes, dia) {
		return "", fmt.Errorf("%w: cumpleaños sin año no válido %q", ErrInvalidContact, v)
	}
	return fmt.Sprintf("--%02d-%02d", mes, dia), nil
}

// mesYDiaValidos comprueba la fecha en un año bisiesto (2000), para que el 29 de
// febrero pase y `time.Date` no convierta un 31 de abril en 1 de mayo.
func mesYDiaValidos(mes, dia int) bool {
	if mes < 1 || mes > 12 || dia < 1 {
		return false
	}
	t := time.Date(2000, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
	return int(t.Month()) == mes && t.Day() == dia
}

// leerCumple proyecta un BDAY: una fecha completa, o un cumpleaños sin año, o nada
// (texto libre como «primavera», que la proyección no entiende y se conserva oculto).
func leerCumple(f *vcard.Field) (completo *time.Time, sinAno string) {
	if f == nil || strings.TrimSpace(f.Value) == "" {
		return nil, ""
	}
	if t, err := parseBirthday(f.Value); err == nil {
		// El año de relleno de Apple: tomarlo por real daría «10 de diciembre de 1604»
		// en la ficha y cuatro siglos de edad en «Próximos cumpleaños».
		if f.Params.Get(paramOmitirAno) == strconv.Itoa(t.Year()) {
			return nil, t.Format("--01-02")
		}
		return &t, ""
	}
	if s, err := NormalizeBirthdayNoYear(f.Value); err == nil {
		return nil, s
	}
	return nil, ""
}

// campoCumple es el BDAY que se guarda: la fecha completa, el cumpleaños sin año o, si
// no viene ninguno, el BDAY de la tarjeta de base que la proyección no entiende (texto
// libre): quien edita por la API no lo ve y no puede reenviarlo, así que se queda. Lo
// que sí se proyecta y no llega se borra, que es como se quita. Nil, sin BDAY.
func campoCumple(c Contact, base *vcard.Field) *vcard.Field {
	switch {
	case c.Birthday != nil:
		return &vcard.Field{Value: c.Birthday.Format(bdayLayout)}
	case c.BirthdayNoYear != "":
		return campoCumpleSinAno(c.BirthdayNoYear, base)
	case base != nil && strings.TrimSpace(base.Value) != "":
		if completo, sinAno := leerCumple(base); completo == nil && sinAno == "" {
			return base
		}
	}
	return nil
}

// campoCumpleSinAno es el BDAY de un cumpleaños sin año. Si la tarjeta de base ya lo
// tenía con ese mismo día, se deja como estaba (el año de relleno de Apple incluido):
// editar el teléfono por la API no debe reescribir un cumpleaños que nadie ha tocado,
// ni quitarle a Contactos de Apple la marca con la que lo reconoce.
func campoCumpleSinAno(sinAno string, base *vcard.Field) *vcard.Field {
	if _, s := leerCumple(base); base != nil && s == sinAno {
		return base
	}
	// vCard 4 solo admite la forma básica de la fecha truncada: «--MMDD».
	return &vcard.Field{Value: "--" + strings.ReplaceAll(sinAno[2:], "-", "")}
}
