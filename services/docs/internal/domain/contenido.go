// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
)

// ContentKey es la clave del registro LWW donde el editor guarda el modelo
// entero de cada tipo: el HTML del documento, o el JSON de SheetModel o de
// SlidesModel. Es el `CONTENT_KEY` del frontend (editor/types.ts); separarlos
// los desincronizaría.
func ContentKey(k Kind) string {
	switch k {
	case KindSpreadsheet:
		return "doc:sheet"
	case KindPresentation:
		return "doc:slides"
	default:
		return "doc:html"
	}
}

// ContentOp es la operación que siembra `content` como contenido inicial de un
// documento de tipo `k`: un único `set` de su registro de contenido, la misma
// forma en que lo siembra la importación del cliente.
//
// El actor es propio y aleatorio, como el de restaurar una versión, para que su
// Dot no pueda coincidir jamás con el de una sesión de edición.
func ContentOp(k Kind, content string) (Op, error) {
	valor, err := json.Marshal(content)
	if err != nil {
		return Op{}, fmt.Errorf("docs: serializar contenido: %w", err)
	}
	tok, err := NewToken()
	if err != nil {
		return Op{}, err
	}
	return Op{
		Kind: OpSet, Key: ContentKey(k), Value: valor,
		Dot: Dot{Actor: "plantilla:" + tok[:16], Counter: 1}, Lamport: 1,
	}, nil
}
