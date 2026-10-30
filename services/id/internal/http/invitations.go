// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Invitaciones a una organización. Dos mitades:
//
//   - Alta (admin, API JSON con JWT): POST /api/v1/admin/invitations — un
//     owner/admin de la org activa invita por email con un rol; se emite un token
//     FIRMADO de un solo uso (mismo primitivo y patrón que el reseteo, mig 0011
//     sobre 0009) cuyo hash se guarda, y se envía el enlace de aceptación por
//     email (interfaz Mailer). Autorización fail-closed por tenant: la org sale
//     SIEMPRE del claim (activeOrg), nunca del cuerpo; un admin no puede invitar
//     como owner.
//
//   - Aceptación (pública, server-rendered, sin sesión): GET/POST
//     /accept-invitation — el invitado valida el enlace y fija su contraseña. La
//     fila de la invitación (org + rol) manda: nada del navegador decide a qué
//     organización se entra. El POST verifica Origin (CSRF) y va rate-limitado
//     como el login. Espeja el flujo de /reset-password (forgot_password.go).
package http

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type inviteUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	// MailboxLocal personaliza la dirección de correo del invitado en el dominio de
	// la organización. Se acepta la parte izquierda sola ("soporte") porque en la
	// consola el dominio se enseña al lado y repetirlo sería pedirle al
	// administrador que copie lo que ya tiene delante. Vacío ⇒ nombre.apellidos.
	MailboxLocal string `json:"mailbox_local"`
	// Password da de alta a la persona DIRECTAMENTE con esa contraseña, sin
	// enlace y sin correo. Existe porque el correo se cae —y cuando se cae, un
	// alta que depende de él deja a alguien esperando sin que nadie se entere—.
	// Vacío ⇒ invitación normal, que sigue siendo el camino recomendado: así la
	// contraseña la elige quien la va a usar y no viaja por ningún canal.
	Password string `json:"password"`
}

type invitationEmailRequest struct {
	Email string `json:"email"`
}

// handleAdminResolveMemberByEmail — POST /api/v1/admin/members:resolve —
// resuelve un miembro existente de la org activa por correo. Es POST para que el
// correo (PII) no acabe en URLs ni access logs. Nunca busca fuera de la org.
func (s *Server) handleAdminResolveMemberByEmail(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req invitationEmailRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a valid email address."))
		return
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))
	userID, role, err := s.store.ResolveOrgMemberByEmail(r.Context(), org.ID, email)
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"user_id": userID, "email": email, "role": role,
	})
}

// handleAdminListInvitations — GET /api/v1/admin/invitations — las invitaciones
// PENDIENTES de la org activa: email, rol y cuándo caduca.
//
// Existía la de emitir y la de revocar, pero no la de mirar, así que revocar
// exigía saberse de memoria el correo exacto de alguien a quien invitaste hace
// tres semanas. Sin esta lista, la consola no podía ofrecer el botón: no tenía
// de dónde sacar la fila.
//
// Lo que sale es exactamente «pendiente de aceptar»: `ListPendingInvitations`
// filtra las caducadas y las aceptadas ya no están (ConsumeInvitation las
// borra). La org sale del claim, como en el resto de este fichero, nunca del
// cliente.
func (s *Server) handleAdminListInvitations(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	invs, err := s.store.ListPendingInvitations(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(invs))
	for _, i := range invs {
		items = append(items, map[string]any{
			"email":      i.Email,
			"role":       i.Role,
			"expires_at": i.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handleAdminRevokeInvitation — POST /api/v1/admin/invitations:revoke — revoca
// el token pendiente de un correo en la org activa. Idempotente para que una
// compensación/reintento de Cloud sea segura.
func (s *Server) handleAdminRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req invitationEmailRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a valid email address."))
		return
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))
	revoked, err := s.store.RevokeInvitationByEmail(r.Context(), org.ID, email)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if revoked {
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.member.invitation_revoked", email, r))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"revoked": revoked})
}

// handleAdminInviteUser — POST /api/v1/admin/invitations — invita a un email a la
// organización activa con un rol. requireRole ya exige owner/admin en la org
// activa; aquí se valida el rol, se impide que un admin conceda owner y se rechaza
// si el correo YA es miembro. Un correo que aún no tiene cuenta —o la tiene pero
// no es miembro— recibe el enlace: al aceptarlo se materializa la membresía (y,
// solo si la cuenta es nueva, se fija la contraseña que elija).
func (s *Server) handleAdminInviteUser(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req inviteUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	role := domain.Role(strings.TrimSpace(req.Role))
	if !role.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid role (owner|admin|member|billing)."))
		return
	}
	// Un admin no puede invitar como owner (solo un owner concede owner): evita la
	// auto-escalada. Mismo criterio que handleAddMember.
	if role == domain.RoleOwner && !hasAnyRole(c.Roles, domain.RoleOwner) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can invite someone as owner."))
		return
	}
	addr, perr := mail.ParseAddress(strings.TrimSpace(req.Email))
	if perr != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a valid email address."))
		return
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))

	ctx := r.Context()
	// La dirección elegida se valida AQUÍ y no al aceptar la invitación: quien la
	// escribe es el administrador y está delante de la pantalla, así que un error
	// de tecleo tiene que responderle a él. Descubrirlo días después, cuando el
	// invitado pulsa el enlace, dejaría el fallo sin nadie a quien contárselo.
	buzon := strings.TrimSpace(req.MailboxLocal)
	if buzon != "" {
		mailDomain, derr := s.store.MailDomainForOrg(ctx, nil, org.ID)
		if derr != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(derr))
			return
		}
		if _, verr := domain.NormalizeMailboxAddress(buzon, mailDomain); verr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				"The email address chosen is not valid.",
			))
			return
		}
	}
	// Alta DIRECTA con contraseña: la salida cuando el correo no llega.
	if pwd := req.Password; pwd != "" {
		s.altaConPassword(w, r, org, email, role, buzon, pwd)
		return
	}
	// El alta por invitación tiene UNA sola implementación (inviteToOrg, en
	// onboarding.go): fila con token firmado de un solo uso + envío del enlace.
	// Aquí solo se traducen sus errores a HTTP.
	exp, err := s.inviteToOrg(ctx, org, email, role, c.Subject, buzon, "")
	switch {
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("That email address is already a member of the organisation."))
		return
	case errors.Is(err, errMailNotSent):
		s.log.ErrorContext(ctx, "enviar invitación falló", "error", err, "org", org.Slug)
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The invitation email could not be sent."))
		return
	case err != nil:
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.member.invited", email, r))
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"email": email, "role": role, "expires_at": exp.UTC().Format(time.RFC3339),
	})
}

// handleAcceptInvitationForm — GET /accept-invitation?token= — valida la firma del
// token Y que la invitación siga viva, y pinta el formulario de contraseña; si es
// inválido, manipulado, caducado o revocado, un aviso para pedir una nuevo.
//
// LA CONSULTA A LA BD NO ESTABA, y con revocación en la consola eso se notó: un
// enlace revocado seguía abriendo el formulario y solo fallaba AL ENVIAR la
// contraseña, dejando a quien lo recibió sin saber si la cuenta se había creado.
// Cuesta un EXISTS por visita, y solo de quien ya trae una firma válida: sin
// ella se sale arriba, así que nadie puede provocar consultas desde fuera.
func (s *Server) handleAcceptInvitationForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(inviteInvalidPage(loc)))
		return
	}
	// Un error de BD se trata como «no vale»: preferible mandar a pedir otra
	// invitación que abrir un formulario cuyo envío no se sabe si funcionará.
	if vive, err := s.store.InvitationPending(r.Context(), domain.HashToken(token), time.Now()); err != nil || !vive {
		if err != nil {
			s.log.ErrorContext(r.Context(), "comprobar invitación falló", "error", err)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(inviteInvalidPage(loc)))
		return
	}
	_, _ = w.Write([]byte(loginPage(loc, "Aceptar invitación", inviteFormHTML(html.EscapeString(token), ""))))
}

// handleAcceptInvitationSubmit — POST /accept-invitation — valida token+política,
// consume la invitación (atómico, un solo uso) y materializa la cuenta: crea el
// usuario con la contraseña elegida si el correo aún no existe, o —si ya existe—
// solo lo añade a la organización SIN tocar su contraseña (la cuenta de la suite
// es global; nadie con el enlace puede reescribir la credencial de otra persona).
func (s *Server) handleAcceptInvitationSubmit(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, "Petición rechazada: origen no permitido.")
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, "Formulario inválido.")
		return
	}
	token := r.Form.Get("token")
	safeToken := html.EscapeString(token)
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(inviteInvalidPage(loc)))
		return
	}
	pw := r.Form.Get("new_password")
	if pw != r.Form.Get("confirm_password") {
		inviteFail(w, r, safeToken, "Las contraseñas no coinciden.")
		return
	}
	ctx := r.Context()
	var (
		inv        store.Invitation
		userID     string
		accountNew bool
	)
	// La política se valida DENTRO de la tx contra la política de la org de la
	// invitación (conocida solo tras consumir el token). Si falla, la tx revierte y
	// el token NO se consume ⇒ una contraseña débil no quema el enlace de un solo uso.
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		got, e := s.store.ConsumeInvitation(ctx, q, domain.HashToken(token), time.Now())
		if e != nil {
			return e
		}
		inv = got
		policy, e := s.store.GetOrgPasswordPolicy(ctx, inv.OrgID)
		if e != nil {
			return e
		}
		if e := domain.ValidatePassword(policy, pw); e != nil {
			return e
		}
		hash, e := domain.HashPassword(pw)
		if e != nil {
			return e
		}
		return s.materializeInvite(ctx, q, inv, hash, &userID, &accountNew)
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			inviteFail(w, r, safeToken, passwordErrMsg(err))
		case errors.Is(err, domain.ErrNotFound):
			// Firma válida pero caducó, ya se aceptó o no existe.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(inviteInvalidPage(loc)))
		default:
			httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo aceptar la invitación.")
		}
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(userID, inv.OrgID, "org.member.joined", userID, r))

	orgName := inv.OrgID
	if o, oerr := s.store.GetOrgByID(ctx, inv.OrgID); oerr == nil {
		orgName = o.Name
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := `<p class="sub">Tu cuenta está lista y ya formas parte de ` + html.EscapeString(orgName) + `. ` +
		`Inicia sesión para entrar en Sytue.</p>`
	if !accountNew {
		body = `<p class="sub">Ya tenías una cuenta de Sytue y se ha añadido a ` + html.EscapeString(orgName) + `. ` +
			`Inicia sesión con tu contraseña habitual.</p>`
	}
	_, _ = w.Write([]byte(loginPage(loc, "Invitación aceptada",
		body+`<p class="alt"><a class="btn-link" href="/login/">Iniciar sesión</a></p>`)))
}

// materializeInvite crea o resuelve la cuenta del invitado y le da la membresía
// con el rol de la invitación, dentro de la tx de aceptación. Cuenta nueva ⇒ se
// fija la contraseña elegida (accountNew=true); cuenta existente ⇒ solo membresía
// (la contraseña NO se toca). Cubre además la carrera en la que el correo se crea
// entre la comprobación y el INSERT (unique_violation ⇒ tratar como existente).
func (s *Server) materializeInvite(ctx context.Context, q pg.Querier, inv store.Invitation, passwordHash string, userID *string, accountNew *bool) error {
	user, gerr := s.store.GetUserByEmail(ctx, inv.Email)
	switch {
	case errors.Is(gerr, domain.ErrNotFound):
		nombre := inv.Name
		if nombre == "" {
			nombre = inv.Email
			if i := strings.IndexByte(nombre, '@'); i > 0 {
				nombre = nombre[:i]
			}
		}
		created, cerr := s.store.CreateUser(ctx, q, domain.User{
			Email: inv.Email, EmailVerified: true, Name: nombre,
			PasswordHash: passwordHash, Status: domain.StatusActive,
		})
		if cerr != nil {
			if errors.Is(cerr, domain.ErrConflict) {
				// Carrera: el correo se creó entre la lectura y el INSERT. Resolver y
				// tratar como cuenta existente (no se le fija la contraseña).
				if user, cerr = s.store.GetUserByEmail(ctx, inv.Email); cerr != nil {
					return cerr
				}
				break
			}
			return cerr
		}
		user = created
		*accountNew = true
	case gerr != nil:
		return gerr
	}
	*userID = user.ID
	// La dirección elegida al invitar se reserva ANTES de la membresía porque
	// UpsertMembership asigna la del patrón por defecto a quien no tenga ninguna:
	// llegando con la personalizada ya puesta, esa asignación no hace nada. Evita
	// tener que arrastrar el dato por las ocho vías que crean membresías.
	//
	// Que la dirección elegida esté cogida NO rompe el alta: EnsureOrgMailbox cae
	// al patrón por defecto. Lo que sí se propaga es un fallo real de base, porque
	// esto corre dentro de la transacción de la aceptación y tragárselo solo
	// serviría para que reventara la sentencia siguiente con un error peor.
	if inv.MailboxLocal != "" {
		if _, err := s.store.EnsureOrgMailbox(ctx, q, inv.OrgID, user.ID, inv.MailboxLocal); err != nil {
			return err
		}
	}
	return s.store.UpsertMembership(ctx, q, domain.Membership{
		OrgID: inv.OrgID, UserID: user.ID, Role: domain.Role(inv.Role),
	})
}

// inviteFormHTML es el formulario de contraseña de la aceptación (token en campo
// oculto), con aviso de error opcional. safeToken llega ya escapado.
func inviteFormHTML(safeToken, errMsg string) string {
	b := `<p class="sub">Te han invitado a Sytue. Elige una contraseña para tu cuenta.</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	return b + `<form method="post" action="/accept-invitation">` +
		`<input type="hidden" name="token" value="` + safeToken + `">` +
		`<label for="new_password">Contraseña</label>` +
		`<input id="new_password" type="password" name="new_password" autocomplete="new-password" minlength="8" autofocus required>` +
		`<label for="confirm_password">Repite la contraseña</label>` +
		`<input id="confirm_password" type="password" name="confirm_password" autocomplete="new-password" minlength="8" required>` +
		`<button type="submit">Crear cuenta y entrar</button></form>`
}

// inviteFail re-renderiza el formulario con un error (422), conservando el token.
func inviteFail(w http.ResponseWriter, r *http.Request, safeToken, msg string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_, _ = w.Write([]byte(loginPage(loc, "Aceptar invitación", inviteFormHTML(safeToken, msg))))
}

// inviteInvalidPage es el aviso de enlace de invitación inválido/caducado.
func inviteInvalidPage(loc string) string {
	return loginPage(loc, "Invitación no válida",
		alertHTML("Esta invitación no es válida o ha caducado.")+
			`<p class="alt"><a class="btn-link" href="/login/">Ir a iniciar sesión</a></p>`)
}

// altaConPassword da de alta a alguien SIN pasar por el correo: crea la cuenta
// (o reutiliza la que ya exista), le fija la contraseña que el administrador
// escribe y le da la membresía, todo en una transacción.
//
// Existe porque el correo se cae, y un alta que depende de él deja a alguien
// esperando un enlace que no va a llegar mientras el administrador cree que ya
// está dentro. Sigue siendo el camino SECUNDARIO: por la invitación normal la
// contraseña la elige quien la va a usar y no viaja por ningún canal, así que
// esto es la salida de emergencia, no el atajo por defecto.
//
// La contraseña se valida contra la política de la organización, igual que si
// la eligiera la persona: que la ponga un administrador no la hace mejor. Y no
// se registra en ningún sitio —ni en la auditoría, que solo anota que hubo alta
// directa—, porque un secreto escrito en un registro es un secreto filtrado.
func (s *Server) altaConPassword(w http.ResponseWriter, r *http.Request, org domain.Organization, email string, role domain.Role, buzon, pwd string) {
	ctx := r.Context()
	c, _ := claimsFrom(ctx)

	// La política es la de la organización que da el alta: la del usuario aún no
	// existe porque el usuario tampoco.
	policy, err := s.store.GetOrgPasswordPolicy(ctx, org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if verr := domain.ValidatePassword(policy, pwd); verr != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(verr.Error()))
		return
	}
	hash, herr := domain.HashPassword(pwd)
	if herr != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(herr))
		return
	}

	var userID string
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		if _, e := s.store.GetUserByEmail(ctx, email); e == nil {
			// Si la cuenta global YA existe, no se le puede cambiar la contraseña
			// desde aquí: hacerlo permitiría al administrador de cualquier
			// organización secuestrar una cuenta existente con solo saber su correo.
			// Para incorporar una cuenta existente se usa la invitación sin contraseña.
			return domain.ErrConflict
		} else if !errors.Is(e, domain.ErrNotFound) {
			return e
		}
		// Igual que en SCIM (#679), que un administrador escriba una dirección no
		// demuestra control del buzón en el directorio global: nace con
		// EmailVerified: false para que no cuente como correo verificado en OIDC,
		// en DuenosGlobales ni al reclamar invitaciones de otras organizaciones.
		// Al quedar como miembro de esta organización en la misma transacción,
		// soloCuentasPersonales es false y entra desde el primer momento sin que
		// haga falta mandarle ningún correo de confirmación.
		u, e := s.store.CreateUser(ctx, q, domain.User{
			Email: email, EmailVerified: false, Status: domain.StatusActive,
		})
		if e != nil {
			return e
		}
		userID = u.ID
		if e := s.store.SetPassword(ctx, q, u.ID, hash); e != nil {
			return e
		}
		if e := s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: u.ID, Role: role,
		}); e != nil {
			return e
		}
		// El buzón va DESPUÉS de la membresía y en la misma transacción: depende
		// de pertenecer a la organización, y un alta que deja a alguien dentro
		// pero sin dirección de correo es media alta. Que no haya dominio
		// configurado no puede tumbarla, así que su fallo no se propaga.
		if _, e := s.store.EnsureOrgMailbox(ctx, q, org.ID, u.ID, buzon); e != nil {
			s.log.WarnContext(ctx, "alta directa: sin buzón de organización",
				"user_id", u.ID, "error", e)
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.member.added_with_password", email, r))
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"email": email, "role": role, "user_id": userID,
		// `invited` distingue las dos altas para que la consola no diga «le hemos
		// enviado un correo» cuando no se ha enviado ninguno.
		"invited": false, "created": true,
	})
}
