// SPDX-License-Identifier: Apache-2.0
package domain

import "context"

type DocumentCallAccess struct {
	DocumentID        string `json:"document_id"`
	Actor             string `json:"actor"`
	Tenant            string `json:"tenant"`
	OrganizationAdmin bool   `json:"organization_admin"`
	IdentityVersion   int64  `json:"identity_version"`
	Version           int64  `json:"version"`
	ExpiresAtMS       int64  `json:"expires_at_ms"`
}
type documentCallAccessKey struct{}

func WithDocumentCallAccess(ctx context.Context, proof DocumentCallAccess) context.Context {
	return context.WithValue(ctx, documentCallAccessKey{}, proof)
}

func DocumentCallAccessFrom(ctx context.Context) (DocumentCallAccess, bool) {
	proof, ok := ctx.Value(documentCallAccessKey{}).(DocumentCallAccess)
	return proof, ok
}
