// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateHub "git.sytue.eu/sytue/one/apps/nucleo/hub"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// hubReceivers guarda el receptor de la entrada privada de cada repositorio abierto:
// Build lo crea con el plan y Mount lo arranca con la vista. Es la misma vida que el
// repositorio: al cerrar sesión o cambiar de cuenta, el plan lo retira.
type hubReceivers struct {
	mu   sync.Mutex
	byID map[*privateHub.Repository]*privateHub.Receiver
}

func (h *hubReceivers) put(repository *privateHub.Repository, receiver *privateHub.Receiver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byID == nil {
		h.byID = map[*privateHub.Repository]*privateHub.Receiver{}
	}
	h.byID[repository] = receiver
}

// get devuelve el receptor del repositorio, o nil (sin interfaz con un puntero nulo
// dentro) si el plan no lo creó.
func (h *hubReceivers) get(repository *privateHub.Repository) hubReceiverRunner {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.byID[repository]; r != nil {
		return r
	}
	return nil
}

func (h *hubReceivers) drop(repository *privateHub.Repository) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.byID, repository)
}

// hubEpoch es la época de la fuente de One en el servidor; vacía mientras es legacy.
func hubEpoch(state storage.AppMigrationState) string {
	if state.Mode == "legacy" {
		return ""
	}
	return state.Epoch
}

func hubPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite, consent *cifrado.IngressConsent, receivers *hubReceivers) (cifrado.ApplicationPlan[*privateHub.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateHub.Repository]
	if resources == nil || suite == nil || consent == nil || receivers == nil {
		return empty, cifrado.ErrPrivateResources
	}
	hubAPI := api.NewHub(suite.URL("hub"), resources.HTTP(), nil)
	source, err := privateHub.NewSource(hubAPI)
	if err != nil {
		return empty, err
	}
	rooms := api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil)
	plan, err := cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateHub.Repository]{
		Gateway:    api.NewDrive(suite.URL("drive"), resources.HTTP(), nil),
		Source:     source,
		Service:    rooms,
		New:        privateHub.New,
		Historical: privateHub.NewHistoricalSource,
	})
	if err != nil {
		return empty, err
	}
	return withHubReceiver(ctx, resources, plan, hubReceiverParts{consent: consent, receivers: receivers, control: hubAPI.PrivateIngress(), rooms: rooms, state: hubAPI.PrivateStorageState})
}

type hubReceiverParts struct {
	consent   *cifrado.IngressConsent
	receivers *hubReceivers
	control   ingress.HTTPControl
	rooms     ingress.HTTPAuthority
	state     func(context.Context, storage.Scope) (storage.AppMigrationState, error)
}

// withHubReceiver añade al plan de registros la entrada privada de avisos (ADR 0086),
// como Mail: alta de la clave de entrada de la época y trabajador que reclama, guarda
// y confirma. Va en las fuentes retenidas del plan, así que su diario se drena al
// desmontar y se retira (o se retiene) con la sesión. Un fallo de la entrada no
// impide abrir los avisos ya guardados: se dice en el registro y se reintenta en la
// próxima apertura; lo pendiente sigue esperando en el Hub.
func withHubReceiver(ctx context.Context, resources *cifrado.Resources[*mx.Client], plan cifrado.ApplicationPlan[*privateHub.Repository], parts hubReceiverParts) (cifrado.ApplicationPlan[*privateHub.Repository], error) {
	authority, err := parts.consent.Authority(ctx, resources)
	if err != nil {
		return cifrado.ApplicationPlan[*privateHub.Repository]{}, errors.Join(err, plan.Retire(ctx))
	}
	scope := resources.PrivateStorageScope()
	receiver, err := privateHub.NewReceiver(privateHub.ReceiverConfig{
		Context: ctx, Repository: plan.Repository, Authority: authority, Journal: resources.Journal,
		Lease: resources.WorkLease(), Device: resources.Devices.Isolated, Control: parts.control, Rooms: parts.rooms,
		Authorize: resources.Revalidate,
		Epoch: func(ctx context.Context) (string, error) {
			state, err := parts.state(ctx, scope)
			if err != nil {
				return "", err
			}
			return hubEpoch(state), nil
		},
	})
	if err != nil {
		return cifrado.ApplicationPlan[*privateHub.Repository]{}, errors.Join(err, plan.Retire(ctx))
	}
	retained, err := cifrado.NewRetainedSources(resources, receiver)
	if err != nil {
		receiver.Close() //nolint:contextcheck // Sin IO todavía: solo cierra su puerta.
		return cifrado.ApplicationPlan[*privateHub.Repository]{}, errors.Join(err, plan.Retire(ctx))
	}
	parts.receivers.put(plan.Repository, receiver)
	prepare, retire, repository := plan.Prepare, plan.Retire, plan.Repository
	plan.Prepare = func(ctx context.Context, r *privateHub.Repository) error {
		if err := prepare(ctx, r); err != nil {
			return err
		}
		if err := retained.Register(ctx); err != nil {
			return err
		}
		if err := receiver.Prepare(ctx); err != nil {
			slog.Warn("hub: entrada privada de avisos sin preparar; lo pendiente sigue en el Hub", "error", err)
		}
		return nil
	}
	plan.Retire = func(ctx context.Context) error {
		err := errors.Join(retained.Retire(ctx), retire(ctx))
		if err == nil {
			parts.receivers.drop(repository)
		}
		return err
	}
	return plan, nil
}

// hubReceiverRunner es lo que la vista arranca de privateHub.Receiver.
type hubReceiverRunner interface {
	Run(context.Context, func(ingress.PollResult, error)) error
}

// runHubReceiver arranca el receptor de la vista montada. Detach lo para (al
// desmontar, cerrar sesión o cambiar de cuenta) y Join espera a que termine.
func runHubReceiver(ctx context.Context, receiver hubReceiverRunner, refresh func()) (stop func(), join func(context.Context) error) {
	done := make(chan struct{})
	if receiver == nil {
		close(done)
		return func() {}, func(context.Context) error { return nil }
	}
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		err := receiver.Run(runCtx, func(result ingress.PollResult, _ error) {
			if result.Completed > 0 && runCtx.Err() == nil && refresh != nil {
				refresh()
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, privateHub.ErrReceiverNotReady) && runCtx.Err() == nil {
			slog.Warn("hub: el receptor de la entrada privada se ha parado", "error", err)
		}
	}()
	return cancel, func(ctx context.Context) error {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
