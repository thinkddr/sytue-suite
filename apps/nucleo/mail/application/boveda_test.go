// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package application_test

// El correo en modo de bóveda (ADR 0087) de punta a punta en el dispositivo:
// preparar (la época del correo retenido del servidor, con la bóveda como
// destino), dar de alta la clave de entrada (también rotarla cuando el plan
// vigente es el de Matrix) y recibir (reclamar, abrir, guardar en la bóveda y
// dar el recibo). La bóveda es la de verdad sobre el servidor falso de
// pqvaulttest, con su capa de ficheros y su contrato de entrada
// (storage/vaultingress); lo que se dobla son los extremos de entrada de Mail
// (estado y fuente del correo retenido, control de la entrada privada) y de
// Rooms (la autoridad del correo). El doble de Mail verifica cada acción firmada
// contra la clave que Sytue ID tiene registrada, como el servidor.

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/application"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultingress"
)

var ambito = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "mail"}

// epocaDeMatrix es la de un correo que ya se retuvo en tiempos de Matrix.
const epocaDeMatrix = "22222222-2222-4222-8222-222222222222"

// recursos es la sesión de la app: el testigo siempre vale, el arrendamiento se
// da y la vida es la de la prueba.
type recursos struct{ ctx context.Context }

func (r recursos) PrivateStorageScope() storage.Scope   { return ambito }
func (r recursos) Check(ctx context.Context) error      { return ctx.Err() }
func (r recursos) Revalidate(ctx context.Context) error { return ctx.Err() }
func (r recursos) Context() context.Context             { return r.ctx }
func (r recursos) Register(context.Context, offline.PrivateWork) (func(), error) {
	return func() {}, nil
}

func (r recursos) WorkLease() offline.WorkLease {
	return func(context.Context) (func(), error) { return func() {}, nil }
}

// rooms es Rooms para el correo de la bóveda: la autoridad del correo y nada de
// Matrix. El corte de identidad no se le pide nunca en modo de bóveda.
type rooms struct {
	mu        sync.Mutex
	autoridad matrixproof.Authority
	cortes    int
}

func (r *rooms) MatrixStorageAuthority(_ context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope != ambito {
		return matrixproof.Authority{}, contract.ErrContract
	}
	return r.autoridad, nil
}

func (r *rooms) corte() (storage.IdentityCutoverState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cortes++
	return storage.IdentityCutoverState{}, errors.New("rooms.cutover en modo de bóveda")
}

func (r *rooms) MatrixStorageMigration(context.Context, storage.Scope) (storage.IdentityCutoverState, error) {
	return r.corte()
}

func (r *rooms) BeginMatrixStorageMigration(context.Context, storage.Scope, string) (storage.IdentityCutoverState, error) {
	return r.corte()
}

func (r *rooms) CompleteMatrixStorageMigration(context.Context, storage.Scope, string, storage.Reference) (storage.IdentityCutoverState, error) {
	return r.corte()
}

func (r *rooms) poner(a matrixproof.Authority) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoridad = a
}

// tarea es una entrega en la cola de Mail: cifrada para un plan, reclamada o no.
type tarea struct {
	task    contract.Task
	cifrado []byte
	claim   *contract.Claim
	firma   string // la clave que firmó la reclamación
	recibo  *contract.Receipt
}

// correo es el servidor de Mail visto por el dispositivo: el estado y la fuente
// del correo retenido y el control de la entrada privada. Retiene cada reto y
// solo acepta la acción firmada por la clave activa en Sytue ID.
type correo struct {
	mu       sync.Mutex
	registro vaultproof.Registered
	estado   storage.LegacyMigrationState
	filas    []storage.StreamSourceRow
	cuerpos  map[storage.LegacySourceCursor][]byte
	planes   map[string][]contract.KeyPlan // por época, de la generación 1 en adelante
	retos    map[string]matrixproof.Action
	firmadas map[string]matrixproof.Action
	tareas   map[string]*tarea
	nonce    int

	begins           []string
	intentosAlta     []contract.KeyPlan
	rechazarRotacion bool
}

func nuevoCorreo(registro vaultproof.Registered) *correo {
	c := &correo{
		registro: registro,
		estado:   storage.LegacyMigrationState{Scope: ambito, Mode: "legacy"},
		cuerpos:  map[storage.LegacySourceCursor][]byte{},
		planes:   map[string][]contract.KeyPlan{},
		retos:    map[string]matrixproof.Action{},
		firmadas: map[string]matrixproof.Action{},
		tareas:   map[string]*tarea{},
	}
	// El correo del servidor: una cuenta, la Entrada y un mensaje con su MIME.
	add := func(kind, id string, raw []byte) {
		sum := sha256.Sum256(raw)
		c.filas = append(c.filas, storage.StreamSourceRow{Kind: kind, ID: id, Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
		c.cuerpos[storage.LegacySourceCursor{Kind: kind, ID: id}] = raw
	}
	add("account", "account", []byte(`{"id":"account","signature":"Firma del servidor"}`))
	add("mailbox", "inbox", []byte(`{"id":"inbox","name":"Inbox","role":"inbox","parent_id":null,"uid_next":2}`))
	add("message", "retenido", []byte(`{"id":"retenido","mailbox_id":"inbox","thread_id":"hilo","rfc822_msg_id":"<retenido@example.test>","subject":"Del servidor","from_addr":"alguien@example.test","to_addrs":["persona@example.test"],"size_bytes":8,"flags":[],"received_at":"2026-11-20T10:00:00Z","sent_at":null,"deleted_at":null,"uid":1}`))
	add("mime_message", "retenido", []byte("original"))
	add("thread", "hilo", []byte(`{"id":"hilo","subject":"Del servidor"}`))
	slices.SortFunc(c.filas, func(a, b storage.StreamSourceRow) int {
		return cmp.Or(strings.Compare(a.Kind, b.Kind), strings.Compare(a.ID, b.ID))
	})
	return c
}

// retenido deja el servidor ya en `frozen` con la época que se diga, como el de
// una cuenta que retuvo su correo con Matrix.
func (c *correo) retenido(epoch string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.congelar(epoch)
}

func (c *correo) congelar(epoch string) {
	started := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	c.estado = storage.LegacyMigrationState{Scope: ambito, Mode: "frozen", Epoch: epoch, StartedAt: &started}
	for _, fila := range c.filas {
		if fila.Kind == "mime_message" {
			c.estado.Blobs++
		} else {
			c.estado.Rows++
		}
	}
}

func (c *correo) vigente(epoch string) (contract.KeyPlan, bool) {
	planes := c.planes[epoch]
	if len(planes) == 0 {
		return contract.KeyPlan{}, false
	}
	return planes[len(planes)-1], true
}

// --- Fuente del correo retenido (privateMail.Source y el Begin del servidor) ---

func (c *correo) PrivateStorageCapabilities(_ context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if scope != ambito {
		return storage.AppMigrationState{}, contract.ErrContract
	}
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: c.estado, MaxBodyBytes: 50 << 20, PrivateIngressReady: true}, nil
}

func (c *correo) PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.estado, nil
}

func (c *correo) PrivateStorageSourcePage(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if scope != ambito || epoch != c.estado.Epoch || c.estado.Mode == "legacy" {
		return storage.StreamSourcePage{}, contract.ErrContract
	}
	page := storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	for _, fila := range c.filas {
		if fila.Kind > after.Kind || fila.Kind == after.Kind && fila.ID > after.ID {
			page.Rows = append(page.Rows, fila)
		}
	}
	return page, nil
}

func (c *correo) OpenPrivateStorageSource(_ context.Context, _ storage.Scope, _ string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, ok := c.cuerpos[storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}]
	if !ok {
		return nil, contract.ErrContract
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// ProvisionAndBeginPrivateStorageMigration es el Begin del servidor: solo con la
// clave de entrada de esa época ya dada de alta, y nunca otra época encima.
func (c *correo) ProvisionAndBeginPrivateStorageMigration(_ context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.begins = append(c.begins, epoch)
	if _, ok := c.vigente(epoch); !ok || scope != ambito {
		return storage.LegacyMigrationState{}, contract.ErrContract
	}
	switch {
	case c.estado.Mode == "legacy":
		c.congelar(epoch)
	case c.estado.Epoch != epoch:
		return storage.LegacyMigrationState{}, contract.ErrContract
	}
	return c.estado, nil
}

func (c *correo) PrivateSubmissions(storage.Scope, string) (api.PrivateMailSubmissionTransport, error) {
	return nil, errors.New("sin envíos en estas pruebas")
}

// --- Control de la entrada privada (ingress.HTTPControl y el estado del alta) ---

func (c *correo) Enrollment(_ context.Context, scope storage.Scope, epoch string) (contract.EnrollmentState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := contract.EnrollmentState{Version: 1, Scope: scope, Epoch: epoch, Status: "absent"}
	if plan, ok := c.vigente(epoch); ok {
		state.Status, state.Generation, state.Plan = "active", plan.Generation, &plan
	}
	return state, nil
}

// autoridad es la que el servidor da por buena: la de la clave activa en Sytue
// ID, nunca la que diga el cliente.
func (c *correo) autoridad() (matrixproof.Authority, error) {
	return vaultproof.Authority(ambito, c.registro)
}

func (c *correo) Challenge(_ context.Context, request contract.ControlRequest, authority matrixproof.Authority) (matrixproof.Action, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	actual, err := c.autoridad()
	if err != nil || authority != actual {
		return matrixproof.Action{}, contract.ErrRevoked
	}
	intent, err := matrixproof.IntentFor(request.Scope, actual.Generation, request.Epoch, request.Kind, request.Target, request)
	if err != nil {
		return matrixproof.Action{}, err
	}
	now := time.Now()
	c.nonce++
	action := matrixproof.Action{Version: 1, Generation: intent.Generation, Scope: intent.Scope, Epoch: intent.Epoch, Kind: intent.Kind, Target: intent.Target, RootKey: actual.RootKey, RequestSHA256: intent.RequestSHA256, Nonce: fmt.Sprintf("reto_%d", c.nonce), IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	c.retos[action.Nonce] = action
	return action, nil
}

// verificar comprueba la firma contra el transcript retenido y la clave activa.
func (c *correo) verificar(proof contract.ControlProof, consumir bool) (matrixproof.Authorization, error) {
	expected, ok := c.retos[proof.Proof.Nonce]
	if !ok {
		expected, ok = c.firmadas[proof.Proof.Nonce]
		if !ok || consumir {
			return matrixproof.Authorization{}, contract.ErrRevoked
		}
	}
	authorization, err := vaultproof.Verify(expected, proof.Proof, c.registro, time.Now())
	if err != nil {
		return matrixproof.Authorization{}, contract.ErrRevoked
	}
	if digest, err := matrixproof.RequestDigest(proof.Request); err != nil || digest != expected.RequestSHA256 {
		return matrixproof.Authorization{}, contract.ErrContract
	}
	if consumir {
		delete(c.retos, expected.Nonce)
		c.firmadas[expected.Nonce] = expected
	}
	return authorization, nil
}

func (c *correo) Apply(_ context.Context, proof contract.ControlProof) (contract.ControlResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	authorization, err := c.verificar(proof, true)
	if err != nil {
		return contract.ControlResult{}, err
	}
	result := contract.ControlResult{Version: 1, Authorization: authorization}
	request := proof.Request
	switch request.Kind {
	case "key.enroll":
		return result, c.alta(*request.Enrollment)
	case "task.claim":
		claim, err := c.reclamar(request)
		if err == nil {
			c.tareas[claim.Task.ID].firma = authorization.SigningKey
		}
		result.Claim = &claim
		return result, err
	case "task.ack":
		receipt, err := c.recibir(request)
		result.Receipt = &receipt
		return result, err
	default:
		return contract.ControlResult{}, contract.ErrContract
	}
}

// alta: la primera generación si no hay ninguna, el mismo plan otra vez, o el
// siguiente si lo autoriza la bóveda (la rotación de la cuarta tanda).
func (c *correo) alta(plan contract.KeyPlan) error {
	c.intentosAlta = append(c.intentosAlta, plan)
	current, ok := c.vigente(plan.Epoch)
	switch {
	case !ok && plan.Generation == 1:
	case ok && plan == current:
		return nil
	case ok && plan.Generation == current.Generation+1 && !c.rechazarRotacion:
	default:
		return contract.ErrRevoked
	}
	c.planes[plan.Epoch] = append(c.planes[plan.Epoch], plan)
	return nil
}

func (c *correo) reclamar(request contract.ControlRequest) (contract.Claim, error) {
	t := c.tareas[request.Claim.TaskID]
	if t == nil || t.recibo != nil || t.task.Epoch != request.Epoch {
		return contract.Claim{}, contract.ErrClaim
	}
	if t.claim != nil && t.claim.Token == request.Claim.Token && t.claim.Target == request.Target {
		return *t.claim, nil
	}
	generation := int64(1)
	if t.claim != nil {
		if t.claim.ExpiresAt.After(time.Now()) {
			return contract.Claim{}, contract.ErrClaim
		}
		generation = t.claim.Generation + 1
	}
	claim := contract.Claim{Version: 1, Task: t.task, Target: request.Target, Token: request.Claim.Token, Generation: generation, ExpiresAt: time.Now().Add(time.Duration(request.Claim.LeaseSeconds) * time.Second).UTC()}
	t.claim = &claim
	return claim, nil
}

func mismaReclamacion(a, b contract.Claim) bool {
	return a.Task == b.Task && a.Target == b.Target && a.Token == b.Token && a.Generation == b.Generation && a.ExpiresAt.Equal(b.ExpiresAt)
}

func (c *correo) recibir(request contract.ControlRequest) (contract.Receipt, error) {
	ack := request.Acknowledgment
	t := c.tareas[ack.Claim.Task.ID]
	if t == nil || t.claim == nil || !mismaReclamacion(*t.claim, ack.Claim) {
		return contract.Receipt{}, contract.ErrClaim
	}
	receipt := ack.Receipt
	t.recibo = &receipt
	return receipt, nil
}

func (c *correo) reclamacionVigente(proof contract.ControlProof) (*tarea, error) {
	if proof.Claim == nil {
		return nil, contract.ErrClaim
	}
	t := c.tareas[proof.Claim.Task.ID]
	if t == nil || t.claim == nil || t.recibo != nil || !mismaReclamacion(*t.claim, *proof.Claim) || !t.claim.ExpiresAt.After(time.Now()) {
		return nil, contract.ErrClaim
	}
	return t, nil
}

func (c *correo) Revalidate(_ context.Context, proof contract.ControlProof) (matrixproof.Authorization, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	authorization, err := c.verificar(proof, false)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if _, err := c.reclamacionVigente(proof); err != nil {
		return matrixproof.Authorization{}, err
	}
	return authorization, nil
}

func (c *correo) Open(_ context.Context, proof contract.ControlProof) (storage.StreamSourceRow, io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.verificar(proof, false); err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	t, err := c.reclamacionVigente(proof)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return t.task.Cipher, io.NopCloser(bytes.NewReader(t.cifrado)), nil
}

func (c *correo) Pending(_ context.Context, scope storage.Scope, epoch, after string, limit int) (contract.PendingPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page := contract.PendingPage{Version: 1, Scope: scope, Epoch: epoch, Tasks: []contract.Task{}}
	ids := make([]string, 0, len(c.tareas))
	for id, t := range c.tareas {
		if id > after && t.recibo == nil && t.task.Epoch == epoch {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids[:min(len(ids), limit)] {
		page.Tasks = append(page.Tasks, c.tareas[id].task)
	}
	return page, nil
}

// Inspect es lo que el servidor sabe de una entrega: su plan y, si alguien la
// reclamó, a qué destino. El receptor lo mira antes de reclamar (traspaso entre
// dispositivos); aquí nadie más la ha reclamado.
func (c *correo) Inspect(_ context.Context, _ storage.Scope, epoch, id string) (contract.TaskState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.tareas[id]
	if t == nil || t.task.Epoch != epoch {
		return contract.TaskState{}, contract.ErrContract
	}
	state := contract.TaskState{Version: 1, Task: t.task}
	for _, plan := range c.planes[epoch] {
		if plan.Generation == t.task.KeyGeneration && plan.KeyID == t.task.KeyID {
			state.Plan = plan
		}
	}
	if t.claim != nil {
		state.Assignment = &contract.Assignment{Target: t.claim.Target, Generation: t.claim.Generation, SigningKey: t.firma, ExpiresAt: t.claim.ExpiresAt}
		state.Receipt = t.recibo
	}
	return state, nil
}

func (c *correo) OpenHandoff(context.Context, contract.ControlProof, contract.Task) (storage.StreamSourceRow, io.ReadCloser, error) {
	return storage.StreamSourceRow{}, nil, errors.New("sin traspasos entre dispositivos en estas pruebas")
}

// entregar es la admisión SMTP: cifra para el plan que se diga el MIME y su
// sobre de enrutado, como persistPrivateInbound.
func (c *correo) entregar(t *testing.T, plan contract.KeyPlan, id string, mime []byte) contract.Task {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{"id": "msg_" + id, "mailbox_id": "inbox", "size": len(mime), "received_at": time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	var cifrado bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &cifrado, plan, id, "mail.receive", metadata, bytes.NewReader(mime), int64(len(mime)))
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tareas[id] = &tarea{task: task, cifrado: cifrado.Bytes()}
	return task
}

func (c *correo) tarea(id string) tarea {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.tareas[id]
}

// --- El banco: la bóveda de verdad, su diario, Rooms y Mail ---

type banco struct {
	fake   *pqvaulttest.Fake
	vault  *pqvault.Vault
	device *vaultingress.Device
	drive  *api.Drive
	rooms  *rooms
	correo *correo
	diario *offline.Store
}

func nuevoBanco(t *testing.T) *banco {
	t.Helper()
	fake, drive, account := pqvaulttest.New(t, ambito)
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: ambito, Server: drive, Registry: account, UnlockKey: bytes.Repeat([]byte{7}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := vaultfiles.New(vault)
	if err != nil {
		t.Fatal(err)
	}
	device, err := vaultingress.New(store)
	if err != nil {
		t.Fatal(err)
	}
	var public recoverywrap.PQVaultVerifyKey
	if len(fake.Signing) != 1 || json.Unmarshal(fake.Signing[0].PublicKey, &public) != nil {
		t.Fatal("la clave de firma de la bóveda no está en Sytue ID")
	}
	registro := vaultproof.Registered{Generation: fake.Signing[0].Generation, KeyID: fake.Signing[0].KeyID, Public: public}
	autoridad, err := vaultproof.Authority(ambito, registro)
	if err != nil {
		t.Fatal(err)
	}
	if err := device.RequireStorageAuthority(autoridad); err != nil {
		t.Fatal("la autoridad del banco no es la de esta bóveda", err)
	}
	diario, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return &banco{fake: fake, vault: vault, device: device, drive: drive, rooms: &rooms{autoridad: autoridad}, correo: nuevoCorreo(registro), diario: diario}
}

func (b *banco) autoridad() matrixproof.Authority {
	b.rooms.mu.Lock()
	defer b.rooms.mu.Unlock()
	return b.rooms.autoridad
}

// app monta el correo en modo de bóveda como lo hace el anfitrión
// (cifrado.NuevoCorreoBoveda): sin origen histórico ni nada de Matrix.
func (b *banco) app(t *testing.T) *application.Application {
	t.Helper()
	app, err := application.New(t.Context(), recursos{ctx: t.Context()}, application.Config{
		Gateway: b.drive, Source: b.correo, Control: b.correo, Rooms: b.rooms, Authority: b.autoridad(),
		Journal: b.diario, Isolated: b.device, Vault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

// planAjeno es un plan de entrada cuya identidad no está en la bóveda: el de
// Matrix de antes del traspaso.
func planAjeno(t *testing.T, epoch string, generation int64) contract.KeyPlan {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := contract.KeyPlan{Version: 1, Scope: ambito, Epoch: epoch, Generation: generation, Profile: contract.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = contract.PublicFingerprint(plan.Recipient)
	return plan
}

// Preparar una cuenta nueva: la época del correo retenido es la inicial del
// ámbito, el servidor solo la empieza con la clave de entrada ya dada de alta
// (firmada por la bóveda y verificada contra Sytue ID), su secreto vive en la
// bóveda y el buzón del servidor queda importado en ella. A Rooms no se le pide
// ningún corte de identidad. Reabrir no empieza nada otra vez.
func TestBovedaPreparaYDaDeAltaLaClaveDeEntrada(t *testing.T) {
	b := nuevoBanco(t)
	app := b.app(t)
	if _, err := app.ReadyRepository(t.Context()); err == nil {
		t.Fatal("listo sin preparar")
	}
	if err := app.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := app.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	epoch, err := transfer.InitialSourceEpoch(ambito)
	if err != nil {
		t.Fatal(err)
	}
	c := b.correo
	if len(c.begins) != 1 || c.begins[0] != epoch || c.estado.Mode != "frozen" || c.estado.Epoch != epoch {
		t.Fatalf("la época del servidor: begins=%v estado=%+v", c.begins, c.estado)
	}
	plan, ok := c.vigente(epoch)
	if !ok || plan.Generation != 1 || len(c.planes[epoch]) != 1 {
		t.Fatalf("alta: %+v", c.planes)
	}
	if _, found, err := b.vault.ReadIngressSecret(t.Context(), keyring.SecretName(ambito, epoch, 1, plan.KeyID)); err != nil || !found {
		t.Fatal("el secreto del plan no está en la bóveda", found, err)
	}
	if b.rooms.cortes != 0 {
		t.Fatal("se pidió un corte de identidad de Matrix en modo de bóveda")
	}
	repository, err := app.ReadyRepository(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	messages, _, err := repository.Messages(t.Context(), "inbox", "", 50)
	if err != nil || len(messages) != 1 || messages[0].ID != "retenido" {
		t.Fatal("el correo del servidor no está en la bóveda", messages, err)
	}

	// Otra apertura sobre la misma bóveda y el mismo diario no empieza otra
	// época ni da de alta otra clave.
	again := b.app(t)
	if err := again.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(c.begins) != 1 || len(c.planes[epoch]) != 1 {
		t.Fatalf("reabrir volvió a empezar: begins=%v, %d planes", c.begins, len(c.planes[epoch]))
	}
	for _, intento := range c.intentosAlta {
		if intento != plan {
			t.Fatalf("reabrir propuso otra clave de entrada: generación %d", intento.Generation)
		}
	}
}

// Si la autoridad del correo cambia entre montar y preparar (otra bóveda tras un
// vaciado, o vuelve Matrix), el alta falla cerrada: ni clave nueva ni época.
func TestBovedaNoDaDeAltaConOtraAutoridad(t *testing.T) {
	b := nuevoBanco(t)
	app := b.app(t)
	otra := b.autoridad()
	otra.Generation++
	b.rooms.poner(otra)
	if err := app.Prepare(t.Context()); !errors.Is(err, contract.ErrRevoked) {
		t.Fatalf("preparó con otra autoridad: %v", err)
	}
	if len(b.correo.begins) != 0 || len(b.correo.intentosAlta) != 0 || len(b.correo.planes) != 0 {
		t.Fatalf("algo cambió en el servidor: begins=%v altas=%v", b.correo.begins, b.correo.intentosAlta)
	}
}

// Tras un traspaso, la época retenida del servidor tiene el plan de entrada de
// Matrix, cuya identidad la bóveda no tiene: el alta propone el siguiente, una
// sola vez. Si el servidor no lo autoriza, nada cambia; el reintento recupera
// el mismo candidato en vez de crear otro.
func TestBovedaRotaElPlanDeEntradaQueNoTiene(t *testing.T) {
	b := nuevoBanco(t)
	c := b.correo
	c.retenido(epocaDeMatrix)
	matrix := planAjeno(t, epocaDeMatrix, 1)
	c.planes[epocaDeMatrix] = []contract.KeyPlan{matrix}
	c.rechazarRotacion = true
	app := b.app(t)
	if err := app.Prepare(t.Context()); !errors.Is(err, contract.ErrRevoked) {
		t.Fatalf("rotó sin autorización: %v", err)
	}
	if len(c.intentosAlta) != 1 || c.intentosAlta[0].Generation != 2 || len(c.planes[epocaDeMatrix]) != 1 {
		t.Fatalf("intentos: %v planes: %v", c.intentosAlta, c.planes)
	}
	candidato := c.intentosAlta[0]

	c.mu.Lock()
	c.rechazarRotacion = false
	c.mu.Unlock()
	if err := app.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.vigente(epocaDeMatrix); got != candidato || len(c.planes[epocaDeMatrix]) != 2 || c.planes[epocaDeMatrix][0] != matrix {
		t.Fatalf("el plan vigente no es el candidato de antes: %+v", c.planes)
	}
	if len(c.begins) != 0 {
		t.Fatal("una época ya retenida se volvió a empezar", c.begins)
	}
	if _, found, err := b.vault.ReadIngressSecret(t.Context(), keyring.SecretName(ambito, epocaDeMatrix, 2, candidato.KeyID)); err != nil || !found {
		t.Fatal("el secreto del plan rotado no está en la bóveda", found, err)
	}
}

// El receptor reclama lo cifrado para SU clave, lo abre en la bóveda, lo guarda
// como un mensaje más de la Entrada y da el recibo con la referencia de la
// bóveda. Lo cifrado para el plan de Matrix se queda en la cola, sin reclamar.
func TestBovedaRecibeYDaElRecibo(t *testing.T) {
	b := nuevoBanco(t)
	c := b.correo
	c.retenido(epocaDeMatrix)
	matrix := planAjeno(t, epocaDeMatrix, 1)
	c.planes[epocaDeMatrix] = []contract.KeyPlan{matrix}
	app := b.app(t)
	if err := app.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	plan, _ := c.vigente(epocaDeMatrix)
	mime := []byte("From: fuera@example.org\r\nTo: persona@example.test\r\nSubject: Recien llegado\r\nMessage-ID: <nuevo@example.org>\r\n\r\nHola desde fuera\r\n")
	c.entregar(t, matrix, "entrega_matrix", []byte("cifrado para Matrix"))
	nueva := c.entregar(t, plan, "entrega_boveda", mime)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	var cambios []ingress.PollResult
	err := app.RunReceiver(ctx, func(result ingress.PollResult, err error) {
		if err != nil {
			t.Error("el receptor falló", err)
		}
		cambios = append(cambios, result)
		if result.Completed > 0 || err != nil {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || len(cambios) == 0 || cambios[0] != (ingress.PollResult{Attempted: 1, Completed: 1}) {
		t.Fatalf("receptor: %v %+v", err, cambios)
	}

	recibida := c.tarea("entrega_boveda")
	if recibida.recibo == nil || recibida.claim == nil || recibida.recibo.TaskID != nueva.ID || recibida.recibo.Commit.RoomID != b.vault.PublicationTarget().RoomID || recibida.claim.Target != b.vault.PublicationTarget() {
		t.Fatalf("recibo: %+v reclamación: %+v", recibida.recibo, recibida.claim)
	}
	if ajena := c.tarea("entrega_matrix"); ajena.claim != nil || ajena.recibo != nil {
		t.Fatal("se reclamó lo cifrado para el plan de Matrix")
	}
	repository, err := app.ReadyRepository(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	body, err := repository.Raw(t.Context(), "msg_entrega_boveda")
	if err != nil {
		t.Fatal("el correo recibido no está en la bóveda", err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(raw, mime) {
		t.Fatal("el MIME recibido cambió", err)
	}
	messages, _, err := repository.Messages(t.Context(), "inbox", "", 50)
	if err != nil || len(messages) != 2 {
		t.Fatal("la Entrada no tiene el retenido y el recibido", messages, err)
	}
}
