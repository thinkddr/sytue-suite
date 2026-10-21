// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"fmt"
	"log/slog"
)

// slogAdapter adapta *slog.Logger al logger que espera go-imap (interfaz con
// Printf). Encamina las trazas del servidor IMAP al logger estructurado.
type slogAdapter struct{ l *slog.Logger }

func newSlogAdapter(l *slog.Logger) *slogAdapter {
	if l == nil {
		l = slog.Default()
	}
	return &slogAdapter{l: l}
}

// Printf implementa la interfaz de logging de imapserver.
//
// El detalle va en `detalle` y no en `msg`: slog NO deduplica claves, así que
// pasar "msg" como atributo producía dos `"msg"` en el mismo objeto JSON. Con la
// clave repetida, quien lea el registro se queda con una de las dos —cuál,
// depende del parser— y la otra desaparece: buscar por el texto del error no
// encontraba nada, o encontraba «imap» y ya.
func (a *slogAdapter) Printf(format string, args ...any) {
	a.l.Info("imap", "detalle", fmt.Sprintf(format, args...))
}
