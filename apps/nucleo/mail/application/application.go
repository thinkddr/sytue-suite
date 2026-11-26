// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/fallo"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

// ResourceGuard retains the same signed application-session witness and broker
// registration on native clients and the scoped host lease in web clients.
type ResourceGuard interface {
	offline.WorkResourceGuard
	Context() context.Context
	WorkLease() offline.WorkLease
}
type Device interface {
	privateMail.Crypto
	ingress.Device
	ingress.EnrollmentDevice
	ingress.HTTPActionSigner
	StoragePublicationTarget() (storage.PublicationTarget, error)
}
type Source interface {
	privateMail.Source
	ProvisionAndBeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error)
	PrivateSubmissions(storage.Scope, string) (api.PrivateMailSubmissionTransport, error)
}
type Rooms interface {
	transfer.CutoverService
	ingress.HTTPAuthority
}
type Config struct {
	// Native SDK2 publication binding; historical SDK1 keeps its own phase policy.
	BindPublicationContext func(context.Context) (context.Context, error)
	Gateway                *api.Drive
	Metadata               privateMail.RoutingMetadata
	Usage                  privateMail.CipherUsageReader
	Files, LegacyFiles     storage.BoundFiles
	Source                 Source
	Control                ingress.HTTPControl
	Rooms                  Rooms
	Authority              matrixproof.Authority
	Journal                *offline.Store
	Isolated               Device
	Legacy                 privateMail.Crypto
	RequireLegacy          bool
	// Vault lo elige el anfitrión, nunca el módulo: el correo vive en la bóveda
	// post-cuántica (ADR 0087). Sin origen histórico ni corte de identidad de
	// Matrix; el correo retenido del servidor se importa directo en Isolated.
	Vault    bool
	CopyFile privateMail.CopyPrivateFile
	Invalid  error
}

// validMode: Matrix con origen histórico exige su almacén; la bóveda no admite
// ninguna pieza de Matrix (origen, ficheros ni copiador): no hay nada que migrar.
func (cfg Config) validMode() bool {
	if cfg.Vault {
		return !cfg.RequireLegacy && cfg.Legacy == nil && cfg.LegacyFiles == nil && cfg.CopyFile == nil
	}
	return !cfg.RequireLegacy || cfg.Legacy != nil
}

// Application is a deferred lifecycle holder, not a MailData
// implementation. The presenter can obtain its real repository only after
// source capture, complete identity cutover and receiver setup have succeeded.
type Application struct {
	resources  ResourceGuard
	cfg        Config
	life       context.Context
	scope      storage.Scope
	gate       *offline.WorkGate
	group      *offline.RetainedSources
	enrollment *ingress.Enrollment
	prepareMu  sync.Mutex
	mu         sync.Mutex
	core       *privateMail.Repository
	historical *privateMail.HistoricalSource
	cutover    *transfer.Cutover
	worker     *ingress.Worker
	ready      bool
}

var _ offline.PrivateWork = (*Application)(nil)

func New(ctx context.Context, resources ResourceGuard, cfg Config) (*Application, error) {
	if cfg.Invalid == nil {
		cfg.Invalid = privateMail.ErrRecord
	}
	if resources == nil || (cfg.Gateway == nil && cfg.Files == nil) || cfg.Source == nil || cfg.Control == nil || cfg.Rooms == nil || cfg.Journal == nil || !cfg.Journal.Encrypted() || cfg.Isolated == nil || !cfg.validMode() || resources.PrivateStorageScope().Product != "mail" {
		return nil, cfg.Invalid
	}
	if err := resources.Check(ctx); err != nil {
		return nil, err
	}
	scope := resources.PrivateStorageScope()
	if cfg.Authority.RequireActive(scope, cfg.Authority.Generation) != nil {
		return nil, cfg.Invalid
	}
	target, err := cfg.Isolated.StoragePublicationTarget()
	if err != nil {
		return nil, err
	}
	holder := &Application{resources: resources, cfg: cfg, life: ctx, scope: scope, gate: offline.NewWorkGateWithLease(ctx, resources.WorkLease())}
	enrollment, err := ingress.NewEnrollment(ingress.EnrollmentConfig{Context: ctx, Scope: scope, Target: target, Authority: cfg.Authority, Journal: cfg.Journal, Lease: resources.WorkLease(), Device: cfg.Isolated, Authorize: resources.Revalidate, Source: func(epoch string) (ingress.EnrollmentSource, error) { return holder.source(epoch) }, Rotate: cfg.Vault})
	if err != nil {
		return nil, err
	}
	holder.enrollment = enrollment
	group, err := offline.NewComposedSources(resources, holder, holder.inventory, cfg.Invalid, enrollment)
	if err != nil {
		enrollment.Close() //nolint:contextcheck // Construction has not admitted IO; Close only cancels this helper's lifetime.
		return nil, err
	}
	holder.group = group
	return holder, nil
}

func (r *Application) PrivateStorageScope() storage.Scope { return r.scope }
func (r *Application) inventory(ctx context.Context) (offline.PendingWork, error) {
	return privateMail.PendingForScope(ctx, r.scope, r.cfg.Journal)
}

func (r *Application) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.group.Pending(ctx)
}

func (r *Application) Drain(ctx context.Context) error {
	if err := r.gate.Drain(ctx); err != nil {
		return err
	}
	return r.group.Drain(ctx)
}

func (r *Application) Resume() error {
	if err := r.group.Resume(r.life); err != nil {
		return err
	}
	if err := r.gate.Resume(); err != nil {
		return errors.Join(err, r.group.Drain(r.life))
	}
	return nil
}
func (r *Application) Close() { r.gate.Close() }

// CloseAndRetain ends this live session after joining all original work. Every
// client-side reader is erased, while the encrypted WAL and its registrations
// remain available for authenticated recovery instead of being acknowledged.
func (r *Application) CloseAndRetain(ctx context.Context) error {
	r.mu.Lock()
	r.ready = false
	r.mu.Unlock()
	if err := r.gate.Drain(ctx); err != nil {
		return err
	}
	if err := r.group.CloseRetaining(ctx); err != nil {
		return err
	}
	r.Close() //nolint:contextcheck // Close only closes the gate and takes no ctx; teardown after the drain must not be cancellable
	return nil
}

func (r *Application) Retire(ctx context.Context) error {
	if err := r.Drain(ctx); err != nil {
		return err
	}
	return r.group.Retire(ctx)
}

func (r *Application) source(epoch string) (*ingress.HTTPSource, error) {
	return ingress.NewHTTPSource(ingress.HTTPSourceConfig{Scope: r.scope, Epoch: epoch, Control: r.cfg.Control, Authority: r.cfg.Rooms, Signer: r.cfg.Isolated, Lease: r.resources.WorkLease()})
}

func (r *Application) Prepare(parent context.Context) error {
	ctx, done, err := r.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	r.prepareMu.Lock()
	defer r.prepareMu.Unlock()
	r.mu.Lock()
	r.ready = false
	r.mu.Unlock()
	if err := r.resources.Revalidate(ctx); err != nil {
		return fallo.En("revalidar", err)
	}
	if err := r.group.Register(ctx); err != nil {
		return fallo.En("registrar", err)
	}
	caps, err := r.cfg.Source.PrivateStorageCapabilities(ctx, r.scope)
	if err != nil {
		return fallo.En("capacidades", err)
	}
	// Cada «no» con su nombre (paquete fallo): el servicio decía
	// `private_ingress_ready: false` a todas las cuentas y no había forma de verlo.
	switch {
	case caps.Validate(r.scope) != nil:
		return fallo.En("capacidades_invalidas", privateMail.ErrNotReady)
	case caps.Shared:
		return fallo.En("compartido", privateMail.ErrNotReady)
	case !caps.PrivateIngressReady:
		return fallo.En("entrada_no_lista", privateMail.ErrNotReady)
	}
	state := caps.LegacyMigrationState
	if r.cfg.Vault {
		if state, err = r.captureVault(ctx, caps); err != nil {
			return fallo.En("origen_boveda", fmt.Errorf("mail vault source: %w", err))
		}
	}
	if r.cfg.RequireLegacy {
		if err := r.prepareHistorical(ctx, caps); err != nil {
			return fallo.En("historico", fmt.Errorf("mail historical source: %w", err))
		}
		state = r.historical.RetainedState()
	}
	plan, err := r.enrollment.Ensure(ctx, state.Epoch)
	if err != nil {
		return fallo.En("inscripcion", fmt.Errorf("mail receiver enrollment: %w", err))
	}
	if err := r.prepareTarget(ctx, caps, state); err != nil {
		return fallo.En("destino", fmt.Errorf("mail private destination: %w", err))
	}
	if r.cfg.Vault {
		if err := r.core.ImportDirectSource(ctx, r.cfg.Source); err != nil {
			return fallo.En("importar", fmt.Errorf("mail vault import: %w", err))
		}
	} else if err := r.cutover.Prepare(ctx); err != nil {
		return fallo.En("traspaso_identidad", fmt.Errorf("mail identity cutover: %w", err))
	}
	if err := r.prepareWorker(ctx, state.Epoch, plan); err != nil {
		return fallo.En("receptor", fmt.Errorf("mail receiver setup: %w", err))
	}
	if err := r.resources.Revalidate(ctx); err != nil {
		return fallo.En("revalidar_final", fmt.Errorf("mail final session validation: %w", err))
	}
	r.mu.Lock()
	r.ready = true
	r.mu.Unlock()
	return nil
}

func (r *Application) configuration(caps storage.AppMigrationState, state storage.LegacyMigrationState, crypto privateMail.Crypto, files storage.BoundFiles) privateMail.Config {
	submission, _ := r.cfg.Source.PrivateSubmissions(r.scope, state.Epoch)
	publication := r.cfg.BindPublicationContext
	if crypto.StorageIdentityVersion() != storage.IsolatedIdentity {
		publication = nil
	}
	return privateMail.Config{Metadata: r.cfg.Metadata, Usage: r.cfg.Usage, BindPublicationContext: publication, Context: r.life, Scope: r.scope, Crypto: crypto, Gateway: r.cfg.Gateway, Files: files, Journal: r.cfg.Journal, WorkLease: r.resources.WorkLease(), MigrationState: state, MaxMessageBytes: caps.MaxBodyBytes, Submission: submission, AuthorizeSubmission: r.resources.Revalidate}
}

// captureVault fija la época del correo retenido con la bóveda como destino. Es
// el mismo Begin que el del origen histórico (precedido por el alta de la clave
// de entrada de esa época), sin repositorio de Matrix en medio. Una vez creado
// el repositorio, su época ya no cambia en esta sesión.
func (r *Application) captureVault(ctx context.Context, caps storage.AppMigrationState) (storage.LegacyMigrationState, error) {
	if r.core != nil {
		return r.core.MigrationState(), nil
	}
	source := mailIngressSource{Source: r.cfg.Source, invalid: r.cfg.Invalid, enrollment: r.enrollment, scope: r.scope}
	return privateMail.CaptureDirectSource(ctx, r.configuration(caps, caps.LegacyMigrationState, r.cfg.Isolated, r.cfg.Files), source)
}

func (r *Application) prepareHistorical(ctx context.Context, caps storage.AppMigrationState) error {
	if r.historical == nil {
		source := mailIngressSource{Source: r.cfg.Source, invalid: r.cfg.Invalid, enrollment: r.enrollment, scope: r.scope}
		previous, err := privateMail.OpenHistoricalSource(ctx, r.configuration(caps, caps.LegacyMigrationState, r.cfg.Legacy, r.cfg.LegacyFiles), source)
		if err != nil {
			return err
		}
		r.historical = previous
	}
	return r.group.Add(ctx, r.historical)
}

func (r *Application) prepareTarget(ctx context.Context, caps storage.AppMigrationState, state storage.LegacyMigrationState) error {
	if r.core == nil {
		target, err := privateMail.New(r.configuration(caps, state, r.cfg.Isolated, r.cfg.Files)) //nolint:contextcheck // Config retains resources lifetime; this preparation context is canceled when its gate ends.
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.core = target
		r.mu.Unlock()
	}
	if err := r.group.Add(ctx, r.core); err != nil {
		return err
	}
	if r.cfg.Vault {
		return nil
	}
	if r.cutover == nil {
		config := privateMail.IdentityCutoverConfig{Scope: r.scope, Service: r.cfg.Rooms, Legacy: r.historical, Authorize: r.resources.Revalidate}
		if r.historical != nil {
			config.CopyFile = r.cfg.CopyFile
			config.AuthorizeLegacy = r.authorizeHistoricalWrite
		}
		cutover, err := r.core.IdentityCutover(config)
		if err != nil {
			return err
		}
		r.cutover = cutover
	}
	return nil
}

func (r *Application) prepareWorker(ctx context.Context, epoch string, plan contract.KeyPlan) error {
	if r.worker == nil {
		source, err := r.source(epoch)
		if err != nil {
			return err
		}
		config := ingress.Config{Context: r.life, Scope: r.scope, Epoch: epoch, Journal: r.cfg.Journal, Lease: r.resources.WorkLease(), Source: source, Device: r.cfg.Isolated, Executor: privateMail.IngressExecutor{Repository: r.core}}
		if r.cfg.Vault {
			// La bóveda solo reclama lo cifrado para su clave: tras un traspaso o un
			// vaciado, lo cifrado para la anterior se queda en la cola, intacto.
			config.KeyID = plan.KeyID
		}
		worker, err := ingress.New(config)
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.worker = worker
		r.mu.Unlock()
	}
	return r.group.Add(ctx, r.worker)
}

func (r *Application) Initialize(parent context.Context) error {
	ctx, done, err := r.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	r.mu.Lock()
	core, ready := r.core, r.ready
	r.mu.Unlock()
	if core == nil || !ready {
		return privateMail.ErrNotReady
	}
	if err := core.Initialize(ctx); err != nil {
		r.mu.Lock()
		r.ready = false
		r.mu.Unlock()
		return err
	}
	return nil
}

func (r *Application) ReadyRepository(ctx context.Context) (*privateMail.Repository, error) {
	if err := r.resources.Check(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ready || r.core == nil {
		return nil, privateMail.ErrNotReady
	}
	return r.core, nil
}

func (r *Application) RunReceiver(ctx context.Context, changed func(ingress.PollResult, error)) error {
	if err := r.resources.Check(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	worker, ready, core := r.worker, r.ready, r.core
	r.mu.Unlock()
	if worker == nil || core == nil || !ready {
		return privateMail.ErrNotReady
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	outboundDone := make(chan error, 1)
	go func() {
		outboundDone <- core.RunOutbound(workerCtx, func(status api.MailOutgoingStatus, err error) {
			if changed != nil && err == nil && (status.Receipt.Accepted() || status.Receipt.Canceled) {
				changed(ingress.PollResult{Completed: 1}, nil)
			}
		})
	}()
	inboundErr := worker.Run(workerCtx, changed)
	cancel()
	return errors.Join(inboundErr, <-outboundDone)
}

type mailIngressSource struct {
	Source
	invalid    error
	enrollment *ingress.Enrollment
	scope      storage.Scope
}

func (s mailIngressSource) SnapshotEpoch(ctx context.Context, scope storage.Scope) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if scope != s.scope {
		return "", s.invalid
	}
	return transfer.InitialSourceEpoch(scope)
}

func (s mailIngressSource) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	if scope != s.scope {
		return storage.LegacyMigrationState{}, s.invalid
	}
	if _, err := s.enrollment.Ensure(ctx, epoch); err != nil {
		return storage.LegacyMigrationState{}, err
	}
	return s.ProvisionAndBeginPrivateStorageMigration(ctx, scope, epoch)
}

// Every fragment/root revalidates the original session and live Matrix phase.
func (r *Application) authorizeHistoricalWrite(ctx context.Context) error {
	if err := r.resources.Revalidate(ctx); err != nil {
		return err
	}
	state, err := r.cfg.Rooms.MatrixStorageMigration(ctx, r.scope)
	if err != nil {
		return err
	}
	if state.Validate(r.scope) != nil || state.Phase != "legacy" {
		return r.cfg.Invalid
	}
	return ctx.Err()
}
