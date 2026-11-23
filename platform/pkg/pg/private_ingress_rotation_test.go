// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestRotacionDeLaClaveDeEntradaTrasElTraspaso(t *testing.T) {
	h := newIngressHarness(t)
	h.seal(t) // una entrega ya cifrada para el plan 1, que no se puede perder
	scope := h.cfg.Scope
	matrix := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "test", RootKey: strings.Repeat("M", 43), Generation: 5, Status: "active"}
	boveda := func(gen int64) matrixproof.Authority {
		return matrixproof.Authority{Version: 1, Scope: scope, UserID: vaultproof.UserID(scope), RootKey: strings.Repeat("V", 42) + "A", Generation: gen, Status: "active"}
	}
	destino := storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"}
	plan := func(gen int64) ingress.KeyPlan {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		p := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: h.cfg.Epoch.String(), Generation: gen, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
		p.KeyID = ingress.PublicFingerprint(p.Recipient)
		return p
	}
	alta := func(p ingress.KeyPlan, autoridad matrixproof.Authority) error {
		// Cada autoridad firma desde su propio dispositivo, como en la realidad.
		target := destino
		if !vaultproof.IsAuthority(autoridad) {
			target = storage.PublicationTarget{RoomID: h.target.RoomID, DeviceID: "MATRIX_B"}
		}
		return h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return pg.EnrollAuthorizedPrivateIngress(ctx, tx, h.cfg, p, target, matrixproof.Authorization{Version: 1, Authority: autoridad, Target: target, SigningKey: autoridad.RootKey})
		})
	}
	vigente := func() ingress.EnrollmentState {
		var state ingress.EnrollmentState
		if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			var err error
			state, err = pg.ReadPrivateIngressEnrollment(ctx, tx, h.cfg)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return state
	}

	// El mismo plan otra vez sigue valiendo como siempre, con cualquier autoridad.
	if err := alta(h.plan, matrix); err != nil {
		t.Fatal("el alta repetida del plan vigente dejó de valer", err)
	}
	segunda := plan(2)
	for nombre, c := range map[string]struct {
		p ingress.KeyPlan
		a matrixproof.Authority
	}{
		"Matrix no rota":            {segunda, matrix},
		"saltarse una generación":   {plan(3), boveda(6)},
		"autoridad de bóveda caída": {segunda, func() matrixproof.Authority { a := boveda(6); a.Status = "revoked"; return a }()},
	} {
		if err := alta(c.p, c.a); err == nil {
			t.Fatalf("%s: rotó", nombre)
		}
		if s := vigente(); s.Status != "active" || *s.Plan != h.plan {
			t.Fatalf("%s: cambió el plan vigente: %+v", nombre, s)
		}
	}

	// Tras el traspaso (autoridad de bóveda, el plan 1 es de antes): rota.
	if err := alta(segunda, boveda(6)); err != nil {
		t.Fatal("la bóveda no pudo rotar tras el traspaso", err)
	}
	if s := vigente(); s.Status != "active" || *s.Plan != segunda {
		t.Fatalf("el plan nuevo no es el vigente: %+v", s)
	}
	if err := alta(segunda, boveda(6)); err != nil {
		t.Fatal("repetir la rotación ya hecha falló", err)
	}
	// La entrega cifrada para el plan 1 sigue en la cola, intacta.
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var generacion int64
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT key_generation FROM private_ingress_tasks WHERE id=$1 AND acknowledged_at IS NULL AND cipher_sha256 IS NOT NULL`, h.task.ID).Scan(&generacion); err != nil {
			return err
		}
		if generacion != 1 {
			return errors.New("la entrega cambió de plan")
		}
		return nil
	}); err != nil {
		t.Fatal("se perdió la entrega encolada", err)
	}

	// La misma bóveda no vuelve a rotar; tras vaciarla (generación mayor), sí.
	if err := alta(plan(3), boveda(6)); err == nil {
		t.Fatal("la misma autoridad rotó dos veces")
	}
	if err := alta(plan(3), boveda(7)); err != nil {
		t.Fatal("tras vaciar la bóveda no pudo rotar", err)
	}

	// La autoridad guardada con cada plan es inmutable.
	sinFilas := errors.New("ninguna fila: la prueba no prueba nada")
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE private_ingress_key_plans SET authority_generation=99 WHERE key_generation=2`)
		if err == nil && tag.RowsAffected() == 0 {
			return sinFilas
		}
		return err
	})
	if err == nil || errors.Is(err, sinFilas) {
		t.Fatal("la autoridad de un plan se pudo cambiar", err)
	}
}
