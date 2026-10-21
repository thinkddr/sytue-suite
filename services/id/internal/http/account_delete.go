// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// «BORRAR MI CUENTA» (ADR 0054): la baja que pide la propia persona.
//
// Es una página de Go y no de la app «Mi cuenta» por la misma razón que el resto
// de formularios de `/account/*`: la reautenticación la hace el IdP con su
// cookie, que la app no ve. Y tiene una URL estable y pública —Google Play la
// pide para enseñarla en la ficha de las trece apps—: sin sesión manda a
// iniciarla y vuelve aquí.
//
// Lo que decide esta pantalla, en orden:
//
//  1. Si la persona es la única que administra alguna organización, NO se
//     puede: se dice cuáles y qué hacer. La cuenta personal no cuenta.
//  2. Tiene que escribir su cuenta (@usuario o correo) y demostrar que es ella
//     AHORA: con su contraseña, o con una identificación de hace menos de cinco
//     minutos (frescuraStepUp). Quien tiene segundo factor no puede saltárselo
//     con la contraseña sola: tiene que volver a entrar con los dos, o con una
//     passkey.
//  3. Al confirmar, en una transacción: baja con ventana de 30 días y borrado de
//     todo lo que sirve para entrar (store.BorrarCuentaPropia). Después se
//     cierra la sesión en todas las apps con el mismo barrido del logout.
//
// Lo que otros servicios guardan de la persona y la anonimización de la ficha
// los hace BarrerCuentasBorradas, en segundo plano.
package http

import (
	"context"
	"errors"
	"html"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// rutaBorrarCuenta es la URL que publica Google Play. No se cambia.
const rutaBorrarCuenta = "/account/delete"

// identificadorCuenta es lo que la persona tiene que escribir para confirmar:
// su @usuario si lo tiene, y si no su correo.
func identificadorCuenta(u domain.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return u.Email
}

// coincideCuenta compara lo escrito con la cuenta sin exigir la arroba ni las
// mayúsculas: lo que se comprueba es la intención, no la mecanografía.
func coincideCuenta(escrito string, u domain.User) bool {
	e := strings.ToLower(strings.TrimSpace(escrito))
	if e == "" {
		return false
	}
	if u.Username != "" && strings.TrimPrefix(e, "@") == strings.ToLower(u.Username) {
		return true
	}
	return strings.EqualFold(e, u.Email)
}

// identificacionReciente dice si la sesión vale por sí sola como
// reautenticación: se abrió hace menos de frescuraStepUp y, si la cuenta tiene
// segundo factor, con él.
//
// El segundo factor tiene que ser uno que VERIFICA a la persona: TOTP (`mfa`/`otp`)
// o una passkey con verificación de usuario, que el login marca también con `mfa`.
// Una llave sin PIN (`webauthn` a secas) solo prueba que se tiene la llave, y
// borrar la cuenta no se deshace.
func identificacionReciente(sess domain.Session, u domain.User) bool {
	fresca := !sess.AuthTime.IsZero() && time.Since(sess.AuthTime) <= frescuraStepUp
	return fresca && (!u.MFAEnabled || slices.Contains(sess.AMR, "mfa") || slices.Contains(sess.AMR, "otp"))
}

// tieneCuentaPersonal dice si alguna de las membresías es la cuenta personal de
// la persona: su contenido se va con ella y la página lo tiene que decir.
func (s *Server) tieneCuentaPersonal(ctx context.Context, mems []domain.Membership) bool {
	for _, m := range mems {
		if m.Role != domain.RoleOwner {
			continue
		}
		if o, err := s.store.GetOrgByID(ctx, m.OrgID); err == nil && o.Personal() {
			return true
		}
	}
	return false
}

// handleAccountDeleteGet — GET /account/delete.
func (s *Server) handleAccountDeleteGet(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	sess, ok := s.currentSession(r)
	if !ok {
		// Vuelta del borrado: ya no hay sesión, y lo que se enseña no depende de
		// quién era. Cualquier otra visita sin sesión va a identificarse.
		if r.URL.Query().Get("hecho") == "1" {
			escribirPagina(w, http.StatusOK, loginPage(loc, i18n.T(loc, "delete.done_title"),
				`<p class="sub">`+html.EscapeString(i18n.T(loc, "delete.done"))+`</p>`+
					`<p class="alt"><a class="btn-link" href="`+html.EscapeString(s.suiteHomeURL())+`">`+
					html.EscapeString(i18n.T(loc, "account.back_to_suite"))+`</a></p>`))
			return
		}
		http.Redirect(w, r, withReturnTo("/login/", rutaBorrarCuenta), http.StatusFound)
		return
	}
	s.pintarBorrarCuenta(w, r, sess, http.StatusOK, "")
}

// pintarBorrarCuenta dibuja la página con un error opcional.
func (s *Server) pintarBorrarCuenta(w http.ResponseWriter, r *http.Request, sess domain.Session, status int, errMsg string) {
	loc := i18n.From(r)
	ctx := r.Context()
	esc := html.EscapeString
	user, err := s.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(loc, "account.load_failed"))
		return
	}
	bloquean, err := s.store.OrgsSinOtroAdministrador(ctx, nil, user.ID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(loc, "account.load_failed"))
		return
	}
	mems, _ := s.store.ListMembershipsForUser(ctx, user.ID)
	ident := identificadorCuenta(user)

	var b strings.Builder
	b.WriteString(`<p class="sub">` + esc(strings.ReplaceAll(i18n.T(loc, "delete.intro"), "{account}", ident)) + `</p>`)
	if errMsg != "" {
		b.WriteString(alertHTML(errMsg))
	}
	parrafo := func(titulo, texto string) {
		b.WriteString(`<h2 class="fp">` + esc(titulo) + `</h2><p class="fp">` + esc(texto) + `</p>`)
	}
	parrafo(i18n.T(loc, "delete.goes_title"), i18n.T(loc, "delete.goes"))
	parrafo(i18n.T(loc, "delete.stays_title"), i18n.T(loc, "delete.stays"))
	if s.tieneCuentaPersonal(ctx, mems) {
		b.WriteString(`<p class="fp">` + esc(i18n.T(loc, "delete.personal")) + `</p>`)
	}
	b.WriteString(`<p class="fp">` + esc(i18n.T(loc, "delete.grace")) + `</p>`)

	volver := `<p class="alt"><a class="btn-link" href="/account">` + esc(i18n.T(loc, "common.cancel")) + `</a></p>`
	if len(bloquean) > 0 {
		b.WriteString(alertHTML(i18n.T(loc, "delete.blocked")) + `<ul>`)
		for _, n := range bloquean {
			b.WriteString(`<li>` + esc(n) + `</li>`)
		}
		b.WriteString(`</ul>` + volver)
		escribirPagina(w, status, loginPage(loc, i18n.T(loc, "delete.title"), b.String()))
		return
	}

	// Cómo se demuestra que es ella. Con una identificación reciente no se pide
	// nada más; sin ella, la contraseña, salvo que la cuenta tenga segundo factor
	// (o no tenga contraseña): entonces solo vale volver a entrar.
	reciente := identificacionReciente(sess, user)
	conContrasena := !reciente && !user.MFAEnabled && user.PasswordHash != ""
	reauth := `<p class="alt"><a class="btn-link" href="` + esc(withReturnTo("/login/", rutaBorrarCuenta)) + `">` +
		esc(i18n.T(loc, "delete.reauth_link")) + `</a></p>`

	if !reciente && !conContrasena {
		b.WriteString(`<p class="fp">` + esc(i18n.T(loc, "delete.reauth")) + `</p>` + reauth + volver)
		escribirPagina(w, status, loginPage(loc, i18n.T(loc, "delete.title"), b.String()))
		return
	}
	b.WriteString(`<form method="post" action="` + rutaBorrarCuenta + `">` +
		`<label for="confirm">` + esc(i18n.T(loc, "delete.confirm")) + ` <strong>` + esc(ident) + `</strong></label>` +
		`<input id="confirm" type="text" name="confirm" autocomplete="off" spellcheck="false" autocapitalize="none" required>`)
	if conContrasena {
		b.WriteString(`<label for="password">` + esc(i18n.T(loc, "common.password")) + `</label>` +
			`<input id="password" type="password" name="password" autocomplete="current-password" required>`)
	} else {
		b.WriteString(`<p class="fp hint">` + esc(i18n.T(loc, "delete.fresh")) + `</p>`)
	}
	b.WriteString(`<button type="submit">` + esc(i18n.T(loc, "delete.submit")) + `</button></form>`)
	if conContrasena {
		b.WriteString(reauth)
	}
	b.WriteString(volver)
	escribirPagina(w, status, loginPage(loc, i18n.T(loc, "delete.title"), b.String()))
}

// escribirPagina responde HTML sin caché: la página lleva datos de la cuenta.
func escribirPagina(w http.ResponseWriter, status int, cuerpo string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(cuerpo))
}

// handleAccountDeletePost — POST /account/delete.
func (s *Server) handleAccountDeletePost(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, withReturnTo("/login/", rutaBorrarCuenta), http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(loc, "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, i18n.T(loc, "common.bad_form"))
		return
	}
	ctx := r.Context()
	user, err := s.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(loc, "account.load_failed"))
		return
	}
	if !coincideCuenta(r.Form.Get("confirm"), user) {
		s.pintarBorrarCuenta(w, r, sess, http.StatusUnprocessableEntity, i18n.T(loc, "delete.err_confirm"))
		return
	}
	if !identificacionReciente(sess, user) {
		// La contraseña solo vale sin segundo factor: con él, bastaría robar la
		// contraseña para borrar una cuenta que para entrar pide los dos.
		if user.MFAEnabled || user.PasswordHash == "" {
			s.pintarBorrarCuenta(w, r, sess, http.StatusUnauthorized, i18n.T(loc, "delete.reauth"))
			return
		}
		// El mismo cupo por cuenta que el login: con una cookie robada, esta página
		// no puede servir para probar contraseñas desde muchas IP.
		if s.acctLimiter != nil {
			if res, lerr := s.acctLimiter.Allow(ctx, "login:"+user.Email); lerr == nil && !res.Allowed {
				_ = s.store.Audit(ctx, nil, auditEntry(user.ID, sess.OrgID, "auth.login.locked", "", r))
				s.pintarBorrarCuenta(w, r, sess, http.StatusTooManyRequests, i18n.T(loc, msgTooManyTries))
				return
			}
		}
		if ok, verr := domain.VerifyPassword(r.Form.Get("password"), user.PasswordHash); verr != nil || !ok {
			_ = s.store.Audit(ctx, nil, auditEntry(user.ID, sess.OrgID, "user.self_delete.bad_password", "", r))
			s.pintarBorrarCuenta(w, r, sess, http.StatusUnauthorized, i18n.T(loc, "delete.err_password"))
			return
		}
	}

	// Las membresías se leen ANTES: después de confirmar siguen ahí —se van en la
	// purga—, pero así el orden no depende de eso.
	mems, _ := s.store.ListMembershipsForUser(ctx, user.ID)
	var borrada bool
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		borrada, e = s.store.BorrarCuentaPropia(ctx, q, user.ID)
		return e
	})
	switch {
	case errors.Is(err, store.ErrUnicoAdministrador):
		s.pintarBorrarCuenta(w, r, sess, http.StatusConflict, "")
		return
	case err != nil:
		s.log.ErrorContext(ctx, "borrar mi cuenta", "user_id", user.ID, "error", err)
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(loc, "account.load_failed"))
		return
	}
	if borrada {
		_ = s.store.Audit(ctx, nil, auditEntry(user.ID, sess.OrgID, "user.self_deleted", user.ID, r))
		// La foto ya no existe en Sytue ID; esto la quita también de las fichas de
		// Contactos de cada organización. Sin esperar: la respuesta no depende de ello.
		sinCancelar := context.WithoutCancel(ctx)
		go func() {
			for _, m := range mems {
				if o, oerr := s.store.GetOrgByID(sinCancelar, m.OrgID); oerr == nil {
					s.propagateAvatarToContactsIn(sinCancelar, o.Slug, user.ID, "")
				}
			}
		}()
	}

	// Sin sesión en ninguna app: el mismo barrido que el logout, y de vuelta aquí.
	s.clearSessionCookie(w)
	destino := rutaBorrarCuenta + "?hecho=1"
	barrido := s.suiteLogoutURIs(ctx)
	if len(barrido) == 0 {
		http.Redirect(w, r, destino, http.StatusSeeOther)
		return
	}
	escribirPagina(w, http.StatusOK, logoutSweepPage(barrido, destino))
}

// Cada cuánto y cuántas bajas propias atiende el barrido en una pasada.
const (
	BarridoCuentasBorradas = 10 * time.Minute
	loteCuentasBorradas    = 50
)

// BarrerCuentasBorradas termina las bajas propias pendientes: borra lo que los
// otros servicios guardan de cada persona y, vencida la ventana, anonimiza su
// ficha. Idempotente y seguro con varias réplicas a la vez: cada paso se
// comprueba en su WHERE.
func (s *Server) BarrerCuentasBorradas(ctx context.Context) error {
	antes := time.Now().Add(-retencionBaja)
	// Un minuto más que la vida del access token, por los relojes.
	borrarDesde := time.Now().Add(-s.cfg.AccessTokenTTL - time.Minute)
	pend, err := s.store.CuentasPropiasPendientes(ctx, borrarDesde, antes, loteCuentasBorradas)
	if err != nil || len(pend) == 0 {
		return err
	}
	pools := map[string]*pg.Pool{}
	defer func() {
		for _, p := range pools {
			p.Close()
		}
	}()
	for _, c := range pend {
		if !c.DatosBorrados {
			if err := s.borrarDatosPersonales(ctx, pools, c.ID); err != nil {
				// Error y no aviso: mientras falle, ni se borran esos datos ni se
				// purga la ficha, y el plazo de un mes del RGPD sigue corriendo.
				s.log.ErrorContext(ctx, "borrar mi cuenta: no se pudieron borrar los datos en otros servicios", "user_id", c.ID, "error", err)
				continue
			}
			if err := s.store.MarcarDatosPersonalesBorrados(ctx, c.ID); err != nil {
				return err
			}
		}
		if !c.ClosedAt.Before(antes) {
			continue
		}
		personales, err := s.store.PurgarCuentaPropia(ctx, c.ID, antes)
		if err != nil {
			return err
		}
		if personales == nil {
			continue
		}
		_ = s.store.Audit(ctx, nil, store.AuditEntry{
			Action: "user.self_deleted.purged", Target: c.ID,
			Detail: map[string]any{"at": time.Now().UTC().Format(time.RFC3339)},
		})
		// El contenido de una cuenta personal (correo, archivos, fotos...) vive en
		// diez bases y en el almacén de objetos, y su borrado todavía es manual
		// (docs/security.md §11.4). Nivel error a propósito: es lo que dispara la
		// alerta, y el plazo de un mes del RGPD corre desde ahora.
		for _, org := range personales {
			s.log.ErrorContext(ctx, "borrar mi cuenta: el contenido de la cuenta personal espera su borrado manual",
				"org_id", org, "user_id", c.ID)
		}
	}
	return nil
}

// borrarDatosPersonales borra en cada servicio de merge.DatosDeLaPersona las filas
// de esta persona. Las conexiones se abren una vez por pasada.
func (s *Server) borrarDatosPersonales(ctx context.Context, pools map[string]*pg.Pool, userID string) error {
	for _, d := range merge.DatosDeLaPersona {
		pool := pools[d.Servicio]
		if pool == nil {
			dsn, err := merge.DSNForDatabase(s.cfg.DatabaseDSN, d.Servicio)
			if err != nil {
				return err
			}
			if pool, err = pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1}); err != nil {
				return err
			}
			pools[d.Servicio] = pool
		}
		if _, err := merge.BorrarPersona(ctx, pool, d.Tablas, userID, s.documentCallRevoker()); err != nil {
			return err
		}
	}
	return nil
}
