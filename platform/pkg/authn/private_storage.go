// SPDX-License-Identifier: Apache-2.0
package authn

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// PrivateStorageIdentity adapts only the already verified claims context. It
// reads no header or JSON, and never creates a capability or readiness grant.
func PrivateStorageIdentity(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
	claims, ok := FromContext(ctx)
	return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
}
