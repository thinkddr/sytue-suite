// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auditlog

import "context"

// ConRegistro deja el registro en el contexto como lo hace el middleware, para
// los tests de fuera del paquete.
func ConRegistro(ctx context.Context, r *Registro) context.Context {
	return context.WithValue(ctx, claveRegistro{}, &peticion{reg: r, requestID: "req-test", ip: "192.0.2.1"})
}
