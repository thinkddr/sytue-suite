// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/identity"
)

// ListMembers lista los colaboradores de la organización activa.
func (s *Service) ListMembers(ctx context.Context) ([]domain.Membership, error) {
	return s.store.ListMemberships(ctx)
}

// InviteMemberResult distingue el alta inmediata de alguien que ya pertenecía
// a la organización de Sytue ID y una invitación pendiente. Solo Member crea un
// puesto facturable; Invitation es metadato sin token ni asiento.
type InviteMemberResult struct {
	Member     *domain.Membership       `json:"member,omitempty"`
	Invitation *domain.MemberInvitation `json:"invitation,omitempty"`
}

func (s *Service) ListMemberInvitations(ctx context.Context) ([]domain.MemberInvitation, error) {
	return s.store.ListMemberInvitations(ctx)
}

// InviteMember orquesta una saga pequeña con Sytue ID como autoridad:
//  1. si el correo ya es miembro de la suite, resuelve su sub y lo da de alta ya;
//  2. si no, ID emite/envía el token y Cloud refleja la invitación pendiente.
//
// Si falla el espejo tras enviar, se revoca el token como compensación.
func (s *Service) InviteMember(ctx context.Context, bearer, rawEmail string, role domain.Role, invitedBy string) (InviteMemberResult, error) {
	if s.identity == nil {
		return InviteMemberResult{}, domain.ErrIdentityUnavailable
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(rawEmail))
	if err != nil {
		return InviteMemberResult{}, fmt.Errorf("%w: correo inválido", domain.ErrInvalidInput)
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))
	if role == "" {
		role = domain.RoleMember
	}
	if !domain.ValidRole(role) {
		return InviteMemberResult{}, fmt.Errorf("%w: rol inválido", domain.ErrInvalidInput)
	}

	member, found, err := s.identity.ResolveMember(ctx, bearer, email)
	if err != nil {
		return InviteMemberResult{}, identityDomainError(err)
	}
	if found {
		m, err := s.AddMember(ctx, member.Sub, role)
		if err != nil {
			return InviteMemberResult{}, err
		}
		_ = s.store.DeleteMemberInvitationByEmail(ctx, email)
		return InviteMemberResult{Member: &m}, nil
	}

	expiresAt, err := s.identity.Invite(ctx, bearer, email, string(role))
	var apiErr *identity.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
		// Carrera: se aceptó/creó la membresía entre resolve e invite.
		if current, yes, resolveErr := s.identity.ResolveMember(ctx, bearer, email); resolveErr == nil && yes {
			m, addErr := s.AddMember(ctx, current.Sub, role)
			if addErr != nil {
				return InviteMemberResult{}, addErr
			}
			_ = s.store.DeleteMemberInvitationByEmail(ctx, email)
			return InviteMemberResult{Member: &m}, nil
		}
	}
	if err != nil {
		return InviteMemberResult{}, identityDomainError(err)
	}
	inv, err := s.store.UpsertMemberInvitation(ctx, email, role, invitedBy, expiresAt)
	if err != nil {
		// Best-effort compensatorio: sin espejo Cloud el token no debe quedar vivo.
		_ = s.identity.RevokeInvitation(ctx, bearer, email)
		return InviteMemberResult{}, err
	}
	s.audit(ctx, "member.invite", email, map[string]any{"role": string(role), "expires_at": expiresAt})
	return InviteMemberResult{Invitation: &inv}, nil
}

// RevokeMemberInvitation revoca primero el espejo Cloud y después el token de
// ID. Si ID falla, restaura el espejo para que siga visible y se pueda reintentar;
// la autoaceptación nunca observa una ventana posterior al fallo.
func (s *Service) RevokeMemberInvitation(ctx context.Context, bearer string, id uuid.UUID) error {
	if s.identity == nil {
		return domain.ErrIdentityUnavailable
	}
	inv, err := s.store.DeleteMemberInvitation(ctx, id)
	if err != nil {
		return err
	}
	if err := s.identity.RevokeInvitation(ctx, bearer, inv.Email); err != nil {
		_, _ = s.store.UpsertMemberInvitation(ctx, inv.Email, inv.Role, inv.InvitedBy, inv.ExpiresAt)
		return identityDomainError(err)
	}
	s.audit(ctx, "member.invitation.revoke", inv.Email, nil)
	return nil
}

// AcceptPendingMember materializa el rol de Cloud cuando el invitado ya ha
// aceptado en ID y entra con un JWT cuyo tenant+email+sub fueron verificados.
func (s *Service) AcceptPendingMember(ctx context.Context, sub, email string) (bool, error) {
	if strings.TrimSpace(sub) == "" || strings.TrimSpace(email) == "" {
		return false, nil
	}
	m, accepted, err := s.store.AcceptMemberInvitation(ctx, sub, email)
	if err != nil || !accepted {
		return accepted, err
	}
	s.audit(ctx, "member.invitation.accept", m.Sub, map[string]any{"role": string(m.Role)})
	return true, nil
}

func identityDomainError(err error) error {
	var apiErr *identity.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusBadRequest:
			return domain.ErrInvalidInput
		case http.StatusForbidden, http.StatusUnauthorized:
			return domain.ErrPermission
		case http.StatusConflict:
			return domain.ErrConflict
		}
	}
	return fmt.Errorf("%w: %w", domain.ErrIdentityUnavailable, err)
}

// AddMember invita/actualiza un colaborador por su sub de Sytue ID. El sub es el
// identificador estable; el email (opcional) es informativo para la UI, no se
// persiste (el directorio de usuarios vive en Sytue ID).
func (s *Service) AddMember(ctx context.Context, sub string, role domain.Role) (domain.Membership, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return domain.Membership{}, fmt.Errorf("%w: sub del colaborador requerido", domain.ErrInvalidInput)
	}
	if role == "" {
		role = domain.RoleMember
	}
	if !domain.ValidRole(role) {
		return domain.Membership{}, fmt.Errorf("%w: rol inválido (owner|admin|member|billing)", domain.ErrInvalidInput)
	}
	// La guarda de «último owner» la aplica el store dentro de la misma
	// transacción que escribe (ver store.guardLastOwner): comprobarla aquí, en
	// una consulta aparte, era un TOCTOU —dos degradaciones simultáneas de dos
	// owners distintos veían ambas «hay 2» y dejaban la organización sin ninguno—.
	m, err := s.store.UpsertMembership(ctx, sub, role)
	if err != nil {
		return domain.Membership{}, err
	}
	// Alta/cambio de rol: cambia QUIÉN tiene acceso y, desde que los puestos de
	// suite se derivan de la membresía, también lo que se factura.
	s.audit(ctx, "member.upsert", sub, map[string]any{"role": string(role)})
	return m, nil
}

// RemoveMember elimina un colaborador de la organización activa. Quitar al último
// owner lo rechaza el store (dejaría la org sin ninguno: recuperación solo por BD).
func (s *Service) RemoveMember(ctx context.Context, sub string) error {
	sub = strings.TrimSpace(sub)
	if err := s.store.DeleteMembership(ctx, sub); err != nil {
		return err
	}
	s.audit(ctx, "member.remove", sub, nil)
	return nil
}

// EffectiveRole resuelve el rol EFECTIVO de un usuario en el tenant activo desde
// memberships. fromMembership=false indica que no hay fila explícita: el llamante
// hace fallback al claim del JWT (para no romper al owner inicial antes de que
// existan memberships). Un error de BD se propaga (no se degrada a "sin rol").
func (s *Service) EffectiveRole(ctx context.Context, sub string) (role domain.Role, fromMembership bool, err error) {
	if strings.TrimSpace(sub) == "" {
		return "", false, nil
	}
	r, err := s.store.GetMembershipRole(ctx, sub)
	if errors.Is(err, domain.ErrNotFound) {
		// Sin fila propia, el llamante cae al rol del JWT de la ORGANIZACIÓN. Ese
		// fallback existe SOLO para el arranque —el primer owner entra antes de que haya
		// ninguna membresía—, así que se autoriza únicamente cuando el tenant está VACÍO
		// de membresías, no cuando le falta la fila a este usuario.
		//
		// Con la semántica anterior («este usuario no tiene fila») quitar a alguien de los
		// colaboradores podía SUBIRLE los privilegios: un usuario fijado a member o
		// billing en Cloud pero admin en la organización estaba denegado mientras existía
		// su fila y, al retirársela, heredaba admin/owner —exec de shell en pods de
		// producción, emitir tokens de CI, re-añadirse, restore de servicios,
		// spend-limit y el secreto HMAC del webhook git—. Los vocabularios de rol de
		// Sytue ID y de Cloud son idénticos, así que el claim encajaba sin más.
		n, cErr := s.store.CountMemberships(ctx)
		if cErr != nil {
			return "", false, cErr
		}
		if n == 0 {
			return "", false, nil // arranque real: decide el claim del JWT
		}
		// Tenant ya poblado: no ser colaborador es una respuesta, no una laguna.
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return r, true, nil
}
