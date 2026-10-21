// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analyticshttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/analytics/internal/config"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

// maxRangeDays acota el rango consultable. No es una limitación técnica de los
// agregados (viven años): evita que una consulta de años bloquee al resto.
const maxRangeDays = 400

// Errores de la API de lectura, con código estable para el catálogo del cliente
// (httpx §13.1: el servidor manda código y datos, el cliente traduce).
var (
	errBadSite = httpx.ErrBadRequest.WithCode("invalid_site_id").
			WithMessage("Site id is not a valid identifier.")
	errBadRange = httpx.ErrBadRequest.WithCode("invalid_range").
			WithMessage("The date range is invalid or longer than {max} days.").
			WithParams(map[string]any{"max": maxRangeDays})
	errBadDimension = httpx.ErrBadRequest.WithCode("unknown_dimension").
			WithMessage(`Unknown dimension "{dim}".`)
	errBadHost = httpx.ErrBadRequest.WithCode("invalid_host").
			WithMessage("A site is a hostname, and this is not one.")
	errSiteTaken = httpx.NewError(http.StatusConflict, "host_already_measured",
		"That hostname is already being measured.")
)

// APIDeps son las dependencias de la API de lectura.
type APIDeps struct {
	Cfg           config.Config
	Verifier      *authn.Verifier
	Store         *store.Store
	ReadyCheckers []httpx.Checker
}

type api struct {
	cfg   config.Config
	store *store.Store
}

// NewAPIRouter construye el router de la API de lectura (puerto 8095).
//
// TODAS sus consultas van contra los agregados; ninguna toca el crudo. No hay
// endpoint que devuelva un identificador de visitante porque no hay nada que
// devolver: los agregados no lo contienen.
func NewAPIRouter(d APIDeps) http.Handler {
	a := &api{cfg: d.Cfg, store: d.Store}
	r := httpx.NewRouter(httpx.RouterOptions{
		CORS: &httpx.CORSConfig{
			AllowedOrigins:   d.Cfg.CORSAllowedOrigins,
			AllowCredentials: true,
		},
		RequestTimeout: 30 * time.Second,
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(d.Verifier.RequireAuth)
		v1.Use(d.Verifier.RequireTenant)

		v1.Get("/sites", a.listSites)
		v1.Post("/sites", a.createSite)
		v1.Put("/sites/{id}/events", a.updateAllowlist)
		v1.Delete("/sites/{id}", a.deleteSite)

		v1.Get("/sites/{id}/summary", a.summary)
		v1.Get("/sites/{id}/timeseries", a.timeseries)
		v1.Get("/sites/{id}/breakdown", a.breakdown)
		// Las dimensiones que existen, para que el panel no las adivine.
		v1.Get("/dimensions", func(w http.ResponseWriter, r *http.Request) {
			httpx.JSON(w, r, http.StatusOK, map[string]any{"dimensions": store.Dimensions})
		})
	})
	if d.Cfg.PagesSecret != "" {
		r.With(httpx.SoloDesdeDentro(d.Cfg.PagesSecret)).Post("/internal/v1/sites", a.ensureSite)
		r.With(httpx.SoloDesdeDentro(d.Cfg.PagesSecret)).Get("/internal/v1/sites/{id}/summary", a.internalSummary)
	}
	return r
}

// internalSummary ofrece a Pages solo el agregado que su panel necesita. La
// ruta no acepta un tenant de la petición: Pages ya resuelve el sitio con RLS
// antes de consultarla y Analytics no expone eventos crudos en ningún caso.
func (a *api) internalSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	tenant, err := tenancy.New(r.Header.Get("X-Sytue-Tenant"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_tenant"))
		return
	}
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	totals, err := a.store.Summary(tenancy.Into(r.Context(), tenant), id, from, to)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"totals": totals, "visitors_are_daily_sum": true,
	})
}

func (a *api) listSites(w http.ResponseWriter, r *http.Request) {
	sites, err := a.store.ListSites(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if sites == nil {
		sites = []store.Site{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sites": sites})
}

type siteRequest struct {
	Host           string   `json:"host"`
	EventAllowlist []string `json:"event_allowlist"`
}

func (a *api) createSite(w http.ResponseWriter, r *http.Request) {
	var req siteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	host := domain.CleanHost(req.Host)
	if host == "" {
		httpx.Fail(w, r, errBadHost)
		return
	}
	if a.reservedHost(host) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("host_reserved").WithMessage("This hostname is reserved for Sytue Web."))
		return
	}
	allowlist, err := cleanAllowlist(req.EventAllowlist)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	site, err := a.store.CreateSite(r.Context(), host, allowlist)
	if err != nil {
		// El host es UNIQUE global: colisionar significa que ya lo mide otro.
		httpx.Fail(w, r, errSiteTaken.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, site)
}

type internalSiteRequest struct {
	Tenant         string   `json:"tenant"`
	Host           string   `json:"host"`
	EventAllowlist []string `json:"event_allowlist"`
}

// ensureSite es la única puerta que puede registrar un hostname reservado.
func (a *api) ensureSite(w http.ResponseWriter, r *http.Request) {
	var req internalSiteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tenant, err := tenancy.New(strings.TrimSpace(req.Tenant))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid tenant."))
		return
	}
	host := domain.CleanHost(req.Host)
	if host == "" || !a.reservedHost(host) {
		httpx.Fail(w, r, errBadHost)
		return
	}
	allowlist, err := cleanAllowlist(req.EventAllowlist)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	site, err := a.store.EnsureSite(tenancy.Into(r.Context(), tenant), host, allowlist)
	if errors.Is(err, store.ErrHostDeOtro) {
		httpx.Fail(w, r, errSiteTaken.Wrap(err))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"site_id": site.ID})
}

func (a *api) reservedHost(host string) bool {
	for _, suffix := range a.cfg.ReservedHostSuffixes {
		suffix = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(suffix)), ".")
		if suffix != "" && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

func (a *api) updateAllowlist(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	var req siteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	allowlist, err := cleanAllowlist(req.EventAllowlist)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	site, err := a.store.UpdateAllowlist(r.Context(), id, allowlist)
	if err != nil {
		httpx.Fail(w, r, apiErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, site)
}

func (a *api) deleteSite(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteSite(r.Context(), id); err != nil {
		httpx.Fail(w, r, apiErr(err))
		return
	}
	httpx.NoContent(w)
}

func (a *api) summary(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	totals, err := a.store.Summary(r.Context(), id, from, to)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"from": from.Format(time.DateOnly), "to": to.Format(time.DateOnly),
		"totals": totals,
		// Contrato explícito con quien pinte esto: NO es un único de rango
		// deduplicado, y no puede serlo (ADR 0026 §1). Etiquetarlo es obligación
		// del panel, y este campo existe para que no se le olvide.
		"visitors_are_daily_sum": true,
	})
}

func (a *api) timeseries(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	hourly := r.URL.Query().Get("interval") == "hour"
	points, err := a.store.Timeseries(r.Context(), id, from, to, hourly)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if points == nil {
		points = []store.Point{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"points": points, "visitors_are_daily_sum": !hourly})
}

func (a *api) breakdown(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	dim := r.URL.Query().Get("dim")
	if !store.ValidDimension(dim) {
		httpx.Fail(w, r, errBadDimension.WithParams(map[string]any{"dim": dim}))
		return
	}
	limit := 10
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	rows, err := a.store.Breakdown(r.Context(), id, dim, from, to, limit)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if rows == nil {
		rows = []store.BreakdownRow{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"dim": dim, "rows": rows})
}

// siteID valida el identificador de la ruta. Que exista o no, y de quién sea, lo
// decide la RLS: aquí solo se comprueba que sea un UUID para no mandarle basura
// a PostgreSQL.
func siteID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, errBadSite)
		return "", false
	}
	return id, true
}

// dateRange lee ?from=YYYY-MM-DD&to=YYYY-MM-DD. Por defecto, los últimos 30 días.
func dateRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	q := r.URL.Query()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	to = today
	from = today.AddDate(0, 0, -29)

	if v := q.Get("from"); v != "" {
		d, err := time.ParseInLocation(time.DateOnly, v, time.UTC)
		if err != nil {
			httpx.Fail(w, r, errBadRange)
			return time.Time{}, time.Time{}, false
		}
		from = d
	}
	if v := q.Get("to"); v != "" {
		d, err := time.ParseInLocation(time.DateOnly, v, time.UTC)
		if err != nil {
			httpx.Fail(w, r, errBadRange)
			return time.Time{}, time.Time{}, false
		}
		to = d
	}
	if to.Before(from) || to.Sub(from) > maxRangeDays*24*time.Hour {
		httpx.Fail(w, r, errBadRange)
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// cleanAllowlist sanea los nombres de evento declarados por el cliente. Es la
// verja que impide que data-sytue-event="pepe@correo.com" convierta el almacén
// anónimo en un canal de exfiltración, así que se aplica al DAR DE ALTA el
// nombre, no solo al recibirlo.
func cleanAllowlist(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		name := domain.CleanShort(raw, domain.MaxName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) > 50 {
		return nil, httpx.ErrBadRequest.WithCode("too_many_events").
			WithMessage("At most 50 event names per site.")
	}
	return out, nil
}

func apiErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.ErrNotFound
	}
	return err
}
