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
