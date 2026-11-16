// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sessionPrivateWorkFixture struct {
	scope   storage.Scope
	gate    *offline.WorkGate
	journal *offline.Store
	closed  bool
	onClose func()
}

func (w *sessionPrivateWorkFixture) PrivateStorageScope() storage.Scope { return w.scope }
func (w *sessionPrivateWorkFixture) Pending(ctx context.Context) (offline.PendingWork, error) {
	if err := ctx.Err(); err != nil {
		return offline.PendingWork{}, err
	}
	count, err := w.journal.CountOwner(w.scope.Tenant+"\x00"+w.scope.Subject+"\x00"+w.scope.Product, w.scope.Product+"-private-v1")
	return offline.PendingWork{Scope: w.scope, Count: count}, err
}
func (w *sessionPrivateWorkFixture) Drain(ctx context.Context) error { return w.gate.Drain(ctx) }
func (w *sessionPrivateWorkFixture) Resume() error                   { return w.gate.Resume() }
func (w *sessionPrivateWorkFixture) Close() {
	w.gate.Close()
	w.closed = true
	if w.onClose != nil {
		w.onClose()
	}
}

func registrarPreparadorAntiguo(s *Suite, work offline.PrivateWork, witness auth.TestigoTrabajo, prepare func(context.Context, context.CancelFunc) (func(), error)) (func(), error) {
	return s.RegistrarPreparadorTransicionPrivada(work, witness, func(ctx context.Context, cancel func() bool, _ EventoTransicionPrivada) (func(EventoTransicionPrivada), error) {
		resume, err := prepare(ctx, func() { cancel() })
		if resume == nil {
			return nil, err
		}
		return func(event EventoTransicionPrivada) {
			if event.Fase == TransicionPreflightAbortado {
				resume()
			}
		}, err
	})
}

func privateSuiteFixture(t *testing.T) (*Suite, *sessionPrivateWorkFixture, auth.Tokens) {
	t.Helper()
	aparte(t)
	app := test.NewApp()
	t.Cleanup(app.Quit)
	s := Abrir(app, "sytue.eu")
	tokens := auth.Tokens{Issuer: "https://id.sytue.eu", Cliente: auth.ClientID, Refresh: "synthetic-private-refresh", Subject: "ana", Organizacion: "acme"}
	if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "notes"}
	witness, err := s.TestigoTrabajo(t.Context(), scope)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}
	work := &sessionPrivateWorkFixture{scope: scope, gate: offline.NewWorkGateWithLease(t.Context(), witness.Compartido), journal: journal}
	unregister, err := s.RegistrarTrabajoPrivado(work, witness)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unregister)
	t.Cleanup(work.gate.Close)
	s.privateRevoke = func(context.Context, auth.TestigoTrabajo, auth.IdentitySnapshot) error { return nil }
	return s, work, tokens
}

func TestPrivateTransitionFailureIsRetryableBeforeOrganizationRequest(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	org := &organizadorDoble{}
	s.orgs = org
	s.reentrar = func(context.Context) error { return nil }
	failure := errors.New("synthetic durable write failure")
	attempts, resumes := 0, 0
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		attempts++
		resume := func() { resumes++ }
		if attempts == 1 {
			return resume, failure
		}
		return resume, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if err := s.CambiarOrganizacion(t.Context(), "beta"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if attempts != 1 || resumes != 1 || len(org.pasos) != 0 {
		t.Fatal(attempts, resumes, org.pasos)
	}
	if err := s.CambiarOrganizacion(t.Context(), "beta"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || resumes != 1 || len(org.pasos) != 1 {
		t.Fatal(attempts, resumes, org.pasos)
	}
}

func TestPrivateLogoutPreflightFailureDoesNotPauseOrRevoke(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic flush failure")
	resumed, revoked := false, false
	s.privateRevoke = func(context.Context, auth.TestigoTrabajo, auth.IdentitySnapshot) error {
		revoked = true
		return nil
	}
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		return func() { resumed = true }, failure
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if err := s.salirProtegidoConTestigo(t.Context(), false, witness); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !resumed || revoked || witness.Validar() != nil {
		t.Fatal("failed preflight affected the session", resumed, revoked)
	}
}

func TestOrganizationReentryFailureKeepsPresenterFrozen(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	org := &organizadorDoble{}
	s.orgs = org
	failure := errors.New("synthetic login failure")
	s.reentrar = func(context.Context) error { return failure }
	resumed := false
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		return func() { resumed = true }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if err := s.CambiarOrganizacion(t.Context(), "beta"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if resumed || len(org.pasos) != 1 || org.pasos[0] != "cambiar beta" {
		t.Fatal("server change or presenter freeze was not preserved", resumed, org.pasos)
	}
}

type appliedOrganizationWithLostResponse struct {
	applied bool
	err     error
}

func (o *appliedOrganizationWithLostResponse) Organizaciones(context.Context) ([]api.Organizacion, error) {
	return nil, nil
}

func (o *appliedOrganizationWithLostResponse) CambiarOrganizacion(context.Context, string) error {
	o.applied = true
	return o.err
}

func TestOrganizationLostResponseIsReportedAsUncertain(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic lost response")
	org := &appliedOrganizationWithLostResponse{err: failure}
	s.orgs = org
	resumed := false
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		return func() { resumed = true }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	err = s.CambiarOrganizacion(t.Context(), "beta")
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "no se pudo confirmar") || resumed || !org.applied {
		t.Fatal("lost response was not treated as uncertain", err, resumed, org.applied)
	}
}

func TestPrivateTransitionExcludesConcurrentRegistrationAndTransition(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(ctx context.Context, _ context.CancelFunc) (func(), error) {
		close(entered)
		select {
		case <-release:
			return func() {}, nil
		case <-ctx.Done():
			return func() {}, ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	result := make(chan error, 1)
	go func() {
		finish, err := s.prepararTransicionPrivada(t.Context(), func() {}, &witness)
		if err == nil {
			finish.fallo(false)
		}
		result <- err
	}()
	<-entered
	if _, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) { return func() {}, nil }); !errors.Is(err, ErrTransicionPrivada) {
		t.Fatal(err)
	}
	if _, err := s.RegistrarTrabajoPrivado(work, witness); !errors.Is(err, ErrTransicionPrivada) {
		t.Fatal(err)
	}
	if _, err := s.prepararTransicionPrivada(t.Context(), func() {}, &witness); !errors.Is(err, ErrTransicionPrivada) {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

type simulatedDeadline struct {
	context.Context
	done chan struct{}
}

func (c simulatedDeadline) Done() <-chan struct{} { return c.done }
func (c simulatedDeadline) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestPrivateTransitionCancellationAndTimeoutResumeOriginal(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			s, work, _ := privateSuiteFixture(t)
			witness, err := s.TestigoTrabajo(t.Context(), work.scope)
			if err != nil {
				t.Fatal(err)
			}
			resumed := false
			unregister, err := registrarPreparadorAntiguo(s, work, witness, func(ctx context.Context, cancel context.CancelFunc) (func(), error) {
				cancel()
				<-ctx.Done()
				return func() { resumed = true }, ctx.Err()
			})
			if err != nil {
				t.Fatal(err)
			}
			defer unregister()
			ctx, cancel := context.WithCancel(t.Context())
			if timeout {
				cancel()
				timed := simulatedDeadline{Context: t.Context(), done: make(chan struct{})}
				var once sync.Once
				ctx, cancel = timed, func() { once.Do(func() { close(timed.done) }) }
			}
			defer cancel()
			_, err = s.prepararTransicionPrivada(ctx, cancel, &witness)
			want := context.Canceled
			if timeout {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || !resumed {
				t.Fatal(err, resumed)
			}
		})
	}
}

func TestTransitionCancellationFenceAndSnapshot(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, cancelFirst := range []bool{true, false} {
		ctx, cancel := context.WithCancel(t.Context())
		transition, err := s.prepararTransicionPrivada(ctx, cancel, &witness)
		if err != nil {
			t.Fatal(err)
		}
		if cancelFirst {
			if !transition.cancelar() {
				t.Fatal("cancellation lost before send")
			}
			if err := transition.empezarEnvio(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		} else {
			if err := transition.empezarEnvio(); err != nil {
				t.Fatal(err)
			}
			if transition.cancelar() {
				t.Fatal("cancellation claimed success after send boundary")
			}
			cancel() // External cancellation remains possible after the boundary.
		}
		transition.fallo(false)
		state, ok := s.EstadoTransicionPrivada(transition.cycle)
		want := TransicionPreflightAbortado
		if !cancelFirst {
			want = TransicionEfectoRemotoIncierto
		}
		if !ok || state.Fase != want || state.Revision < 2 {
			t.Fatal(state, ok)
		}
		cancel()
	}
}

func TestTransitionCancelRacesSend(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transition, err := s.prepararTransicionPrivada(ctx, cancel, &witness)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	cancelled, sends := make(chan bool, 1), make(chan int, 1)
	go func() { <-start; cancelled <- transition.cancelar() }()
	go func() {
		<-start
		if transition.empezarEnvio() == nil {
			sends <- 1
		} else {
			sends <- 0
		}
	}()
	close(start)
	accepted, count := <-cancelled, <-sends
	if accepted && count != 0 || !accepted && count != 1 {
		t.Fatal(accepted, count)
	}
	transition.fallo(false)
}

func TestTransitionEventsAndSnapshotKeepMonotonicRevision(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	var events []EventoTransicionPrivada
	unregister, err := s.RegistrarPreparadorTransicionPrivada(work, witness, func(_ context.Context, _ func() bool, initial EventoTransicionPrivada) (func(EventoTransicionPrivada), error) {
		events = append(events, initial)
		return func(event EventoTransicionPrivada) { events = append(events, event) }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transition, err := s.prepararTransicionPrivada(ctx, cancel, &witness)
	if err != nil {
		t.Fatal(err)
	}
	if err := transition.empezarEnvio(); err != nil {
		t.Fatal(err)
	}
	transition.fallo(false)
	if len(events) != 3 || events[0].Fase != TransicionGuardando || events[1].Fase != TransicionPeticionEnviada || events[2].Fase != TransicionEfectoRemotoIncierto {
		t.Fatal(events)
	}
	for i, event := range events {
		if event.Ciclo != transition.cycle || event.Revision != uint64(i+1) {
			t.Fatal(events)
		}
	}
	latest, ok := s.EstadoTransicionPrivada(transition.cycle)
	if !ok || latest != events[2] {
		t.Fatal(latest, ok)
	}
}

func TestPrivateTransitionDetachDuringPrepareAbortsWithoutResume(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	resumed := make(chan struct{}, 1)
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		close(entered)
		<-release
		return func() { resumed <- struct{}{} }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := s.prepararTransicionPrivada(t.Context(), func() {}, &witness); result <- err }()
	<-entered
	unregister()
	close(release)
	if err := <-result; !errors.Is(err, ErrTransicionPrivada) {
		t.Fatal(err)
	}
	select {
	case <-resumed:
		t.Fatal("detached presenter was resumed")
	default:
	}
}

func TestPrivateTransitionNeverResumesOrRunsStaleWitness(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	resumed, called := false, false
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		called = true
		if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
			return nil, err
		}
		return func() { resumed = true }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if _, err := s.prepararTransicionPrivada(t.Context(), func() {}, &witness); err == nil {
		t.Fatal("stale witness accepted")
	}
	if !called || resumed {
		t.Fatal(called, resumed)
	}
	called = false
	if _, err := s.prepararTransicionPrivada(t.Context(), func() {}, &witness); err == nil {
		t.Fatal("stale witness accepted on retry")
	}
	if called {
		t.Fatal("stale callback ran")
	}
}

func TestLegacyLogoutRejectsActivePrivatePresenter(t *testing.T) {
	s, work, _ := privateSuiteFixture(t)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	unregister, err := registrarPreparadorAntiguo(s, work, witness, func(context.Context, context.CancelFunc) (func(), error) {
		return func() {}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if err := s.salirClasicoConGuardia(t.Context(), nil); !errors.Is(err, ErrTransicionPrivada) {
		t.Fatal("legacy cleanup reached an active private presenter", err)
	}
	if err := witness.Validar(); err != nil {
		t.Fatal("legacy cleanup invalidated the witness", err)
	}
}

func TestPrivateLogoutRequiresExplicitDiscardAndKeepsKeysWhenRevocationFails(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	owner := "acme\x00ana\x00notes"
	if err := work.journal.Write(owner, "notes-private-v1", "snapshot", "unconfirmed"); err != nil {
		t.Fatal(err)
	}
	if err := auth.GuardarSecreto("rooms-matrix-private-test", []byte("synthetic-device-key")); err != nil {
		t.Fatal(err)
	}
	revoked := false
	failure := errors.New("synthetic provider unavailable")
	s.privateRevoke = func(context.Context, auth.TestigoTrabajo, auth.IdentitySnapshot) error {
		revoked = true
		return failure
	}
	if err := s.SalirConError(); !errors.Is(err, ErrTrabajoPendiente) || revoked {
		t.Fatal("pending logout reached revocation", err, revoked)
	}
	if err := s.SalirDescartando(t.Context()); !errors.Is(err, failure) {
		t.Fatal("revocation failure was hidden", err)
	}
	persisted, err := s.almacen.Cargar()
	if err != nil || persisted.Refresh != tokens.Refresh || s.Sesion.Subject() != "ana" {
		t.Fatal("failed revocation removed credentials", err)
	}
	if count, err := work.journal.CountOwner(owner, "notes-private-v1"); err != nil || count != 1 {
		t.Fatal("failed revocation discarded WAL", count, err)
	}
	if key, err := auth.LeerSecreto("rooms-matrix-private-test"); err != nil || len(key) == 0 {
		t.Fatal("failed revocation destroyed device key", err)
	}
	if work.closed {
		t.Fatal("failed logout closed key adapter")
	}
	if _, done, err := work.gate.Start(t.Context()); err != nil {
		t.Fatal("aborted logout did not resume same scope", err)
	} else {
		done()
	}
}

func TestExplicitPrivateDiscardKeepsOtherAccountsAndNewLogin(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	for _, subject := range []string{"ana", "bea"} {
		owner := "acme\x00" + subject + "\x00notes"
		if err := work.journal.Write(owner, "notes-private-v1", "snapshot", subject); err != nil {
			t.Fatal(err)
		}
	}
	work.onClose = func() {
		tokens.Subject = "bea"
		tokens.Refresh = "synthetic-new-login"
		if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
			t.Error(err)
		}
	}
	if err := s.SalirDescartando(t.Context()); err != nil {
		t.Fatal(err)
	}
	saved, err := s.almacen.Cargar()
	if err != nil || saved.Subject != "bea" || saved.Refresh != "synthetic-new-login" || s.Sesion.Subject() != "bea" {
		t.Fatal("old cleanup erased new login", err)
	}
	if count, err := work.journal.CountOwner("acme\x00ana\x00notes", "notes-private-v1"); err != nil || count != 0 {
		t.Fatal("explicit old-scope discard did not complete", count, err)
	}
	if count, err := work.journal.CountOwner("acme\x00bea\x00notes", "notes-private-v1"); err != nil || count != 1 {
		t.Fatal("old discard erased other account", count, err)
	}
}

func TestPrivateLogoutDoesNotPassActiveCleanupOrEraseConcurrentLogin(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	job, done, err := work.gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	result := make(chan error, 1)
	go func() { result <- s.SalirConError() }()
	select {
	case <-job.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel active IO")
	}
	select {
	case err := <-result:
		t.Fatal("shutdown passed active cleanup", err)
	default:
	}
	tokens.Subject = "bea"
	tokens.Refresh = "synthetic-concurrent-login"
	if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	done()
	if err := <-result; err == nil {
		t.Fatal("old logout accepted concurrent login")
	}
	saved, err := s.almacen.Cargar()
	if err != nil || saved.Subject != "bea" || s.Sesion.Subject() != "bea" {
		t.Fatal("concurrent login removed", err)
	}
}

type privateQueueTransport struct {
	writes int
	state  storage.AppMigrationState
}

func (transport *privateQueueTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodGet {
		transport.writes++
	}
	raw, err := json.Marshal(transport.state)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: request}, nil
}

func TestPrivateCaptureQueueNeverFallsBackToPlaintextAfterFreeze(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	tokens.Access = "synthetic-access"
	tokens.Expiry = time.Now().Add(time.Hour)
	if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	capture := notas.Captura{ClientID: "synthetic-private-capture", Cuaderno: "book", In: api.NoteInput{Title: "must remain inside encrypted queue"}}
	if err := work.journal.Write("ana/acme", notas.EspacioCapturas, capture.ClientID, capture); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	transport := &privateQueueTransport{state: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: work.scope, Mode: "frozen", Epoch: "12345678-1234-1234-1234-123456789abc", StartedAt: &now, Rows: 1}}}
	previous := api.Transporte
	api.Transporte = transport
	t.Cleanup(func() { api.Transporte = previous })
	if pending := s.SubirNotepads(t.Context()); pending == 0 {
		t.Fatal("frozen capture queue disappeared")
	}
	if transport.writes != 0 {
		t.Fatal("private queue reached legacy plaintext write", transport.writes)
	}
}

func TestCapturedExitRejectsAnotherLoginAndABAWithoutErasingEitherOwner(t *testing.T) {
	for _, subjectPath := range [][]string{{"bea"}, {"bea", "ana"}} {
		t.Run(strings.Join(subjectPath, "-"), func(t *testing.T) {
			s, work, tokens := privateSuiteFixture(t)
			if err := work.journal.Write("acme\x00ana\x00notes", "notes-private-v1", "snapshot", "unconfirmed-a"); err != nil {
				t.Fatal(err)
			}
			captured, err := s.AutorizarSalida()
			if err != nil {
				t.Fatal(err)
			}
			for _, subject := range subjectPath {
				tokens.Subject = subject
				tokens.Refresh = "synthetic-new-" + subject
				if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
					t.Fatal(err)
				}
			}
			if err := captured.Cerrar(t.Context(), true); err == nil {
				t.Fatal("old dialog accepted a new login")
			}
			saved, err := s.almacen.Cargar()
			if err != nil || saved.Subject != subjectPath[len(subjectPath)-1] {
				t.Fatal("captured close erased a new account", err)
			}
			if count, err := work.journal.CountOwner("acme\x00ana\x00notes", "notes-private-v1"); err != nil || count != 1 {
				t.Fatal("stale discard erased A's retained work", count, err)
			}
		})
	}
}

func TestCapturedExitAcceptsSamePersistentNonceAfterSharedRefresh(t *testing.T) {
	s, _, tokens := privateSuiteFixture(t)
	captured, err := s.AutorizarSalida()
	if err != nil {
		t.Fatal(err)
	}
	tokens.Refresh = "synthetic-shared-refreshed"
	if err := s.almacen.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	s.Sesion.Sincronizar(tokens)
	if err := captured.Cerrar(t.Context(), false); err != nil {
		t.Fatal("refresh invalidated the captured native action", err)
	}
	if s.Sesion.Subject() != "" {
		t.Fatal("captured exit did not close same login")
	}
}

type changingQueueTransport struct {
	s       *Suite
	next    auth.Tokens
	scope   storage.Scope
	writes  int
	changed bool
}

func (transport *changingQueueTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/private-storage/migration") {
		if err := storage.ValidateRequestScope(request.Header, transport.scope); err != nil {
			return nil, err
		}
		transport.changed = true
		if err := transport.s.Sesion.AdoptarCon(request.Context(), transport.next); err != nil {
			return nil, err
		}
		state := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: transport.scope, Mode: "legacy"}}
		raw, err := json.Marshal(state)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: request}, nil
	}
	transport.writes++
	return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"unexpected"}`)), Request: request}, nil
}

func TestCapturedQueueStateResponseCannotSwitchPlaintextSubmissionToAnotherAccount(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	tokens.Access = "synthetic-a-access"
	tokens.Expiry = time.Now().Add(time.Hour)
	if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	capture := notas.Captura{ClientID: "synthetic-captured-a", Cuaderno: "book-a", In: api.NoteInput{Title: "retained A draft"}}
	if err := work.journal.Write("ana/acme", notas.EspacioCapturas, capture.ClientID, capture); err != nil {
		t.Fatal(err)
	}
	next := tokens
	next.Subject = "bea"
	next.Refresh = "synthetic-b-refresh"
	next.Access = "synthetic-b-access"
	transport := &changingQueueTransport{s: s, next: next, scope: work.scope}
	previous := api.Transporte
	api.Transporte = transport
	t.Cleanup(func() { api.Transporte = previous })
	if count := s.SubirNotepads(t.Context()); count == 0 {
		t.Fatal("A's unconfirmed capture vanished")
	}
	if !transport.changed || transport.writes != 0 {
		t.Fatal("A's state response permitted an unrelated plaintext write", transport.changed, transport.writes)
	}
	if count, err := work.journal.CountOwner("ana/acme", notas.EspacioCapturas); err != nil || count != 1 {
		t.Fatal("account change discarded A's capture", count, err)
	}
}

func TestPrivateLogoutCannotEraseLegacyWorkCreatedByAConcurrentNewLogin(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	tokens.Subject = "bea"
	tokens.Refresh = "synthetic-later-b"
	const owner = "bea/acme"
	work.onClose = func() {
		if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
			t.Error(err)
			return
		}
		if err := work.journal.Write(owner, notas.EspacioBorradores, "new-b", "new B draft after A logout"); err != nil {
			t.Error(err)
		}
	}
	if err := s.SalirConError(); err != nil {
		t.Fatal(err)
	}
	saved, err := s.almacen.Cargar()
	if err != nil || saved.Subject != "bea" || s.Sesion.Subject() != "bea" {
		t.Fatal("A's cleanup erased B's login", err)
	}
	count, err := work.journal.CountOwner(owner, notas.EspacioBorradores)
	if err != nil || count != 1 {
		t.Fatal("A's cleanup erased B's newly created legacy draft", count, err)
	}
}
