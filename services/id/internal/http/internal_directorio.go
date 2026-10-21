// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"maps"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// POST /internal/directory/resolve
//
//	{"org": "<slug>", "emails": [...], "candidatos": {"<correo>": ["<sub>", ...]}}
//	→ {"users": {"<correo>": "<sub>"}, "con_permiso": {"<correo>": "<sub>"}}
//
// Es `directory:resolve` sin sesión, para la reconstrucción de avisos de Docs: una
// mención se escribió con una dirección, y solo se reconstruye si esa dirección es HOY
// de alguien que HOY tiene permiso sobre el documento (lo segundo lo mira Docs). Las
// mismas barreras que la puerta con sesión:
//
//  1. `emails`: SOLO ESA ORGANIZACIÓN, y solo cuentas activas. Una dirección de fuera
//     o que no existe simplemente no sale en la respuesta: no se distingue una de otra.
//     `candidatos` (quien colabora desde otra organización, ADR 0040): el directorio
//     GLOBAL, pero solo sale el `sub` si es uno de los que quien pregunta ya traía
//     —los que tienen permiso explícito—. Lo único que se aprende es que esa dirección
//     es de esa persona, que es justo lo que hace falta para avisarla.
//  2. SOLO LO MÍNIMO: el identificador. Ni nombre, ni rol, ni estado.
//  3. CON TOPE: hasta maxDireccionesDirectorio direcciones de cada clase y
//     peticiones.MaxCandidatosDirectorio candidatos por llamada, sin listado ni prefijos.
//
// Vale la dirección de entrar y las de los buzones de la organización (sin las que se
// están retirando). La verja es un secreto PROPIO (Config.DirectorioSecret).
//
// Y porque ese secreto vive en un Deployment que da la cara a internet (Docs):
//   - solo desde DENTRO (httpx.SoloDesdeDentro): con X-Forwarded-For es 404 antes de
//     mirar el secreto, como las rutas internas del Hub;
//   - se CIERRA con la reconstrucción (peticiones.DirectorioInternoCierre, 410);
//   - límite por organización y un registro de cada llamada (organización y número
//     de direcciones, nunca las direcciones).
const maxDireccionesDirectorio = peticiones.MaxDireccionesDirectorio

type internalDirectoryReq struct {
	Org    string   `json:"org"`
	Emails []string `json:"emails"`
	// Candidatos (peticiones.ConsultaDirectorio): por dirección, los `sub` con permiso.
	Candidatos map[string][]string `json:"candidatos"`
	// Subs: de estos, cuáles son hoy miembros activos.
	Subs []string `json:"subs"`
}

func (s *Server) handleInternalDirectoryResolve(w http.ResponseWriter, r *http.Request) {
	if !time.Now().Before(peticiones.DirectorioInternoCierre) {
		httpx.Fail(w, r, httpx.NewError(http.StatusGone, "gone", "This maintenance operation is closed."))
		return
	}
	var req internalDirectoryReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, ok := leerConsultaDirectorio(req)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many addresses, or candidates and subs together."))
		return
	}
	// El techo global va ANTES de nada: también frena probar organizaciones inventadas.
	if !s.permiteDirectorio(w, r, s.directorioGlobal, "directorio-interno") {
		return
	}
	org, err := s.store.GetOrgBySlug(r.Context(), strings.TrimSpace(req.Org))
	if errors.Is(err, domain.ErrNotFound) || (err == nil && org.Status != domain.StatusActive) {
		// Una organización que no existe, o que no está activa (suspendida, borrada,
		// fusionada), no tiene a nadie: la misma respuesta vacía.
		httpx.JSON(w, r, http.StatusOK, peticiones.Resueltas{})
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Y el de la organización, por su id: un nombre inventado no crea filas del limitador.
	if !s.permiteDirectorio(w, r, s.directorioLimiter, "directorio-interno:"+org.ID) {
		return
	}
	// Las dos vías que responden por elemento se cobran POR ELEMENTO y de golpe, cada
	// una en su limitador y sin depender de la organización: rotando organizaciones no
	// se suman ráfagas, y un cobro que no cabe no gasta nada.
	if !s.permiteDirectorioN(w, r, s.directorioCandidatos, "directorio-interno:candidatos", c.pares) ||
		!s.permiteDirectorioN(w, r, s.directorioSubs, "directorio-interno:subs", len(c.subs)) {
		return
	}
	out, err := s.resolverDirectorio(r, org.ID, c)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	s.log.Info("directorio interno: resolución", "org", org.Slug, "direcciones", len(c.emails),
		"con_candidatos", len(c.pedidos), "candidatos", c.pares, "subs", len(c.subs),
		"miembros", len(out.Miembros), "con_permiso", len(out.ConPermiso), "activos", len(out.Activos))
	httpx.JSON(w, r, http.StatusOK, out)
}

// consultaDirectorio es la petición ya validada y normalizada.
type consultaDirectorio struct {
	emails  []string
	pedidos map[string]map[string]bool // dirección → candidatos
	pares   int
	subs    []string
}

// leerConsultaDirectorio valida los topes y normaliza. false = se pasa de los topes.
// Los `sub` que no son un UUID (los de Sytue ID lo son) se descartan sin más, y los que
// sí, en minúsculas.
func leerConsultaDirectorio(req internalDirectoryReq) (consultaDirectorio, bool) {
	c := consultaDirectorio{emails: direccionesValidas(req.Emails), pedidos: map[string]map[string]bool{}}
	// Candidatos y `subs` se cobran en limitadores distintos: juntos, un segundo cobro
	// que no cupiera dejaría gastado el primero. Van siempre en llamadas separadas.
	if len(req.Emails) > maxDireccionesDirectorio || len(req.Candidatos) > maxDireccionesDirectorio ||
		len(req.Subs) > peticiones.MaxSubsDirectorio || (len(req.Candidatos) > 0 && len(req.Subs) > 0) {
		return c, false
	}
	for e, subs := range req.Candidatos {
		if len(subs) > peticiones.MaxCandidatosPorDireccion {
			return c, false
		}
		d := direccionesValidas([]string{e})
		if len(d) == 0 {
			continue
		}
		for _, sub := range subs {
			if sub = subValido(sub); sub == "" {
				continue
			}
			if c.pedidos[d[0]] == nil {
				c.pedidos[d[0]] = map[string]bool{}
			}
			if !c.pedidos[d[0]][sub] {
				c.pedidos[d[0]][sub] = true
				c.pares++
			}
		}
	}
	if c.pares > peticiones.MaxCandidatosDirectorio {
		return c, false
	}
	for _, sub := range req.Subs {
		if sub = subValido(sub); sub != "" && !slices.Contains(c.subs, sub) {
			c.subs = append(c.subs, sub)
		}
	}
	return c, true
}

// subValido es el `sub` normalizado (UUID en minúsculas, como los guarda Sytue ID), o "".
func subValido(v string) string {
	u, err := uuid.Parse(strings.TrimSpace(v))
	if err != nil || len(strings.TrimSpace(v)) != 36 {
		return ""
	}
	return u.String()
}

// resolverDirectorio contesta las tres preguntas:
//
//   - `users`: las direcciones de gente de la organización. Si la dirección es también
//     de otra cuenta en el directorio global, no sale: no se sabe de quién es.
//   - `con_permiso`: quien colabora desde otra organización. El directorio GLOBAL, pero
//     solo sale el `sub` si es uno de los candidatos que ya traía quien pregunta, y no
//     es de la organización (esos salen por `users`). Sin candidato no se dice nada, ni
//     siquiera que la dirección existe.
//   - `activos`: de los `subs` dados, los que hoy son miembros activos.
func (s *Server) resolverDirectorio(r *http.Request, orgID string, c consultaDirectorio) (peticiones.Resueltas, error) {
	out := peticiones.Resueltas{Miembros: map[string]string{}, ConPermiso: map[string]string{}}
	ctx := r.Context()
	deOrg, err := s.store.ResolverDireccionesDeOrg(ctx, orgID, c.emails)
	if err != nil {
		return out, err
	}
	todas := slices.Concat(slices.Collect(maps.Keys(deOrg)), slices.Collect(maps.Keys(c.pedidos)))
	globales, err := s.store.DuenosGlobales(ctx, todas)
	if err != nil {
		return out, err
	}
	for e, sub := range deOrg {
		if !slices.ContainsFunc(globales[e], func(d store.DuenoGlobal) bool { return d.Sub != sub }) {
			out.Miembros[e] = sub
		}
	}
	var candidatos []string
	for e := range c.pedidos {
		if sub := store.Unico(globales[e]); c.pedidos[e][sub] {
			candidatos = append(candidatos, sub)
		}
	}
	yaMiembros, err := s.store.MiembrosActivos(ctx, orgID, candidatos)
	if err != nil {
		return out, err
	}
	for e := range c.pedidos {
		if sub := store.Unico(globales[e]); c.pedidos[e][sub] && !slices.Contains(yaMiembros, sub) {
			out.ConPermiso[e] = sub
		}
	}
	out.Activos, err = s.store.MiembrosActivos(ctx, orgID, c.subs)
	return out, err
}

// direccionesValidas son las que se leen como dirección, normalizadas.
func direccionesValidas(entrada []string) []string {
	var out []string
	for _, e := range entrada {
		if a, err := mail.ParseAddress(strings.TrimSpace(e)); err == nil {
			out = append(out, strings.ToLower(a.Address))
		}
	}
	return out
}

// permiteDirectorioN cobra n fichas de golpe y contesta 429 si no caben.
func (s *Server) permiteDirectorioN(w http.ResponseWriter, r *http.Request, l *pg.RateLimiter, clave string, n int) bool {
	res, err := l.AllowN(r.Context(), clave, n)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return false
	}
	if !res.Allowed {
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return false
	}
	return true
}

// permiteDirectorio aplica un limitador y contesta 429 si no deja pasar.
func (s *Server) permiteDirectorio(w http.ResponseWriter, r *http.Request, l httpx.RateLimiter, clave string) bool {
	res, err := l.Allow(r.Context(), clave)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return false
	}
	if !res.Allowed {
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return false
	}
	return true
}
