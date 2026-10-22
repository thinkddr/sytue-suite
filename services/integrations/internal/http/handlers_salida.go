// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Llevarse LO SUYO a Google o Microsoft sin la delegación de la organización:
// cuentas personales, owners y admins (las rutas de escritura ya exigen ese rol; a
// quien no administra, llevarse contenido de la organización lo decide quien la
// administra). Se lee lo mismo que la copia de «Tus datos»: la entrega de cada
// producto, abierta con el token de quien lanza el trabajo.

type salidaPropiaReq struct {
	InstanceID string   `json:"instance_id"`
	Domains    []string `json:"domains"`
}

// destinosDeSalida: plantilla del catálogo → destino que sabe escribir.
var destinosDeSalida = map[string]domain.MigrationSource{
	"google-workspace": domain.SourceGoogle,
	"microsoft365":     domain.SourceM365,
}

// CreateSelfOutbound — POST /api/v1/migrations/self/out — lanza (o continúa) la
// salida propia de los contenidos pedidos hacia una conexión autorizada.
func (h *Handlers) CreateSelfOutbound(w http.ResponseWriter, r *http.Request) {
	if !exigirAdminMigracion(w, r) {
		return
	}
	var req salidaPropiaReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validUUID(req.InstanceID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid instance_id."))
		return
	}
	var doms []domain.MigrationDomain
	for _, raw := range req.Domains {
		d := domain.MigrationDomain(strings.TrimSpace(raw))
		switch d {
		case domain.DomainDrive, domain.DomainContacts, domain.DomainMail, domain.DomainCalendar:
		default:
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Files, contacts, mail and calendar can be taken out."))
			return
		}
		doms = append(doms, d)
	}
	if len(doms) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Say what to take out."))
		return
	}
	h.lanzarSalida(w, r, req.InstanceID, doms)
}

// lanzarSalida deja listos los trabajos, abre la entrega de cada uno con el token de
// quien llama y los ejecuta en segundo plano, uno detrás de otro.
func (h *Handlers) lanzarSalida(w http.ResponseWriter, r *http.Request, instanceID string, doms []domain.MigrationDomain) {
	sub, _ := selfCaller(r)
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if sub == "" || token == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	inst, err := h.store.LoadInstanceWithSecrets(r.Context(), instanceID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	src, ok := destinosDeSalida[inst.TemplateKey]
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Content can only be taken out to Google Workspace or Microsoft 365."))
		return
	}

	jobs := make([]domain.MigrationJob, 0, len(doms))
	for _, d := range doms {
		job, err := h.store.LaunchSelfOutboundJob(r.Context(), store.NewMigrationJob{
			InstanceID: instanceID, TargetUser: sub, Source: src, Domain: d, Direction: domain.DirectionOut,
		})
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		jobs = append(jobs, job)
	}

	// Fuera de la petición: la copia dura mucho más que ella. Se corta al apagar el
	// servicio, y entonces cada trabajo se cierra diciendo que se interrumpió.
	ctx, cancel := context.WithCancel(log.Into(context.WithoutCancel(r.Context()), log.From(r.Context()).With("salida", sub)))
	soltar := context.AfterFunc(h.fondo, cancel) //nolint:contextcheck // deliberado: la copia muere con el proceso, no con la petición.
	terminar := func() { soltar(); cancel() }
	entregas := h.abrirEntregas(ctx, token, jobs)
	rt := h.egressRuntime(inst, log.From(ctx))
	rt.HTTP.Timeout = h.migrationTimeout
	dst, err := migrate.NewOutboundDestination(ctx, rt, inst, jobs[0])
	if err != nil {
		for i, j := range jobs {
			cerrarEntrega(entregas[i])
			_ = h.store.FinishMigrationJob(ctx, j.ID, domain.MigrationError, err.Error())
		}
		terminar()
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
		return
	}
	go func() {
		defer terminar()
		for i, j := range jobs {
			if entregas[i].err != nil {
				_ = h.store.FinishMigrationJob(ctx, j.ID, domain.MigrationError, entregas[i].err.Error())
				continue
			}
			err := migrate.RunEntregas(ctx, migrate.Deps{Dst: dst, Store: h.store}, j, entregas[i].resp)
			if ctx.Err() != nil {
				cierre, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				_ = h.store.FinishMigrationJob(cierre, j.ID, domain.MigrationError,
					"se interrumpió por un reinicio del servicio: vuelve a lanzarla, continúa donde se quedó")
				cancel()
			} else if err != nil {
				log.From(ctx).Warn("salida propia terminó con error", "job", j.ID, "error", err)
			}
		}
	}()
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"migrations": jobs})
}

type entregaAbierta struct {
	resp *http.Response
	err  error
}

// abrirEntregas pide A LA VEZ la entrega de cada contenido, mientras el token de la
// persona vale: cada producto lo comprueba al recibir la petición, y a partir de ahí
// da igual cuánto tarde en leerse (el mismo patrón que la copia del Hub).
func (h *Handlers) abrirEntregas(ctx context.Context, token string, jobs []domain.MigrationJob) []entregaAbierta {
	out := make([]entregaAbierta, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() {
			base := map[domain.MigrationDomain]string{
				domain.DomainDrive: h.driveBaseURL, domain.DomainContacts: h.contactsBaseURL,
				domain.DomainMail: h.mailBaseURL, domain.DomainCalendar: h.calendarBaseURL,
			}[j.Domain]
			if base == "" {
				out[i].err = fmt.Errorf("este despliegue no tiene %s configurado", j.Domain)
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/export?format=json", http.NoBody)
			if err != nil {
				out[i].err = err
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := clienteEntregas.Do(req)
			switch {
			case err != nil:
				out[i].err = errors.New("no se pudo leer el contenido de Sytue: el servicio no respondió")
			case resp.StatusCode != http.StatusOK:
				_ = resp.Body.Close()
				out[i].err = fmt.Errorf("no se pudo leer el contenido de Sytue (HTTP %d)", resp.StatusCode)
			default:
				out[i].resp = resp //nolint:bodyclose // lo cierra RunEntregas o cerrarEntrega.
			}
		})
	}
	wg.Wait()
	return out
}

func cerrarEntrega(e entregaAbierta) {
	if e.resp != nil {
		_ = e.resp.Body.Close()
	}
}

// clienteEntregas no tiene Timeout: el corte lo pone el contexto de la copia.
var clienteEntregas = &http.Client{}
