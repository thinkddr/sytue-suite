// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
)

// Denuncia y bloqueo (docs/decisions.md, «Cast: denuncia y bloqueo»). Quién puede
// qué lo decide `social`; aquí van las rutas, el reto ALTCHA de la denuncia sin
// sesión y su tope por IP.

const (
	// ambitoDenuncia ata el reto ALTCHA a las denuncias: uno resuelto para otra
	// cosa no vale aquí.
	ambitoDenuncia = "cast:reports"
	// dificultadDenuncia es el número máximo de la prueba de trabajo (décimas de
	// segundo en un móvil), como en los formularios públicos de Bases.
	dificultadDenuncia = 100_000
	vidaRetoDenuncia   = 10 * time.Minute
)

// montarModeracion: con sesión, bajo /api/v1.
func montarModeracion(api chi.Router, h *Handlers) {
	if h.social == nil {
		return
	}
	api.Post("/videos/{id}/reports", h.Denunciar)
	api.Post("/channels/{org}/reports", h.DenunciarCanal)
	api.Post("/videos/{id}/blocks", h.Bloquear)
	api.Get("/blocks", h.Bloqueados)
	api.Delete("/blocks/{sub}", h.Desbloquear)
	// La cola y los bloqueos de la organización: solo quien la administra.
	api.Get("/moderation/summary", h.ResumenModeracion)
	api.Get("/moderation/reports", h.Denuncias)
	api.Post("/moderation/reports/{rid}", h.ResolverDenuncia)
	api.Get("/moderation/blocks", h.BloqueadosEnOrg)
	api.Delete("/moderation/blocks/{sub}", h.DesbloquearEnOrg)
	api.Get("/moderation/audit", h.AuditoriaModeracion)
	// La cola del OPERADOR de la plataforma: todas las organizaciones. Quién pasa
	// lo decide Sytue ID en cada petición (operadorDe).
	api.Get("/platform/moderation/reports", h.DenunciasOperador)
	api.Post("/platform/moderation/reports/{rid}", h.ResolverDenunciaOperador)
}

// montarModeracionPublica: la denuncia sin sesión, bajo /public y /api/v1/public.
// Encima del tope público por IP lleva el suyo, mucho más estrecho y compartido
// entre réplicas, y la prueba de trabajo.
func montarModeracionPublica(publico chi.Router, h *Handlers, db pg.Querier) {
	if h.social == nil || h.altcha == nil {
		return
	}
	// Diez de golpe y una cada treinta segundos por IP.
	tope := httpx.RateLimit(httpx.RateLimitConfig{
		Limiter: pg.NewRateLimiter(db, pg.RateLimiterConfig{PerSecond: 1.0 / 30, Burst: 10}),
		KeyFunc: func(r *http.Request) string { return "cast-report:" + quienDe(r).IP },
	})
	publico.Get("/reports/altcha", h.RetoDenuncia)
	publico.With(tope).Post("/videos/{id}/reports", h.DenunciarSinSesion)
	publico.With(tope).Post("/channels/{org}/reports", h.DenunciarCanalSinSesion)
}

type denunciaEntrada struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
	Altcha  string `json:"altcha"`
}

func (d denunciaEntrada) nueva() social.NuevaDenuncia {
	return social.NuevaDenuncia{Kind: d.Kind, Target: d.Target, Reason: d.Reason, Details: d.Details}
}

// RetoDenuncia — GET /public/reports/altcha.
func (h *Handlers) RetoDenuncia(w http.ResponseWriter, r *http.Request) {
	reto, err := h.altcha.CrearReto(ambitoDenuncia, dificultadDenuncia, vidaRetoDenuncia)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("altcha_failed").Wrap(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, reto)
}

// denunciar es el cuerpo común: con sesión no hay reto; sin ella es obligatorio.
// La respuesta es siempre la misma y vacía: ni si ya estaba denunciado, ni quién
// lo escribió, ni un identificador.
func (h *Handlers) denunciar(w http.ResponseWriter, r *http.Request, conReto bool, hacer func(context.Context, social.Quien, social.NuevaDenuncia) error) {
	var in denunciaEntrada
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	quien := socialDe(quienDe(r))
	if conReto {
		// Por la ruta pública nadie tiene sesión, traiga el token que traiga.
		quien = social.Quien{Anonimo: true, IP: quien.IP}
		if err := h.altcha.Comprobar(r.Context(), ambitoDenuncia, dificultadDenuncia/2, in.Altcha); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid").WithMessage("The proof of work is missing or not valid.").Wrap(err))
			return
		}
	}
	if err := hacer(r.Context(), quien, in.nueva()); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) denunciarVideo(w http.ResponseWriter, r *http.Request, conReto bool) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.denunciar(w, r, conReto, func(ctx context.Context, q social.Quien, d social.NuevaDenuncia) error {
		return h.social.Denunciar(ctx, q, id, d)
	})
}

func (h *Handlers) denunciarCanal(w http.ResponseWriter, r *http.Request, conReto bool) {
	org := chi.URLParam(r, "org")
	h.denunciar(w, r, conReto, func(ctx context.Context, q social.Quien, d social.NuevaDenuncia) error {
		// El canal existe si tiene algo que enseñar: la misma lectura que lo pinta.
		if p, err := h.svc.Canal(ctx, org, "", 1); err != nil || len(p.Videos) == 0 {
			return social.ErrNoExiste
		}
		return h.social.DenunciarCanal(ctx, q, org, d)
	})
}

// Denunciar — POST /api/v1/videos/{id}/reports {"kind","target","reason","details"}.
func (h *Handlers) Denunciar(w http.ResponseWriter, r *http.Request) { h.denunciarVideo(w, r, false) }

// DenunciarSinSesion — POST /public/videos/{id}/reports, con `altcha`.
func (h *Handlers) DenunciarSinSesion(w http.ResponseWriter, r *http.Request) {
	h.denunciarVideo(w, r, true)
}

// DenunciarCanal — POST /api/v1/channels/{org}/reports.
func (h *Handlers) DenunciarCanal(w http.ResponseWriter, r *http.Request) {
	h.denunciarCanal(w, r, false)
}

// DenunciarCanalSinSesion — POST /public/channels/{org}/reports, con `altcha`.
func (h *Handlers) DenunciarCanalSinSesion(w http.ResponseWriter, r *http.Request) {
	h.denunciarCanal(w, r, true)
}

// subDeRuta lee un sub de la ruta. Llega escapado: un sub puede llevar «|» o «:».
func subDeRuta(r *http.Request) (string, error) {
	sub, err := url.PathUnescape(chi.URLParam(r, "sub"))
	if err != nil || sub == "" || len(sub) > 255 {
		return "", httpx.ErrBadRequest.WithCode("invalid_id").WithMessage("Invalid identifier.")
	}
	return sub, nil
}

// Bloquear — POST /api/v1/videos/{id}/blocks {"sub":"…","scope":"me"|"org"}.
func (h *Handlers) Bloquear(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Sub   string `json:"sub"`
		Scope string `json:"scope"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Scope != "" && in.Scope != "me" && in.Scope != "org" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown scope."))
		return
	}
	if err := h.social.Bloquear(r.Context(), socialDe(quienDe(r)), id, in.Sub, in.Scope == "org"); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

func bloqueadosJSON(w http.ResponseWriter, r *http.Request, lista []social.Bloqueado, err error) {
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if lista == nil {
		lista = []social.Bloqueado{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"blocks": lista})
}

// Bloqueados — GET /api/v1/blocks: la lista personal.
func (h *Handlers) Bloqueados(w http.ResponseWriter, r *http.Request) {
	lista, err := h.social.Bloqueados(r.Context(), socialDe(quienDe(r)))
	bloqueadosJSON(w, r, lista, err)
}

// Desbloquear — DELETE /api/v1/blocks/{sub}.
func (h *Handlers) Desbloquear(w http.ResponseWriter, r *http.Request) {
	sub, err := subDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.social.Desbloquear(r.Context(), socialDe(quienDe(r)), sub); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// BloqueadosEnOrg — GET /api/v1/moderation/blocks.
func (h *Handlers) BloqueadosEnOrg(w http.ResponseWriter, r *http.Request) {
	lista, err := h.social.BloqueadosEnOrg(r.Context(), socialDe(quienDe(r)))
	bloqueadosJSON(w, r, lista, err)
}

// DesbloquearEnOrg — DELETE /api/v1/moderation/blocks/{sub}.
func (h *Handlers) DesbloquearEnOrg(w http.ResponseWriter, r *http.Request) {
	sub, err := subDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.social.DesbloquearEnOrg(r.Context(), socialDe(quienDe(r)), sub); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// ResumenModeracion — GET /api/v1/moderation/summary: {"open": n}.
func (h *Handlers) ResumenModeracion(w http.ResponseWriter, r *http.Request) {
	n, err := h.social.DenunciasAbiertas(r.Context(), socialDe(quienDe(r)))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"open": n})
}

// Denuncias — GET /api/v1/moderation/reports?status=open|resolved&cursor=&limit=.
func (h *Handlers) Denuncias(w http.ResponseWriter, r *http.Request) {
	filas, siguiente, err := h.social.Denuncias(r.Context(), socialDe(quienDe(r)),
		r.URL.Query().Get("status") == "resolved", r.URL.Query().Get("cursor"), limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if filas == nil {
		filas = []social.Denuncia{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"reports": filas, "next_cursor": siguiente})
}

// ResolverDenuncia — POST /api/v1/moderation/reports/{rid} {"action":"dismiss"|…}.
func (h *Handlers) ResolverDenuncia(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "rid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.social.ResolverDenuncia(r.Context(), socialDe(quienDe(r)), id, in.Action); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// AuditoriaModeracion — GET /api/v1/moderation/audit: la traza de denuncias y
// acciones de la organización, con los filtros y el CSV de `auditlog`.
func (h *Handlers) AuditoriaModeracion(w http.ResponseWriter, r *http.Request) {
	q, err := auditlog.LeerConsulta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entradas, err := h.social.Auditoria(r.Context(), socialDe(quienDe(r)), auditlog.VentanaMax)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	entradas = auditlog.Filtrar(entradas, q)
	if q.CSV {
		auditlog.EscribirCSV(w, r, "cast", entradas)
		return
	}
	pagina, siguiente := auditlog.Paginar(entradas, q.After, q.Limit)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": pagina, "nextCursor": siguiente})
}

// ── La cola del operador ────────────────────────────────────────────────────

// deLaConsola: la cola del operador solo se abre con un token de la consola de
// plataforma. Va ANTES de mirar nada en la base.
func (h *Handlers) deLaConsola(w http.ResponseWriter, r *http.Request) (authn.Claims, bool) {
	c, ok := authn.FromContext(r.Context())
	if !ok || c.Subject == "" || !operador.DeLaConsola(c.Audience) || h.operador == nil || h.social == nil {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return authn.Claims{}, false
	}
	return c, true
}

// operadorDe pregunta a Sytue ID si quien llama opera la plataforma, diciéndole
// qué va a hacer: Sytue ID deja la traza y solo entonces contesta que sí. Si no
// es que sí, ya ha respondido.
func (h *Handlers) operadorDe(w http.ResponseWriter, r *http.Request, c authn.Claims, a operador.Acceso) (social.Quien, bool) {
	a.Servicio = "cast"
	if err := h.operador(r.Context(), authn.Token(r), a); err != nil {
		if errors.Is(err, operador.ErrNoEsOperador) {
			httpx.Fail(w, r, httpx.ErrForbidden)
		} else {
			httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		}
		return social.Quien{}, false
	}
	return social.Quien{Sub: c.Subject, Tenant: a.Org, Nombre: c.Name, Operador: true}, true
}

// DenunciasOperador — GET /api/v1/platform/moderation/reports?status=open|resolved&cursor=&limit=.
func (h *Handlers) DenunciasOperador(w http.ResponseWriter, r *http.Request) {
	c, ok := h.deLaConsola(w, r)
	if !ok {
		return
	}
	estado, cursor := "open", r.URL.Query().Get("cursor")
	if r.URL.Query().Get("status") == "resolved" {
		estado = "resolved"
	}
	quien, ok := h.operadorDe(w, r, c, operador.Acceso{Accion: "reports.list", Detalle: map[string]string{"status": estado, "cursor": cursor}})
	if !ok {
		return
	}
	filas, siguiente, err := h.social.DenunciasOperador(r.Context(), quien, estado == "resolved", cursor, limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if filas == nil {
		filas = []social.Denuncia{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"reports": filas, "next_cursor": siguiente})
}

// ResolverDenunciaOperador — POST /api/v1/platform/moderation/reports/{rid} {"action"}.
func (h *Handlers) ResolverDenunciaOperador(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "rid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, ok := h.deLaConsola(w, r)
	if !ok {
		return
	}
	// La organización se mira antes de preguntar a Sytue ID para que la traza diga
	// sobre cuál se actúa; y se pregunta también si no existe, para no contestar
	// «no existe» a quien no es operador.
	org, err := h.social.OrgDeDenuncia(r.Context(), id)
	if err != nil && !errors.Is(err, social.ErrNoExiste) {
		httpx.Fail(w, r, traducir(err))
		return
	}
	quien, ok := h.operadorDe(w, r, c, operador.Acceso{
		Accion: "report.resolve", Org: org, Objetivo: id.String(), Detalle: map[string]string{"action": in.Action},
	})
	if !ok {
		return
	}
	if org == "" {
		httpx.Fail(w, r, traducir(social.ErrNoExiste))
		return
	}
	if err := h.social.ResolverDenunciaOperador(r.Context(), quien, id, in.Action); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}
