// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// maxSourceUserLen acota el identificador del usuario de origen (UPN/email).
const maxSourceUserLen = 320

// maxTargets acota cuántas personas se migran de una tacada. No es una limitación
// del mecanismo (cada persona es un trabajo independiente en el outbox), sino un
// tope sano para una sola petición: por encima, el asistente lanza otra tanda.
const maxTargets = 200

// migrationTarget es una pareja «de qué cuenta del origen» → «a la cuenta de quién
// en Sytue». Una por persona a migrar.
type migrationTarget struct {
	SourceUser string `json:"source_user"` // UPN de M365 o email del Workspace
	TargetUser string `json:"target_user"` // correo o uuid del usuario de Sytue
}

type createMigrationReq struct {
	InstanceID string            `json:"instance_id"`
	Source     string            `json:"source"`  // m365 | google | github
	Domains    []string          `json:"domains"` // drive | contacts | …
	Targets    []migrationTarget `json:"targets"`
	// Direction: "in" (por defecto) trae el contenido a Sytue; "out" se lo lleva
	// al tercero. Irse es parte del producto, así que sale por la MISMA puerta.
	Direction string `json:"direction"`
}

// CreateMigration — POST /api/v1/migrations — da de alta los trabajos de migración
// y los encola en el outbox (202: corren en asíncrono).
//
// El contenido aterriza en la cuenta de CADA PERSONA, no en la de quien lanza: la
// escritura va con un token canjeado (RFC 8693) acotado a ese usuario. La ruta
// exige owner/admin (router) porque es quien posee el conector de la org y quien
// responde de leer el buzón de otra persona en el origen.
//
// Un trabajo por (persona × tipo de contenido): así cada uno tiene su cursor, su
// mapa de idempotencia y su informe, y el fallo de uno no arrastra a los demás.
func (h *Handlers) CreateMigration(w http.ResponseWriter, r *http.Request) {
	var req createMigrationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validUUID(req.InstanceID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid instance_id."))
		return
	}
	src := domain.MigrationSource(strings.TrimSpace(req.Source))
	if !src.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The source must be m365, google or github."))
		return
	}
	direction := domain.MigrationDirection(strings.TrimSpace(req.Direction)).OrIn()
	if !direction.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The direction must be in (bring content in) or out (take content out)."))
		return
	}
	doms, apiErr := parseDomains(src, req.Domains)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if direction == domain.DirectionOut {
		if apiErr := validOutbound(src, doms); apiErr != nil {
			httpx.Fail(w, r, apiErr)
			return
		}
	}
	targets, apiErr := parseTargets(req.Targets)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}

	// Alta trabajo a trabajo: cada uno es independiente por diseño. Si uno falla, se
	// devuelve el error con los ya creados en el cuerpo — están encolados y son
	// válidos; no se cancelan porque uno posterior no cupiera.
	jobs := make([]domain.MigrationJob, 0, len(targets)*len(doms))
	for _, t := range targets {
		for _, dom := range doms {
			job, err := h.store.CreateMigrationJob(r.Context(), store.NewMigrationJob{
				InstanceID: req.InstanceID,
				TargetUser: t.TargetUser,
				SourceUser: t.SourceUser,
				Source:     src,
				Domain:     dom,
				Direction:  direction,
			})
			if err != nil {
				httpx.Fail(w, r, toAPIError(err))
				return
			}
			jobs = append(jobs, job)
		}
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"migrations": jobs})
}

// validOutbound comprueba que lo que se quiere LLEVAR se pueda llevar de verdad.
//
// Se rechaza al crear el trabajo, no a mitad de la copia: un asistente que acepta
// todo y falla luego es peor que uno que dice que no desde el principio.
func validOutbound(src domain.MigrationSource, doms []domain.MigrationDomain) *httpx.Error {
	if src != domain.SourceGoogle && src != domain.SourceM365 {
		return httpx.ErrBadRequest.WithMessage(
			"Content can only be taken out to Google Workspace or Microsoft 365.",
		)
	}
	for _, d := range doms {
		if d != domain.DomainDrive && d != domain.DomainContacts {
			return httpx.ErrBadRequest.WithMessage(
				"For now only files and contacts can be taken out. " +
					"Mail is exported as mbox and the calendar as iCalendar from their own apps.",
			)
		}
	}
	return nil
}

// parseDomains valida los tipos de contenido pedidos (al menos uno, sin repetir) y
// que ESE origen sepa traerlos: pedirle contactos a GitHub o repositorios a Google
// no es un trabajo vacío, es uno imposible, y se rechaza al crearlo en vez de morir
// más tarde en el outbox.
func parseDomains(src domain.MigrationSource, raw []string) ([]domain.MigrationDomain, error) {
	out := make([]domain.MigrationDomain, 0, 2)
	seen := map[domain.MigrationDomain]bool{}
	for _, v := range raw {
		d := domain.MigrationDomain(strings.TrimSpace(v))
		if !d.Valid() || !src.Supports(d) {
			return nil, httpx.ErrBadRequest.WithMessage(
				"That source cannot bring that content. It can bring: " + domainList(src) + ".",
			)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage("Say what content to bring: " + domainList(src) + ".")
	}
	return out, nil
}

// domainList enumera en llano lo que un origen sabe traer, para los mensajes de error.
func domainList(src domain.MigrationSource) string {
	doms := src.Domains()
	names := make([]string, 0, len(doms))
	for _, d := range doms {
		names = append(names, string(d))
	}
	if len(names) == 0 {
		return "nothing yet"
	}
	return strings.Join(names, ", ")
}

// parseTargets valida las parejas origen→destino. Ambos lados son obligatorios:
// sin el destino no sabríamos en qué cuenta escribir, y adivinarlo (p. ej. por el
// mismo correo) sería copiar los datos de alguien a la cuenta equivocada.
func parseTargets(raw []migrationTarget) ([]migrationTarget, error) {
	if len(raw) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage("Say who the migration is for (at least one person).")
	}
	if len(raw) > maxTargets {
		return nil, httpx.ErrBadRequest.WithMessage(
			"That is too many people at once (maximum " + strconv.Itoa(maxTargets) + "). Run it in several batches.",
		)
	}
	out := make([]migrationTarget, 0, len(raw))
	for _, t := range raw {
		src := strings.TrimSpace(t.SourceUser)
		dst := strings.TrimSpace(t.TargetUser)
		if src == "" || len(src) > maxSourceUserLen {
			return nil, httpx.ErrBadRequest.WithMessage(
				"Say which account in the source is copied from (source_user: that person's email address or UPN).",
			)
		}
		if dst == "" || len(dst) > maxSourceUserLen {
			return nil, httpx.ErrBadRequest.WithMessage(
				"Say which Sytue account the content lands in (target_user: that person's email address).",
			)
		}
		out = append(out, migrationTarget{SourceUser: src, TargetUser: dst})
	}
	return out, nil
}

// ListMigrations — GET /api/v1/migrations — trabajos de la org, más reciente
// primero. limit opcional (1..200, 50 por defecto).
func (h *Handlers) ListMigrations(w http.ResponseWriter, r *http.Request) {
	limit, ok := limitParam(w, r, 50, 200)
	if !ok {
		return
	}
	jobs, err := h.store.ListMigrationJobs(r.Context(), limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"migrations": jobs})
}

// GetMigration — GET /api/v1/migrations/{id} — progreso e INFORME del trabajo:
// contadores, último error legible y la lista de lo OMITIDO con su motivo. Lo
// omitido es tan parte del resultado como lo migrado: nada se cuenta como copiado
// sin haberse copiado.
func (h *Handlers) GetMigration(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	job, err := h.store.GetMigrationJob(ctx, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	limit, ok := limitParam(w, r, 100, 500)
	if !ok {
		return
	}
	skipped, err := h.store.ListMigrationSkipped(ctx, id, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"migration": job,
		"skipped":   skipped,
	})
}

// CancelMigration — POST /api/v1/migrations/{id}/cancel — detiene un trabajo vivo.
// Lo ya migrado se queda; el cursor también, así que puede reanudarse después.
func (h *Handlers) CancelMigration(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.store.CancelMigrationJob(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "canceled", "migration_id": id})
}

// ResumeMigration — POST /api/v1/migrations/{id}/resume — reanuda un trabajo
// detenido POR EL CURSOR. Idempotente: lo ya migrado no se vuelve a escribir. El
// permiso de escritura se vuelve a canjear al arrancar, así que reanudar tras
// arreglar el problema (una persona reactivada, la delegación aprovisionada) basta.
func (h *Handlers) ResumeMigration(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// La salida propia no se reencola: se vuelve a lanzar con el token de su dueña.
	if job, err := h.store.GetMigrationJob(r.Context(), id); err == nil &&
		job.Direction == domain.DirectionOut && job.SourceUser == "" {
		if sub, _ := selfCaller(r); sub != job.TargetUser {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only the person whose content it is can resume it."))
			return
		}
		h.lanzarSalida(w, r, job.InstanceID, []domain.MigrationDomain{job.Domain})
		return
	}
	if err := h.store.ResumeMigrationJob(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"status": "queued", "migration_id": id})
}

// --- helpers -----------------------------------------------------------------

// limitParam lee ?limit acotado a (1..upper), con un valor por defecto.
func limitParam(w http.ResponseWriter, r *http.Request, def, upper int) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > upper {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1.."+strconv.Itoa(upper)+")."))
		return 0, false
	}
	return n, true
}
