// SPDX-License-Identifier: Apache-2.0
package cast

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// AuditSource checks the retained frozen source and private binaries at the
// time of the call. It does not authorize or perform source retirement.
func (r *Repository) AuditSource(ctx context.Context) (records.SourceAudit, error) {
	return r.engine.AuditSource(ctx)
}
