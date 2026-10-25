// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// dueñoCache scopes disposable HTTP records to deployment, configured issuer,
// subject and tenant. JSON separates components without delimiter collisions.
func (s *Suite) dueñoCache() string {
	if s == nil || s.Sesion == nil {
		return ""
	}
	identity := s.Sesion.SnapshotIdentity()
	if identity.Subject == "" {
		return ""
	}
	issuer := api.ServiceURL(s.apex, "id")
	if s.oidc != nil && s.oidc.Issuer != "" {
		issuer = strings.TrimRight(s.oidc.Issuer, "/")
	}
	key, _ := json.Marshal([]string{s.apex, issuer, identity.Subject, identity.Organization}) // strings cannot fail JSON encoding
	return string(key)
}

// dueñoNotas retains the existing queue/draft format used by notes.claveAlmacen.
// Changing the disposable HTTP namespace must never strand unsent user drafts.
func (s *Suite) dueñoNotas() string {
	if s == nil || s.Sesion == nil {
		return ""
	}
	identity := s.Sesion.SnapshotIdentity()
	if identity.Subject == "" {
		return ""
	}
	return identity.Subject + "/" + identity.Organization
}
