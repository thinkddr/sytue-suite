// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Tokens de API (credenciales programáticas para CI/CLI)
// ---------------------------------------------------------------------------

// TokenScope es el alcance de una credencial de API. Es una escalera monótona
// (read < write < deploy): un token de mayor alcance puede lo del inferior. Los
// tokens autorizan por scope; los humanos autorizan por rol de membership.
type TokenScope string

const (
	// ScopeRead solo permite operaciones de lectura (métodos seguros GET/HEAD).
	ScopeRead TokenScope = "read"
	// ScopeWrite añade mutaciones de configuración (proyectos, env, dominios…).
	ScopeWrite TokenScope = "write"
	// ScopeDeploy añade disparar despliegues y rollbacks (el más privilegiado).
	ScopeDeploy TokenScope = "deploy"
	// ScopeHook es el de una URL de despliegue (/hooks/deploy/{token}). Fuera de
	// esa ruta no vale para nada: la URL se pega en un CMS ajeno y pasa por los
	// logs del borde, y filtrarla no puede dar control sobre lo que se ejecuta
	// (CLOUD-17). No está en la escalera: AtLeast(read) es falso.
	ScopeHook TokenScope = "hook"
)

// scopeRank ordena los scopes; el cero-valor (no válido) queda por debajo de read.
func scopeRank(s TokenScope) int {
	switch s {
	case ScopeRead:
		return 1
	case ScopeWrite:
		return 2
	case ScopeDeploy:
		return 3
	default:
		return 0
	}
}

// ValidTokenScope indica si s es un scope conocido.
func ValidTokenScope(s TokenScope) bool { return scopeRank(s) > 0 || s == ScopeHook }

// AtLeast indica si el scope alcanza (>=) el mínimo requerido en la escalera.
func (s TokenScope) AtLeast(minScope TokenScope) bool { return scopeRank(s) >= scopeRank(minScope) }

// APIToken es una credencial programática del tenant. El secreto en claro solo
// existe al crearla (Plaintext, no persistido); en reposo se guarda su hash.
type APIToken struct {
	ID     uuid.UUID  `json:"id"`
	Tenant string     `json:"-"`
	Name   string     `json:"name"`
	Scope  TokenScope `json:"scope"`
	// ProjectID confina el token a un proyecto: si != nil, el token solo puede
	// operar bajo /api/v1/projects/{ProjectID}/* (deploy-key de ESE proyecto). nil
	// = alcance de organización (tenant-wide), el comportamiento por defecto.
	ProjectID  *uuid.UUID `json:"project_id,omitempty"`
	CreatedBy  string     `json:"created_by,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Plaintext es el token en claro; solo se rellena en la respuesta de creación
	// (se muestra una vez) y nunca se persiste ni se relee.
	Plaintext string `json:"token,omitempty"`
}

// ---------------------------------------------------------------------------
// Membership / RBAC de la organización
// ---------------------------------------------------------------------------

// Role es el rol de un colaborador dentro de la organización (tenant).
type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleMember  Role = "member"
	RoleBilling Role = "billing"
)

// ValidRole indica si r es un rol conocido.
func ValidRole(r Role) bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleBilling:
		return true
	default:
		return false
	}
}

// Membership asocia un usuario (sub de Sytue ID) a un tenant con un rol.
type Membership struct {
	Tenant    string    `json:"-"`
	Sub       string    `json:"sub"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// MemberInvitation es el espejo NO sensible de una invitación emitida por
// Sytue ID. El token nunca entra en Cloud: aquí solo vive el correo normalizado,
// el rol que se aplicará al `sub` autenticado y su caducidad.
type MemberInvitation struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	InvitedBy string    `json:"invited_by,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
