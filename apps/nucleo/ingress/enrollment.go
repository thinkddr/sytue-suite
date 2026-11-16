// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type EnrollmentSource interface {
	Authority(context.Context, storage.Scope) (matrixproof.Authority, error)
	Enrollment(context.Context) (contract.EnrollmentState, error)
	Enroll(context.Context, contract.KeyPlan, storage.PublicationTarget, matrixproof.Authority) error
}
type EnrollmentDevice interface {
	CheckStoragePublicationTarget(storage.PublicationTarget) error
	CreateIngressKey(context.Context, storage.Scope, string, int64) (contract.KeyPlan, error)
	RecoverIngressEnrollment(context.Context, contract.KeyPlan) (contract.KeyPlan, error)
}
type EnrollmentConfig struct {
	Context   context.Context
	Scope     storage.Scope
	Target    storage.PublicationTarget
	Authority matrixproof.Authority
	Journal   *offline.Store
	Lease     offline.WorkLease
	Device    EnrollmentDevice
	Source    func(string) (EnrollmentSource, error)
	Authorize func(context.Context) error
}

// Enrollment owns public routing intent in the encrypted application WAL. Age
// identities remain exclusively inside the maintained SDK and its SSSS backup.
type (
	Enrollment struct {
		cfg  EnrollmentConfig
		gate *offline.WorkGate
	}
	enrollmentIntent struct {
		Version   int                       `json:"v"`
		Scope     storage.Scope             `json:"scope"`
		Epoch     string                    `json:"epoch"`
		Target    storage.PublicationTarget `json:"target"`
		Authority matrixproof.Authority     `json:"authority"`
		Plan      *contract.KeyPlan         `json:"plan,omitempty"`
		Rejected  []contract.KeyPlan        `json:"rejected,omitempty"`
	}
)

func NewEnrollment(cfg EnrollmentConfig) (*Enrollment, error) {
	if cfg.Context == nil || !cfg.Scope.Valid() || (cfg.Scope.Product != "mail" && cfg.Scope.Product != "calendar") || !cfg.Target.Valid() || cfg.Authority.RequireActive(cfg.Scope, cfg.Authority.Generation) != nil || !cfg.Journal.Encrypted() || cfg.Lease == nil || cfg.Device == nil || cfg.Source == nil || cfg.Authorize == nil {
		return nil, contract.ErrContract
	}
	return &Enrollment{cfg: cfg, gate: offline.NewWorkGateWithLease(cfg.Context, cfg.Lease)}, nil
}

func (e *Enrollment) owner() string {
	return e.cfg.Scope.Tenant + "\x00" + e.cfg.Scope.Subject + "\x00" + e.cfg.Scope.Product
}
func (e *Enrollment) PrivateStorageScope() storage.Scope { return e.cfg.Scope }
func (e *Enrollment) Drain(ctx context.Context) error    { return e.gate.Drain(ctx) }
func (e *Enrollment) Resume() error                      { return e.gate.Resume() }
func (e *Enrollment) Close()                             { e.gate.Close() }
func (e *Enrollment) Pending(ctx context.Context) (offline.PendingWork, error) {
	out := offline.PendingWork{Scope: e.cfg.Scope}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	count, err := e.cfg.Journal.CountOwner(e.owner(), storage.PrivateIngressEnrollmentNamespace)
	out.Count = count
	return out, err
}

// Ensure is explicit receipt setup for one source epoch, before SQL Begin or
// automatic queue execution. Only an authenticated absent response permits a
// new candidate. Errors and revoked generations never generate or rotate keys.
func (e *Enrollment) Ensure(parent context.Context, epoch string) (contract.KeyPlan, error) {
	if !storage.MigrationEpoch(epoch) {
		return contract.KeyPlan{}, contract.ErrContract
	}
	ctx, done, err := e.gate.Start(parent)
	if err != nil {
		return contract.KeyPlan{}, err
	}
	defer done()
	var plan contract.KeyPlan
	err = e.cfg.Journal.Exclusive(ctx, e.owner(), storage.PrivateIngressEnrollmentNamespace, epoch, func() error {
		return e.ensureAdmitted(ctx, epoch, &plan)
	})
	return plan, err
}

func (e *Enrollment) ensureAdmitted(ctx context.Context, epoch string, plan *contract.KeyPlan) error {
	source, err := e.cfg.Source(epoch)
	if err != nil {
		return err
	}
	if source == nil {
		return contract.ErrContract
	}
	if err := e.revalidate(ctx, source); err != nil {
		return err
	}
	state, err := source.Enrollment(ctx)
	if err != nil {
		return err
	}
	if state.Validate(e.cfg.Scope, epoch) != nil {
		return contract.ErrContract
	}
	if state.Status == "revoked" {
		return contract.ErrRevoked
	}
	work, err := e.loadIntent(epoch)
	if err != nil {
		return err
	}
	if err := e.selectPlan(ctx, state, &work); err != nil {
		return err
	}
	if err := e.revalidate(ctx, source); err != nil {
		return err
	}
	if err := source.Enroll(ctx, *work.Plan, e.cfg.Target, e.cfg.Authority); err != nil {
		return err
	}
	confirmed, err := source.Enrollment(ctx)
	if err != nil {
		return err
	}
	if confirmed.Validate(e.cfg.Scope, epoch) != nil || confirmed.Status != "active" || confirmed.Plan == nil || *confirmed.Plan != *work.Plan {
		return contract.ErrRevoked
	}
	if err := e.revalidate(ctx, source); err != nil {
		return err
	}
	if err := e.archive(ctx, work); err != nil {
		return err
	}
	*plan = *work.Plan
	return nil
}

func (e *Enrollment) revalidate(ctx context.Context, source EnrollmentSource) error {
	if err := e.cfg.Authorize(ctx); err != nil {
		return err
	}
	if err := e.cfg.Device.CheckStoragePublicationTarget(e.cfg.Target); err != nil {
		return err
	}
	fresh, err := source.Authority(ctx, e.cfg.Scope)
	if err != nil {
		return err
	}
	if fresh != e.cfg.Authority {
		return contract.ErrRevoked
	}
	return ctx.Err()
}

func (e *Enrollment) loadIntent(epoch string) (enrollmentIntent, error) {
	var work enrollmentIntent
	found, err := e.cfg.Journal.Read(e.owner(), storage.PrivateIngressEnrollmentNamespace, epoch, &work)
	if err != nil {
		return work, err
	}
	if !found {
		work = enrollmentIntent{Version: 1, Scope: e.cfg.Scope, Epoch: epoch, Target: e.cfg.Target, Authority: e.cfg.Authority}
	}
	if work.Version != 1 || work.Scope != e.cfg.Scope || work.Epoch != epoch || work.Target != e.cfg.Target || work.Authority != e.cfg.Authority {
		return work, contract.ErrRevoked
	}
	if work.Plan != nil && work.Plan.Validate(e.cfg.Scope, epoch) != nil {
		return work, contract.ErrContract
	}
	for _, rejected := range work.Rejected {
		if rejected.Validate(e.cfg.Scope, epoch) != nil {
			return work, contract.ErrContract
		}
	}
	return work, nil
}

func (e *Enrollment) selectPlan(ctx context.Context, state contract.EnrollmentState, work *enrollmentIntent) error {
	history, found, err := e.history(work.Epoch)
	if err != nil {
		return err
	}
	if found {
		if state.Plan != nil && *state.Plan != *history.Plan {
			return contract.ErrRevoked
		}
		if work.Plan == nil {
			work.Plan = history.Plan
		}
	}
	switch state.Status {
	case "active":
		if err := e.recover(ctx, *state.Plan); err != nil {
			return err
		}
		if work.Plan != nil && *work.Plan != *state.Plan {
			work.Rejected = append(work.Rejected, *work.Plan)
		}
		plan := *state.Plan
		work.Plan = &plan
	case "absent":
		if work.Plan == nil {
			if err := e.save(*work); err != nil {
				return err
			} // Retain scope/epoch/device before the first SDK candidate write.
			plan, err := e.cfg.Device.CreateIngressKey(ctx, e.cfg.Scope, work.Epoch, 1)
			if err != nil {
				return err
			}
			work.Plan = &plan
		} else {
			if err := e.recover(ctx, *work.Plan); err != nil {
				return err
			}
		}
	default:
		return contract.ErrContract
	}
	if work.Plan == nil || work.Plan.Validate(e.cfg.Scope, work.Epoch) != nil {
		return contract.ErrContract
	}
	return e.save(*work)
}

func (e *Enrollment) save(work enrollmentIntent) error {
	return e.cfg.Journal.Write(e.owner(), storage.PrivateIngressEnrollmentNamespace, work.Epoch, work)
}

func (e *Enrollment) historyID(epoch string) string {
	raw, _ := json.Marshal(e.cfg.Target)
	sum := sha256.Sum256(raw)
	return epoch + "_" + hex.EncodeToString(sum[:])
}

func (e *Enrollment) history(epoch string) (enrollmentIntent, bool, error) {
	var history enrollmentIntent
	found, err := e.cfg.Journal.Read(e.owner(), storage.PrivateIngressEnrollmentHistoryNamespace, e.historyID(epoch), &history)
	if err != nil || !found {
		return history, found, err
	}
	if history.Version != 1 || history.Scope != e.cfg.Scope || history.Epoch != epoch || history.Target != e.cfg.Target || history.Authority.RootKey != e.cfg.Authority.RootKey || history.Plan == nil || history.Plan.Validate(e.cfg.Scope, epoch) != nil {
		return history, true, contract.ErrRevoked
	}
	return history, true, nil
}

func (e *Enrollment) archive(ctx context.Context, work enrollmentIntent) error {
	previous, found, err := e.history(work.Epoch)
	if err != nil {
		return err
	}
	if found {
		if *previous.Plan != *work.Plan {
			return contract.ErrRevoked
		}
	} else {
		if err := e.cfg.Journal.Write(e.owner(), storage.PrivateIngressEnrollmentHistoryNamespace, e.historyID(work.Epoch), work); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.cfg.Journal.Delete(e.owner(), storage.PrivateIngressEnrollmentNamespace, work.Epoch)
}

func (e *Enrollment) recover(ctx context.Context, expected contract.KeyPlan) error {
	actual, err := e.cfg.Device.RecoverIngressEnrollment(ctx, expected)
	if err != nil {
		return err
	}
	if actual != expected {
		return contract.ErrContract
	}
	return nil
}
