// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// InboundLegacyAdmission uses the SAME transaction for the storage fence, blob
// reservation and message insert. It never reacquires the pool while holding a
// row lock, so a one-connection pool remains usable during SMTP admission.
type InboundLegacyAdmission struct {
	tx        pgx.Tx
	recipient ResolvedAddress
}

func (a *InboundLegacyAdmission) Mailbox(ctx context.Context, role domain.SpecialUse) (domain.Mailbox, error) {
	return mailboxByRoleTx(ctx, a.tx, a.recipient.AccountID, role)
}

func (a *InboundLegacyAdmission) Filters(ctx context.Context) ([]domain.Filter, error) {
	return queryFiltersTx(ctx, a.tx, a.recipient.AccountID, true)
}

func (a *InboundLegacyAdmission) Insert(ctx context.Context, message *domain.Message) error {
	if message == nil || message.AccountID != a.recipient.AccountID {
		return domain.ErrNotFound
	}
	return insertMessageTx(ctx, a.tx, a.recipient.Tenant, message)
}

// InsertEntrante es Insert para el correo legítimo: mira el hilo antes de decidir
// el buzón (InsertMessageEntrante), en la misma transacción de la admisión. La
// respuesta a un hilo silenciado va al Archivo (silenciado=true: no debe sonar) y la
// de uno pospuesto lo devuelve a la Entrada.
func (a *InboundLegacyAdmission) InsertEntrante(ctx context.Context, message *domain.Message) (silenciado bool, err error) {
	if message == nil || message.AccountID != a.recipient.AccountID {
		return false, domain.ErrNotFound
	}
	return insertarEntranteTx(ctx, a.tx, a.recipient.Tenant, message)
}

func (a *InboundLegacyAdmission) ReservarBlobCompartido(ctx context.Context, hash, key string, size int64) (bool, bool, error) {
	return reserveSharedBlobTx(ctx, a.tx, hash, key, size)
}

func (a *InboundLegacyAdmission) MarcarBlobListo(ctx context.Context, hash string) error {
	return markSharedBlobReadyTx(ctx, a.tx, hash)
}

func (a *InboundLegacyAdmission) LiberarBlobCompartido(ctx context.Context, hash string) error {
	return releaseSharedBlobTx(ctx, a.tx, hash)
}

type PrivateInboundPlan struct {
	Config     pg.PrivateIngressConfig
	Key        ingress.KeyPlan
	TaskID     string
	MailboxID  uuid.UUID
	ReceivedAt time.Time
}

func privateInboundConfig(scope storage.Scope, epoch uuid.UUID) pg.PrivateIngressConfig {
	return pg.PrivateIngressConfig{Scope: scope, Epoch: epoch, Sealed: func(ctx context.Context, tx pgx.Tx, id string) error {
		event, err := events.NewEvent("private_ingress", "private_ingress.sealed", scope.Tenant, map[string]string{"task_id": id})
		if err != nil {
			return err
		}
		return events.Publish(ctx, tx, event)
	}}
}

// AdmitInbound serializes the recipient's legacy/private decision with the
// actual legacy write. Private input gets a durable task ID and retained public
// age key before any encryption or object I/O. A missing private enrollment is
// a temporary SMTP failure, never permission to fall back to plaintext.
func (s *Store) AdmitInbound(ctx context.Context, recipient ResolvedAddress, role domain.SpecialUse, legacy func(*InboundLegacyAdmission) error) (*PrivateInboundPlan, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != recipient.Tenant || recipient.AccountID == uuid.Nil || legacy == nil {
		return nil, domain.ErrNotFound
	}
	var result *PrivateInboundPlan
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var owner, kind string
		if err := tx.QueryRow(ctx, `SELECT subject,kind FROM accounts WHERE tenant=$1 AND id=$2 FOR UPDATE`, tenant.Slug, recipient.AccountID).Scan(&owner, &kind); err != nil {
			return err
		}
		if owner != recipient.Subject {
			return domain.ErrNotFound
		}
		if kind != "user" {
			return legacy(&InboundLegacyAdmission{tx: tx, recipient: recipient})
		}
		scope := storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "mail"}
		if !scope.Valid() {
			return domain.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.private_mail_owner',$1,true)`, owner); err != nil {
			return err
		}
		if err := pg.EnsurePrivateStorageState(ctx, tx, "mail_private_storage_states", scope.Tenant, owner, &recipient.AccountID); err != nil {
			return err
		}
		var mode string
		var epoch *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT mode,epoch FROM mail_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND account_id=$3 FOR SHARE`, scope.Tenant, owner, recipient.AccountID).Scan(&mode, &epoch); err != nil {
			return err
		}
		if mode == "legacy" {
			return legacy(&InboundLegacyAdmission{tx: tx, recipient: recipient})
		}
		if (mode != "frozen" && mode != "private") || epoch == nil || *epoch == uuid.Nil {
			return ErrPrivateStorageFrozen
		}
		result, err = planPrivateInboundTx(ctx, tx, scope, *epoch, recipient, role)
		return err
	})
	return result, err
}

func (s *Store) CommitPrivateInbound(ctx context.Context, plan PrivateInboundPlan, task ingress.Task, blobID string) error {
	if plan.Config.Scope.Product != "mail" || task.ID != plan.TaskID || task.Validate(plan.Config.Scope, plan.Config.Epoch.String()) != nil {
		return ingress.ErrContract
	}
	return s.privateTx(ctx, plan.Config.Scope.Subject, func(tx pgx.Tx) error {
		state, err := privateState(ctx, tx, plan.Config.Scope.Tenant, plan.Config.Scope.Subject)
		if err != nil {
			return err
		}
		if state.Epoch != plan.Config.Epoch.String() || (state.Mode != "frozen" && state.Mode != "private") {
			return ErrPrivateStorageEpoch
		}
		return pg.CommitPrivateIngressBody(ctx, tx, plan.Config, task, blobID)
	})
}

func planPrivateInboundTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, recipient ResolvedAddress, role domain.SpecialUse) (*PrivateInboundPlan, error) {
	cfg := privateInboundConfig(scope, epoch)
	if err := pg.BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return nil, err
	}
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT key_generation FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND revoked_at IS NULL ORDER BY key_generation DESC LIMIT 1 FOR SHARE`, scope.Tenant, scope.Subject, scope.Product, epoch).Scan(&generation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pg.ErrPrivateIngressMissing
		}
		return nil, err
	}
	plan, err := pg.ReadPrivateIngressKey(ctx, tx, cfg, generation)
	if err != nil {
		return nil, err
	}
	mailbox, err := mailboxByRoleTx(ctx, tx, recipient.AccountID, role)
	if err != nil {
		return nil, err
	}
	taskID := uuid.NewString()
	if err := pg.PlanPrivateIngress(ctx, tx, cfg, plan, taskID, "mail.receive"); err != nil {
		return nil, err
	}
	return &PrivateInboundPlan{Config: cfg, Key: plan, TaskID: taskID, MailboxID: mailbox.ID, ReceivedAt: time.Now().UTC()}, nil
}
