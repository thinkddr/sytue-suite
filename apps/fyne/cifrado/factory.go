// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	ErrPrivateResources = errors.New("cifrado: private application resources do not match")
	ErrPrivatePending   = errors.New("cifrado: private application has retained work")
)

// StorageDevice is the maintained Matrix client's verification and key boundary.
// It deliberately has no Logout or Close: application resources borrow devices,
// including the legacy reader while an isolated migration is being retried.
type StorageDevice interface {
	NativeConsumer() string
	WaitReady(context.Context) error
	StorageIdentityVersion() storage.IdentityVersion
	VerificationStatus(context.Context) (mx.VerificationStatus, error)
	Identity() (string, string)
	DeviceID() string
	PrepareStorage(context.Context, storage.Scope) (storage.Binding, error)
	StorageBindings(context.Context) (storage.Binding, error)
}

// Devices retains both accounts across preparation, freezing and retry. A fully
// isolated account may omit Legacy. A migration must explicitly RequireLegacy.
type Devices[D StorageDevice] struct {
	Legacy, Isolated D
	RequireLegacy    bool
}

// Factory opens the common suite journal after checking both borrowed devices.
// Open must receive Session.WithAuthorizationIntent on the presenter's lifetime;
// comparing an identity snapshot alone would revive an old A→B→A presenter.
type Factory[D StorageDevice] struct {
	Product            string
	NativeConsumer     string
	configurePanel     func(*Panel) error
	OpenJournal        func(context.Context) (*offline.Store, error)
	CaptureWork        func(context.Context, storage.Scope) (auth.TestigoTrabajo, error)
	RegisterWork       func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error)
	RegisterTransition func(offline.PrivateWork, auth.TestigoTrabajo, sesion.PrepararTransicionPrivada) (func(), error)
	CurrentIdentity    func() (auth.IdentitySnapshot, bool)
	BindHTTP           func(context.Context, auth.TestigoTrabajo) (api.Session, error)
}

// Resources contains no credential or key. Its clients retain their own native
// key adapters; creating or closing a repository never tears either client down.
type Resources[D StorageDevice] struct {
	Identity        auth.IdentitySnapshot
	Journal         *offline.Store
	Devices         Devices[D]
	life            context.Context
	session         *ApplicationSession
	scope           storage.Scope
	legacy          storage.Binding
	isolated        storage.Binding
	nativeConsumer  string
	publicationMu   sync.RWMutex
	publicationBind offline.WorkContextBinder
}

var _ StorageDevice = (*mx.Client)(nil)

// Open authenticates device verification, acknowledged recovery and owner keys
// BEFORE any application callback can freeze a historical source.
func (f Factory[D]) Open(ctx context.Context, devices Devices[D]) (*Resources[D], error) {
	application, err := f.Capture(ctx)
	if err != nil {
		return nil, err
	}
	return f.open(application.Context(), application, devices) //nolint:contextcheck // Capture replaces only the short intent; native work retains the same cancellation with a persistent witness.
}

func (f Factory[D]) open(ctx context.Context, application *ApplicationSession, devices Devices[D]) (*Resources[D], error) {
	if application == nil || application.scope.Product != f.Product || nilDevice(devices.Isolated) || devices.RequireLegacy && nilDevice(devices.Legacy) {
		return nil, ErrPrivateResources
	}
	r := &Resources[D]{Identity: application.identity, Devices: devices, life: application.Context(), session: application, scope: application.scope, nativeConsumer: f.NativeConsumer}
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	if err := r.prepareDevices(ctx); err != nil {
		return nil, err
	}
	open := f.OpenJournal
	if open == nil {
		open = sesion.AlmacenConContexto
	}
	journal, err := open(ctx)
	r.Journal = journal
	if err != nil {
		return nil, err
	}
	if !r.Journal.Encrypted() {
		return nil, offline.ErrEncryptedStore
	}
	if err := r.Revalidate(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// Release the device-preparation SH before a native journal migration takes
// EX. Revalidate obtains a fresh SH after opening the retained journal.
func (r *Resources[D]) prepareDevices(ctx context.Context) error {
	release, err := r.WorkLease()(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !nilDevice(r.Devices.Legacy) {
		r.legacy, err = r.prepare(ctx, r.Devices.Legacy, storage.LegacyIdentity)
		if err != nil {
			return err
		}
	}
	r.isolated, err = r.prepare(ctx, r.Devices.Isolated, storage.IsolatedIdentity)
	if err != nil {
		return err
	}
	if !nilDevice(r.Devices.Legacy) {
		oldUser, _ := r.Devices.Legacy.Identity()
		newUser, _ := r.Devices.Isolated.Identity()
		if oldUser == newUser || r.legacy.Target.RoomID == r.isolated.Target.RoomID {
			return ErrPrivateResources
		}
	}
	return nil
}

func nilDevice[D StorageDevice](device D) bool { return nilReference(device) }

func nilReference(value any) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Context is the retained presenter lifetime, for repository Config.Context.
// Request contexts passed to preparation are cancelled after each operation.
func (r *Resources[D]) Context() context.Context { return r.life }

func (r *Resources[D]) PrivateStorageScope() storage.Scope { return r.scope }
func (r *Resources[D]) WorkLease() offline.WorkLease       { return r.session.WorkLease() }
func (r *Resources[D]) HTTP() api.Session                  { return r.session.HTTP() }
func (r *Resources[D]) Register(ctx context.Context, work offline.PrivateWork) (func(), error) {
	return r.session.Register(ctx, work)
}

func (r *Resources[D]) RegisterTransition(ctx context.Context, work offline.PrivateWork, prepare sesion.PrepararTransicionPrivada) (func(), error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	return r.session.RegisterTransition(ctx, work, prepare)
}

// Check rejects cancellation and session adoption, including a later login to
// the same account. It does not acquire a token or hold an OIDC lock over IO.
func (r *Resources[D]) Check(ctx context.Context) error {
	if r.Identity != r.session.identity {
		return auth.ErrAuthorizationChanged
	}
	return r.session.Check(ctx)
}

func (r *Resources[D]) prepare(ctx context.Context, device D, version storage.IdentityVersion) (storage.Binding, error) {
	if err := device.WaitReady(ctx); err != nil {
		return storage.Binding{}, err
	}
	if err := r.verify(ctx, device, version); err != nil {
		return storage.Binding{}, err
	}
	binding, err := device.PrepareStorage(ctx, r.scope)
	if err != nil {
		return storage.Binding{}, err
	}
	if !binding.Valid() || binding.Scope != r.scope || binding.Target.DeviceID != device.DeviceID() {
		return storage.Binding{}, ErrPrivateResources
	}
	binding.Rooms = slices.Clone(binding.Rooms)
	return binding, r.verify(ctx, device, version)
}

func (r *Resources[D]) verify(ctx context.Context, device D, version storage.IdentityVersion) error {
	if err := r.Check(ctx); err != nil {
		return err
	}
	user, tenant := device.Identity()
	if device.NativeConsumer() != r.nativeConsumer || device.StorageIdentityVersion() != version || tenant != r.scope.Tenant || user == "" {
		return ErrPrivateResources
	}
	if version == storage.IsolatedIdentity {
		prefix := storage.IsolatedMatrixUserPrefix(r.scope)
		if !strings.HasPrefix(user, prefix) || len(user) <= len(prefix) {
			return ErrPrivateResources
		}
	}
	status, err := device.VerificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.HasIdentity || !status.Verified || status.DeviceID == "" || status.DeviceID != device.DeviceID() {
		return mx.ErrUnverified
	}
	if status.RecoverySetupPending {
		return mx.ErrRecoveryRequired
	}
	return r.Check(ctx)
}

// Revalidate authenticates the original routing again around application phases.
// Rotation or verification loss cannot silently move a partially frozen source.
func (r *Resources[D]) Revalidate(ctx context.Context) error {
	release, err := r.WorkLease()(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !nilDevice(r.Devices.Legacy) {
		if err := r.revalidate(ctx, r.Devices.Legacy, storage.LegacyIdentity, r.legacy); err != nil {
			return err
		}
	}
	return r.revalidate(ctx, r.Devices.Isolated, storage.IsolatedIdentity, r.isolated)
}

func (r *Resources[D]) revalidate(ctx context.Context, device D, version storage.IdentityVersion, original storage.Binding) error {
	if err := r.verify(ctx, device, version); err != nil {
		return err
	}
	current, err := device.PrepareStorage(ctx, r.scope)
	if err != nil {
		return err
	}
	if !original.Same(current) {
		return ErrPrivateResources
	}
	return r.verify(ctx, device, version)
}
