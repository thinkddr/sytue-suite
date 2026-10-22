// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// PROPIETARIOS Y CUENTAS de cualquier organización, desde la consola de
// plataforma.
//
// Soporte se encontraba con organizaciones sin nadie que las administrara —el
// propietario se fue, o el alta por contrato se hizo sin él— y con cuentas que
// atender —una contraseña perdida, una cuenta comprometida, una baja— sin más
// palanca que pedírselo a un administrador de esa organización, que a veces era
// justo quien faltaba.
//
// Nada de esto inventa mecanismos: cada acción es la que ya hacía la consola de
// la organización, con su misma pieza.
//
//   - Asignar propietario es el cambio de rol de siempre (aplicarRol). Una
//     organización puede tener VARIOS propietarios —el dominio lo modela así:
//     CountOrgOwners, «a un owner solo lo toca otro owner»—, así que asignar uno
//     no degrada a los que ya había. Quitarlos sigue siendo cosa de la consola de
//     la organización.
//   - Crear propietario es una invitación con rol owner (inviteToOrg): la
//     persona elige su contraseña al aceptar. Aquí no se fija ninguna a mano.
//   - Restablecer la contraseña es el enlace de «olvidé mi contraseña»
//     (sendResetLink) o, si el correo no llega, una contraseña TEMPORAL que se
//     enseña una vez y obliga a cambiarla al entrar (mig 0113).
//   - Bloquear es suspender (users.status) y eliminar es la baja con ventana de
//     30 días (CloseUser), la misma que puede deshacer el administrador de la
//     organización.
//
// Las rutas cuelgan de la organización desde la que se actúa: la auditoría dice
// quién, a quién y en qué organización, y un usuario que no es miembro de ella
// da 404. Las que escriben piden step-up (requireStepUp), como el resto de
// acciones sensibles sobre una cuenta.
package http

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// errMotivo: toda escritura de plataforma lleva motivo (ver platform.go).
var errMotivo = httpx.ErrBadRequest.WithMessage("A reason is required.")

// errUnicoPropietario es la respuesta de bloquear o eliminar a quien es el único
// que administra alguna organización. Dice cuáles, porque lo siguiente que hay
// que hacer es ir a esas y nombrar a otro propietario.
func errUnicoPropietario(orgs []string) *httpx.Error {
	return httpx.ErrConflict.WithCode("sole_owner").
		WithMessage("This user is the only owner of {orgs}: assign another owner there first.").
		WithParams(map[string]any{"orgs": strings.Join(orgs, ", ")})
}

// orgDePlataforma resuelve la organización {id} de la ruta. Si no, ya ha
// contestado.
func (s *Server) orgDePlataforma(w http.ResponseWriter, r *http.Request) (domain.Organization, bool) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return domain.Organization{}, false
	}
	org, err := s.store.GetOrgByID(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return domain.Organization{}, false
	}
	return org, true
}

// orgParaPropietario es orgDePlataforma más lo que impide ponerle propietario:
// una fusionada ya no tiene miembros que gestionar —pasaron al destino— y una
// cuenta personal es de una persona sola (ADR 0038).
func (s *Server) orgParaPropietario(w http.ResponseWriter, r *http.Request) (domain.Organization, bool) {
	org, ok := s.orgDePlataforma(w, r)
	if !ok {
		return org, false
	}
	switch {
	case org.Status == domain.Status("merged"):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"That organisation was merged into another one: its members are in the target now."))
		return org, false
	case org.Personal():
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"A personal account belongs to one person and cannot get another owner."))
		return org, false
	}
	return org, true
}

// handlePlatformListMembers — GET /api/v1/platform/orgs/{id}/members — el
// directorio de la organización, el mismo que ve su consola.
func (s *Server) handlePlatformListMembers(w http.ResponseWriter, r *http.Request) {
	org, ok := s.orgDePlataforma(w, r)
	if !ok {
		return
	}
	s.responderUsuariosDeOrg(w, r, org.ID)
}

// handlePlatformAssignOwner — POST /api/v1/platform/orgs/{id}/owners — hace
// propietaria a una cuenta que ya existe. Si no era miembro, entra como
// propietaria; si lo era, sube de rol. Los propietarios que ya hubiera siguen.
func (s *Server) handlePlatformAssignOwner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string `json:"email"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	org, ok := s.orgParaPropietario(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	u, err := s.store.GetUserByEmail(ctx, strings.ToLower(trim(in.Email)))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("no_such_user").WithMessage(
			"There is no user with that email address: invite them as a new owner instead."))
		return
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Un propietario que no puede entrar no administra nada: se desbloquea antes.
	if !u.Active() {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("account_inactive").WithMessage(
			"That account is blocked or closed: unblock it before making it an owner."))
		return
	}
	actual := domain.Role("")
	mem, err := s.store.GetMembership(ctx, org.ID, u.ID)
	switch {
	case err == nil:
		actual = mem.Role
	case !errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if actual != domain.RoleOwner {
		// La misma transacción y los mismos guardas que el cambio de rol de la
		// consola de la organización; subir a owner no dispara ninguno.
		if _, err := s.aplicarRol(ctx, org.ID, u.ID, actual, domain.RoleOwner); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		s.auditPlatform(r, "platform.org.owner_assigned", org.ID, u.Email+": "+in.Reason)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"user_id": u.ID, "email": u.Email, "role": domain.RoleOwner,
		"already_owner": actual == domain.RoleOwner,
	})
}

// handlePlatformInviteOwner — POST /api/v1/platform/orgs/{id}/owner-invitations —
// da de alta a una persona NUEVA como propietaria: la invitación de siempre, con
// rol owner y su nombre. Elige la contraseña al aceptar.
func (s *Server) handlePlatformInviteOwner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string `json:"email"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	if in.Name = trim(in.Name); in.Name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The name is required."))
		return
	}
	addr, err := mail.ParseAddress(trim(in.Email))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a valid email address."))
		return
	}
	email := strings.ToLower(addr.Address)
	org, ok := s.orgParaPropietario(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	// Con cuenta ya, esto no es «crear»: invitarla la dejaría esperando un correo
	// para algo que asignar hace en el acto.
	_, err = s.store.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("account_exists").WithMessage(
			"That email already has a Sytue account: assign it as owner instead."))
		return
	case !errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	exp, err := s.inviteToOrg(ctx, org, email, domain.RoleOwner, platformActor(r), "", in.Name)
	switch {
	case errors.Is(err, errMailNotSent):
		s.log.ErrorContext(ctx, "plataforma: no salió la invitación de propietario", "error", err, "org", org.Slug)
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The invitation email could not be sent."))
		return
	case err != nil:
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	s.auditPlatform(r, "platform.org.owner_invited", org.ID, email+": "+in.Reason)
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"email": email, "name": in.Name, "role": domain.RoleOwner,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}

// miembroDePlataforma resuelve {id} y {userID}, que tiene que ser miembro de
// esa organización. Si no, ya ha contestado.
//
// Dos cuentas no se tocan desde aquí: la propia —para eso está «Mi cuenta»— y la
// de otro administrador de plataforma. Bloquearla o fijarle una contraseña sería
// quitarle el alcance global por la puerta de atrás, sin el aviso a todos los
// administradores que acompaña a retirarlo (avisarCambioDeAlcance).
func (s *Server) miembroDePlataforma(w http.ResponseWriter, r *http.Request) (domain.Organization, domain.User, bool) {
	org, ok := s.orgDePlataforma(w, r)
	if !ok {
		return org, domain.User{}, false
	}
	ctx := r.Context()
	uid := chi.URLParam(r, "userID")
	if _, err := uuid.Parse(uid); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return org, domain.User{}, false
	}
	if _, err := s.store.GetMembership(ctx, org.ID, uid); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return org, domain.User{}, false
	}
	if uid == platformActor(r) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("You cannot do this to your own account; use My account."))
		return org, domain.User{}, false
	}
	u, err := s.store.GetUserByID(ctx, uid)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return org, domain.User{}, false
	}
	esAdmin, err := s.store.IsPlatformAdmin(ctx, uid)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return org, domain.User{}, false
	}
	if esAdmin {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("platform_admin").WithMessage(
			"This account administers the platform: revoke its global scope first."))
		return org, domain.User{}, false
	}
	return org, u, true
}

// contrasenaTemporal genera una contraseña que cumple la política de la persona:
// el alfabeto de las contraseñas de aplicación, que se dicta sin confundir
// letras, y al menos 16 caracteres o los que pida la política.
func contrasenaTemporal(p domain.PasswordPolicy) (string, error) {
	n := max(p.MinLength, 16)
	// Con complejidad exigida, una de cada cien sale sin dígitos: se repite. Ocho
	// intentos fallidos seguidos no son mala suerte, son un fallo.
	for range 8 {
		pwd, err := generarClave(n)
		if err != nil {
			return "", err
		}
		if domain.ValidatePassword(p, pwd) == nil {
			return pwd, nil
		}
	}
	return "", errors.New("id: ninguna contraseña temporal cumplió la política")
}

// handlePlatformResetPassword — POST /api/v1/platform/orgs/{id}/members/{userID}/password.
//
// `mode`:
//   - "link": el correo de «olvidé mi contraseña», a su correo y al de
//     recuperación. La persona elige la nueva.
//   - "temporary": una contraseña que se devuelve UNA vez en esta respuesta, no se
//     guarda en claro ni va a la auditoría, y obliga a cambiarla al entrar.
//
// En los dos se cierran sus sesiones y se revocan sus refresh tokens: quien pide
// esto suele sospechar que alguien más tiene la cuenta.
func (s *Server) handlePlatformResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode   string `json:"mode"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	if in.Mode != "link" && in.Mode != "temporary" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The mode must be link or temporary."))
		return
	}
	org, u, ok := s.miembroDePlataforma(w, r)
	if !ok {
		return
	}
	if u.Status == domain.StatusDeleted {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account has been closed."))
		return
	}
	ctx := r.Context()
	if in.Mode == "link" {
		if u.PasswordHash == "" {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
				"This account does not use a password (sign-in is SSO/MFA only)."))
			return
		}
		if err := s.sendResetLink(ctx, u); err != nil {
			s.log.ErrorContext(ctx, "plataforma: no salió el enlace de restablecimiento", "error", err)
			httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The reset email could not be sent."))
			return
		}
		revocadas, err := s.store.RevokeAllUserSessions(ctx, nil, u.ID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		s.auditPlatform(r, "platform.user.password_reset_link", org.ID, u.Email+": "+in.Reason)
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"id": u.ID, "mode": in.Mode, "sent": true, "sessions_revoked": revocadas,
		})
		return
	}
	policy, err := s.store.EffectivePasswordPolicyForUser(ctx, u.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	pwd, err := contrasenaTemporal(policy)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	hash, err := domain.HashPassword(pwd)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	var revocadas int
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		if e := s.store.SetTemporaryPassword(ctx, q, u.ID, hash); e != nil {
			return e
		}
		var e error
		revocadas, e = s.store.RevokeAllUserSessions(ctx, q, u.ID)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	s.auditPlatform(r, "platform.user.password_temporary", org.ID, u.Email+": "+in.Reason)
	s.avisarSeguridad(ctx, org.ID, u.ID, "security.password.changed", nil)
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": u.ID, "mode": in.Mode, "temporary_password": pwd, "sessions_revoked": revocadas,
	})
}

// handlePlatformBlockUser — POST /api/v1/platform/orgs/{id}/members/{userID}/block.
//
// Bloquear es SUSPENDER: el estado que ya miran el login, el refresh y cada
// access token (cuentaNoActiva), y que arrastra sesiones y refresh tokens
// (revokeIfInactive). Se deshace desbloqueando.
func (s *Server) handlePlatformBlockUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	org, u, ok := s.miembroDePlataforma(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var (
		bloquean  []string
		revocadas int
		cambiada  bool
	)
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if bloquean, e = s.store.OrgsSinOtroAdministradorBloqueando(ctx, q, u.ID); e != nil || len(bloquean) > 0 {
			return e
		}
		// El recuento antes: SetUserStatus ya revoca por dentro.
		if revocadas, e = s.store.CountLiveSessions(ctx, q, u.ID); e != nil {
			return e
		}
		cambiada, e = s.store.SetUserStatus(ctx, q, u.ID, domain.StatusSuspended)
		return e
	})
	switch {
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	case len(bloquean) > 0:
		httpx.Fail(w, r, errUnicoPropietario(bloquean))
		return
	case !cambiada:
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account has been closed."))
		return
	}
	s.auditPlatform(r, "platform.user.blocked", org.ID, u.Email+": "+in.Reason)
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": u.ID, "status": domain.StatusSuspended, "sessions_revoked": revocadas,
	})
}

// handlePlatformUnblockUser — POST /api/v1/platform/orgs/{id}/members/{userID}/unblock.
// No reabre sesiones: la persona vuelve a entrar.
func (s *Server) handlePlatformUnblockUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	org, u, ok := s.miembroDePlataforma(w, r)
	if !ok {
		return
	}
	cambiada, err := s.store.SetUserStatus(r.Context(), nil, u.ID, domain.StatusActive)
	switch {
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	case !cambiada:
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account has been closed."))
		return
	}
	s.auditPlatform(r, "platform.user.unblocked", org.ID, u.Email+": "+in.Reason)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": u.ID, "status": domain.StatusActive})
}

// handlePlatformDeleteUser — POST /api/v1/platform/orgs/{id}/members/{userID}/delete.
//
// La baja de siempre (CloseUser): la cuenta queda `deleted`, sin credenciales
// vivas, y el administrador de cualquiera de sus organizaciones puede
// restaurarla durante retencionBaja. Se pide repetir el correo, como la fusión
// pide repetir el identificador: un id equivocado en una llamada daría de baja a
// otra persona.
func (s *Server) handlePlatformDeleteUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ConfirmEmail string `json:"confirm_email"`
		Reason       string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, errMotivo)
		return
	}
	org, u, ok := s.miembroDePlataforma(w, r)
	if !ok {
		return
	}
	if !strings.EqualFold(trim(in.ConfirmEmail), u.Email) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("confirm_mismatch").WithMessage(
			"Type the account's email address again to confirm."))
		return
	}
	ctx := r.Context()
	var (
		bloquean []string
		cerrada  bool
	)
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if bloquean, e = s.store.OrgsSinOtroAdministradorBloqueando(ctx, q, u.ID); e != nil || len(bloquean) > 0 {
			return e
		}
		cerrada, e = s.store.CloseUser(ctx, q, u.ID, platformActor(r), validation.TruncateRunes(in.Reason, 500))
		return e
	})
	switch {
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	case len(bloquean) > 0:
		httpx.Fail(w, r, errUnicoPropietario(bloquean))
		return
	case !cerrada:
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This account was already closed."))
		return
	}
	s.auditPlatform(r, "platform.user.deleted", org.ID, u.Email+": "+in.Reason)
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": u.ID, "status": domain.StatusDeleted,
		"restorable_until": time.Now().Add(retencionBaja).UTC().Format(time.RFC3339),
	})
}
