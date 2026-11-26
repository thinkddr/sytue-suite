// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// ContactsBatchData es el lote de Contacts que la fachada privada aún no tenía:
// exportar varias vCard en un `.vcf` y deshacer un borrado en lote (Anexo A de
// PLAN-EDICION-LOCAL.md). Sin él, la fachada privada sigue fallando cerrada.
type ContactsBatchData interface {
	BatchVCard(ctx context.Context, ids []string) ([]byte, error)
	BatchRestore(ctx context.Context, uids []string) (int, error)
}
