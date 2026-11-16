// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// ApplicationState describes the actual mounted boundary, never just device
// verification or a server's routing phase. ApplicationReady requires Open.
type ApplicationState uint8

const (
	ApplicationSignedOut ApplicationState = iota
	ApplicationLocked
	ApplicationPreparing
	ApplicationReady
	ApplicationFailed
	ApplicationRetiring
)

// ApplicationView owns one presenter's callbacks. Detach runs on the UI thread
// before the repository is drained; it must cancel its requests and stale UI.
type ApplicationView struct {
	Object fyne.CanvasObject
	Detach func()
	// Join waits for admitted presenter work after Detach canceled it. The host
	// calls it away from the UI, before retiring the repository and SDKs.
	Join func(context.Context) error
}

// ApplicationPlan leaves migration rules with their domain and shares all
// presenter/repository teardown. Retire owns auxiliary readers/stagers only;
// neither it nor the presenter may close the borrowed Matrix SDK clients.
type ApplicationPlan[R PrivateRepository] struct {
	Repository R
	Prepare    func(context.Context, R) error
	Validate   func(context.Context, R) error
	Retire     func(context.Context) error
}

type applicationPanels interface {
	Start(context.Context) error
	Retry()
	Show()
	Invalidate()
}

type privateApplicationConfig[D StorageDevice, R PrivateRepository] struct {
	identity   func() (auth.IdentitySnapshot, bool)
	capture    func(context.Context) context.Context
	canStart   func() bool
	setup      func(context.Context, func(context.Context, *Resources[D]), func(error)) (applicationPanels, func(context.Context) error, error)
	beforeOpen func(context.Context, *Resources[D]) error
	build      func(context.Context, *Resources[D]) (ApplicationPlan[R], error)
	mount      func(context.Context, *Resources[D], R) (ApplicationView, error)
	state      func(ApplicationState, error)
	replace    func(fyne.CanvasObject)
	dispatch   func(func())
}

type privateApplication[D StorageDevice, R PrivateRepository] struct {
	cfg         privateApplicationConfig[D, R]
	owner       auth.IdentitySnapshot
	inside      bool
	generation  uint64
	active      *applicationLifetime[D, R]
	previous    <-chan struct{}
	retirements []*applicationLifetime[D, R]
	closed      bool
}

type applicationLifetime[D StorageDevice, R PrivateRepository] struct {
	life               context.Context
	cancel             context.CancelFunc
	panels             applicationPanels
	closePanels        func(context.Context) error
	resources          *Resources[D]
	cycle              *RepositoryCycle[R]
	plan               ApplicationPlan[R]
	view               ApplicationView
	generation         uint64
	initializing       bool
	initializationDone chan struct{}
	retired            bool
	retireMu           sync.Mutex
	repositoryRetired  bool
	retiredDone        chan struct{}
	previous           <-chan struct{}
	retireRunning      bool
	presenterMu        sync.Mutex
	presenterJoins     []*applicationPresenterJoin
}

type applicationPresenterJoin struct{ run func(context.Context) error }

func (life *applicationLifetime[D, R]) detachView(view ApplicationView) {
	if view.Detach != nil {
		view.Detach()
	}
	if view.Join != nil {
		life.presenterMu.Lock()
		life.presenterJoins = append(life.presenterJoins, &applicationPresenterJoin{run: view.Join})
		life.presenterMu.Unlock()
	}
}

func (life *applicationLifetime[D, R]) joinPresenters(ctx context.Context) error {
	for {
		life.presenterMu.Lock()
		if len(life.presenterJoins) == 0 {
			life.presenterMu.Unlock()
			return ctx.Err()
		}
		entry := life.presenterJoins[0]
		life.presenterMu.Unlock()
		if err := entry.run(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		life.presenterMu.Lock()
		life.presenterJoins = life.presenterJoins[1:]
		life.presenterMu.Unlock()
	}
}

func (a *privateApplication[D, R]) current(life *applicationLifetime[D, R]) bool {
	if a.closed || life == nil || a.active != life || life.retired || life.life.Err() != nil {
		return false
	}
	owner, inside := a.cfg.identity()
	if !inside || owner != a.owner {
		return false
	}
	return life.resources == nil || life.resources.Check(life.life) == nil
}

// adopt runs on the UI thread. A refreshed token keeps its persisted witness;
// an A→B→A adoption cannot revive a retired presenter with the same subject.
func (a *privateApplication[D, R]) adopt() {
	if a.closed {
		return
	}
	owner, inside := a.cfg.identity()
	if a.inside == inside && a.owner == owner && a.current(a.active) {
		return
	}
	a.retire()
	a.owner, a.inside = owner, inside
	if !inside {
		a.cfg.state(ApplicationSignedOut, nil)
		return
	}
	a.generation++
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: this identity lifetime transfers cancel to applicationLifetime; retire always invokes it.
	life := &applicationLifetime[D, R]{life: ctx, cancel: cancel, generation: a.generation, retiredDone: make(chan struct{}), previous: a.previous}
	a.active = life
	a.cfg.state(ApplicationLocked, nil)
	a.start(life)
}

func (a *privateApplication[D, R]) start(life *applicationLifetime[D, R]) {
	if !a.current(life) {
		return
	}
	if a.previous != nil {
		select {
		case <-a.previous:
			a.previous = nil
		default:
			a.cfg.state(ApplicationRetiring, nil)
			return
		}
	}
	if a.cfg.canStart != nil && !a.cfg.canStart() {
		a.cfg.state(ApplicationPreparing, nil)
		return
	}
	if life.panels != nil {
		life.panels.Retry()
		return
	}
	ctx := a.cfg.capture(life.life)
	panels, closePanels, err := a.cfg.setup(ctx, func(ctx context.Context, resources *Resources[D]) { a.ready(ctx, life, resources) }, func(err error) { a.failed(life, err) })
	if err != nil {
		a.failed(life, err)
		return
	}
	life.panels, life.closePanels = panels, closePanels
	if err := panels.Start(ctx); err != nil {
		a.failed(life, err)
	}
}

func (a *privateApplication[D, R]) ready(ctx context.Context, life *applicationLifetime[D, R], resources *Resources[D]) {
	if !a.current(life) || resources == nil || resources.Check(ctx) != nil {
		return
	}
	if life.resources != nil && life.resources != resources {
		a.failed(life, ErrPrivateResources)
		return
	}
	life.resources = resources
	if !nilReference(life.view.Object) {
		return
	}
	if life.cycle == nil {
		cycle, err := NewRepositoryCycle(resources, RepositoryConfig[R]{
			Create: func(ctx context.Context) (R, error) {
				plan, err := a.cfg.build(ctx, resources)
				if err == nil {
					life.plan = plan
				}
				return plan.Repository, err
			},
			Prepare: func(ctx context.Context, repo R) error {
				if life.plan.Prepare != nil {
					return life.plan.Prepare(ctx, repo)
				}
				return nil
			},
		})
		if err != nil {
			a.failed(life, err)
			return
		}
		life.cycle = cycle
	}
	if life.initializing {
		return
	}
	life.initializing = true
	life.initializationDone = make(chan struct{})
	a.cfg.state(ApplicationPreparing, nil)
	go a.initialize(ctx, life, resources)
}

func (a *privateApplication[D, R]) initialize(ctx context.Context, life *applicationLifetime[D, R], resources *Resources[D]) {
	err := life.joinPresenters(ctx)
	if err == nil && a.cfg.beforeOpen != nil {
		err = resources.Revalidate(ctx)
		if err == nil {
			err = a.cfg.beforeOpen(ctx, resources)
		}
	}
	if err == nil {
		err = life.cycle.Open(ctx)
	}
	if err == nil && life.plan.Validate != nil {
		err = life.cycle.Run(ctx, func(ctx context.Context, repo R) error {
			if err := resources.Revalidate(ctx); err != nil {
				return err
			}
			if err := life.plan.Validate(ctx, repo); err != nil {
				return err
			}
			return resources.Revalidate(ctx)
		})
	}
	close(life.initializationDone)
	a.cfg.dispatch(func() { a.initialized(life, err) })
}

func (a *privateApplication[D, R]) initialized(life *applicationLifetime[D, R], err error) {
	if !a.current(life) {
		return
	}
	life.initializing = false
	if err != nil {
		a.failed(life, err)
		return
	}
	life.cycle.Dispatch(a.cfg.dispatch, func(repo R) { a.mount(life, repo) })
}

func (a *privateApplication[D, R]) mount(life *applicationLifetime[D, R], repo R) {
	if !a.current(life) {
		return
	}
	view, err := a.cfg.mount(life.resources.Context(), life.resources, repo)
	if err == nil && nilReference(view.Object) {
		err = ErrPrivateResources
	}
	if err != nil {
		life.detachView(view)
		a.failed(life, err)
		return
	}
	life.detachView(life.view)
	life.view = view
	a.cfg.replace(view.Object)
	a.cfg.state(ApplicationReady, nil)
}

func (a *privateApplication[D, R]) failed(life *applicationLifetime[D, R], err error) {
	if a.current(life) {
		a.cfg.state(ApplicationFailed, err)
	}
}

func (a *privateApplication[D, R]) retry() {
	for _, retired := range a.retirements {
		select {
		case <-retired.retiredDone:
		default:
			a.beginRetirement(retired)
		}
	}
	if a.active == nil {
		a.adopt()
		return
	}
	if !a.current(a.active) {
		a.adopt()
		return
	}
	life := a.active
	if life.resources != nil {
		a.ready(life.resources.Context(), life, life.resources)
		return
	}
	a.start(life)
}

func (a *privateApplication[D, R]) show() {
	if a.active != nil && a.current(a.active) && a.active.panels != nil {
		a.active.panels.Show()
	} else {
		a.retry()
	}
}

// retire detaches immediately and joins work away from the UI. A failed count or
// drain retains both devices and the retirement barrier; it never becomes zero.
func (a *privateApplication[D, R]) retire() {
	life := a.active
	a.active = nil
	if life == nil {
		return
	}
	life.retired = true
	if life.panels != nil {
		life.panels.Invalidate()
	}
	life.cancel()
	life.detachView(life.view)
	life.view = ApplicationView{}
	a.previous = life.retiredDone
	a.retirements = append(a.retirements, life)
	a.beginRetirement(life)
}

func (a *privateApplication[D, R]) beginRetirement(life *applicationLifetime[D, R]) {
	if life.retireRunning {
		return
	}
	life.retireRunning = true
	go func() {
		err := life.retirePreservingWork(context.Background())
		a.cfg.dispatch(func() {
			life.retireRunning = false
			if err != nil {
				if !a.closed {
					a.cfg.state(ApplicationFailed, err)
				}
				return
			}
			a.pruneRetirements()
			if !a.closed {
				a.retry()
			}
		})
	}()
}

func (life *applicationLifetime[D, R]) retirePreservingWork(ctx context.Context) error {
	life.retireMu.Lock()
	defer life.retireMu.Unlock()
	if life.previous != nil {
		select {
		case <-life.previous:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if life.initializationDone != nil {
		select {
		case <-life.initializationDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := life.joinPresenters(ctx); err != nil {
		return err
	}
	if !life.repositoryRetired && life.cycle != nil {
		if _, err := life.cycle.RetirePreservingWork(ctx); err != nil {
			return err
		}
		life.repositoryRetired = true
	}
	if life.plan.Retire != nil {
		if err := life.plan.Retire(ctx); err != nil {
			return err
		}
		life.plan.Retire = nil
	}
	if life.closePanels != nil {
		if err := life.closePanels(ctx); err != nil {
			return err
		}
		life.closePanels = nil
	}
	select {
	case <-life.retiredDone:
	default:
		close(life.retiredDone)
	}
	return nil
}

func (a *privateApplication[D, R]) close() {
	if a.closed {
		return
	}
	a.closed = true
	a.retire()
}

func (a *privateApplication[D, R]) pruneRetirements() {
	retained := a.retirements[:0]
	for _, life := range a.retirements {
		select {
		case <-life.retiredDone:
			life.resources = nil
			life.cycle = nil
			life.plan = ApplicationPlan[R]{}
		default:
			retained = append(retained, life)
		}
	}
	clear(a.retirements[len(retained):])
	a.retirements = retained
}
