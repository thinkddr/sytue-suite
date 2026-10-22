// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// MIGRACIÓN SELF-SERVICE: traerse UNO SUS PROPIOS datos.
//
// El resto de /migrations es de administrador porque mueve los datos de TODA la
// organización. Pero quien acaba de llegar a Sytue y abre Mail con el buzón vacío no
// puede pedirle a su administrador que le migre el correo cada vez: necesita un botón.
//
// Lo que hace segura esta puerta es que NO acepta a quién migrar. El destino es siempre
// quien llama y el origen su propia cuenta; ni el sub ni el correo salen del cuerpo de la
// petición, así que no hay nada que manipular para leer el buzón de otra persona. Solo
// «traer» (nunca «llevarse»), y solo tipos de contenido PERSONALES.
//
// La conexión con el origen la sigue autorizando un admin: sin una instancia activa aquí
// no se migra nada, y desactivarla corta esta puerta igual que las demás.

// selfMigrationDomains son los tipos de contenido que una persona puede traerse sola:
// los suyos. Los repositorios, los equipos y los recursos de nube pertenecen a la
// organización y los migra quien la administra.
var selfMigrationDomains = map[domain.MigrationDomain]struct{}{
	domain.DomainMail:     {},
	domain.DomainDrive:    {},
	domain.DomainCalendar: {},
	domain.DomainContacts: {},
}

type selfMigrationReq struct {
	InstanceID string   `json:"instance_id"`
	Source     string   `json:"source"`
	Domains    []string `json:"domains"`
}

// CreateSelfMigration — POST /api/v1/migrations/self.
//
// Da de alta un trabajo por tipo de contenido, todos con destino la cuenta de quien
// llama. El contenido aterriza con un token canjeado para esa persona (RFC 8693), que es
// lo que impide que el trabajo lea nada que no sea suyo aunque algo fallara aquí.
func (h *Handlers) CreateSelfMigration(w http.ResponseWriter, r *http.Request) {
	sub, email := selfCaller(r)
	if sub == "" || email == "" {
		// Sin correo no hay a qué cuenta del origen mirar. Es un token sin el claim, no
		// un fallo del usuario: se dice en vez de migrar «algo».
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("missing_identity"))
		return
	}

	var req selfMigrationReq
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
	doms, apiErr := parseDomains(src, req.Domains)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	for _, d := range doms {
		if _, ok := selfMigrationDomains[d]; !ok {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage(
				"That content belongs to the organisation: whoever administers it does the migration.",
			))
			return
		}
	}
	if h.store != nil {
		inst, err := h.store.GetInstance(r.Context(), req.InstanceID)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		if tplSrc, ok := sourceForTemplate(inst.TemplateKey); !ok || tplSrc != src {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The instance template does not match the requested migration source."))
			return
		}
	}

	jobs := make([]domain.MigrationJob, 0, len(doms))
	for _, dom := range doms {
		job, err := h.store.CreateMigrationJob(r.Context(), store.NewMigrationJob{
			InstanceID: req.InstanceID,
			// Destino y origen SIEMPRE quien llama: es lo que acota la puerta.
			TargetUser: sub,
			SourceUser: email,
			Source:     src,
			Domain:     dom,
			// Solo traer. Llevarse el contenido fuera de Sytue mueve datos de la
			// organización a un tercero, y eso lo decide quien la administra.
			Direction: domain.DirectionIn,
		})
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		jobs = append(jobs, job)
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"migrations": jobs})
}

// ListSelfMigrations — GET /api/v1/migrations/self: los trabajos de quien llama.
//
// Filtra por destino y no por quien los creó: si un administrador lanzó la migración de
// toda la organización, cada persona ve la SUYA. Es la misma información que ya iba a
// recibir en forma de correo nuevo, y saber en qué punto va evita que la pida por otro
// canal.
func (h *Handlers) ListSelfMigrations(w http.ResponseWriter, r *http.Request) {
	sub, _ := selfCaller(r)
	if sub == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("missing_identity"))
		return
	}
	jobs, err := h.store.ListMigrationJobsForTarget(r.Context(), sub, 50)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if jobs == nil {
		jobs = []domain.MigrationJob{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"migrations": jobs})
}

// selfCaller devuelve el sub y el correo de quien llama.
func selfCaller(r *http.Request) (sub, email string) {
	c, ok := authn.FromContext(r.Context())
	if !ok {
		return "", ""
	}
	return c.Subject, strings.ToLower(strings.TrimSpace(c.Email))
}
