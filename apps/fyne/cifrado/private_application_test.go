// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type applicationPanelFixture struct {
	start                                 func(context.Context) error
	starts, retries, shows, invalidations int
}

func TestPrivateApplicationPresenterJoinPrecedesRepositoryAndSDKRetirement(t *testing.T) {
	fixture := newApplicationFixture(t)
	canceled, joining, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	first := true
	fixture.app.cfg.mount = func(context.Context, *Resources[*factoryDevice], *cycleRepository) (ApplicationView, error) {
		fixture.mounts++
		if !first {
			return ApplicationView{Object: &fyne.Container{}}, nil
		}
		first = false
		return ApplicationView{Object: &fyne.Container{}, Detach: func() { fixture.detaches++; close(canceled) }, Join: func(ctx context.Context) error {
			select {
			case <-canceled:
			default:
				return errors.New("join preceded UI cancellation")
			}
			close(joining)
			select {
			case <-release:
				return ctx.Err()
			case <-ctx.Done():
				return ctx.Err()
			}
		}}, nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	previous := fixture.app.active
	repo := fixture.repos[0]
	fixture.adopt(t, "bob")
	select {
	case <-joining:
	case <-time.After(5 * time.Second):
		t.Fatal("presenter join was not called")
	}
	if repo.closed.Load() != 0 || fixture.panelCloses != 0 {
		t.Fatal("repository or SDK retired before presenter joined")
	}
	fixture.adopt(t, "alice")
	if fixture.setups != 1 || fixture.mounts != 1 || fixture.detaches != 1 {
		t.Fatal("ABA identity bypassed presenter retirement")
	}
	unblock()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady && fixture.mounts == 2 })
	select {
	case <-previous.retiredDone:
	default:
		t.Fatal("joined presenter lost prior retirement barrier")
	}
	if repo.closed.Load() != 1 || fixture.panelCloses != 1 {
		t.Fatal("joined presenter did not permit exact repository/SDK teardown")
	}
}

func TestPrivateApplicationPresenterJoinErrorRetainsCallbackAndJournal(t *testing.T) {
	fixture := newApplicationFixture(t)
	var failed atomic.Bool
	failed.Store(true)
	var calls atomic.Int32
	t.Cleanup(func() { failed.Store(false) })
	first := true
	fixture.app.cfg.mount = func(context.Context, *Resources[*factoryDevice], *cycleRepository) (ApplicationView, error) {
		fixture.mounts++
		if !first {
			return ApplicationView{Object: &fyne.Container{}}, nil
		}
		first = false
		// Join must be retained even when a presenter needs no UI Detach callback.
		return ApplicationView{Object: &fyne.Container{}, Join: func(ctx context.Context) error {
			calls.Add(1)
			if failed.Load() {
				return errors.New("presenter did not drain")
			}
			return ctx.Err()
		}}, nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	previous := fixture.app.active
	repo := fixture.repos[0]
	repo.mu.Lock()
	repo.count = 9
	repo.mu.Unlock()
	fixture.adopt(t, "bob")
	fixture.pump(t, func() bool { return fixture.state == ApplicationFailed })
	if calls.Load() != 1 || repo.closed.Load() != 0 || fixture.panelCloses != 0 || fixture.setups != 1 {
		t.Fatal("failed join allowed repository/SDK replacement")
	}
	select {
	case <-previous.retiredDone:
		t.Fatal("failed join released retirement barrier")
	default:
	}
	fixture.adopt(t, "alice")
	failed.Store(false)
	fixture.app.retry()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady && fixture.mounts == 2 })
	repo.mu.Lock()
	count := repo.count
	repo.mu.Unlock()
	if calls.Load() != 2 || count != 9 || repo.closed.Load() != 1 || fixture.panelCloses != 1 {
		t.Fatal("join retry forgot callback or discarded journal")
	}
}

func TestPrivateApplicationCanceledJoinCannotEraseRetainedCallback(t *testing.T) {
	life := &applicationLifetime[*factoryDevice, *cycleRepository]{}
	calls := 0
	life.detachView(ApplicationView{Join: func(context.Context) error { calls++; return nil }})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := life.joinPresenters(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled join was treated as complete", err)
	}
	if err := life.joinPresenters(t.Context()); err != nil || calls != 2 {
		t.Fatal("canceled join discarded retained callback", err, calls)
	}
}

func (p *applicationPanelFixture) Start(ctx context.Context) error { p.starts++; return p.start(ctx) }
func (p *applicationPanelFixture) Retry()                          { p.retries++ }
func (p *applicationPanelFixture) Show()                           { p.shows++ }
func (p *applicationPanelFixture) Invalidate()                     { p.invalidations++ }

type applicationFixture struct {
	app                                   *privateApplication[*factoryDevice, *cycleRepository]
	session                               *auth.Session
	factory                               Factory[*factoryDevice]
	devices                               Devices[*factoryDevice]
	queue                                 chan func()
	state                                 ApplicationState
	setups, mounts, detaches, panelCloses int
	repos                                 []*cycleRepository
	build                                 func(context.Context, *Resources[*factoryDevice]) (ApplicationPlan[*cycleRepository], error)
}

func newApplicationFixture(t *testing.T) *applicationFixture {
	t.Helper()
	factory, _, session, devices := factoryFixture(t)
	fixture := &applicationFixture{session: session, factory: factory, devices: devices, queue: make(chan func(), 128)}
	fixture.app = &privateApplication[*factoryDevice, *cycleRepository]{}
	fixture.app.cfg = privateApplicationConfig[*factoryDevice, *cycleRepository]{
		identity: factory.CurrentIdentity, capture: session.WithAuthorizationIntent,
		setup: func(_ context.Context, ready func(context.Context, *Resources[*factoryDevice]), _ func(error)) (applicationPanels, func(context.Context) error, error) {
			fixture.setups++
			panel := &applicationPanelFixture{}
			panel.start = func(ctx context.Context) error {
				resources, err := factory.Open(ctx, devices)
				if err != nil {
					return err
				}
				ready(resources.Context(), resources)
				return nil
			}
			return panel, func(ctx context.Context) error { fixture.panelCloses++; return ctx.Err() }, nil
		},
		build: func(ctx context.Context, resources *Resources[*factoryDevice]) (ApplicationPlan[*cycleRepository], error) {
			if fixture.build != nil {
				return fixture.build(ctx, resources)
			}
			repo := &cycleRepository{scope: resources.PrivateStorageScope()}
			fixture.repos = append(fixture.repos, repo)
			return ApplicationPlan[*cycleRepository]{Repository: repo}, nil
		},
		mount: func(context.Context, *Resources[*factoryDevice], *cycleRepository) (ApplicationView, error) {
			fixture.mounts++
			return ApplicationView{Object: &fyne.Container{}, Detach: func() { fixture.detaches++ }}, nil
		},
		state: func(state ApplicationState, _ error) { fixture.state = state }, replace: func(fyne.CanvasObject) {}, dispatch: func(fn func()) { fixture.queue <- fn },
	}
	t.Cleanup(func() {
		fixture.app.close()
		fixture.pump(t, func() bool {
			for _, life := range fixture.app.retirements {
				select {
				case <-life.retiredDone:
				default:
					return false
				}
			}
			return true
		})
	})
	return fixture
}

func (f *applicationFixture) pump(t *testing.T, done func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for !done() {
		select {
		case fn := <-f.queue:
			fn()
		case <-ctx.Done():
			t.Fatal("private application did not reach its lifecycle barrier")
		}
	}
	for {
		select {
		case fn := <-f.queue:
			fn()
		default:
			return
		}
	}
}

func (f *applicationFixture) adopt(t *testing.T, subject string) {
	t.Helper()
	tokens := factoryTokens(subject)
	ctx := f.session.WithAuthorizationIntent(t.Context())
	witness, err := f.factory.CaptureWork(ctx, f.devices.Isolated.binding.Scope)
	if err != nil {
		t.Fatal(err)
	}
	factoryAdopt(t, witness, f.session, tokens)
	f.app.adopt()
}

func TestPrivateApplicationNeverMountsFailedPreparationAndReusesItsSource(t *testing.T) {
	fixture := newApplicationFixture(t)
	preparations, builds := 0, 0
	var retained *cycleRepository
	fixture.build = func(_ context.Context, resources *Resources[*factoryDevice]) (ApplicationPlan[*cycleRepository], error) {
		builds++
		retained = &cycleRepository{scope: resources.PrivateStorageScope()}
		return ApplicationPlan[*cycleRepository]{Repository: retained, Prepare: func(context.Context, *cycleRepository) error {
			preparations++
			if preparations == 1 {
				return errors.New("retained source is incomplete")
			}
			return nil
		}}, nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationFailed })
	if fixture.mounts != 0 || retained.initialized.Load() != 0 || builds != 1 {
		t.Fatal("failed source was exposed as ready", fixture.mounts, retained.initialized.Load(), builds)
	}
	fixture.app.retry()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	if fixture.mounts != 1 || builds != 1 || preparations != 2 || retained.initialized.Load() != 1 {
		t.Fatal("retry replaced its retained source or skipped preparation", fixture.mounts, builds, preparations)
	}
	if fixture.devices.Legacy.closes+fixture.devices.Isolated.closes != 0 {
		t.Fatal("application owned borrowed SDK clients")
	}
}

func TestPrivateApplicationChecksBinaryAvailabilityBeforeMount(t *testing.T) {
	fixture := newApplicationFixture(t)
	available := false
	fixture.build = func(_ context.Context, resources *Resources[*factoryDevice]) (ApplicationPlan[*cycleRepository], error) {
		return ApplicationPlan[*cycleRepository]{Repository: &cycleRepository{scope: resources.PrivateStorageScope()}, Validate: func(context.Context, *cycleRepository) error {
			if !available {
				return errors.New("encrypted original is unavailable")
			}
			return nil
		}}, nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationFailed })
	if fixture.mounts != 0 {
		t.Fatal("metadata validity was mistaken for media availability")
	}
	available = true
	fixture.app.retry()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	if fixture.mounts != 1 {
		t.Fatal("verified media did not permit mounting")
	}
}

func TestPrivateApplicationABARetainsRetirementChainBeforeOpeningAnotherSDK(t *testing.T) {
	fixture := newApplicationFixture(t)
	release := make(chan struct{})
	retiring := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	first := true
	fixture.build = func(_ context.Context, resources *Resources[*factoryDevice]) (ApplicationPlan[*cycleRepository], error) {
		plan := ApplicationPlan[*cycleRepository]{Repository: &cycleRepository{scope: resources.PrivateStorageScope()}}
		if first {
			first = false
			plan.Retire = func(context.Context) error { close(retiring); <-release; return nil }
		}
		return plan, nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	fixture.adopt(t, "bob")
	select {
	case <-retiring:
	case <-time.After(5 * time.Second):
		t.Fatal("old presenter did not start retirement")
	}
	fixture.adopt(t, "alice")
	if fixture.setups != 1 || fixture.mounts != 1 || fixture.detaches != 1 {
		t.Fatal("new account bypassed joined retirement", fixture.setups, fixture.mounts, fixture.detaches)
	}
	unblock()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady && fixture.mounts == 2 })
	if fixture.setups != 2 || fixture.panelCloses != 1 {
		t.Fatal("retirement chain lost SDK close or reopened the intermediate account", fixture.setups, fixture.panelCloses)
	}
}

func TestPrivateApplicationRetirementErrorPreservesSDKAndRetainedWork(t *testing.T) {
	fixture := newApplicationFixture(t)
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady })
	repo := fixture.repos[0]
	repo.mu.Lock()
	repo.count = 9
	repo.pendingErr = errors.New("journal cannot be authenticated")
	repo.mu.Unlock()
	fixture.adopt(t, "bob")
	fixture.pump(t, func() bool { return fixture.state == ApplicationFailed })
	if fixture.panelCloses != 0 || fixture.setups != 1 || repo.closed.Load() != 0 {
		t.Fatal("unknown retained work permitted SDK teardown", fixture.panelCloses, fixture.setups, repo.closed.Load())
	}
	fixture.adopt(t, "alice")
	repo.mu.Lock()
	repo.pendingErr = nil
	repo.mu.Unlock()
	fixture.app.retry()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady && fixture.mounts == 2 })
	repo.mu.Lock()
	count := repo.count
	repo.mu.Unlock()
	if count != 9 || fixture.panelCloses != 1 {
		t.Fatal("retirement discarded retained work", count, fixture.panelCloses)
	}
}

// An exclusive session change can enter while the person considers consent.
// A lease held by RepositoryCycle.Open would make this fail before construction.
func TestPrivateApplicationBeforeOpenHasNoExteriorWorkLease(t *testing.T) {
	fixture := newApplicationFixture(t)
	attempts := 0
	fixture.app.cfg.beforeOpen = func(ctx context.Context, resources *Resources[*factoryDevice]) error {
		attempts++
		deadline, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		release, err := resources.session.witness.Exclusivo(deadline)
		if err != nil {
			return err
		}
		release()
		if attempts == 1 {
			return errors.New("explicit consent canceled")
		}
		return nil
	}
	fixture.app.adopt()
	fixture.pump(t, func() bool { return fixture.state == ApplicationFailed })
	if fixture.mounts != 0 || len(fixture.repos) != 0 {
		t.Fatal("canceled consent constructed or exposed a repository")
	}
	fixture.app.retry()
	fixture.pump(t, func() bool { return fixture.state == ApplicationReady || fixture.state == ApplicationFailed })
	if fixture.state != ApplicationReady || fixture.mounts != 1 || attempts != 2 {
		t.Fatal("retry retained a work lease across consent", fixture.state, attempts)
	}
}

func TestPrivateApplicationRetirementJoinsCanceledBeforeOpen(t *testing.T) {
	fixture := newApplicationFixture(t)
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	fixture.app.cfg.beforeOpen = func(ctx context.Context, _ *Resources[*factoryDevice]) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	}
	fixture.app.adopt()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("pre-open consent was not entered")
	}
	fixture.app.close()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement did not cancel consent")
	}
	if fixture.panelCloses != 0 || len(fixture.repos) != 0 {
		t.Fatal("SDK closed or repository built before consent joined")
	}
	retired := fixture.app.previous
	if retired == nil {
		t.Fatal("closed presenter did not retain its retirement barrier")
	}
	unblock()
	fixture.pump(t, func() bool {
		select {
		case <-retired:
			return true
		default:
			return false
		}
	})
	if fixture.panelCloses != 1 {
		t.Fatal("joined consent did not retire its panel")
	}
}
