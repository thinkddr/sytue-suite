// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// ErrReceiverNotReady: la fuente de One sigue en legacy (no hay nada que recibir) o
// el receptor todavía no se ha preparado.
var ErrReceiverNotReady = errors.New("hub: private notice receiver is not ready")

// ReceiverDevice es el dispositivo aislado del anfitrión: el mismo que usa Mail.
type ReceiverDevice interface {
	ingress.Device
	ingress.EnrollmentDevice
	ingress.HTTPActionSigner
	StoragePublicationTarget() (storage.PublicationTarget, error)
}

// ReceiverConfig son las piezas del anfitrión. Epoch lee del servidor la época de la
// fuente de One: vacía mientras es `legacy`.
type ReceiverConfig struct {
	Context    context.Context
	Repository *Repository
	Authority  matrixproof.Authority
	Journal    *offline.Store
	Lease      offline.WorkLease
	Device     ReceiverDevice
	Control    ingress.HTTPControl
	Rooms      ingress.HTTPAuthority
	Authorize  func(context.Context) error
	Epoch      func(context.Context) (string, error)
}

// Receiver es la entrada privada de avisos de One en un aparato (ADR 0086): da de
// alta la clave de entrada de la época de la persona y reclama, guarda
// (IngressExecutor) y confirma lo que el Hub le dejó cifrado. Es el mismo alta y el
// mismo trabajador que Mail (mail/application); es trabajo privado del anfitrión
// (offline.PrivateWork): su diario se drena, se retiene o se retira con la sesión.
type Receiver struct {
	cfg        ReceiverConfig
	scope      storage.Scope
	enrollment *ingress.Enrollment
	mu         sync.Mutex
	epoch      string
	worker     *ingress.Worker
}

var _ offline.PrivateWork = (*Receiver)(nil)

func NewReceiver(cfg ReceiverConfig) (*Receiver, error) {
	if cfg.Context == nil || cfg.Repository == nil || cfg.Journal == nil || cfg.Lease == nil || cfg.Device == nil || cfg.Control == nil || cfg.Rooms == nil || cfg.Authorize == nil || cfg.Epoch == nil {
		return nil, contract.ErrContract
	}
	scope := cfg.Repository.scope
	target, err := cfg.Device.StoragePublicationTarget()
	if err != nil {
		return nil, err
	}
	r := &Receiver{cfg: cfg, scope: scope}
	enrollment, err := ingress.NewEnrollment(ingress.EnrollmentConfig{
		Context: cfg.Context, Scope: scope, Target: target, Authority: cfg.Authority, Journal: cfg.Journal,
		Lease: cfg.Lease, Device: cfg.Device, Authorize: cfg.Authorize,
		Source: func(epoch string) (ingress.EnrollmentSource, error) { return r.source(epoch) },
	})
	if err != nil {
		return nil, err
	}
	r.enrollment = enrollment
	return r, nil
}

func (r *Receiver) source(epoch string) (*ingress.HTTPSource, error) {
	return ingress.NewHTTPSource(ingress.HTTPSourceConfig{Scope: r.scope, Epoch: epoch, Control: r.cfg.Control, Authority: r.cfg.Rooms, Signer: r.cfg.Device, Lease: r.cfg.Lease})
}

// Prepare da de alta la clave de la época actual y deja listo su trabajador. Con la
// fuente en legacy no hace nada. Una época distinta de la ya preparada (se soltó la
// fuente y se volvió a congelar) no reutiliza el trabajador anterior.
func (r *Receiver) Prepare(ctx context.Context) error {
	epoch, err := r.cfg.Epoch(ctx)
	if err != nil {
		return err
	}
	if epoch == "" {
		return nil
	}
	r.mu.Lock()
	ready := r.worker != nil && r.epoch == epoch
	previous := r.worker
	r.mu.Unlock()
	if ready {
		return nil
	}
	if previous != nil {
		return ErrReceiverNotReady
	}
	if _, err := r.enrollment.Ensure(ctx, epoch); err != nil {
		return err
	}
	source, err := r.source(epoch)
	if err != nil {
		return err
	}
	worker, err := ingress.New(ingress.Config{Context: r.cfg.Context, Scope: r.scope, Epoch: epoch, Journal: r.cfg.Journal, Lease: r.cfg.Lease, Source: source, Device: r.cfg.Device, Executor: IngressExecutor{Repository: r.cfg.Repository}})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.worker, r.epoch = worker, epoch
	r.mu.Unlock()
	return nil
}

// Run reclama y confirma hasta que ctx acabe (al desmontar, cerrar sesión o cambiar
// de cuenta) o el trabajador se cierre. changed se llama tras cada vuelta con trabajo.
func (r *Receiver) Run(ctx context.Context, changed func(ingress.PollResult, error)) error {
	r.mu.Lock()
	worker := r.worker
	r.mu.Unlock()
	if worker == nil {
		return ErrReceiverNotReady
	}
	return worker.Run(ctx, changed)
}

func (r *Receiver) PrivateStorageScope() storage.Scope { return r.scope }

func (r *Receiver) members() []offline.PrivateWork {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []offline.PrivateWork{r.enrollment}
	if r.worker != nil {
		out = append(out, r.worker)
	}
	return out
}

func (r *Receiver) Pending(ctx context.Context) (offline.PendingWork, error) {
	out := offline.PendingWork{Scope: r.scope}
	for _, member := range r.members() {
		pending, err := member.Pending(ctx)
		if err != nil {
			return out, err
		}
		out.Count += pending.Count
	}
	return out, nil
}

func (r *Receiver) Drain(ctx context.Context) error {
	var err error
	for _, member := range r.members() {
		err = errors.Join(err, member.Drain(ctx))
	}
	return err
}

func (r *Receiver) Resume() error {
	var err error
	for _, member := range r.members() {
		err = errors.Join(err, member.Resume())
	}
	return err
}

func (r *Receiver) Close() {
	for _, member := range r.members() {
		member.Close()
	}
}
