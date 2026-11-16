// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// ReadResourceBootstrapSource is the internal metadata boundary. The fixed
// Rooms service authenticates the producer before this hop; this product still
// resolves owner, freeze epoch and ACL version from its ORIGINAL normal-RLS SQL.
func (s *Store) ReadResourceBootstrapSource(ctx context.Context, query resource.BootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error) {
	if !query.Valid() || resource.OriginServiceID(query.Resource.Product) != "docs" {
		return resource.BootstrapSourceSnapshot{}, resource.ErrInvalid
	}
	original, err := NewResourceBootstrapSource(s, query.OwnerScope)
	if err != nil {
		return resource.BootstrapSourceSnapshot{}, err
	}
	return original.BootstrapSource(ctx, query.Resource)
}
