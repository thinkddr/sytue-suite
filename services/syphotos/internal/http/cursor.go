// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// El CURSOR de la línea de tiempo, opaco para el cliente.
//
// POR QUÉ OPACO Y NO DOS PARÁMETROS. La paginación por (fecha, id) tiene que
// usar exactamente la misma expresión de orden que el índice; en cuanto el
// cliente compone la posición a mano, alguien pasa una fecha con otra precisión
// o sin el id, y la página siguiente empieza a repetir o a saltarse filas —sin
// error, que es lo peor—. Opaco significa que solo lo produce el servidor.
//
// NO VA FIRMADO, y es deliberado: no autoriza nada. Quien lo manipule solo puede
// pedirse a sí mismo una posición distinta de SU propia línea de tiempo —la
// consulta filtra por `owner_sub` y por organización en todo caso—, así que
// firmarlo costaría una clave que gestionar para no proteger nada.
//
// RFC3339 con nanosegundos porque una ráfaga comparte segundo: truncar la
// precisión haría que el desempate por id no bastara y se repetirían fotos.

func cifrarCursor(c store.Cursor) string {
	crudo := c.Fecha.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(crudo))
}

func descifrarCursor(s string) (*store.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("cursor ilegible: %w", err)
	}
	partes := strings.SplitN(string(b), "|", 2)
	if len(partes) != 2 {
		return nil, fmt.Errorf("cursor incompleto")
	}
	t, err := time.Parse(time.RFC3339Nano, partes[0])
	if err != nil {
		return nil, fmt.Errorf("fecha del cursor ilegible: %w", err)
	}
	id, err := uuid.Parse(partes[1])
	if err != nil {
		return nil, fmt.Errorf("id del cursor ilegible: %w", err)
	}
	return &store.Cursor{Fecha: t, ID: id}, nil
}

// cursorDeFecha convierte `desde=` —un año, un mes o un día: `2019`,
// `2019-07`, `2019-07-14`— en la posición desde la que sigue la línea de
// tiempo: justo ANTES del primer instante del periodo siguiente. Así la página
// empieza por la foto más reciente de ese periodo y, al seguir bajando, entra
// en los anteriores con la paginación de siempre.
//
// Es un cursor como cualquier otro, y por eso el resto de páginas usan el
// opaco: el servidor sigue siendo el único que compone posiciones intermedias.
// `uuid.Nil` como id hace la comparación estricta —`(fecha, id) < (límite,
// Nil)` equivale a `fecha < límite`—, de modo que una foto hecha justo a las
// 00:00 del uno de enero siguiente no se cuela en el año pedido.
//
// En UTC, como la fecha guardada: la galería no conoce la zona de quien mira.
func cursorDeFecha(s string) (*store.Cursor, error) {
	s = strings.TrimSpace(s)
	var (
		t   time.Time
		err error
	)
	switch len(s) {
	case 4:
		t, err = time.Parse("2006", s)
		t = t.AddDate(1, 0, 0)
	case 7:
		t, err = time.Parse("2006-01", s)
		t = t.AddDate(0, 1, 0)
	case 10:
		t, err = time.Parse("2006-01-02", s)
		t = t.AddDate(0, 0, 1)
	default:
		return nil, fmt.Errorf("fecha %q: se espera AAAA, AAAA-MM o AAAA-MM-DD", s)
	}
	if err != nil {
		return nil, fmt.Errorf("fecha %q ilegible: %w", s, err)
	}
	return &store.Cursor{Fecha: t, ID: uuid.Nil}, nil
}
