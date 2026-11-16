// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"fmt"
)

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
