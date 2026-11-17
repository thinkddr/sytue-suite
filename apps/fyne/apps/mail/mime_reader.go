// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

func leerMIME(ctx context.Context, source any, reader io.Reader) (eml.Mensaje, error) {
	limit := int64(eml.DefaultMessageByteLimit)
	if captured, ok := source.(interface{ MessageByteLimit() int64 }); ok {
		limit = captured.MessageByteLimit()
	}
	return eml.LeerConLimite(ctx, reader, limit)
}
