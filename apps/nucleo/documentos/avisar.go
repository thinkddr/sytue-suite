// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux || android

package documentos

import "log/slog"

// avisar apunta lo que no ha salido al conectar o desconectar: el sistema es de quien
// lo usa y un fallo suyo no para la app.
func avisar(que string, err error) {
	if err != nil {
		slog.Warn("documentos: "+que, "error", err)
	}
}
