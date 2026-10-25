// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

const personalCapacityTimeout = 2500 * time.Millisecond

type personalCapacity struct {
	Connected bool `json:"connected"`
	objectstore.Capacity
}

// InternalPersonalStorageCapacity devuelve sólo cifras del actor indicado por
// Drive, nunca credenciales ni inventario de otras personas del tenant. Esta
// ruta sólo se monta detrás de requireInternal y SinPasarPorElIngress.
func (h *Handlers) InternalPersonalStorageCapacity(w http.ResponseWriter, r *http.Request) {
	t, err := tenantDe(r)
	actor := r.URL.Query().Get("actor")
	if err != nil || strings.TrimSpace(actor) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tenant and actor are required"))
		return
	}
	ctx, cancel := context.WithTimeout(tenancy.Into(r.Context(), t), personalCapacityTimeout)
	defer cancel()
	ctx = authn.Into(ctx, authn.Claims{Subject: actor, Tenant: t.Slug})
	instances, err := h.store.ListExternalChatInstances(ctx, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	disabled, err := h.store.DisabledIntegrations(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	out := personalStorageCapacity(ctx, instances, disabled, func(ctx context.Context, inst domain.AppInstance) (objectstore.Capacity, error) {
		// Se comprueba otra vez al cargar: el listado no delega la credencial.
		if err := h.store.RequireExternalChatActor(ctx, inst.ID, actor); err != nil {
			return objectstore.Capacity{}, err
		}
		conn, _, err := h.oauthConnection(ctx, inst.ID, esPlantillaDeAlmacen)
		if err != nil {
			return objectstore.Capacity{}, err
		}
		rt := h.egressRuntime(conn, log.From(ctx))
		reader, err := objectstore.ProviderCapacityReader(personalStorageProvider(conn.TemplateKey), rt.HTTP,
			func(context.Context) (string, error) { return conn.Secrets[store.SecretKeyBearer], nil })
		if err != nil {
			return objectstore.Capacity{}, err
		}
		return reader.Capacity(ctx)
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, out)
}

// La lista ya está filtrada por tenant y actor por el store. El presupuesto es
// global y sólo hay tres consultas simultáneas, aunque haya muchas cuentas.
func personalStorageCapacity(ctx context.Context, instances []domain.AppInstance, disabled map[string]bool,
	read func(context.Context, domain.AppInstance) (objectstore.Capacity, error),
) personalCapacity {
	accounts := make([]domain.AppInstance, 0, len(instances))
	seen := make(map[string]bool, len(instances))
	for _, inst := range instances {
		provider := personalStorageProvider(inst.TemplateKey)
		if inst.ID == "" || seen[inst.ID] || provider == "" || disabled[provider] || inst.Status != domain.StatusActive || !inst.HasSecrets {
			continue
		}
		seen[inst.ID] = true
		accounts = append(accounts, inst)
	}
	if len(accounts) == 0 {
		return personalCapacity{}
	}
	capacities := make([]objectstore.Capacity, len(accounts))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(3, len(accounts)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if capacity, err := read(ctx, accounts[i]); err == nil {
					capacities[i] = capacity
				}
			}
		})
	}
enqueue:
	for i := range accounts {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break enqueue
		}
	}
	close(jobs)
	workers.Wait()
	return personalCapacity{Connected: true, Capacity: objectstore.SumCapacities(capacities...)}
}
