// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Endpoint INTERNO service-to-service: POST /api/v1/internal/app-passwords/verify
//
// Lo consulta Sytue Mail en cada AUTH de IMAP/SMTP. Las credenciales viven aquí
// —son de la identidad, no del correo— y el buzón no puede tener una copia: eso
// sería un segundo sitio del que filtrarlas y un segundo sitio que revocar.
//
// No lleva JWT de usuario: el que autentica es un SERVICIO, y quien pregunta lo
// hace justamente porque todavía no hay identidad. La autorización es el secreto
// compartido en `Authorization: Bearer`, comparado en tiempo constante, igual que
// el `/internal/channel-access` que Drive consulta a Rooms.
//
// Responde 200 con la identidad si la contraseña es válida y 401 si no. NUNCA
// distingue «no existe esa cuenta» de «la contraseña no vale»: son la misma
// respuesta, o el endpoint se convierte en un enumerador de direcciones.

type verifyAppPasswordReq struct {
	// Identity es lo que el cliente de correo escribió como usuario. Puede ser el
	// correo de registro o el buzón de la organización: la comprobación se hace
	// contra las dos, como en el token (ver idauth del servicio de correo).
	Identity string `json:"identity"`
	Password string `json:"password"`
}

type verifyAppPasswordResp struct {
	Subject string `json:"subject"`
	OrgID   string `json:"org_id"`
	Tenant  string `json:"tenant"`
	Email   string `json:"email"`
	Mailbox string `json:"mailbox,omitempty"`
}

// internalSecretAuth exige el secreto compartido en Authorization: Bearer, en
// tiempo constante, y rechaza con 404 si llega por el Ingress (X-Forwarded-For).
// Un secreto vacío rechaza siempre: sin él la ruta no se monta, pero la defensa
// en profundidad no cuesta nada.
func (s *Server) internalSecretAuth(next http.Handler) http.Handler {
	return httpx.SoloDesdeDentro(s.cfg.InternalSecret)(next)
}

// handleInternalVerifyAppPassword valida una contraseña de aplicación y devuelve
// a quién pertenece.
func (s *Server) handleInternalVerifyAppPassword(w http.ResponseWriter, r *http.Request) {
	d, err := s.duenoDeAppPassword(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, verifyAppPasswordResp{
		Subject: d.user.ID,
		OrgID:   d.org.ID,
		Tenant:  d.org.Slug,
		Email:   d.user.Email,
		Mailbox: d.buzon,
	})
}

// dueno es a quién pertenece una contraseña de aplicación y en qué organización entra.
type dueno struct {
	user  domain.User
	org   domain.Organization
	buzon string
}

// appPasswordFallida descuenta el presupuesto por cuenta (5/min, ráfaga 5) solo
// cuando la contraseña de aplicación falla, igual que credencialFallida en
// login.go, para no bloquear con 429 a clientes legítimos de correo o WebDAV.
func (s *Server) appPasswordFallida(ctx context.Context, identity string, invalida error) (dueno, error) {
	if idCuenta := strings.ToLower(strings.TrimSpace(identity)); idCuenta != "" && s.acctLimiter != nil {
		if res, lerr := s.acctLimiter.Allow(ctx, "app-pwd:"+idCuenta); lerr == nil && !res.Allowed {
			return dueno{}, httpx.ErrTooManyRequests
		}
	}
	return dueno{}, invalida
}

// duenoDeAppPassword lee {identity, password} del cuerpo y resuelve su dueño. Todo
// rechazo es el MISMO 401, sea la cuenta que no existe o la contraseña que no vale.
func (s *Server) duenoDeAppPassword(w http.ResponseWriter, r *http.Request) (dueno, error) {
	var req verifyAppPasswordReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		return dueno{}, httpx.ErrBadRequest.WithMessage("Invalid body.")
	}
	invalida := httpx.ErrUnauthorized.WithMessage("Invalid credentials.")
	pass := NormalizeAppPassword(req.Password)
	if pass == "" || s.store == nil {
		return s.appPasswordFallida(r.Context(), req.Identity, invalida)
	}

	// La búsqueda es por el HASH, que es único: la contraseña identifica por sí
	// sola a su dueño. La identidad presentada solo se contrasta después.
	user, org, err := s.store.UserByAppPassword(r.Context(), domain.HashToken(pass))
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return dueno{}, err
		}
		return s.appPasswordFallida(r.Context(), req.Identity, invalida)
	}
	// Una cuenta suspendida, dada de baja o borrada por su titular no entra por
	// IMAP/SMTP aunque conserve una contraseña de aplicación: la que se creara con
	// un access token aún vivo tras la baja abría el buzón hasta la purga.
	if !user.Active() {
		return s.appPasswordFallida(r.Context(), req.Identity, invalida)
	}

	// La credencial ya viene ligada a su organización. El store ha comprobado la
	// membresía actual; nunca se elige otra aunque la original desaparezca.

	// El buzón de la organización, con la MISMA llamada que usa la emisión del
	// token (`userClaims`): idempotente y, en el caso normal, una lectura por
	// índice. Si falla, se sigue con el correo de registro en vez de tumbar la
	// autenticación — igual que allí, donde es best-effort.
	buzon, _ := s.store.EnsureOrgMailbox(r.Context(), nil, org.ID, user.ID, "")

	// Anti-spoofing: quien escribe una dirección en Thunderbird tiene que ser el
	// dueño de la contraseña. Sin esto, una contraseña válida serviría para
	// presentarse como cualquiera, que es exactamente lo que el MSA impide en su
	// MAIL FROM y no puede volver a comprobar por su cuenta.
	if id := strings.TrimSpace(req.Identity); id != "" &&
		!strings.EqualFold(id, user.Email) && !strings.EqualFold(id, buzon) {
		return s.appPasswordFallida(r.Context(), req.Identity, invalida)
	}
	return dueno{user: user, org: org, buzon: buzon}, nil
}

// audienciaAppPasswordToken es la del token que se canjea por una contraseña de
// aplicación: el de Drive, que es quien lo pide para su WebDAV.
const audienciaAppPasswordToken = "sytue-drive" //nolint:gosec // G101: es una audiencia, no una credencial

// handleInternalAppPasswordToken — POST /internal/app-passwords/token — canjea una
// contraseña de aplicación por un access token de la persona, con la MISMA identidad
// que el de su sesión (`userClaims`: roles, grupos, cuota).
//
// Lo pide Sytue Drive cuando un cliente WebDAV —el gestor de ficheros de Linux, el
// proveedor de documentos de Android— entra con Basic. Drive ya autoriza por token y
// la cuota viaja en él: canjearla aquí evita una segunda forma de construir la
// identidad allí. El token no sale de Drive; vale lo de siempre y Drive lo guarda en
// memoria hasta que caduca, así que esto no se pide en cada petición.
func (s *Server) handleInternalAppPasswordToken(w http.ResponseWriter, r *http.Request) {
	d, err := s.duenoDeAppPassword(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now()
	claims := s.userClaims(r.Context(), domain.Client{ID: audienciaAppPasswordToken}, d.user, d.org.ID, "openid drive", now)
	at, err := s.issuer.IssueAccessToken(claims, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	writeTokens(w, r, &tokenResponse{
		AccessToken: at,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.issuer.AccessTokenTTL().Seconds()),
	})
}
