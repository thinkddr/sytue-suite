// SPDX-License-Identifier: Apache-2.0
package merge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Admission retains the original merge intent, not an open lock/transaction.
// Each closure commits before the cross-database data preflight starts.
type Admission struct {
	dsn            string
	owner          *Ownership
	job            uuid.UUID
	source, target string
	services       []string
}

func PrepareAdmission(ctx context.Context, idDSN, jobID, source, target string) (*Admission, error) {
	return prepareAdmission(ctx, idDSN, nil, jobID, source, target)
}

func PrepareOwnedAdmission(ctx context.Context, owner *Ownership, source, target string) (*Admission, error) {
	if owner == nil {
		return nil, fmt.Errorf("merge: missing ownership")
	}
	return prepareAdmission(ctx, "", owner, owner.JobID(), source, target)
}

func prepareAdmission(ctx context.Context, idDSN string, owner *Ownership, jobID, source, target string) (*Admission, error) {
	job, err := uuid.Parse(jobID)
	if err != nil || job == uuid.Nil || !tenancy.ValidSlug(source) || !tenancy.ValidSlug(target) || source == target {
		return nil, fmt.Errorf("merge: invalid admission intent")
	}
	admission := &Admission{dsn: idDSN, owner: owner, job: job, source: source, target: target}
	admission.services, err = admissionServices(ctx, idDSN, owner)
	if err == nil {
		err = admission.transition(ctx, "close")
	}
	if err == nil {
		err = preflight(ctx, idDSN, owner, source, target)
	}
	if err != nil {
		// Reopen only this job's still-closed pairs after a failed preparation.
		// Cancellation must not strand unrelated valid scopes behind our closure.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, admission.Abort(cleanup))
	}
	return admission, nil
}

func admissionServices(ctx context.Context, idDSN string, owner *Ownership) ([]string, error) {
	services := []string{}
	var failures []error
	for _, service := range Services {
		pool, closeDatabase, err := serviceDatabase(ctx, idDSN, owner, service)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", service, err))
			continue
		}
		inventory, err := pg.ConsultaInventarioDeFusion(ctx, pool)
		if err == nil && len(inventory.ScopeBound)+len(inventory.PrivateStates) > 0 {
			services = append(services, service)
			err = pg.RequireScopeAdmissionCoverage(ctx, pool, inventory)
		}
		closeDatabase()
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", service, err))
		}
	}
	return services, errors.Join(failures...)
}

func (a *Admission) transition(ctx context.Context, phase string) error {
	var failures []error
	for _, service := range a.services {
		err := a.serviceTransition(ctx, service, phase)
		if err != nil {
			failures = append(failures, fmt.Errorf("merge admission %s %s: %w", service, phase, err))
			if phase == "close" {
				break
			}
		}
	}
	return errors.Join(failures...)
}

func (a *Admission) serviceTransition(ctx context.Context, service, phase string) error {
	pool, closeDatabase, err := serviceDatabase(ctx, a.dsn, a.owner, service)
	if err != nil {
		return err
	}
	defer closeDatabase()
	return pool.InTx(ctx, func(tx pgx.Tx) error { return pg.TransitionScopeAdmission(ctx, tx, a.job, a.source, a.target, phase) })
}

func (a *Admission) Abort(ctx context.Context) error {
	if a == nil {
		return nil
	}
	return a.transition(ctx, "abort")
}

func (a *Admission) Complete(ctx context.Context) error {
	if a == nil {
		return fmt.Errorf("merge: admission completion missing")
	}
	return a.transition(ctx, "complete")
}

// CompleteAdmission reconciles only the retained job after ID has committed
// the merge. It does not move domains or reject new, valid target ciphertext.
func CompleteAdmission(ctx context.Context, idDSN, jobID, source, target string) error {
	return completeAdmission(ctx, idDSN, nil, jobID, source, target)
}

func CompleteOwnedAdmission(ctx context.Context, owner *Ownership, source, target string) error {
	if owner == nil {
		return fmt.Errorf("merge: missing ownership")
	}
	return completeAdmission(ctx, "", owner, owner.JobID(), source, target)
}

func completeAdmission(ctx context.Context, idDSN string, owner *Ownership, jobID, source, target string) error {
	job, err := uuid.Parse(jobID)
	if err != nil || job == uuid.Nil {
		return fmt.Errorf("merge: invalid completion intent")
	}
	services, err := admissionServices(ctx, idDSN, owner)
	if err != nil {
		return err
	}
	return (&Admission{dsn: idDSN, owner: owner, job: job, source: source, target: target, services: services}).Complete(ctx)
}
