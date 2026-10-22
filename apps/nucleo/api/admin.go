// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Admin es el cliente de gobierno de la organización contra Sytue ID
// (`services/id/internal/http`, rutas `/api/v1/admin/*`). Es el servicio del producto
// Sytue Admin, cuya clave técnica sigue siendo `console`.
//
// Lo que hay aquí es lo que el servicio sostiene HOY y se usa a diario: las personas y
// su ciclo de vida, lo que pasa de verdad si se les toca, sus sesiones, los
// dispositivos de la organización, los roles con sus permisos y quién los tiene, las
// aplicaciones conectadas, los dominios de correo y el registro de auditoría. Lo que el
// servicio no tiene no se finge aquí.
type Admin struct{ c *Client }

// NewAdmin crea el cliente sobre la raíz de Sytue ID (ServiceURL(apex, "id")).
func NewAdmin(servicio string, s Session, h *http.Client) *Admin {
	return &Admin{c: New(servicio+"/api/v1", s, h)}
}

// AdminInsights son los cinco números de la portada. Los cuenta el SERVIDOR sobre la
// organización entera: deducirlos de la primera página del directorio daría una
// respuesta inventada en cuanto la organización pasa de cincuenta personas.
type AdminInsights struct {
	SinMFA                int `json:"without_mfa"`
	Suspendidos           int `json:"suspended"`
	SinVerificar          int `json:"unverified_email"`
	InvitacionesCaducadas int `json:"invitations_expired"`
	InvitacionesVivas     int `json:"invitations_pending"`
}

// Insights devuelve qué hay que mirar hoy en esta organización.
func (a *Admin) Insights(ctx context.Context) (AdminInsights, error) {
	return JSON[AdminInsights](ctx, a.c, Request{Path: "/admin/insights"})
}

// AdminPersona es una fila del directorio de la organización.
type AdminPersona struct {
	ID               string    `json:"id"`
	Email            string    `json:"email"`
	CorreoVerificado bool      `json:"email_verified"`
	Nombre           string    `json:"name"`
	MFA              bool      `json:"mfa_enabled"`
	Estado           string    `json:"status"`
	Rol              string    `json:"role"`
	CreadaEn         time.Time `json:"created_at"`
	Departamento     string    `json:"department"`
	Puesto           string    `json:"title"`
	Ubicacion        string    `json:"location"`
}

// AdminPaginaPersonas es una página del directorio, con su cursor opaco.
type AdminPaginaPersonas struct {
	Items []AdminPersona `json:"items"`
	// nextCursor, en camelCase: es como lo escribe el servicio (services/id).
	Siguiente string `json:"nextCursor"` //nolint:tagliatelle // lo emite Sytue ID así
}

// Personas lista el directorio de la organización activa. Va por el método QUERY
// cuando el servicio lo entiende: `q` son nombres y correos de personas concretas, y en
// la URL acabarían en el log de accesos de cada salto y en el Referer.
func (a *Admin) Personas(ctx context.Context, q, cursor string, limite int) (AdminPaginaPersonas, error) {
	consulta := url.Values{"limit": {strconv.Itoa(limite)}}
	if q != "" {
		consulta.Set("q", q)
	}
	if cursor != "" {
		consulta.Set("cursor", cursor)
	}
	return Consulta[AdminPaginaPersonas](ctx, a.c, Request{Path: "/admin/users", Query: consulta})
}

// AdminImpacto es lo que pasa de VERDAD si se toca a esta persona. Lo calcula el
// servidor y es una lectura: informa la decisión, no la autoriza.
type AdminImpacto struct {
	SesionesVivas int  `json:"active_sessions"`
	Grupos        int  `json:"groups"`
	UltimoDueno   bool `json:"last_owner"`
}

// Impacto pregunta el alcance de una acción ANTES de aplicarla.
func (a *Admin) Impacto(ctx context.Context, id string) (AdminImpacto, error) {
	return JSON[AdminImpacto](ctx, a.c, Request{Path: "/admin/users/" + url.PathEscape(id) + "/impact"})
}

// AdminSesion es una sesión viva de una persona.
type AdminSesion struct {
	ID          string    `json:"id"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	Dispositivo string    `json:"device"`
	AMR         []string  `json:"amr"`
	Entrada     time.Time `json:"auth_time"`
	Caduca      time.Time `json:"expires_at"`
}

// Sesiones lista las sesiones SSO vigentes de una persona.
func (a *Admin) Sesiones(ctx context.Context, id string) ([]AdminSesion, error) {
	r, err := JSON[struct {
		Sesiones []AdminSesion `json:"sessions"`
	}](ctx, a.c, Request{Path: "/admin/users/" + url.PathEscape(id) + "/sessions"})
	return r.Sesiones, err
}

// AdminResultado es lo que contestan las acciones sobre una persona.
type AdminResultado struct {
	ID                string `json:"id"`
	Estado            string `json:"status"`
	SesionesRevocadas int    `json:"sessions_revoked"`
}

// Suspender bloquea el acceso y revoca sesiones y refresh en la misma transacción.
func (a *Admin) Suspender(ctx context.Context, id string) (AdminResultado, error) {
	return a.accion(ctx, id, "/suspend")
}

// Reactivar vuelve a dar acceso. No reabre sesiones: la persona entra de nuevo.
func (a *Admin) Reactivar(ctx context.Context, id string) (AdminResultado, error) {
	return a.accion(ctx, id, "/reactivate")
}

// CerrarSesiones fuerza el cierre de TODAS las sesiones sin tocar el estado de la
// cuenta: es lo que se hace con una cuenta que se cree comprometida.
func (a *Admin) CerrarSesiones(ctx context.Context, id string) (AdminResultado, error) {
	return a.accion(ctx, id, "/sessions/revoke")
}

func (a *Admin) accion(ctx context.Context, id, cola string) (AdminResultado, error) {
	return JSON[AdminResultado](ctx, a.c, Request{
		Method: http.MethodPost, Path: "/admin/users/" + url.PathEscape(id) + cola,
	})
}

// AdminRol es un rol personalizado con sus permisos y cuánta gente lo tiene.
type AdminRol struct {
	ID       string   `json:"id"`
	Nombre   string   `json:"name"`
	Permisos []string `json:"permissions"`
	Miembros int      `json:"member_count"`
}

// AdminMiembro es quien tiene un rol.
type AdminMiembro struct {
	ID     string `json:"user_id"`
	Email  string `json:"email"`
	Nombre string `json:"name"`
}

// AdminRolDetalle es el rol con la lista de quién lo tiene: el alcance efectivo de ese
// rol, que es lo que hace falta ver ANTES de tocarle los permisos.
type AdminRolDetalle struct {
	Rol      AdminRol       `json:"role"`
	Miembros []AdminMiembro `json:"members"`
}

// Roles lista los roles personalizados de la organización.
func (a *Admin) Roles(ctx context.Context) ([]AdminRol, error) {
	r, err := JSON[struct {
		Roles []AdminRol `json:"roles"`
	}](ctx, a.c, Request{Path: "/admin/roles"})
	return r.Roles, err
}

// Rol devuelve un rol con quién lo tiene.
func (a *Admin) Rol(ctx context.Context, id string) (AdminRolDetalle, error) {
	return JSON[AdminRolDetalle](ctx, a.c, Request{Path: "/admin/roles/" + url.PathEscape(id)})
}

// AdminDispositivo es un dispositivo registrado de la organización, con su dueño.
type AdminDispositivo struct {
	ID          string `json:"id"`
	Nombre      string `json:"name"`
	UserAgent   string `json:"user_agent"`
	DeConfianza bool   `json:"trusted"`
	VistoPor    string `json:"last_seen"`
	UltimaIP    string `json:"last_ip"`
	PersonaID   string `json:"user_id"`
	Email       string `json:"user_email"`
	Persona     string `json:"user_name"`
}

// Dispositivos lista los dispositivos de la organización, recientes primero.
func (a *Admin) Dispositivos(ctx context.Context) ([]AdminDispositivo, error) {
	r, err := JSON[struct {
		Dispositivos []AdminDispositivo `json:"devices"`
	}](ctx, a.c, Request{Path: "/admin/devices"})
	return r.Dispositivos, err
}

// RevocarDispositivo retira un dispositivo de la organización.
func (a *Admin) RevocarDispositivo(ctx context.Context, id string) error {
	_, err := JSON[struct{}](ctx, a.c, Request{
		Method: http.MethodDelete, Path: "/admin/devices/" + url.PathEscape(id),
	})
	return err
}

// AdminAplicacion es una app de terceros conectada por OIDC a esta organización. El
// secreto NO viaja: el servicio solo guarda su huella y lo enseña una vez al crearlo.
type AdminAplicacion struct {
	ClientID   string   `json:"client_id"`
	Nombre     string   `json:"name"`
	Tipo       string   `json:"type"`
	Redirects  []string `json:"redirect_uris"`
	Scopes     []string `json:"scopes"`
	ConSecreto bool     `json:"has_secret"`
	PKCE       bool     `json:"require_pkce"`
	CreadaEn   string   `json:"created_at"`
}

// Aplicaciones lista las apps conectadas de la organización.
func (a *Admin) Aplicaciones(ctx context.Context) ([]AdminAplicacion, error) {
	r, err := JSON[struct {
		Items []AdminAplicacion `json:"items"`
	}](ctx, a.c, Request{Path: "/admin/apps/oidc-clients"})
	return r.Items, err
}

// AdminDominio es un dominio de correo de la organización, con el registro TXT que hay
// que poner y si el DNS ya lo devuelve.
type AdminDominio struct {
	ID           string `json:"id"`
	Host         string `json:"host"`
	Registro     string `json:"record_name"`
	Valor        string `json:"record_value"`
	Verificado   bool   `json:"verified"`
	Principal    bool   `json:"primary"`
	VerificadoEn string `json:"verified_at,omitempty"`
	Estado       string `json:"status,omitempty"`
}

// AdminDominios son los dominios de la organización y cuál se usa hoy.
type AdminDominios struct {
	Items    []AdminDominio `json:"items"`
	DeCorreo string         `json:"mail_domain"`
	Base     string         `json:"base_domain"`
}

// Dominios lista los dominios de correo. El servicio consulta el DNS de los pendientes
// en cada carga, así que esta llamada tarda más que las demás.
func (a *Admin) Dominios(ctx context.Context) (AdminDominios, error) {
	return JSON[AdminDominios](ctx, a.c, Request{Path: "/admin/mail-domains"})
}

// AdminAuditoria es una línea del registro de auditoría de la organización.
type AdminAuditoria struct {
	ID         string `json:"id"`
	OcurridoEn string `json:"occurred_at"`
	ActorID    string `json:"actor_id"`
	ActorEmail string `json:"actor_email"`
	Accion     string `json:"action"`
	Objetivo   string `json:"target"`
	IP         string `json:"ip"`
}

// AdminFiltroAuditoria acota el registro. `Accion` es un PREFIJO («user.»), `Actor` un
// uuid, y las fechas van en RFC3339 o AAAA-MM-DD.
type AdminFiltroAuditoria struct {
	Accion string
	Actor  string
	Desde  string
	Hasta  string
}

func (f AdminFiltroAuditoria) valores() url.Values {
	q := url.Values{}
	for clave, valor := range map[string]string{
		"action": f.Accion, "actor": f.Actor, "since": f.Desde, "until": f.Hasta,
	} {
		if valor != "" {
			q.Set(clave, valor)
		}
	}
	return q
}

// AdminPaginaAuditoria es una página del registro, en keyset descendente.
type AdminPaginaAuditoria struct {
	Items     []AdminAuditoria `json:"items"`
	Siguiente string           `json:"nextCursor"` //nolint:tagliatelle // lo emite Sytue ID así
}

// Auditoria devuelve una página del registro con los filtros aplicados.
func (a *Admin) Auditoria(ctx context.Context, f AdminFiltroAuditoria, cursor string, limite int) (AdminPaginaAuditoria, error) {
	q := f.valores()
	q.Set("limit", strconv.Itoa(limite))
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return JSON[AdminPaginaAuditoria](ctx, a.c, Request{Path: "/admin/audit", Query: q})
}

// ExportarAuditoria descarga el registro filtrado COMPLETO en CSV, tal y como lo emite
// el servicio en streaming. Quien llama cierra el lector.
func (a *Admin) ExportarAuditoria(ctx context.Context, f AdminFiltroAuditoria) (io.ReadCloser, error) {
	resp, err := a.c.Do(ctx, Request{Path: "/admin/audit/export.csv", Query: f.valores()})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
