// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// autoridadDePrueba es la autoridad de Rooms que la entrada exige: aquí acepta todo
// recibo. Lo que se prueba es la cola y la copia del Hub, no la verificación de Rooms.
type autoridadDePrueba struct{}

func (autoridadDePrueba) ReadStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error) {
	return matrixproof.Authority{}, errors.New("sin autoridad en la prueba")
}

func (autoridadDePrueba) VerifyStorageAction(context.Context, matrixproof.Action, matrixproof.SignedAction) (matrixproof.Authorization, error) {
	return matrixproof.Authorization{}, errors.New("sin autoridad en la prueba")
}

func (autoridadDePrueba) VerifyStorageReceipt(context.Context, storage.Scope, storage.PublicationTarget, storage.Reference) error {
	return nil
}

// La entrada privada de avisos sobre el esquema real (0014 + 0015): con la fuente de
// One de ana congelada, ningún aviso se pierde —ni sin clave, ni de seguridad, ni al
// soltar la fuente—, la guarda cierra `notifications` y `notification_prefs`, y un
// aparato reclama, descifra y confirma lo suyo.
func TestEntradaPrivadaDeAvisos(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	svc := notify.NewService(st, notify.NewHub(8))
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	admin, err := pgx.Connect(acme, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(acme) }()
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := admin.QueryRow(acme, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	avisosDeAna := func() int {
		return count(`SELECT count(*) FROM notifications WHERE tenant='acme' AND subject='ana'`)
	}
	copias := func(sellados bool) int {
		q := `SELECT count(*) FROM hub_private_notices WHERE tenant='acme' AND subject='ana' AND sealed_at IS NULL`
		if sellados {
			q = `SELECT count(*) FROM hub_private_notices WHERE tenant='acme' AND subject='ana' AND sealed_at IS NOT NULL`
		}
		return count(q)
	}
	seguridad := func(title string) domain.NewNotification {
		n := sampleNotification("ana", title)
		n.Type = "security.storage.released"
		n.Level = domain.LevelWarning
		return n
	}

	// En legacy, todo como siempre.
	legado, err := svc.Create(acme, sampleNotification("ana", "antes de congelar"))
	if err != nil {
		t.Fatal(err)
	}
	viejo, err := st.CreateNotification(acme, sampleNotification("ana", "leído hace mucho"))
	if err != nil {
		t.Fatal(err)
	}
	deBea, err := st.CreateNotification(acme, sampleNotification("bea", "de bea, leído hace mucho"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(acme, `UPDATE notifications SET read=true, read_at=now()-interval '90 days' WHERE id = ANY($1)`,
		[]uuid.UUID{viejo.ID, deBea.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(globex, sampleNotification("ana", "en otra organización")); err != nil {
		t.Fatal(err)
	}
	if err := st.SetNotificationPref(acme, "ana", domain.NotificationPref{Source: domain.SourceMail, Muted: true}); err != nil {
		t.Fatal(err)
	}

	epoch := uuid.New()
	if state, err := st.BeginPrivateStorageMigration(acme, "ana", epoch); err != nil || state.Mode != "frozen" || state.Rows != 3 {
		t.Fatalf("congelar: %+v %v", state, err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "hub"}
	q := st.PrivateIngressQueue(autoridadDePrueba{})
	enLaCola := func(kind string, fn func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		return q.Transaction(acme, scope, epoch.String(), kind, fn)
	}

	var deSeguridad domain.Notification
	t.Run("sin clave, el aviso de seguridad se queda en la copia y no en la fuente", func(t *testing.T) {
		n, err := svc.Create(acme, seguridad("soltaste la fuente en otro aparato"))
		deSeguridad = n
		if err != nil || n.ID == uuid.Nil {
			t.Fatalf("aviso a una fuente congelada: %+v %v", n, err)
		}
		if got := avisosDeAna(); got != 2 {
			t.Fatalf("ana tiene %d avisos en acme, quiero los 2 congelados", got)
		}
		if copias(false) != 1 {
			t.Fatal("el aviso no quedó retenido a la espera de clave")
		}
		if _, err := svc.Create(acme, sampleNotification("bea", "a bea sí")); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(globex, sampleNotification("ana", "a ana en globex sí")); err != nil {
			t.Fatal(err)
		}
		if count(`SELECT count(*) FROM notifications WHERE tenant='globex' AND subject='ana'`) != 2 {
			t.Fatal("la fuente de acme congeló también globex")
		}
	})

	t.Run("la guarda rechaza escribir la fuente congelada", func(t *testing.T) {
		if _, err := st.CreateNotification(acme, sampleNotification("ana", "a mano")); !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
			t.Errorf("crear: %v", err)
		}
		if err := st.SetNotificationPref(acme, "ana", domain.NotificationPref{Source: domain.SourceDrive, Kind: "share", Muted: true}); !errors.Is(err, store.ErrPrivateStorageFrozen) {
			t.Errorf("preferencia: %v", err)
		}
		if err := st.SetNotificationPref(acme, "ana", domain.NotificationPref{Source: domain.SourceMail}); !errors.Is(err, store.ErrPrivateStorageFrozen) {
			t.Errorf("quitar preferencia: %v", err)
		}
		ana := domain.ConBuzonesPersonales(acme, "ana", []string{"ana"}, time.Now())
		if err := st.MarkRead(ana, legado.ID, []string{"ana"}); !errors.Is(err, store.ErrPrivateStorageFrozen) {
			t.Errorf("marcar: %v", err)
		}
		if _, err := admin.Exec(acme, `UPDATE notifications SET subject='bea' WHERE id=$1`, legado.ID); err == nil {
			t.Error("un superusuario sacó un aviso de la fuente congelada")
		}
		if _, err := admin.Exec(acme, `UPDATE notifications SET subject='ana' WHERE id=$1`, deBea.ID); err == nil {
			t.Error("un superusuario metió un aviso en la fuente congelada")
		}
	})

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: epoch.String(), Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	target := storage.PublicationTarget{RoomID: "!hub:test", DeviceID: "DEVICE_A"}

	t.Run("dar de alta una clave cifra lo que esperaba", func(t *testing.T) {
		if err := enLaCola("key.enroll", func(tx pgx.Tx, cfg pg.AuthorizedPrivateIngressConfig) error {
			return pg.EnrollPrivateIngress(acme, tx, cfg.PrivateIngressConfig, plan, target, "signing-key")
		}); err != nil {
			t.Fatal(err)
		}
		if copias(false) != 0 || copias(true) != 1 {
			t.Fatalf("tras el alta: %d sin cifrar, %d cifradas", copias(false), copias(true))
		}
		if count(`SELECT count(*) FROM private_ingress_tasks WHERE product='hub' AND owner_sub='ana' AND kind='hub.receive' AND cipher_sha256 IS NOT NULL`) != 1 {
			t.Fatal("no hay entrega cifrada en la cola")
		}
	})

	var nuevo domain.Notification
	t.Run("con clave, un aviso nuevo se cifra al llegar", func(t *testing.T) {
		var err error
		nuevo, err = svc.Create(acme, sampleNotification("ana", "con clave"))
		if err != nil || nuevo.ID == uuid.Nil {
			t.Fatalf("%+v %v", nuevo, err)
		}
		if copias(true) != 2 || avisosDeAna() != 2 {
			t.Fatalf("%d cifradas, %d en la fuente", copias(true), avisosDeAna())
		}
	})

	t.Run("el aparato descifra, reclama y confirma; la copia se va con el recibo", func(t *testing.T) {
		var delivery pg.PrivateIngressDelivery
		if err := enLaCola("task.claim", func(tx pgx.Tx, cfg pg.AuthorizedPrivateIngressConfig) error {
			var err error
			delivery, err = pg.ReadPrivateIngressDelivery(acme, tx, cfg.PrivateIngressConfig, nuevo.ID.String())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		body, err := q.Body(acme, scope, delivery.BlobID)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("con clave")) {
			t.Fatal("el cuerpo guardado lleva el aviso en claro")
		}
		metadata, _, err := agecipher.Open(acme, bytes.NewReader(raw), delivery.Task, identity)
		if err != nil {
			t.Fatal(err)
		}
		var aviso store.AvisoCifrado
		if err := json.Unmarshal(metadata, &aviso); err != nil || aviso.ID != nuevo.ID.String() || aviso.Title != "con clave" || aviso.Tenant != "acme" {
			t.Fatalf("aviso descifrado: %+v %v", aviso, err)
		}

		var claim ingress.Claim
		if err := enLaCola("task.claim", func(tx pgx.Tx, cfg pg.AuthorizedPrivateIngressConfig) error {
			var err error
			claim, err = pg.ClaimPrivateIngress(acme, tx, cfg.PrivateIngressConfig, nuevo.ID.String(), target, "token-1", time.Minute)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if copias(true) != 2 {
			t.Fatal("reclamar no es confirmar: la copia debe seguir")
		}
		receipt := ingress.Receipt{Version: 1, Scope: scope, Epoch: epoch.String(), TaskID: claim.Task.ID, CipherSHA256: claim.Task.Cipher.SHA256, ClaimGeneration: claim.Generation, Commit: storage.Reference{RoomID: target.RoomID, EventID: "$commit"}}
		if err := enLaCola("task.ack", func(tx pgx.Tx, cfg pg.AuthorizedPrivateIngressConfig) error {
			return pg.AcknowledgePrivateIngress(acme, tx, cfg.PrivateIngressConfig, claim, receipt)
		}); err != nil {
			t.Fatal(err)
		}
		if count(`SELECT count(*) FROM hub_private_notices WHERE id=$1`, nuevo.ID) != 0 || copias(true) != 1 {
			t.Fatalf("tras confirmar quedan %d copias cifradas, quiero 1", copias(true))
		}
	})

	t.Run("la campana web ve lo que espera y lo marca leído en la copia", func(t *testing.T) {
		ana := domain.ConBuzonesPersonales(acme, "ana", []string{"ana"}, time.Now())
		lista, err := st.ListNotifications(ana, []string{"ana"}, false, nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		var vista *domain.Notification
		for i := range lista {
			if lista[i].ID == deSeguridad.ID {
				vista = &lista[i]
			}
		}
		if vista == nil || !vista.Private || vista.Read || vista.Title != deSeguridad.Title {
			t.Fatalf("la lista no enseña el aviso que espera: %+v", vista)
		}
		for _, n := range lista {
			if n.ID == nuevo.ID {
				t.Fatal("la lista enseña también un aviso ya confirmado por un aparato")
			}
		}
		sinLeer, err := st.UnreadCount(ana, []string{"ana"})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.MarkRead(ana, deSeguridad.ID, []string{"ana"}); err != nil {
			t.Fatalf("marcar la copia: %v", err)
		}
		if err := st.MarkRead(ana, deSeguridad.ID, []string{"ana"}); err != nil {
			t.Fatalf("marcar otra vez: %v", err)
		}
		if despues, _ := st.UnreadCount(ana, []string{"ana"}); despues != sinLeer-1 {
			t.Fatalf("sin leer: %d → %d", sinLeer, despues)
		}
		if count(`SELECT count(*) FROM hub_private_notices WHERE id=$1 AND read_at IS NOT NULL`, deSeguridad.ID) != 1 {
			t.Fatal("la copia no guardó la lectura")
		}
		// «Marcar todo», también desde otra organización: lo suyo y las copias sí, lo
		// congelado no (no puede cambiar hasta soltar la fuente) y sin error.
		enGlobex := domain.ConBuzonesPersonales(globex, "ana", []string{"ana"}, time.Now())
		if n, err := st.MarkAllRead(enGlobex, []string{"ana"}, ""); err != nil || n != 2 {
			t.Fatalf("desde globex marcó %d (%v), quiero los 2 suyos", n, err)
		}
		if n, err := st.MarkAllRead(ana, []string{"ana"}, ""); err != nil || n != 0 {
			t.Fatalf("en acme marcó %d (%v), quiero 0: lo congelado no cambia", n, err)
		}
		if count(`SELECT count(*) FROM notifications WHERE id=$1 AND NOT read`, legado.ID) != 1 {
			t.Fatal("cambió un aviso de la fuente congelada")
		}
	})

	t.Run("la purga se salta lo congelado y sigue con los demás", func(t *testing.T) {
		if _, err := st.PurgeReadBefore(context.Background(), time.Now().Add(-30*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if count(`SELECT count(*) FROM notifications WHERE id=$1`, viejo.ID) != 1 || count(`SELECT count(*) FROM notifications WHERE id=$1`, deBea.ID) != 0 {
			t.Fatal("la purga no respetó la fuente congelada o no purgó a bea")
		}
	})

	t.Run("un buzón por correo no tiene fuente ni copia", func(t *testing.T) {
		if _, err := svc.Create(acme, sampleNotification("email:ana@acme.test", "a su buzón")); err != nil {
			t.Fatal(err)
		}
		if _, err := st.OlvidarBuzon(context.Background(), "email:ana@acme.test"); err != nil {
			t.Fatal(err)
		}
		if got := count(`SELECT count(*) FROM hub_private_storage_states WHERE owner_sub LIKE 'email:%'`); got != 0 {
			t.Fatalf("%d estados de buzones", got)
		}
	})

	t.Run("la baja de la cuenta borra, la fusión no reescribe", func(t *testing.T) {
		tx, err := admin.Begin(acme)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(acme) }()
		if _, err := tx.Exec(acme, `SELECT set_config('app.merge','on',true)`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(acme, `SAVEPOINT s; UPDATE notifications SET tenant='globex' WHERE tenant='acme' AND subject='ana'`); err == nil {
			t.Fatal("la fusión movió avisos de una fuente congelada")
		}
		if _, err := tx.Exec(acme, `ROLLBACK TO SAVEPOINT s`); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"notifications", "notification_prefs", "hub_private_notices"} {
			if _, err := tx.Exec(acme, `DELETE FROM `+table+` WHERE subject='ana'`); err != nil {
				t.Fatalf("baja en %s: %v", table, err)
			}
		}
	})

	t.Run("soltar la fuente devuelve lo no confirmado y no tira nada", func(t *testing.T) {
		pendiente := count(`SELECT count(*) FROM hub_private_notices WHERE tenant='acme' AND subject='ana'`)
		if pendiente != 1 {
			t.Fatalf("antes de soltar: %d copias, quiero 1 (la de seguridad, sin confirmar)", pendiente)
		}
		antes := avisosDeAna()
		if err := st.ReleasePrivateStorage(acme, scope, pg.PrivateStorageRelease{Epoch: epoch, AuthTime: time.Now(), AMR: []string{"pwd"}}); err != nil {
			t.Fatal(err)
		}
		if got := avisosDeAna(); got != antes+1 {
			t.Fatalf("tras soltar ana tiene %d avisos, quiero %d", got, antes+1)
		}
		if count(`SELECT count(*) FROM notifications WHERE id=$1 AND type='security.storage.released' AND read AND read_at IS NOT NULL`, deSeguridad.ID) != 1 {
			t.Fatal("el aviso de seguridad retenido no volvió a la campana, con su id y su lectura")
		}
		if count(`SELECT count(*) FROM hub_private_notices WHERE tenant='acme' AND subject='ana'`) != 0 {
			t.Fatal("quedan copias tras soltar")
		}
		if _, err := svc.Create(acme, sampleNotification("ana", "otra vez legacy")); err != nil {
			t.Fatal(err)
		}
		if got := avisosDeAna(); got != antes+2 {
			t.Fatalf("tras soltar, un aviso nuevo no fue a la fuente: %d", got)
		}
		if err := st.SetNotificationPref(acme, "ana", domain.NotificationPref{Source: domain.SourceCalendar, Muted: true}); err != nil {
			t.Fatalf("preferencia tras soltar: %v", err)
		}
	})
}

// Los topes cuentan también lo que va a la entrada privada: congelar no abre la
// puerta a llenar la campana de nadie.
func TestEntradaPrivadaRespetaLosTopes(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	svc := notify.NewService(st, notify.NewHub(8)).ConTopePersona(2)
	acme := tenantCtx("acme")
	if _, err := st.BeginPrivateStorageMigration(acme, "ana", uuid.New()); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, err := svc.Create(acme, sampleNotification("ana", "uno")); err != nil {
			t.Fatalf("aviso %d: %v", i, err)
		}
	}
	if _, err := svc.Create(acme, sampleNotification("ana", "de más")); !errors.Is(err, domain.ErrTooMany) {
		t.Fatalf("el tercero: %v, quiero ErrTooMany", err)
	}
}
