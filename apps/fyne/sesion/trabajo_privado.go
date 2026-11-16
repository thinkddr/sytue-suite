// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrTrabajoPendiente = errors.New("sesion: private work must finish or be explicitly discarded before logout")

// ErrTransicionPrivada makes concurrent local transitions and registrations
// retryable. It does not coordinate transitions made by another process.
var ErrTransicionPrivada = errors.New("sesion: private transition already in progress")

type FaseTransicionPrivada string

const (
	TransicionGuardando                     FaseTransicionPrivada = "saving"
	TransicionPeticionEnviada               FaseTransicionPrivada = "request_sent"
	TransicionPreflightAbortado             FaseTransicionPrivada = "preflight_aborted"
	TransicionEfectoRemotoIncierto          FaseTransicionPrivada = "remote_unknown"
	TransicionRemotaConfirmadaAccesoFallido FaseTransicionPrivada = "remote_confirmed_auth_failed"
	TransicionCompletada                    FaseTransicionPrivada = "completed"
)

type EventoTransicionPrivada struct {
	Ciclo    uint64
	Revision uint64
	Fase     FaseTransicionPrivada
}

// PrepararTransicionPrivada freezes editing and verifies durable visible state
// using the original work. The initial event carries its cycle and saving phase.
// The returned notifier must enqueue without blocking, reject obsolete UI
// lifetimes and order events by cycle/revision. Cancel returns false after the
// remote effect boundary. Neither callback runs under a Suite/session OS lock.
// If the presenter was frozen, prepare returns its notifier even on error so
// that the preflight-aborted event can restore that same presenter.
type PrepararTransicionPrivada func(context.Context, func() bool, EventoTransicionPrivada) (notificar func(EventoTransicionPrivada), err error)

type privateTransitionParticipant struct {
	work    offline.PrivateWork
	witness auth.TestigoTrabajo
	prepare PrepararTransicionPrivada
}

// RegistrarPreparadorTransicionPrivada binds a presenter to an already
// registered work owner and its original persisted login witness. The caller
// must register before making the presenter editable and unregister after it
// can no longer accept edits. prepare runs outside all Suite and session locks.
func (s *Suite) RegistrarPreparadorTransicionPrivada(work offline.PrivateWork, witness auth.TestigoTrabajo, prepare PrepararTransicionPrivada) (func(), error) {
	if work == nil || prepare == nil || !reflect.ValueOf(work).Comparable() || s.almacen == nil || witness.Almacen != s.almacen.Identificador() {
		return nil, auth.ErrTrabajoSesion
	}
	if err := witness.Validar(); err != nil {
		return nil, err
	}
	identity := s.Sesion.SnapshotIdentity()
	scope := work.PrivateStorageScope()
	if !scope.Valid() || scope.Subject != identity.Subject || scope.Tenant != identity.Organization || witness.Acceso.Identidad != identity.IdentidadSesion() {
		return nil, auth.ErrTrabajoSesion
	}
	s.privateWorkMu.Lock()
	if s.privateTransitionBusyLocked() {
		s.privateWorkMu.Unlock()
		return nil, ErrTransicionPrivada
	}
	registered := false
	for _, existing := range s.privateWork {
		if reflect.ValueOf(existing.work).Comparable() && existing.work == work && existing.witness == witness {
			registered = true
			break
		}
	}
	if !registered {
		s.privateWorkMu.Unlock()
		return nil, auth.ErrTrabajoSesion
	}
	if s.privateTransitions == nil {
		s.privateTransitions = make(map[uint64]privateTransitionParticipant)
	}
	s.privateTransitionNext++
	id := s.privateTransitionNext
	s.privateTransitions[id] = privateTransitionParticipant{work: work, witness: witness, prepare: prepare}
	s.privateWorkMu.Unlock()
	unregister := func() {
		s.privateWorkMu.Lock()
		delete(s.privateTransitions, id)
		s.privateWorkMu.Unlock()
	}
	if err := witness.Validar(); err != nil {
		unregister()
		return nil, err
	}
	return unregister, nil
}

// privateTransitionBusyLocked requires privateWorkMu. An unresolved failed
// adoption keeps every later transition and registration fenced.
func (s *Suite) privateTransitionBusyLocked() bool {
	return s.privateTransitionActive || s.privateTransitionUnresolved
}

// EstadoTransicionPrivada returns the latest non-secret status of a cycle. A
// caller must discard a result from an earlier cycle or presenter.
func (s *Suite) EstadoTransicionPrivada(ciclo uint64) (EventoTransicionPrivada, bool) {
	s.privateWorkMu.Lock()
	defer s.privateWorkMu.Unlock()
	return s.privateTransitionLatest, s.privateTransitionLatest.Ciclo == ciclo && ciclo != 0
}

type transitionPrepared struct {
	id      uint64
	witness auth.TestigoTrabajo
	notify  func(EventoTransicionPrivada)
}

type privateTransition struct {
	suite    *Suite
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	sent     bool
	finished bool
	cycle    uint64
	revision uint64
	prepared []transitionPrepared
}

func (t *privateTransition) registered(id uint64) bool {
	s := t.suite
	s.privateWorkMu.Lock()
	defer s.privateWorkMu.Unlock()
	participant, ok := s.privateTransitions[id]
	if !ok {
		return false
	}
	for _, work := range s.privateWork {
		if reflect.ValueOf(work.work).Comparable() && work.work == participant.work && work.witness == participant.witness {
			return true
		}
	}
	return false
}

func (t *privateTransition) event(phase FaseTransicionPrivada) EventoTransicionPrivada {
	t.revision++
	event := EventoTransicionPrivada{Ciclo: t.cycle, Revision: t.revision, Fase: phase}
	t.suite.privateWorkMu.Lock()
	t.suite.privateTransitionLatest = event
	t.suite.privateWorkMu.Unlock()
	return event
}

func (t *privateTransition) broadcast(event EventoTransicionPrivada) {
	for _, p := range t.prepared {
		valid := p.witness.ValidarCierre()
		if event.Fase == TransicionPreflightAbortado {
			valid = p.witness.Validar()
		}
		if p.notify != nil && t.registered(p.id) && valid == nil && t.suite.Sesion.SnapshotIdentity().IdentidadSesion() == p.witness.Acceso.Identidad {
			p.notify(event)
		}
	}
}

// cancelar and empezarEnvio share a lock. A successful UI cancellation always
// precedes the send boundary; external context cancellation after it is uncertain.
func (t *privateTransition) cancelar() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sent || t.finished {
		return false
	}
	t.cancel()
	return true
}

func (t *privateTransition) empezarEnvio() error {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return ErrTransicionPrivada
	}
	if err := t.ctx.Err(); err != nil {
		t.mu.Unlock()
		return err
	}
	if t.sent {
		t.mu.Unlock()
		return nil
	}
	t.sent = true
	t.mu.Unlock()
	t.broadcast(t.event(TransicionPeticionEnviada))
	return nil
}

func (t *privateTransition) fallo(remotoConfirmado bool) {
	t.mu.Lock()
	sent := t.sent
	t.finished = true
	t.mu.Unlock()
	phase := TransicionPreflightAbortado
	if sent {
		phase = TransicionEfectoRemotoIncierto
	}
	if remotoConfirmado {
		phase = TransicionRemotaConfirmadaAccesoFallido
	}
	t.broadcast(t.event(phase))
	t.suite.privateWorkMu.Lock()
	t.suite.privateTransitionActive = false
	t.suite.privateWorkMu.Unlock()
}

func (t *privateTransition) completada() {
	t.mu.Lock()
	t.finished = true
	t.mu.Unlock()
	t.broadcast(t.event(TransicionCompletada))
	t.suite.privateWorkMu.Lock()
	t.suite.privateTransitionActive = false
	t.suite.privateWorkMu.Unlock()
}

// prepararTransicionPrivada coordinates this process only; another process may
// invalidate the original witness before this callback is able to run.
func (s *Suite) prepararTransicionPrivada(ctx context.Context, cancel context.CancelFunc, expected *auth.TestigoTrabajo) (*privateTransition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.privateWorkMu.Lock()
	if s.privateTransitionBusyLocked() {
		s.privateWorkMu.Unlock()
		return nil, ErrTransicionPrivada
	}
	s.privateTransitionActive = true
	s.privateTransitionCycle++
	cycle := s.privateTransitionCycle
	ids := make([]uint64, 0, len(s.privateTransitions))
	for id, participant := range s.privateTransitions {
		if expected == nil || participant.witness == *expected {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	type participantAt struct {
		id    uint64
		value privateTransitionParticipant
	}
	participants := make([]participantAt, 0, len(ids))
	for _, id := range ids {
		participants = append(participants, participantAt{id: id, value: s.privateTransitions[id]})
	}
	s.privateWorkMu.Unlock()
	t := &privateTransition{suite: s, ctx: ctx, cancel: cancel, cycle: cycle}
	initial := t.event(TransicionGuardando)
	for _, entry := range participants {
		participant := entry.value
		if err := ctx.Err(); err != nil {
			t.fallo(false)
			return nil, err
		}
		if !t.registered(entry.id) {
			t.fallo(false)
			return nil, ErrTransicionPrivada
		}
		if err := participant.witness.Validar(); err != nil || s.Sesion.SnapshotIdentity().IdentidadSesion() != participant.witness.Acceso.Identidad {
			t.fallo(false)
			return nil, errors.Join(auth.ErrTrabajoSesion, err)
		}
		notify, err := participant.prepare(ctx, t.cancelar, initial)
		t.prepared = append(t.prepared, transitionPrepared{id: entry.id, witness: participant.witness, notify: notify})
		if err == nil && (!t.registered(entry.id) || notify == nil) {
			err = ErrTransicionPrivada
		}
		if err == nil {
			err = participant.witness.Validar()
		}
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			t.fallo(false)
			return nil, err
		}
	}
	return t, nil
}

type privateWorkRegistration struct {
	work    offline.PrivateWork
	witness auth.TestigoTrabajo
}

func (s *Suite) TestigoTrabajo(ctx context.Context, scope storage.Scope) (auth.TestigoTrabajo, error) {
	if s.almacen == nil || !scope.Valid() {
		return auth.TestigoTrabajo{}, auth.ErrTrabajoSesion
	}
	identity := s.Sesion.SnapshotIdentity()
	if identity.Subject != scope.Subject || identity.Organization != scope.Tenant {
		return auth.TestigoTrabajo{}, auth.ErrTrabajoSesion
	}
	witness, err := s.almacen.TestigoTrabajo(ctx)
	if err != nil || witness.Acceso.Identidad != identity.IdentidadSesion() {
		return auth.TestigoTrabajo{}, errors.Join(auth.ErrTrabajoSesion, err)
	}
	return witness, nil
}

// RegistrarTrabajoPrivado registers a fully bound native repository. Its factory
// supplies the persistent native WorkLease before starting any repository IO.
// The returned function removes only this registration, never a later cycle.
func (s *Suite) RegistrarTrabajoPrivado(work offline.PrivateWork, witness auth.TestigoTrabajo) (func(), error) {
	if work == nil || s.almacen == nil || witness.Almacen != s.almacen.Identificador() {
		return nil, auth.ErrTrabajoSesion
	}
	scope := work.PrivateStorageScope()
	identity := s.Sesion.SnapshotIdentity()
	if !scope.Valid() || scope.Subject != identity.Subject || scope.Tenant != identity.Organization || witness.Acceso.Identidad != identity.IdentidadSesion() {
		return nil, auth.ErrTrabajoSesion
	}
	if err := witness.Validar(); err != nil {
		return nil, err
	}
	s.privateWorkMu.Lock()
	defer s.privateWorkMu.Unlock()
	if s.privateTransitionBusyLocked() {
		return nil, ErrTransicionPrivada
	}
	if s.privateWork == nil {
		s.privateWork = make(map[uint64]privateWorkRegistration)
	}
	s.privateWorkNext++
	id := s.privateWorkNext
	s.privateWork[id] = privateWorkRegistration{work, witness}
	return func() { s.privateWorkMu.Lock(); delete(s.privateWork, id); s.privateWorkMu.Unlock() }, nil
}

func (s *Suite) trabajosDe(witness auth.TestigoTrabajo) []offline.PrivateWork {
	s.privateWorkMu.Lock()
	defer s.privateWorkMu.Unlock()
	out := make([]offline.PrivateWork, 0, len(s.privateWork))
	for _, registered := range s.privateWork {
		if registered.witness == witness {
			out = append(out, registered.work)
		}
	}
	return out
}

func (s *Suite) pendientesPrivados(ctx context.Context, identity auth.IdentitySnapshot) (int, error) {
	dirs, err := s.directoriosAlmacenPrivado(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, dir := range dirs {
		// An app can own its native scope and a Drive-consumer scope. Account and
		// product remain in the owner hash; enumeration never decrypts their values.
		for _, product := range dir.productos {
			namespaces := append(storage.PrivateJournalNamespaces(product), storage.PendingUploadsNamespace)
			owner := identity.Organization + "\x00" + identity.Subject + "\x00" + product
			count, err := offline.CountDirectoryOwner(dir.ruta, owner, namespaces...)
			if err != nil {
				return total, err
			}
			total += count
		}
	}
	return total, nil
}

// PendientesSalida includes closed native apps, without decrypting their staged
// blocks. A read error remains an error; it is never permission to erase work.
func (s *Suite) PendientesSalida() (int, error) { return s.PendientesSalidaCon(context.Background()) }

// PendientesSalidaCon retains cancellation for native callers that already own a lifetime.
func (s *Suite) PendientesSalidaCon(ctx context.Context) (int, error) {
	var count int
	var err error
	if s.Sesion == nil {
		count, err = s.contarAlmacenPrivado(ctx, storage.AllPrivateJournalNamespaces()...)
	} else {
		identity := s.Sesion.SnapshotActionIdentity().Identity
		count, err = s.pendientesPrivados(ctx, identity)
		if err == nil {
			extra, native, nativeErr := s.pendientesTrabajoNativo(identity)
			if nativeErr != nil {
				return count, nativeErr
			}
			if native {
				if identity.Organization == "" {
					count = max(count, extra)
				} else {
					count += extra
				}
			}
		}
	}
	if err != nil {
		return count, err
	}
	identity := auth.IdentitySnapshot{}
	if s.Sesion != nil {
		identity = s.Sesion.SnapshotActionIdentity().Identity
	}
	legacy, err := s.pendientesLegados(ctx, identity)
	return count + legacy, err
}

func (s *Suite) SalirConError() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.salirProtegido(ctx, false)
}

// SalirDescartando is used only by the explicit discard action. Its lease and
// witness remain the same throughout; no WAL from another account is discarded.
func (s *Suite) SalirDescartando(ctx context.Context) error { return s.salirProtegido(ctx, true) }

type privateShutdown struct {
	suite     *Suite
	identity  auth.IdentitySnapshot
	witness   auth.TestigoTrabajo
	works     []offline.PrivateWork
	committed bool
	native    *ajustes.TrabajoNativo
	scopes    []storage.Scope
	discard   bool
}

func (s *Suite) salirProtegido(ctx context.Context, discard bool) error {
	identity := s.Sesion.SnapshotIdentity()
	available, err := ajustes.EstadoTrabajoSesionCompartido()
	if err != nil {
		return err
	}
	if s.almacen == nil || identity.IdentidadSesion() == "" || !available {
		count, err := s.contarAlmacenPrivado(ctx, storage.AllPrivateJournalNamespaces()...)
		if err != nil || count != 0 {
			return errors.Join(ErrTrabajoPendiente, err)
		}
		return s.salirClasico(ctx)
	}
	witness, err := s.almacen.TestigoTrabajo(ctx)
	if err != nil {
		return err
	}
	return s.salirProtegidoConTestigo(ctx, discard, witness)
}

//nolint:gocognit // la salida protegida es una secuencia ordenada (testigo, pendientes, revocaciones, borrado); partirla esconde el orden que la hace segura.
func (s *Suite) salirProtegidoConTestigo(ctx context.Context, discard bool, witness auth.TestigoTrabajo) error {
	if s.almacen == nil || witness.Almacen != s.almacen.Identificador() {
		return auth.ErrTrabajoSesion
	}
	identity := s.Sesion.SnapshotIdentity()
	if witness.Acceso.Identidad != identity.IdentidadSesion() {
		return auth.ErrTrabajoSesion
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	transition, err := s.prepararTransicionPrivada(ctx, cancel, &witness)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			transition.completada()
		} else {
			transition.fallo(false)
		}
	}()
	shutdown := privateShutdown{suite: s, identity: identity, witness: witness, discard: discard}
	defer shutdown.abort(ctx)
	release, err := shutdown.prepare(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := shutdown.checkPending(ctx, discard); err != nil {
		return err
	}
	if err := transition.empezarEnvio(); err != nil {
		return err
	}
	// Revocations and persistence run after every repository released its shared
	// lease. A failure keeps journals and key adapters available to retry.
	s.olvidarDespertares(context.Background(), false) //nolint:contextcheck // as in salir: device revocation must not be abandoned because the logout context is cancelled.
	if a := ajustes.Leer(); a.DespertarWidgets || a.DespertarDe != "" {
		//nolint:contextcheck // as in salir: clearing the wake-up consent must finish even if the logout context is cancelled.
		if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.DespertarWidgets, a.DespertarDe = false, "" }); err != nil {
			slog.Warn("sesion: no se ha podido apagar el despertar de los widgets", "error", err)
		}
	}
	if err := s.revocarCierrePrivado(ctx, witness, identity); err != nil {
		return err
	}
	if err := s.Sesion.LogoutConTestigoConLimpieza(ctx, s.almacen, witness, func() error {
		if discard {
			if shutdown.native != nil {
				for _, scope := range shutdown.scopes {
					if err := descartarAmbitoPrivadoUID(ctx, scope); err != nil {
						return err
					}
				}
			}
			if err := s.discardPrivateOwner(ctx, identity); err != nil {
				return err
			}
		}
		return s.vaciarAlmacenCon(ctx)
	}); err != nil {
		return err
	}
	shutdown.committed = true
	committed = true
	// Repository callbacks run after releasing session locks. No cleanup capable
	// of erasing a later login's data remains after this point.
	for _, work := range shutdown.works {
		work.Close()
	}
	s.mu.Lock()
	s.generacionSesion = ""
	s.mu.Unlock()
	s.aplicarSesionCerrada(ctx)
	return nil
}

func (shutdown *privateShutdown) abort(parent context.Context) {
	if shutdown.committed {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer cancel()
	if shutdown.witness.Pausar(ctx, false) == nil {
		for _, work := range shutdown.works {
			_ = work.Resume()
		}
	}
}

func (shutdown *privateShutdown) prepare(ctx context.Context) (func(), error) {
	if err := shutdown.witness.Pausar(ctx, true); err != nil {
		return nil, err
	}
	shutdown.works = shutdown.suite.trabajosDe(shutdown.witness)
	for _, work := range shutdown.works {
		if err := work.Drain(ctx); err != nil {
			return nil, err
		}
	}
	control, native, err := ajustes.TrabajoSesionNativo(ctx, shutdown.witness.Almacen, shutdown.witness.Acceso)
	if err != nil {
		return nil, err
	}
	if !native {
		return shutdown.witness.Exclusivo(ctx)
	}
	shutdown.native = control
	shutdown.scopes = shutdown.ambitosPropios()
	release, err := control.PrepararCierre(ctx, shutdown.scopes, metadatosPrivadosUID(), shutdown.discard)
	if errors.Is(err, ajustes.ErrTrabajoNativoPendiente) {
		return nil, errors.Join(ErrTrabajoPendiente, err)
	}
	return release, err
}

func (shutdown *privateShutdown) ambitosPropios() []storage.Scope {
	seen := map[storage.Scope]bool{}
	scopes := make([]storage.Scope, 0, len(storage.Products()))
	add := func(scope storage.Scope) {
		if scope.Valid() && !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	for _, work := range shutdown.works {
		add(work.PrivateStorageScope())
	}
	if shutdown.identity.Organization != "" {
		for _, product := range storage.Products() {
			add(storage.Scope{Tenant: shutdown.identity.Organization, Subject: shutdown.identity.Subject, Product: product})
		}
	}
	// A session without an active organization does not invent a storage tenant.
	// The broker still accounts for every UID; resolved scopes come from work.
	return scopes
}

func (shutdown *privateShutdown) checkPending(ctx context.Context, discard bool) error {
	for _, work := range shutdown.works {
		pending, err := work.Pending(ctx)
		if err != nil {
			return err
		}
		if pending.Scope != work.PrivateStorageScope() {
			return auth.ErrTrabajoSesion
		}
		if pending.Count > 0 && !discard {
			return ErrTrabajoPendiente
		}
	}
	pending, err := shutdown.suite.pendientesPrivados(ctx, shutdown.identity)
	if err != nil {
		return err
	}
	if pending > 0 && !discard {
		return ErrTrabajoPendiente
	}
	legacy, err := shutdown.suite.pendientesLegados(ctx, shutdown.identity)
	if err != nil {
		return err
	}
	if legacy > 0 && !discard {
		return ErrTrabajoPendiente
	}
	return nil
}

func (s *Suite) discardPrivateOwner(ctx context.Context, identity auth.IdentitySnapshot) error {
	dirs, err := s.directoriosAlmacenPrivado(ctx)
	if err != nil {
		return err
	}
	moved, err := s.cacheReubicada(ctx)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		// An app can own its native scope and a Drive-consumer scope. Account and
		// product remain in the owner hash; enumeration never decrypts their values.
		for _, product := range dir.productos {
			namespaces := append(storage.PrivateJournalNamespaces(product), storage.PrivateStagingNamespaces(product)...)
			namespaces = append(namespaces, storage.PendingUploadsNamespace)
			owner := identity.Organization + "\x00" + identity.Subject + "\x00" + product
			if err := offline.DiscardDirectoryOwnerContext(ctx, dir.ruta, owner, namespaces...); err != nil {
				return err
			}
		}
		if moved {
			if err := offline.DiscardDirectoryOwnerContext(ctx, dir.ruta, identity.Subject+"/"+identity.Organization, notas.EspacioCapturas, notas.EspacioBorradores); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Suite) revocarCierrePrivado(ctx context.Context, witness auth.TestigoTrabajo, identity auth.IdentitySnapshot) error {
	if s.privateRevoke != nil {
		return s.privateRevoke(ctx, witness, identity)
	}
	if err := witness.ValidarCierre(); err != nil {
		return err
	}
	intentCtx := s.Sesion.WithAuthorizationIntent(ctx)
	intent := auth.RequestAuthorizationIntent(intentCtx)
	if intent.Identity() != identity {
		return auth.ErrTrabajoSesion
	}
	bound := s.autorizacionDriveSistema(identity)
	current := bound.vigente
	bound.vigente = func(job context.Context) bool { return witness.ValidarCierre() == nil && current(job) }
	fixed, err := bound.AccessToken(intentCtx)
	if err != nil {
		return err
	}
	account := api.NewAccount(s.URL("id"), tokenFijo(fixed), nil)
	raw, err := auth.LeerSecreto(CuentaArchivos)
	if err != nil {
		return err
	}
	defer clear(raw)
	if err := witness.ValidarCierre(); err != nil {
		return err
	}
	if _, subject, _ := strings.Cut(string(raw), " "); len(raw) > 0 && subject == identity.Subject {
		if err := revocarArchivosCapturada(ctx, account, raw); err != nil {
			return err
		}
	}
	if err := witness.ValidarCierre(); err != nil {
		return err
	}
	if documentos.Guardada() {
		if err := documentos.RetirarIdentidad(ctx, account, identidadDocumentos(identity, s.almacen.Identificador())); err != nil {
			return err
		}
	}
	saved, err := s.leerSesionCompartida(ctx, witness.ValidarCierre)
	if err != nil {
		return err
	}
	if err := witness.ValidarCierre(); err != nil {
		return err
	}
	if saved.Refresh != "" && s.oidc != nil {
		return s.oidc.Revocar(ctx, saved.Refresh)
	}
	return nil
}

func (s *Suite) salirLegadoAutorizado(ctx context.Context, discard bool, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	count, err := s.contarAlmacenPrivado(ctx, storage.AllPrivateJournalNamespaces()...)
	if err != nil || count != 0 {
		return errors.Join(ErrTrabajoPendiente, err)
	}
	if !discard {
		count, err = contarAlmacenCon(ctx, notas.EspacioCapturas, notas.EspacioBorradores)
		if err != nil || count != 0 {
			return errors.Join(ErrTrabajoPendiente, err)
		}
	}
	return s.salirClasicoConGuardia(ctx, guard)
}
