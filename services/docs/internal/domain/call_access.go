// SPDX-License-Identifier: Apache-2.0
package domain

import "errors"

var ErrCallAccessUnavailable = errors.New("docs: encrypted call access is unavailable")

// DocumentCallAccess is an authorization assertion, never document content.
type DocumentCallAccess struct {
	DocumentID        string `json:"document_id"`
	Actor             string `json:"actor"`
	Tenant            string `json:"tenant"`
	OrganizationAdmin bool   `json:"organization_admin"`
	IdentityVersion   int64  `json:"identity_version"`
	Version           int64  `json:"version"`
	ExpiresAtMS       int64  `json:"expires_at_ms"`
}
