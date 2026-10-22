// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

// Subject identifica a quién se está autorizando: su `sub` de Sytue ID, sus
// grupos y su tenant. Se construye a partir de los claims del JWT.
type Subject struct {
	Sub    string
	Groups []string
	Tenant string
}

// ResolveRole calcula el rol efectivo de un subject sobre un documento aplicando
// deny-by-default: el owner del documento es siempre owner; en otro caso se toma
// el MÁXIMO rol entre las entradas de ACL aplicables (usuario, grupos, tenant).
// Devuelve ("", false) si el subject no tiene ningún acceso.
//
// Las entradas 'public' NO se resuelven aquí: el acceso público pasa por
// ResolveShare con un token de enlace, no por la identidad del subject.
func ResolveRole(doc *Document, perms []Permission, subj Subject) (Role, bool) {
	if doc.OwnerSub != "" && doc.OwnerSub == subj.Sub {
		return RoleOwner, true
	}

	groups := make(map[string]struct{}, len(subj.Groups))
	for _, g := range subj.Groups {
		groups[g] = struct{}{}
	}

	best := Role("")
	found := false
	consider := func(r Role) {
		if !r.Valid() {
			return
		}
		if !found || roleRank[r] > roleRank[best] {
			best, found = r, true
		}
	}

	for _, p := range perms {
		switch p.PrincipalKind {
		case PrincipalUser:
			if p.PrincipalID == subj.Sub {
				consider(p.Role)
			}
		case PrincipalGroup:
			if _, ok := groups[p.PrincipalID]; ok {
				consider(p.Role)
			}
		case PrincipalTenant:
			if subj.Tenant != "" && p.PrincipalID == subj.Tenant {
				consider(p.Role)
			}
		default: // PrincipalPublic se resuelve por token de enlace (ResolveShare), no por identidad.
		}
	}
	return best, found
}

// ResolveShare devuelve el rol concedido por un enlace de compartición activo.
// Aplica deny-by-default: enlace inexistente, revocado o caducado => sin acceso.
func ResolveShare(link *ShareLink, nowAfter func(*ShareLink) bool) (Role, bool) {
	if link == nil || !validShareRole(link.Role) || nowAfter == nil || nowAfter(link) {
		return "", false
	}
	return link.Role, true
}

// Los enlaces pueden conceder lectura, comentario o edición, nunca propiedad.
// Se aplica al crear y al resolver, también frente a datos heredados inválidos.
func validShareRole(role Role) bool { return role.Valid() && role != RoleOwner }
