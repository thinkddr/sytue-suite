// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type factoryDevice struct {
	consumer              string
	version               storage.IdentityVersion
	user, tenant          string
	status                mx.VerificationStatus
	binding               storage.Binding
	prepareErr, statusErr error
	afterPrepare          func()
	preparations, closes  int
}

func (d *factoryDevice) NativeConsumer() string                          { return d.consumer }
func (d *factoryDevice) WaitReady(ctx context.Context) error             { return ctx.Err() }
func (d *factoryDevice) StorageIdentityVersion() storage.IdentityVersion { return d.version }
func (d *factoryDevice) VerificationStatus(context.Context) (mx.VerificationStatus, error) {
	return d.status, d.statusErr
}
func (d *factoryDevice) Identity() (string, string) { return d.user, d.tenant }
func (d *factoryDevice) DeviceID() string           { return d.status.DeviceID }
func (d *factoryDevice) PrepareStorage(ctx context.Context, _ storage.Scope) (storage.Binding, error) {
	d.preparations++
	if d.afterPrepare != nil {
		d.afterPrepare()
	}
	if err := ctx.Err(); err != nil {
		return storage.Binding{}, err
	}
	return d.binding, d.prepareErr
}

func (d *factoryDevice) StorageBindings(ctx context.Context) (storage.Binding, error) {
	return d.binding, ctx.Err()
}

// These methods detect accidental ownership even if StorageDevice is expanded.
func (d *factoryDevice) Close() error                 { d.closes++; return nil }
func (d *factoryDevice) Logout(context.Context) error { d.closes++; return nil }

func factoryTokens(subject string) auth.Tokens {
	return auth.Tokens{Issuer: "https://id.example.test", Cliente: "suite", Subject: subject, Organizacion: "acme", Access: "synthetic-factory-access", Refresh: "synthetic-factory-refresh", Expiry: time.Now().Add(time.Hour)}
}

func factoryFixture(t *testing.T) (Factory[*factoryDevice], context.Context, *auth.Session, Devices[*factoryDevice]) {
	t.Helper()
	session := auth.NewSession(factoryTokens("alice"), nil, nil)
	ctx := session.WithAuthorizationIntent(t.Context())
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("APPDATA", configDir)
	t.Setenv("HOME", configDir)
	witnessStore := "sesion-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ports, err := nativestoragetest.NewSessionPorts(ctx, session, witnessStore, func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	device := func(version storage.IdentityVersion, name string) *factoryDevice {
		user := "@legacy:rooms.example.test"
		if version == storage.IsolatedIdentity {
			user = storage.IsolatedMatrixUserPrefix(scope) + "rooms.example.test"
		}
		return &factoryDevice{
			version: version, user: user, tenant: "acme",
			status:  mx.VerificationStatus{HasIdentity: true, Verified: true, DeviceID: name},
			binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!" + name + ":rooms.example.test", DeviceID: name}, Rooms: []string{"!" + name + ":rooms.example.test"}},
		}
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	factory := Factory[*factoryDevice]{Product: "mail", OpenJournal: func(ctx context.Context) (*offline.Store, error) {
		return journal, ctx.Err()
	}, CaptureWork: ports.CaptureWork, RegisterWork: ports.RegisterWork, CurrentIdentity: ports.CurrentIdentity, BindHTTP: ports.BindHTTP}

	return factory, ctx, session, Devices[*factoryDevice]{Legacy: device(storage.LegacyIdentity, "LEGACY"), Isolated: device(storage.IsolatedIdentity, "PRIVATE"), RequireLegacy: true}
}

func factoryAdopt(t *testing.T, witness auth.TestigoTrabajo, session *auth.Session, tokens auth.Tokens) {
	t.Helper()
	identity := auth.IdentitySnapshot{Issuer: tokens.Issuer, Client: tokens.Cliente, Subject: tokens.Subject, Organization: tokens.Organizacion}
	if err := ajustes.PonerAccesoSesionCon(t.Context(), witness.Almacen, identity.IdentidadSesion(), tokens.Subject != ""); err != nil {
		t.Fatal(err)
	}
	session.Sincronizar(tokens)
}

func TestFactoryRetainsBothAuthenticatedDevicesAndJournalAcrossRetry(t *testing.T) {
	f, ctx, _, devices := factoryFixture(t)
	devices.Legacy.binding.Rooms = append(devices.Legacy.binding.Rooms, "!retained:rooms.example.test")
	r, err := f.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	if r.Devices.Legacy != devices.Legacy || r.Devices.Isolated != devices.Isolated || r.Journal == nil || r.Identity.Subject != "alice" {
		t.Fatal("factory replaced a source, destination, journal or owner")
	}
	if devices.Legacy.preparations != 2 || devices.Isolated.preparations != 2 {
		t.Fatal("both authenticated owner keys were not prepared")
	}
	// Harmless room ordering is not rotation; dropping retained history is.
	slices.Reverse(devices.Legacy.binding.Rooms)
	if err := r.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	devices.Isolated.status.RecoverySetupPending = true
	if err := r.Revalidate(ctx); !errors.Is(err, mx.ErrRecoveryRequired) {
		t.Fatal("unacknowledged destination recovery remained usable", err)
	}
	devices.Isolated.status.RecoverySetupPending = false
	if err := r.Revalidate(ctx); err != nil {
		t.Fatal("retry could not retain both clients", err)
	}
	if devices.Legacy.closes+devices.Isolated.closes != 0 {
		t.Fatal("resource validation revoked borrowed clients")
	}
	devices.Legacy.binding.Rooms = devices.Legacy.binding.Rooms[1:]
	if err := r.Revalidate(ctx); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("lost retained source room was not detected", err)
	}
}

func TestFactoryRevalidateRequiresBothAuthenticatedOwnerKeys(t *testing.T) {
	for _, source := range []bool{true, false} {
		f, ctx, _, devices := factoryFixture(t)
		r, err := f.Open(ctx, devices)
		if err != nil {
			t.Fatal(err)
		}
		device := devices.Isolated
		if source {
			device = devices.Legacy
		}
		device.prepareErr = mx.ErrRecoveryRequired
		if err := r.Revalidate(ctx); !errors.Is(err, mx.ErrRecoveryRequired) {
			t.Fatal("lost authenticated owner key allowed source freeze", source, err)
		}
		device.prepareErr = nil
		if err := r.Revalidate(ctx); err != nil {
			t.Fatal("restored key could not resume retained migration", err)
		}
	}
}

func TestFactoryRejectsInvalidResourceBeforeOpeningJournal(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Devices[*factoryDevice])
		want   error
	}{
		{"missing source", func(d *Devices[*factoryDevice]) { d.Legacy = nil }, ErrPrivateResources},
		{"missing destination", func(d *Devices[*factoryDevice]) { d.Isolated = nil }, ErrPrivateResources},
		{"legacy destination", func(d *Devices[*factoryDevice]) { d.Isolated.version = storage.LegacyIdentity }, ErrPrivateResources},
		{"foreign owner", func(d *Devices[*factoryDevice]) { d.Isolated.binding.Scope.Subject = "bob" }, ErrPrivateResources},
		{"foreign product", func(d *Devices[*factoryDevice]) { d.Legacy.binding.Scope.Product = "drive" }, ErrPrivateResources},
		{"foreign tenant", func(d *Devices[*factoryDevice]) { d.Isolated.tenant = "other" }, ErrPrivateResources},
		{"shared namespace", func(d *Devices[*factoryDevice]) { d.Isolated.user = d.Legacy.user }, ErrPrivateResources},
		{"same room", func(d *Devices[*factoryDevice]) {
			d.Isolated.binding.Target.RoomID = d.Legacy.binding.Target.RoomID
			d.Isolated.binding.Rooms = slices.Clone(d.Legacy.binding.Rooms)
		}, ErrPrivateResources},
		{"unverified source", func(d *Devices[*factoryDevice]) { d.Legacy.status.Verified = false }, mx.ErrUnverified},
		{"unverified destination", func(d *Devices[*factoryDevice]) { d.Isolated.status.Verified = false }, mx.ErrUnverified},
		{"unacknowledged recovery", func(d *Devices[*factoryDevice]) { d.Isolated.status.RecoverySetupPending = true }, mx.ErrRecoveryRequired},
		{"lost source owner key", func(d *Devices[*factoryDevice]) { d.Legacy.prepareErr = mx.ErrRecoveryRequired }, mx.ErrRecoveryRequired},
		{"lost destination owner key", func(d *Devices[*factoryDevice]) { d.Isolated.prepareErr = mx.ErrRecoveryRequired }, mx.ErrRecoveryRequired},
		{"verification error", func(d *Devices[*factoryDevice]) { d.Isolated.statusErr = mx.ErrSecureStore }, mx.ErrSecureStore},
		{"state changes during preparation", func(d *Devices[*factoryDevice]) {
			d.Isolated.afterPrepare = func() { d.Isolated.status.Verified = false }
		}, mx.ErrUnverified},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, ctx, _, devices := factoryFixture(t)
			test.change(&devices)
			opened := 0
			original := f.OpenJournal
			f.OpenJournal = func(ctx context.Context) (*offline.Store, error) { opened++; return original(ctx) }
			if r, err := f.Open(ctx, devices); r != nil || !errors.Is(err, test.want) {
				t.Fatal("invalid device resources admitted", r, err)
			}
			if opened != 0 {
				t.Fatal("invalid device created application journal")
			}
		})
	}
}

func TestFactoryRequiresCurrentAuthorizationIntentIncludingABA(t *testing.T) {
	f, ctx, session, devices := factoryFixture(t)
	if _, err := f.Open(t.Context(), devices); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("unbound lifetime admitted", err)
	}
	r, err := f.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	factoryAdopt(t, r.session.witness, session, factoryTokens("bob"))
	factoryAdopt(t, r.session.witness, session, factoryTokens("alice"))
	if err := r.Check(ctx); err == nil {
		t.Fatal("old resources revived after A→B→A", err)
	}
	if _, err := f.Open(ctx, devices); !errors.Is(err, auth.ErrAuthorizationChanged) {
		t.Fatal("new resources adopted a later session", err)
	}
	if devices.Legacy.closes+devices.Isolated.closes != 0 {
		t.Fatal("stale presenter revoked another client's keys")
	}
}

func TestFactoryAlreadyIsolatedNeedsNoLegacyAndPinsRouting(t *testing.T) {
	f, ctx, _, devices := factoryFixture(t)
	devices.Legacy, devices.RequireLegacy = nil, false
	r, err := f.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	devices.Isolated.binding.Target.RoomID = "!rotated:rooms.example.test"
	devices.Isolated.binding.Rooms = append(devices.Isolated.binding.Rooms, devices.Isolated.binding.Target.RoomID)
	if err := r.Revalidate(ctx); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("target rotation moved a prepared migration", err)
	}
}

func TestFactoryRejectsJournalFailureAndLifetimeCancellation(t *testing.T) {
	for _, test := range []string{"unencrypted", "io error", "cancelled", "identity changed"} {
		t.Run(test, func(t *testing.T) {
			f, ctx, session, devices := factoryFixture(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			original := f.OpenJournal
			want := offline.ErrEncryptedStore
			f.OpenJournal = func(ctx context.Context) (*offline.Store, error) {
				switch test {
				case "unencrypted":
					return new(offline.Store), nil
				case "io error":
					return nil, offline.ErrWorkClosed
				case "cancelled":
					cancel()
				case "identity changed":
					session.Sincronizar(factoryTokens("bob"))
				}
				return original(ctx)
			}
			switch test {
			case "io error":
				want = offline.ErrWorkClosed
			case "cancelled":
				want = context.Canceled
			case "identity changed":
				want = auth.ErrAuthorizationChanged
			}
			if r, err := f.Open(ctx, devices); r != nil || !errors.Is(err, want) {
				t.Fatal("failed journal/lifetime opened resources", r, err)
			}
		})
	}
}

func TestConsumerFactoryRejectsDriveOrAnotherAppsDeviceBeforePreparation(t *testing.T) {
	f, ctx, _, devices := factoryFixture(t)
	f.NativeConsumer = "pdf"
	f.Product = "drive"
	for _, device := range []*factoryDevice{devices.Legacy, devices.Isolated} {
		device.binding.Scope.Product = "drive"
		if device.version == storage.IsolatedIdentity {
			device.user = storage.IsolatedMatrixUserPrefix(device.binding.Scope) + "rooms.example.test"
		}
	}
	if _, err := f.Open(ctx, devices); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("consumer factory adopted product owner device", err)
	}
	if devices.Legacy.preparations != 0 || devices.Isolated.preparations != 0 {
		t.Fatal("foreign device touched application storage")
	}
	devices.Legacy.consumer = "pdf"
	devices.Isolated.consumer = "docs"
	if _, err := f.Open(ctx, devices); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("consumer factory adopted another recipient device", err)
	}
	devices.Isolated.consumer = "pdf"
	if _, err := f.Open(ctx, devices); err != nil {
		t.Fatal("consumer factory rejected own devices", err)
	}
}

func TestFactoryJournalMigrationCanTakeExclusiveSessionLeaseAfterDeviceChecks(t *testing.T) {
	factory, ctx, _, devices := factoryFixture(t)
	original := factory.OpenJournal
	factory.OpenJournal = func(job context.Context) (*offline.Store, error) {
		witness, err := factory.CaptureWork(job, storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"})
		if err != nil {
			return nil, err
		}
		release, err := witness.Exclusivo(job)
		if err != nil {
			return nil, err
		}
		defer release()
		if devices.Legacy.preparations != 1 || devices.Isolated.preparations != 1 {
			t.Fatal("journal migration preceded device checks")
		}
		return original(job)
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := factory.Open(bounded, devices); err != nil {
		t.Fatal("verified journal migration deadlocked behind its own shared lease", err)
	}
}
