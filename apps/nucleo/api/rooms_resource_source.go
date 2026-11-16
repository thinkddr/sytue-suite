// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MatrixResourceBootstrapSource uses the caller's original API session and
// retained SDK signature. No third-party/backend credential reaches the app.
func (r *Rooms) MatrixResourceBootstrapSource(ctx context.Context, product string, query resource.SignedBootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error) {
	if r == nil || r.c == nil || !storage.Product(product) || len(query.Signatures) == 0 || !query.Valid() || query.Resource.Product != product || query.OwnerScope.Product != product {
		return resource.BootstrapSourceSnapshot{}, ErrMatrixSSO
	}
	result, err := PostJSON[resource.BootstrapSourceSnapshot](ctx, r.c, "/matrix/resources/"+product+"/bootstrap/source", query)
	if err != nil {
		return resource.BootstrapSourceSnapshot{}, err
	}
	if !result.Valid() || result.Resource != query.Resource || result.OwnerScope != query.OwnerScope {
		return resource.BootstrapSourceSnapshot{}, ErrMatrixSSO
	}
	return result, nil
}
