// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

const (
	searchDefaultLimit  = 20 // resultados fusionados por defecto
	searchMaxLimit      = 50 // tope duro del límite total
	searchMaxConnectors = 16 // tope de instancias consultadas por búsqueda (anti fan-out patológico)
	searchConcurrency   = 8  // fan-out concurrente ACOTADO (no N ilimitado)
)

// Search — GET /api/v1/search?q=&connections=&limit= — búsqueda federada sobre los
// SaaS CONECTADOS de la organización. Hace fan-out CONCURRENTE (acotado) a las
// instancias ACTIVAS cuyo driver implementa connector.SearchCapability, fusiona los
// hits (límite total) y DEGRADA por-conector: una instancia que falla (upstream
// caído, destino bloqueado por SSRF…) se registra y se omite, sin romper el resto.
//
// Aislamiento por org: el store corre bajo RLS con el tenant del ctx, así que solo
// se enumeran y consultan las instancias de la org del llamante. Y de ellas, solo las
// que quien busca puede consultar (NEG-06): las demás no participan, sin aviso, igual
// que una sin capacidad de búsqueda. `connections` filtra por id de instancia (CSV);
// vacío = todas las activas.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`: esta ruta está montada con
	// `GetYQuery`, y por QUERY los parámetros viajan en el CUERPO —la query
	// string llega vacía—. Leyéndolos de la URL, la búsqueda federada de integraciones se quedaba sin
	// argumentos y respondía como si no le hubieran pedido nada. Pasó con el
	// buscador global del Hub, que devolvía 400 en cada pulsación.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	query := strings.TrimSpace(params.Get("q"))
	if query == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The search parameter 'q' is required."))
		return
	}
	limit := parseSearchLimit(params.Get("limit"))
	only := connectionSet(params.Get("connections"))

	insts, err := h.store.ListInstances(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// nil = administra la organización y puede con todas.
	var consultables map[string]bool
	if c, _ := authn.FromContext(ctx); !esAdmin(c) {
		if consultables, err = h.store.Consultables(ctx, c.Subject, c.Groups); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
	}

	// Construir un job por instancia activa con capacidad de búsqueda: carga sus
	// secretos y fabrica el Runtime (único cliente de egress). Es N+1 lecturas, pero
	// el nº de conectores por org es pequeño y va acotado por searchMaxConnectors.
	// ponytail: una carga en lote de secretos si el nº de conectores por org creciera.
	jobs := make([]searchJob, 0, len(insts))
	for _, inst := range insts {
		if inst.Status != domain.StatusActive {
			continue
		}
		if len(only) > 0 && !only[inst.ID] {
			continue
		}
		if consultables != nil && !consultables[inst.ID] {
			continue
		}
		// La búsqueda la sirve el driver de DATOS de la plantilla (los que además
		// implementan SearchCapability). Una instancia sin esa capacidad simplemente no
		// participa en la federación: degradar, nunca romper.
		// Ya se han saltado las inactivas arriba; falta respetar el interruptor de
		// capacidad, que hasta ahora no gateaba nada.
		if !capabilityEnabled(inst.LevelEnabled, manifest.CapData) {
			continue
		}
		drv, _, err := h.resolveCapability(ctx, inst.TemplateKey, inst.ConfigMap(), manifest.CapData)
		if err != nil {
			continue
		}
		sc, ok := drv.(connector.SearchCapability)
		if !ok {
			continue
		}
		loaded, err := h.loadInstance(ctx, inst.ID)
		if err != nil {
			// Degradación: una instancia ilegible no rompe el resto de la federación.
			log.From(ctx).Warn("search federada: no se pudo cargar la instancia", "instance", inst.ID, "error", err)
			continue
		}
		rt := h.egressRuntime(loaded, log.From(ctx).With("instance", loaded.ID, "org", loaded.OrgID))
		// El driver recibe la instancia SIN el "bearer" (inyectado en el Runtime); los
		// demás secretos sí (mismo criterio que QueryData/EmbedToken).
		driverInst := loaded
		driverInst.Secrets = driverSecrets(loaded.Secrets)

		jobs = append(jobs, searchJob{cap: sc, inst: driverInst, rt: rt})
		if len(jobs) >= searchMaxConnectors {
			break
		}
	}

	hits := fanOutSearch(ctx, jobs, query, limit)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"hits": hits})
}

// searchJob es una instancia lista para consultarse: su capacidad de búsqueda, la
// instancia (ya sin el bearer del vault) y el Runtime de egress fabricado para ella.
type searchJob struct {
	cap  connector.SearchCapability
	inst connector.Instance
	rt   connector.Runtime
}

// fanOutSearch consulta los jobs EN PARALELO con concurrencia ACOTADA (semáforo),
// fusiona sus hits y los recorta al límite total. DEGRADACIÓN por-conector: un job
// que falla se registra y se omite, sin abortar el resto. Aislada del store/HTTP
// para poder testearse offline con capacidades mock.
func fanOutSearch(ctx context.Context, jobs []searchJob, query string, limit int) []connector.ExternalHit {
	all := make([]connector.ExternalHit, 0, limit)
	if len(jobs) == 0 {
		return all
	}
	sem := make(chan struct{}, searchConcurrency)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, j := range jobs {
		wg.Add(1)
		go func(j searchJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			hits, err := j.cap.Search(ctx, j.inst, j.rt, query, limit)
			if err != nil {
				log.From(ctx).Warn("search federada: conector falló (degradado)",
					"instance", j.inst.ID, "template", j.inst.TemplateKey, "error", err)
				return
			}
			mu.Lock()
			all = append(all, hits...)
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// parseSearchLimit parsea el límite TOTAL de resultados fusionados, acotado a
// [1, searchMaxLimit] con searchDefaultLimit por defecto.
func parseSearchLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return searchDefaultLimit
	}
	if n > searchMaxLimit {
		return searchMaxLimit
	}
	return n
}

// connectionSet parsea el filtro CSV `connections` (ids de instancia) a un conjunto.
// Vacío ⇒ nil (sin filtro: todas las instancias activas de la org).
func connectionSet(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out[p] = true
		}
	}
	return out
}
