// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

const receiverEpoch = "0b5c7c2e-1f7a-4d43-8d0e-5a3c9b1e2f40"

// receiverDevice es el dispositivo aislado de mentira: el almacén en memoria de las
// otras pruebas y la clave de entrada con la que el Hub cifró la entrega.
type receiverDevice struct {
	*memoryStorage
	identity *age.X25519Identity
}

func (d receiverDevice) RequireRecoverableIngress(ctx context.Context, _ storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	return ctx, d.CheckStoragePublicationTarget(target)
}

func (d receiverDevice) RequireAuthorizedStoragePublication(ctx context.Context, _ storage.Scope, target storage.PublicationTarget, policy matrix.StoragePublicationPolicy) (context.Context, error) {
	if policy == nil {
		return nil, contract.ErrRevoked
	}
	return ctx, d.CheckStoragePublicationTarget(target)
}

func (d receiverDevice) OpenIngressBody(ctx context.Context, task contract.Task, reader io.ReaderAt) ([]byte, *io.SectionReader, error) {
	return agecipher.Open(ctx, reader, task, d.identity)
}

func (d receiverDevice) CreateIngressKey(context.Context, storage.Scope, string, int64) (contract.KeyPlan, error) {
	return contract.KeyPlan{}, errors.New("no se usa en esta prueba")
}

func (d receiverDevice) RecoverIngressEnrollment(context.Context, contract.KeyPlan) (contract.KeyPlan, error) {
	return contract.KeyPlan{}, errors.New("no se usa en esta prueba")
}

func (d receiverDevice) SignIngressAction(context.Context, offline.WorkLease, matrixproof.Intent, matrixproof.Action) (matrixproof.SignedAction, error) {
	return matrixproof.SignedAction{}, errors.New("no se usa en esta prueba")
}

func (d receiverDevice) StoragePublicationTarget() (storage.PublicationTarget, error) {
	return d.binding.Target, nil
}

// hubQueue hace de cola del Hub: una entrega cifrada, su reclamo y su recibo.
type hubQueue struct {
	task         contract.Task
	cipher       []byte
	claims, acks int
	receipt      *contract.Receipt
}

func authorityFor(scope storage.Scope) matrixproof.Authority {
	return matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "fixture", RootKey: strings.Repeat("A", 43), Generation: 1, Status: "active"}
}

func (q *hubQueue) Authority(_ context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	return authorityFor(scope), nil
}

func (q *hubQueue) Revalidate(context.Context, contract.Claim, matrixproof.Authority) error {
	return nil
}

func (q *hubQueue) Claim(_ context.Context, _ storage.Scope, _, id string, target storage.PublicationTarget, token string, _ matrixproof.Authority) (contract.Claim, error) {
	if id != q.task.ID {
		return contract.Claim{}, contract.ErrClaim
	}
	q.claims++
	return contract.Claim{Version: 1, Task: q.task, Target: target, Token: token, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (q *hubQueue) Open(_ context.Context, claim contract.Claim, _ matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error) {
	return claim.Task.Cipher, io.NopCloser(bytes.NewReader(q.cipher)), nil
}

func (q *hubQueue) Acknowledge(_ context.Context, claim contract.Claim, receipt contract.Receipt, _ matrixproof.Authority) (contract.Receipt, error) {
	if receipt.Validate(claim) != nil {
		return contract.Receipt{}, contract.ErrReceipt
	}
	q.acks++
	q.receipt = &receipt
	return receipt, nil
}

// De punta a punta en el aparato: el trabajador compartido reclama la entrega que
// el Hub cifró, la descifra, IngressExecutor guarda el aviso en el almacén privado y
// el recibo vuelve al Hub con el commit de la sala del reclamo.
func TestIngressWorkerClaimsStoresAndAcknowledgesHubNotice(t *testing.T) {
	r, port, _ := setupPrivate(t)
	scope := port.binding.Scope
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	recipient := identity.Recipient().String()
	plan := contract.KeyPlan{Version: 1, Scope: scope, Epoch: receiverEpoch, Generation: 1, KeyID: contract.PublicFingerprint(recipient), Profile: contract.Profile, Recipient: recipient}
	metadata := bytes.ReplaceAll(ingressMetadata("Soltaste la fuente"), []byte(`"tenant":"acme"`), []byte(`"tenant":"`+scope.Tenant+`"`))
	var cipher bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &cipher, plan, ingressNoticeID, IngressKind, metadata, bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatal(err)
	}
	queue := &hubQueue{task: task, cipher: cipher.Bytes()}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := ingress.New(ingress.Config{
		Context: t.Context(), Scope: scope, Epoch: receiverEpoch, Journal: journal,
		Lease:  func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() },
		Source: queue, Device: receiverDevice{memoryStorage: port, identity: identity}, Executor: IngressExecutor{Repository: r},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Close)
	if err := worker.Process(t.Context(), ingressNoticeID); err != nil {
		t.Fatal(err)
	}
	if queue.acks != 1 || queue.receipt == nil || queue.receipt.Commit.RoomID != port.binding.Target.RoomID {
		t.Fatalf("recibo: %d %+v", queue.acks, queue.receipt)
	}
	got, err := r.List(t.Context())
	if err != nil || len(got) != 1 || got[0].ID != ingressNoticeID || got[0].Type != "security.storage.released" {
		t.Fatal("aviso guardado", got, err)
	}
	if pending, err := worker.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("el diario retiene una entrega ya confirmada", pending, err)
	}
	// Otra vuelta no reclama ni guarda otra vez.
	if err := worker.Process(t.Context(), ingressNoticeID); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.List(t.Context()); len(got) != 1 {
		t.Fatal("reprocesar duplicó el aviso", got)
	}
}

// El receptor del anfitrión: con la fuente en legacy no da de alta nada ni arranca
// el trabajador; sin preparar, Run lo dice; y sin sus piezas no se construye.
func TestReceiverIdleWhileSourceIsLegacy(t *testing.T) {
	r, port, _ := setupPrivate(t)
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	epochReads := 0
	cfg := ReceiverConfig{
		Context: t.Context(), Repository: r, Authority: authorityFor(port.binding.Scope), Journal: journal,
		Lease:  func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() },
		Device: receiverDevice{memoryStorage: port}, Control: noControl{}, Rooms: noRooms{},
		Authorize: func(ctx context.Context) error { return ctx.Err() },
		Epoch:     func(context.Context) (string, error) { epochReads++; return "", nil },
	}
	receiver, err := NewReceiver(cfg)
	if err != nil {
		t.Fatal("hub no es un producto de entrada privada", err)
	}
	t.Cleanup(receiver.Close)
	if err := receiver.Prepare(t.Context()); err != nil || epochReads != 1 {
		t.Fatal(err, epochReads)
	}
	if err := receiver.Run(t.Context(), nil); !errors.Is(err, ErrReceiverNotReady) {
		t.Fatal("Run sin trabajador", err)
	}
	if pending, err := receiver.Pending(t.Context()); err != nil || pending.Count != 0 || pending.Scope != port.binding.Scope {
		t.Fatal(pending, err)
	}
	if err := receiver.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := receiver.Resume(); err != nil {
		t.Fatal(err)
	}
	failing := cfg
	failing.Epoch = func(context.Context) (string, error) { return "", errors.New("hub caído") }
	broken, err := NewReceiver(failing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broken.Close)
	if err := broken.Prepare(t.Context()); err == nil {
		t.Fatal("sin poder leer la época, Prepare no lo dice")
	}
	for name, mutate := range map[string]func(*ReceiverConfig){
		"sin repositorio": func(c *ReceiverConfig) { c.Repository = nil },
		"sin control":     func(c *ReceiverConfig) { c.Control = nil },
		"sin época":       func(c *ReceiverConfig) { c.Epoch = nil },
		"sin autoridad":   func(c *ReceiverConfig) { c.Authority = matrixproof.Authority{} },
	} {
		bad := cfg
		mutate(&bad)
		if got, err := NewReceiver(bad); err == nil {
			got.Close()
			t.Errorf("%s: construido", name)
		}
	}
}

type (
	noControl struct{ ingress.HTTPControl }
	noRooms   struct{}
)

func (noRooms) MatrixStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error) {
	return matrixproof.Authority{}, errors.New("no se usa")
}
