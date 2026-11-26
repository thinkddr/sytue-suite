// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package autorizacion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// IdentitySnapshot is the session's existing non-secret identity, read under one
// lock. It is not evidence that an arbitrary bearer token belongs to that identity.
type IdentitySnapshot struct {
	Issuer, Client, Subject, Organization string
}

// IdentidadDrive es un contexto opaco estable. Cambiar un refresh token no cambia
// este hash; un emisor, cliente, persona u organización diferentes sí lo hacen.
func (i IdentitySnapshot) IdentidadDrive() string {
	if i.Organization == "" {
		return ""
	}
	return i.IdentidadSesion()
}

// IdentidadSesion is the shared opaque context for device work, independent of
// whether this user mounted Drive in the operating system.
func (i IdentitySnapshot) IdentidadSesion() string {
	raw := i.ContextoSesion()
	if raw == nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ContextoSesion es el contexto en claro del que sale IdentidadSesion (emisor sin la
// barra final, cliente, persona y organización), o nil si falta alguno de los tres
// primeros. No lleva nada secreto: la sesión lo publica junto al hash.
func (i IdentitySnapshot) ContextoSesion() []byte {
	if i.Issuer == "" || i.Client == "" || i.Subject == "" {
		return nil
	}
	raw, _ := json.Marshal([]string{strings.TrimRight(i.Issuer, "/"), i.Client, i.Subject, i.Organization})
	return raw
}
