// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"io"
	"testing"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type enrollmentFixture struct {
	cfg    EnrollmentConfig
	epoch  string
	source *enrollmentSourceFixture
	device *enrollmentDeviceFixture
}
type enrollmentDeviceFixture struct {
	t                 *testing.T
	journal           *offline.Store
	target            storage.PublicationTarget
	public            string
	known             map[contract.KeyPlan]bool
	creates, recovers int
	beforeRecover     func()
}

func (d *enrollmentDeviceFixture) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != d.target {
		return contract.ErrRevoked
	}
	return nil
}

func (d *enrollmentDeviceFixture) CreateIngressKey(_ context.Context, scope storage.Scope, epoch string, generation int64) (contract.KeyPlan, error) {
	var retained enrollmentIntent
	found, err := d.journal.Read(scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product, storage.PrivateIngressEnrollmentNamespace, epoch, &retained)
	if err != nil || !found || retained.Target != d.target || retained.Plan != nil {
		d.t.Fatal("SDK candidate preceded its original scoped intent", retained, err)
	}
	d.creates++
	plan := contract.KeyPlan{Version: 1, Scope: scope, Epoch: epoch, Generation: generation, Profile: contract.Profile, Recipient: d.public, KeyID: contract.PublicFingerprint(d.public)}
	d.known[plan] = true
	return plan, nil
}

func (d *enrollmentDeviceFixture) RecoverIngressEnrollment(_ context.Context, plan contract.KeyPlan) (contract.KeyPlan, error) {
	d.recovers++
	if d.beforeRecover != nil {
		d.beforeRecover()
	}
	if !d.known[plan] {
		return contract.KeyPlan{}, matrix.ErrRecoveryRequired
	}
	return plan, nil
}

type enrollmentSourceFixture struct {
	t              *testing.T
	journal        *offline.Store
	state          contract.EnrollmentState
	authority      matrixproof.Authority
	reads, enrolls int
	loseACK        bool
	failure        error
}

func (s *enrollmentSourceFixture) Authority(context.Context, storage.Scope) (matrixproof.Authority, error) {
	return s.authority, nil
}

func (s *enrollmentSourceFixture) Enrollment(context.Context) (contract.EnrollmentState, error) {
	s.reads++
	return s.state, s.failure
}

func (s *enrollmentSourceFixture) Enroll(_ context.Context, plan contract.KeyPlan, target storage.PublicationTarget, authority matrixproof.Authority) error {
	var retained enrollmentIntent
	found, err := s.journal.Read(plan.Scope.Tenant+"\x00"+plan.Scope.Subject+"\x00"+plan.Scope.Product, storage.PrivateIngressEnrollmentNamespace, plan.Epoch, &retained)
	if err != nil || !found || retained.Plan == nil || *retained.Plan != plan || retained.Target != target || retained.Authority != authority {
		s.t.Fatal("server enrollment preceded its exact retained SDK plan", retained, err)
	}
	s.enrolls++
	if authority != s.authority {
		return contract.ErrRevoked
	}
	s.state.Status = "active"
	s.state.Generation = plan.Generation
	retainedPlan := plan
	s.state.Plan = &retainedPlan
	if s.loseACK {
		s.loseACK = false
		return io.ErrUnexpectedEOF
	}
	return nil
}

func newEnrollmentFixture(t *testing.T) *enrollmentFixture {
	t.Helper()
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	authority, err := (&workerSource{}).Authority(t.Context(), scope)
	if err != nil {
		t.Fatal(err)
	}
	device := &enrollmentDeviceFixture{t: t, journal: journal, target: storage.PublicationTarget{RoomID: "!inbox:fixture", DeviceID: "ORIGINAL"}, public: identity.Recipient().String(), known: map[contract.KeyPlan]bool{}}
	source := &enrollmentSourceFixture{t: t, journal: journal, authority: authority, state: contract.EnrollmentState{Version: 1, Scope: scope, Epoch: epoch, Status: "absent"}}
	cfg := EnrollmentConfig{Context: t.Context(), Scope: scope, Target: device.target, Authority: authority, Journal: journal, Device: device, Lease: func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }, Authorize: func(ctx context.Context) error { return ctx.Err() }, Source: func(wanted string) (EnrollmentSource, error) {
		if wanted != epoch {
			return nil, contract.ErrContract
		}
		return source, nil
	}}
	return &enrollmentFixture{cfg: cfg, epoch: epoch, source: source, device: device}
}

func (f *enrollmentFixture) open(t *testing.T) *Enrollment {
	t.Helper()
	e, err := NewEnrollment(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}

func TestEnrollmentLostAcceptedReplyRetainsOriginalPlanAndArchivesOnlyAfterConfirmation(t *testing.T) {
	f := newEnrollmentFixture(t)
	f.source.loseACK = true
	e := f.open(t)
	if _, err := e.Ensure(t.Context(), f.epoch); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	pending, err := e.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("uncertain enrollment lost its durable operation", pending, err)
	}
	var original enrollmentIntent
	found, err := e.cfg.Journal.Read(e.owner(), storage.PrivateIngressEnrollmentNamespace, f.epoch, &original)
	if err != nil || !found || original.Plan == nil {
		t.Fatal(original, err)
	}
	if err := e.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	reads := f.source.reads
	if _, err := e.Ensure(t.Context(), f.epoch); !errors.Is(err, offline.ErrWorkPaused) || f.source.reads != reads {
		t.Fatal("drained initializer admitted a request", err)
	}
	e.Close()
	e = f.open(t)
	plan, err := e.Ensure(t.Context(), f.epoch)
	if err != nil || plan != *original.Plan || f.device.creates != 1 || f.device.recovers != 1 || f.source.enrolls != 2 {
		t.Fatal("restart replaced its accepted original candidate", plan, err, f.device.creates, f.device.recovers)
	}
	pending, err = e.Pending(t.Context())
	if err != nil || pending.Count != 0 {
		t.Fatal("confirmed enrollment stayed pending", pending, err)
	}
	retained, found, err := e.history(f.epoch)
	if err != nil || !found || retained.Plan == nil || *retained.Plan != plan || retained.Authority != original.Authority {
		t.Fatal("confirmation discarded original encrypted evidence", retained, err)
	}
}

func TestEnrollmentErrorsRevocationAndMissingSSSSNeverGenerateFallbackKeys(t *testing.T) {
	for _, fault := range []string{"http-404", "revoked", "foreign", "missing-ssss"} {
		t.Run(fault, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			e := f.open(t)
			switch fault {
			case "http-404":
				f.source.failure = errors.New("HTTP404 is not absence")
			case "revoked":
				f.source.state.Status = "revoked"
				f.source.state.Generation = 1
			case "foreign":
				f.source.state.Scope.Subject = "other"
			case "missing-ssss":
				plan := contract.KeyPlan{Version: 1, Scope: f.cfg.Scope, Epoch: f.epoch, Generation: 1, Profile: contract.Profile, Recipient: f.device.public, KeyID: contract.PublicFingerprint(f.device.public)}
				f.source.state.Status = "active"
				f.source.state.Generation = 1
				f.source.state.Plan = &plan
			}
			if _, err := e.Ensure(t.Context(), f.epoch); err == nil {
				t.Fatal("unsafe enrollment became a new candidate", fault)
			}
			if f.device.creates != 0 || f.source.enrolls != 0 {
				t.Fatal("ambiguous/revoked/missing recovery generated fallback routing", f.device.creates, f.source.enrolls)
			}
		})
	}
}

func TestEnrollmentRevalidatesRootAfterSDKRecoveryBeforeEffects(t *testing.T) {
	f := newEnrollmentFixture(t)
	e := f.open(t)
	if _, err := e.Ensure(t.Context(), f.epoch); err != nil {
		t.Fatal(err)
	}
	enrolls := f.source.enrolls
	f.device.beforeRecover = func() { f.source.authority.Generation++ }
	if _, err := e.Ensure(t.Context(), f.epoch); !errors.Is(err, contract.ErrRevoked) {
		t.Fatal("old root generation authorized fresh enrollment", err)
	}
	if f.source.enrolls != enrolls {
		t.Fatal("revoked authority reached server effects")
	}
	var original enrollmentIntent
	found, err := e.cfg.Journal.Read(e.owner(), storage.PrivateIngressEnrollmentNamespace, f.epoch, &original)
	if err != nil || !found || original.Authority != f.cfg.Authority || original.Plan == nil {
		t.Fatal("revoked original intent was discarded", original, err)
	}
}

func TestEnrollmentConfirmedHistoryRestoresSamePlanAndRejectsSilentRotation(t *testing.T) {
	f := newEnrollmentFixture(t)
	e := f.open(t)
	original, err := e.Ensure(t.Context(), f.epoch)
	if err != nil {
		t.Fatal(err)
	}
	f.source.state.Status = "absent"
	f.source.state.Generation = 0
	f.source.state.Plan = nil
	restored, err := e.Ensure(t.Context(), f.epoch)
	if err != nil || restored != original || f.device.creates != 1 {
		t.Fatal("explicit absence discarded the known confirmed recipient", restored, err, f.device.creates)
	}
	changed := original
	changed.Generation++
	f.device.known[changed] = true
	f.source.state.Generation = changed.Generation
	f.source.state.Plan = &changed
	enrolls := f.source.enrolls
	if _, err := e.Ensure(t.Context(), f.epoch); !errors.Is(err, contract.ErrRevoked) {
		t.Fatal("confirmed recipient silently rotated", err)
	}
	if f.source.enrolls != enrolls || f.device.creates != 1 {
		t.Fatal("unconsented rotation reached effects")
	}
	history, found, err := e.history(f.epoch)
	if err != nil || !found || *history.Plan != original {
		t.Fatal("rotation replaced original enrollment evidence", history, err)
	}
}
