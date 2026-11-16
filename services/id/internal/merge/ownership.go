// SPDX-License-Identifier: Apache-2.0
package merge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

type ownedDatabase struct {
	pool    *pg.Pool
	session *pg.Session
}

// Ownership holds the existing job's lock on the very sessions that execute
// its SQL. No clock/heartbeat can transfer uncertain writes to another worker.
// All databases are acquired before work; identity is released last.
type Ownership struct {
	job       string
	databases map[string]ownedDatabase
	order     []string
}

func AcquireOwnership(ctx context.Context, idDSN, jobID string) (*Ownership, error) {
	job, err := uuid.Parse(jobID)
	if err != nil || job == uuid.Nil {
		return nil, fmt.Errorf("merge: invalid ownership intent")
	}
	owner := &Ownership{job: job.String(), databases: make(map[string]ownedDatabase)}
	direct := os.Getenv(pg.DirectDSNEnv)
	if direct == "" {
		direct = idDSN
	}
	for _, service := range append([]string{"id"}, Services...) {
		dsn, err := DSNForDatabase(direct, service)
		if err == nil {
			err = owner.acquire(ctx, dsn, service)
		}
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			closeErr := owner.Close(cleanup)
			cancel()
			return nil, errors.Join(fmt.Errorf("merge: own %s: %w", service, err), closeErr)
		}
	}
	return owner, nil
}

func (o *Ownership) acquire(ctx context.Context, dsn, service string) error {
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, DirectDSN: dsn, MaxConns: 1})
	if err != nil {
		return err
	}
	session, err := pg.TrySessionLock(ctx, pool, "organization-operation:"+o.job, pg.OrganizationOperationLockGroup)
	if err != nil {
		pool.Close()
		return err
	}
	o.databases[service] = ownedDatabase{pool, session}
	o.order = append(o.order, service)
	return nil
}
func (o *Ownership) Identity() pg.Database { return o.Service("id") }
func (o *Ownership) Service(service string) pg.Database {
	if o == nil {
		return nil
	}
	db, exists := o.databases[service]
	if !exists {
		return nil
	}
	return db.session
}

func (o *Ownership) JobID() string {
	if o == nil {
		return ""
	}
	return o.job
}

func (o *Ownership) Close(ctx context.Context) error {
	if o == nil {
		return nil
	}
	var failures []error
	for i := len(o.order) - 1; i >= 0; i-- {
		service := o.order[i]
		db := o.databases[service]
		if err := db.session.Close(ctx); err != nil {
			failures = append(failures, fmt.Errorf("merge: close owned %s: %w", service, err))
		}
		db.pool.Close()
	}
	return errors.Join(failures...)
}

func serviceDatabase(ctx context.Context, idDSN string, owner *Ownership, service string) (pg.Database, func(), error) {
	if owner != nil {
		db := owner.Service(service)
		if db == nil {
			return nil, nil, fmt.Errorf("merge: database %s not owned", service)
		}
		return db, func() {}, nil
	}
	dsn, err := DSNForDatabase(idDSN, service)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
	if err != nil {
		return nil, nil, err
	}
	return pool, pool.Close, nil
}
