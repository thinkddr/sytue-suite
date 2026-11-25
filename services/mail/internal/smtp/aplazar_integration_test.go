// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package smtp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"filippo.io/age"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// roomsDePrueba es Rooms por el canal interno: la autoridad del correo de cada
// persona, o un error (la retirada de Matrix, una avería).
type roomsDePrueba struct {
	mu        sync.Mutex
	autoridad map[string]matrixproof.Authority
	err       error
	preguntas []string
}

func (r *roomsDePrueba) ReadStorageAuthority(_ context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preguntas = append(r.preguntas, scope.Subject)
	if r.err != nil {
		return matrixproof.Authority{}, r.err
	}
	return r.autoridad[scope.Subject], nil
}

func (r *roomsDePrueba) contestar(err error, autoridad map[string]matrixproof.Authority) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err, r.autoridad, r.preguntas = err, autoridad, nil
}

func (r *roomsDePrueba) preguntado() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.preguntas...)
}

type bancoAplazado struct {
	pool  *pg.Pool
	store *store.Store
	ctx   context.Context
}

// privada pasa la cuenta a la entrada privada (época frozen) y da de alta su
// plan 1: sin autoridad (un plan de Matrix de antes de guardarla) o con la que se
// diga.
func (b *bancoAplazado) privada(t *testing.T, account domain.Account, autoridad *matrixproof.Authority) pg.PrivateIngressConfig {
	t.Helper()
	epoch := uuid.New()
	if _, err := b.store.BeginPrivateStorageMigration(b.ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("buzón vacío: no hay MIME que leer")
	}); err != nil {
		t.Fatal(err)
	}
	cfg := pg.PrivateIngressConfig{Scope: storage.Scope{Tenant: "acme", Subject: account.Subject, Product: "mail"}, Epoch: epoch}
	b.alta(t, cfg, 1, autoridad)
	return cfg
}

func (b *bancoAplazado) alta(t *testing.T, cfg pg.PrivateIngressConfig, generation int64, autoridad *matrixproof.Authority) {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := ingress.KeyPlan{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Generation: generation, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	if err := b.pool.InTenantTx(b.ctx, func(tx pgx.Tx) error {
		if autoridad == nil {
			return pg.EnrollPrivateIngress(b.ctx, tx, cfg, plan, storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "MATRIX"}, "clave pública de firma de prueba")
		}
		target := storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"}
		return pg.EnrollAuthorizedPrivateIngress(b.ctx, tx, cfg, plan, target, matrixproof.Authorization{Version: 1, Authority: *autoridad, Target: target, SigningKey: autoridad.RootKey})
	}); err != nil {
		t.Fatal(err)
	}
}

func (b *bancoAplazado) tareas(t *testing.T) int {
	t.Helper()
	var n int
	if err := b.pool.InMaintenanceTx(b.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(b.ctx, `SELECT count(*) FROM private_ingress_tasks`).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// El MX aplaza en RCPT, con 450 4.2.1, SOLO al destinatario cuyo correo se
// cifraría para un plan de entrada de Matrix que ya nadie reclama (ADR 0087): los
// demás del mismo mensaje lo reciben una vez. Ni se planifica ni se cifra nada
// para él. En cuanto la bóveda rota el plan, vuelve a recibir sin preguntar a
// Rooms.
func TestRcptAplazaElCorreoDeUnaCuentaDeMatrixRetirada(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	b := &bancoAplazado{pool: h.Pool, store: store.New(h.Pool), ctx: tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})}
	cuenta := func(subject, address string) domain.Account {
		account, err := b.store.EnsureAccount(b.ctx, subject, address, subject, 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		return account
	}
	matrix := cuenta("persona-matrix", "matrix@example.test")
	boveda := cuenta("persona-boveda", "boveda@example.test")
	legacy := cuenta("persona-legacy", "legacy@example.test")
	ambito := func(account domain.Account) storage.Scope {
		return storage.Scope{Tenant: "acme", Subject: account.Subject, Product: "mail"}
	}
	deBoveda := func(account domain.Account, generation int64) matrixproof.Authority {
		return matrixproof.Authority{Version: 1, Scope: ambito(account), UserID: vaultproof.UserID(ambito(account)), RootKey: strings.Repeat("V", 42) + "A", Generation: generation, Status: "active"}
	}
	deMatrix := matrixproof.Authority{Version: 1, Scope: ambito(matrix), UserID: storage.IsolatedMatrixUserPrefix(ambito(matrix)) + "matrix.test", RootKey: strings.Repeat("M", 43), Generation: 5, Status: "active"}
	cfgMatrix := b.privada(t, matrix, nil)
	autoridadBoveda := deBoveda(boveda, 1)
	b.privada(t, boveda, &autoridadBoveda)

	rooms := &roomsDePrueba{}
	inbound := delivery.NewInbound(b.store, nil, nil, nil, slog.New(slog.DiscardHandler), delivery.InboundConfig{MaxBytes: 1 << 20, Autoridad: rooms})
	sesion := func() *mxSession {
		s := &mxSession{d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler), Inbound: inbound}}
		if err := s.Mail("fuera@example.org", nil); err != nil {
			t.Fatal(err)
		}
		return s
	}
	aplazado := func(t *testing.T, err error) {
		t.Helper()
		var e *gosmtp.SMTPError
		if !errors.As(err, &e) || e.Code != 450 || e.EnhancedCode != (gosmtp.EnhancedCode{4, 2, 1}) {
			t.Fatalf("quiero 450 4.2.1: %v", err)
		}
	}

	t.Run("Matrix retirada", func(t *testing.T) {
		rooms.contestar(callaccess.ErrStorageMatrixRetired, nil)
		s := sesion()
		aplazado(t, s.Rcpt(matrix.Address, nil))
		if err := s.Rcpt(legacy.Address, nil); err != nil {
			t.Fatal("una cuenta sin entrada privada no se aplaza", err)
		}
		if err := s.Rcpt(boveda.Address, nil); err != nil {
			t.Fatal("una cuenta con plan de la bóveda no se aplaza", err)
		}
		if len(s.rcpts) != 2 || s.rcpts[0] != legacy.Address || s.rcpts[1] != boveda.Address {
			t.Fatalf("destinatarios del mensaje: %v", s.rcpts)
		}
		if got := rooms.preguntado(); len(got) != 1 || got[0] != matrix.Subject {
			t.Fatalf("solo se pregunta a Rooms por el plan de Matrix: %v", got)
		}
		if n := b.tareas(t); n != 0 {
			t.Fatalf("el correo aplazado dejó %d tareas en la cola", n)
		}
	})

	t.Run("Rooms no contesta", func(t *testing.T) {
		rooms.contestar(callaccess.ErrUnavailable, nil)
		aplazado(t, sesion().Rcpt(matrix.Address, nil))
	})

	t.Run("Matrix aún honrada", func(t *testing.T) {
		rooms.contestar(nil, map[string]matrixproof.Authority{matrix.Subject: deMatrix})
		if err := sesion().Rcpt(matrix.Address, nil); err != nil {
			t.Fatal("con Matrix honrada el correo entra como siempre", err)
		}
	})

	t.Run("traspaso sin rotar y después rotado", func(t *testing.T) {
		traspasada := deBoveda(matrix, 6)
		rooms.contestar(nil, map[string]matrixproof.Authority{matrix.Subject: traspasada})
		aplazado(t, sesion().Rcpt(matrix.Address, nil))
		// La bóveda abre el correo y rota el plan: desde ahí, sin preguntar a Rooms.
		b.alta(t, cfgMatrix, 2, &traspasada)
		rooms.contestar(callaccess.ErrStorageMatrixRetired, nil)
		if err := sesion().Rcpt(matrix.Address, nil); err != nil {
			t.Fatal("tras rotar a la bóveda sigue aplazado", err)
		}
		if got := rooms.preguntado(); len(got) != 0 {
			t.Fatalf("con el plan de la bóveda se preguntó a Rooms: %v", got)
		}
	})
}
