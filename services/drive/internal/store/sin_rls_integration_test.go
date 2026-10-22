// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El ámbito de Drive NO puede depender solo de la RLS.
//
// «Mi unidad», cada unidad de equipo y cada canal de Rooms son ámbitos distintos
// dentro de una misma organización, y los separa la política de las migraciones
// 0010 y 0014. El resto de tests conectan con el rol de la aplicación, así que
// demuestran que la política separa. Estos conectan con el rol DUEÑO, que se la
// salta —como hace hoy el despliegue que entrega a los servicios el superusuario
// de arranque de PostgreSQL—, y exigen lo mismo: lo que tiene que sostenerlo
// aquí es el SQL del store.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// montarSinRLS da un store sobre una conexión que se salta la RLS.
func montarSinRLS(t *testing.T) *store.Store {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	var super, bypass bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if !super && !bypass {
		t.Fatal("el rol dueño del harness no se salta la RLS: este test no probaría el SQL")
	}
	return store.New(pool)
}

// ambitos monta una organización con un fichero en cada ámbito.
type ambitos struct {
	st                        *store.Store
	ctx                       context.Context
	raiz                      domain.Folder
	legal, ventas             domain.TeamDrive
	personal, enLegal, enVent domain.File
	canal                     uuid.UUID
	adjunto                   domain.File
}

func montarAmbitos(t *testing.T) ambitos {
	t.Helper()
	a := ambitos{st: montarSinRLS(t), ctx: tenantCtx("acme"), canal: uuid.New()}
	var err error
	if a.raiz, _, err = a.st.EnsureBase(a.ctx, 1<<30, false); err != nil {
		t.Fatal(err)
	}
	a.personal = seedFile(t, a.ctx, a.st, a.raiz.ID, "personal.txt", 10)
	if a.legal, err = a.st.CreateTeamDrive(a.ctx, "Legal", "ana"); err != nil {
		t.Fatal(err)
	}
	if a.ventas, err = a.st.CreateTeamDrive(a.ctx, "Ventas", "bea"); err != nil {
		t.Fatal(err)
	}
	if a.enLegal, err = a.st.TeamCreateFile(a.ctx, a.legal.ID, store.NewFileRecord{
		FolderID: a.legal.RootFolderID, Name: "contrato.pdf", StorageKey: "files/legal/1", SizeBytes: 5, ContentType: "application/pdf", OwnerSub: "ana",
	}); err != nil {
		t.Fatal(err)
	}
	if a.enVent, err = a.st.TeamCreateFile(a.ctx, a.ventas.ID, store.NewFileRecord{
		FolderID: a.ventas.RootFolderID, Name: "tarifas.pdf", StorageKey: "files/ventas/1", SizeBytes: 6, ContentType: "application/pdf", OwnerSub: "bea",
	}); err != nil {
		t.Fatal(err)
	}
	if a.adjunto, err = a.st.CreateChannelFile(a.ctx, a.canal, store.NewFileRecord{
		Name: "nomina.pdf", StorageKey: "files/canal/1", SizeBytes: 7, ContentType: "application/pdf", OwnerSub: "ana",
	}); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSinRLSMiUnidadNoVeOtrosAmbitos(t *testing.T) {
	a := montarAmbitos(t)
	st, ctx := a.st, a.ctx
	ajenos := map[string]domain.File{"unidad de equipo": a.enLegal, "canal": a.adjunto}

	noExiste := func(que string, err error) {
		t.Helper()
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: %v, esperaba que no existiera", que, err)
		}
	}

	// Lecturas por identificador y listados de «Mi unidad».
	for de, f := range ajenos {
		_, err := st.GetFile(ctx, f.ID)
		noExiste("GetFile de un fichero de "+de, err)
	}
	if _, err := st.GetFile(ctx, a.personal.ID); err != nil {
		t.Errorf("«Mi unidad» ya no ve su propio fichero: %v", err)
	}
	_, err := st.GetFolder(ctx, a.legal.RootFolderID)
	noExiste("GetFolder de la raíz de una unidad de equipo", err)
	for _, q := range []string{"contrato", "tarifas", "nomina"} {
		if es, err := st.SearchFiles(ctx, q, 50); err != nil || len(es) != 0 {
			t.Errorf("buscar %q desde «Mi unidad» devuelve %d resultados (%v)", q, len(es), err)
		}
	}
	recientes, _, err := st.ListRecent(ctx, store.RecentCursor{}, 50)
	if err != nil || len(recientes) != 1 || recientes[0].ID != a.personal.ID {
		t.Errorf("recientes de «Mi unidad»: %d entradas (%v), esperaba solo la personal", len(recientes), err)
	}
	if pagina, err := st.ListFilesPage(ctx, a.legal.RootFolderID, store.FileCursor{}, 50); err == nil && len(pagina.Files) != 0 {
		t.Errorf("«Mi unidad» lista %d ficheros de la raíz de una unidad de equipo", len(pagina.Files))
	}
	zs, err := st.CollectZip(ctx, []uuid.UUID{a.enLegal.ID, a.adjunto.ID}, []uuid.UUID{a.legal.RootFolderID}, 0)
	if err != nil || len(zs) != 0 {
		t.Errorf("el ZIP de «Mi unidad» recoge %d ficheros de otros ámbitos (%v)", len(zs), err)
	}

	// Escrituras por identificador.
	for de, f := range ajenos {
		_, err := st.RenameFile(ctx, f.ID, "robado.pdf")
		noExiste("RenameFile de un fichero de "+de, err)
		_, err = st.MoveFile(ctx, f.ID, a.raiz.ID)
		noExiste("MoveFile de un fichero de "+de, err)
		_, err = st.CopyFile(ctx, f.ID, a.raiz.ID, "zoe")
		noExiste("CopyFile de un fichero de "+de, err)
		_, err = st.SetFileLabel(ctx, f.ID, domain.Label{})
		noExiste("SetFileLabel de un fichero de "+de, err)
		_, err = st.DeleteFile(ctx, f.ID)
		noExiste("DeleteFile de un fichero de "+de, err)
	}
	if res, err := st.BatchMove(ctx, []uuid.UUID{a.enLegal.ID, a.adjunto.ID}, nil, a.raiz.ID); err != nil || res.Files != 0 {
		t.Errorf("BatchMove movió %d ficheros de otros ámbitos (%v)", res.Files, err)
	}
	if del, _, err := st.BatchTrash(ctx, []uuid.UUID{a.enLegal.ID, a.adjunto.ID}, nil); err != nil || len(del) != 0 {
		t.Errorf("BatchTrash tiró %d ficheros de otros ámbitos (%v)", len(del), err)
	}
	// Y un fichero personal no se muda a la carpeta de una unidad desde «Mi unidad».
	if _, err := st.MoveFile(ctx, a.personal.ID, a.legal.RootFolderID); !errors.Is(err, domain.ErrInvalidParent) {
		t.Errorf("MoveFile de un fichero personal a la raíz de una unidad: %v", err)
	}
}

func TestSinRLSCadaUnidadYCadaCanalVenSoloLoSuyo(t *testing.T) {
	a := montarAmbitos(t)
	st, ctx := a.st, a.ctx

	if f, err := st.TeamGetFile(ctx, a.legal.ID, a.enLegal.ID); err != nil || f.ID != a.enLegal.ID {
		t.Errorf("la unidad no ve su fichero: %v", err)
	}
	for de, f := range map[string]domain.File{"otra unidad": a.enVent, "«Mi unidad»": a.personal, "un canal": a.adjunto} {
		if _, err := st.TeamGetFile(ctx, a.legal.ID, f.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("la unidad Legal lee un fichero de %s: %v", de, err)
		}
		if _, err := st.TeamDeleteFile(ctx, a.legal.ID, f.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("la unidad Legal tira un fichero de %s: %v", de, err)
		}
	}
	if _, err := st.ChannelFile(ctx, uuid.New(), a.adjunto.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("otro canal lee el adjunto: %v", err)
	}

	// Nada de lo anterior tocó lo ajeno.
	if _, err := st.TeamGetFile(ctx, a.ventas.ID, a.enVent.ID); err != nil {
		t.Errorf("el fichero de Ventas ya no está: %v", err)
	}
	if _, err := st.ChannelFile(ctx, a.canal, a.adjunto.ID); err != nil {
		t.Errorf("el adjunto del canal ya no está: %v", err)
	}
	if _, err := st.GetFile(ctx, a.personal.ID); err != nil {
		t.Errorf("el fichero personal ya no está: %v", err)
	}
}

// «Guardar en mi Drive» es el único cruce de ámbito del paquete, y lo hace una
// consulta que lo dice. Tiene que funcionar igual con la RLS evaluada (el rol de la
// aplicación) que sin ella: antes solo funcionaba sin ella, por accidente.
func TestGuardarEnMiDriveCruzaDeAmbitoAProposito(t *testing.T) {
	montajes := map[string]func(*testing.T) *store.Store{
		"sin RLS": montarSinRLS,
		"con RLS": func(t *testing.T) *store.Store {
			t.Helper()
			return store.New(pgtest.New(t, db.Migrations, db.MigrationsDir).Pool)
		},
	}
	for nombre, montar := range montajes {
		t.Run(nombre, func(t *testing.T) {
			st, ctx := montar(t), store.WithOwner(tenantCtx("acme"), "bea")
			raiz, _, err := st.EnsureBase(ctx, 1<<30, false)
			if err != nil {
				t.Fatal(err)
			}
			canal := uuid.New()
			adjunto, err := st.CreateChannelFile(ctx, canal, store.NewFileRecord{
				Name: "nomina.pdf", StorageKey: "files/canal/1", SizeBytes: 7, ContentType: "application/pdf", OwnerSub: "ana",
			})
			if err != nil {
				t.Fatal(err)
			}
			// Por el camino de «Mi unidad» el adjunto no existe.
			if _, err := st.CopyFile(ctx, adjunto.ID, raiz.ID, "bea"); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("CopyFile de «Mi unidad» copia un adjunto de canal: %v", err)
			}
			// Nombrando otro canal, tampoco.
			if _, err := st.CopiarAdjuntoAMiUnidad(ctx, uuid.New(), adjunto.ID, raiz.ID, "bea"); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("se copia el adjunto nombrando otro canal: %v", err)
			}
			copia, err := st.CopiarAdjuntoAMiUnidad(ctx, canal, adjunto.ID, raiz.ID, "bea")
			if err != nil {
				t.Fatalf("guardar el adjunto en «Mi unidad»: %v", err)
			}
			// La copia es de «Mi unidad», de quien la guarda, y comparte blob.
			got, err := st.GetFile(ctx, copia.ID)
			if err != nil || got.StorageKey != adjunto.StorageKey || got.OwnerSub != "bea" || got.FolderID != raiz.ID {
				t.Errorf("la copia no quedó en «Mi unidad»: %+v (%v)", got, err)
			}
			// Y el adjunto sigue en su canal, invisible desde «Mi unidad».
			if _, err := st.ChannelFile(ctx, canal, adjunto.ID); err != nil {
				t.Errorf("el adjunto ya no está en su canal: %v", err)
			}
			if _, err := st.GetFile(ctx, adjunto.ID); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("«Mi unidad» ve el adjunto del canal: %v", err)
			}
		})
	}
}

// TestSinRLSMiUnidadAisladaPorPersona verifica que dos personas de la misma
// organización ("acme") tienen raíces personales distintas y que ni con RLS ni
// sin RLS (superusuario) puede una persona ver, listar, buscar, mover, copiar,
// etiquetar, borrar, compartir ni consultar versiones de los ficheros o
// carpetas de «Mi unidad» de otra persona, salvo compartición explícita.
func TestSinRLSMiUnidadAisladaPorPersona(t *testing.T) {
	montajes := map[string]func(*testing.T) *store.Store{
		"sin RLS": montarSinRLS,
		"con RLS": func(t *testing.T) *store.Store {
			t.Helper()
			return store.New(pgtest.New(t, db.Migrations, db.MigrationsDir).Pool)
		},
	}
	for nombre, montar := range montajes {
		t.Run(nombre, func(t *testing.T) {
			st := montar(t)
			ctxAlice := store.WithOwner(tenantCtx("acme"), "alice")
			ctxBob := store.WithOwner(tenantCtx("acme"), "bob")

			raizAlice, _, err := st.EnsureBase(ctxAlice, 1<<30, false)
			if err != nil {
				t.Fatalf("EnsureBase alice: %v", err)
			}
			raizBob, _, err := st.EnsureBase(ctxBob, 1<<30, false)
			if err != nil {
				t.Fatalf("EnsureBase bob: %v", err)
			}
			if raizAlice.ID == raizBob.ID {
				t.Fatalf("alice y bob comparten carpeta raíz (%s): «Mi unidad» sigue siendo del tenant", raizAlice.ID)
			}
			if raizAlice.OwnerSub != "alice" || raizBob.OwnerSub != "bob" {
				t.Fatalf("owner_sub de raíces = (%q, %q), esperaba (alice, bob)", raizAlice.OwnerSub, raizBob.OwnerSub)
			}

			carpetaAlice, err := st.CreateFolder(ctxAlice, raizAlice.ID, "SecretoAlice", "alice", false)
			if err != nil {
				t.Fatalf("CreateFolder alice: %v", err)
			}
			ficheroAlice, err := st.CreateFile(ctxAlice, store.NewFileRecord{
				FolderID:    carpetaAlice.ID,
				Name:        "nomina-alice.pdf",
				StorageKey:  "files/acme/alice-1",
				SizeBytes:   42,
				ContentType: "application/pdf",
				Checksum:    "chk-alice-1",
				OwnerSub:    "alice",
			})
			if err != nil {
				t.Fatalf("CreateFile alice: %v", err)
			}

			noExiste := func(que string, err error) {
				t.Helper()
				if !errors.Is(err, domain.ErrNotFound) {
					t.Errorf("%s: %v, esperaba ErrNotFound", que, err)
				}
			}

			// Bob no ve ni la raíz, ni la subcarpeta, ni el fichero de Alice.
			_, err = st.GetFolder(ctxBob, raizAlice.ID)
			noExiste("GetFolder raíz de alice por bob", err)
			_, err = st.GetFolder(ctxBob, carpetaAlice.ID)
			noExiste("GetFolder subcarpeta de alice por bob", err)
			_, err = st.GetFile(ctxBob, ficheroAlice.ID)
			noExiste("GetFile de alice por bob", err)

			if carpetas, err := st.ListSubfolders(ctxBob, raizAlice.ID); err != nil || len(carpetas) != 0 {
				t.Errorf("ListSubfolders de raíz de alice por bob: %d (%v)", len(carpetas), err)
			}
			_, err = st.ListFilesPage(ctxBob, carpetaAlice.ID, store.FileCursor{}, 50)
			noExiste("ListFilesPage de carpeta de alice por bob", err)
			if res, err := st.SearchFiles(ctxBob, "nomina-alice", 50); err != nil || len(res) != 0 {
				t.Errorf("SearchFiles por bob encontró %d ficheros de alice (%v)", len(res), err)
			}
			if rec, _, err := st.ListRecent(ctxBob, store.RecentCursor{}, 50); err != nil || len(rec) != 0 {
				t.Errorf("ListRecent por bob devolvió %d ficheros de alice (%v)", len(rec), err)
			}
			if vers, _, err := st.ListVersions(ctxBob, ficheroAlice.ID, 50, 0); err != nil || len(vers) != 0 {
				t.Errorf("ListVersions por bob devolvió %d versiones de alice (%v)", len(vers), err)
			}
			_, err = st.GetVersion(ctxBob, ficheroAlice.ID, 1)
			noExiste("GetVersion de alice por bob", err)

			// Bob tampoco puede modificar, mover, copiar, borrar ni compartir lo de Alice.
			_, err = st.RenameFile(ctxBob, ficheroAlice.ID, "robado.pdf")
			noExiste("RenameFile de alice por bob", err)
			_, err = st.MoveFile(ctxBob, ficheroAlice.ID, raizBob.ID)
			noExiste("MoveFile de alice por bob", err)
			_, err = st.CopyFile(ctxBob, ficheroAlice.ID, raizBob.ID, "bob")
			noExiste("CopyFile de alice por bob", err)
			_, err = st.DeleteFile(ctxBob, ficheroAlice.ID)
			noExiste("DeleteFile de alice por bob", err)
			_, err = st.RenameFolder(ctxBob, carpetaAlice.ID, "Robada")
			noExiste("RenameFolder de alice por bob", err)
			_, err = st.DeleteFolder(ctxBob, carpetaAlice.ID)
			noExiste("DeleteFolder de alice por bob", err)
			_, err = st.CreateShare(ctxBob, store.NewShareRecord{
				FileID:     &ficheroAlice.ID,
				Kind:       domain.ShareLink,
				Permission: domain.PermRead,
				Token:      "tok-robo-" + nombre,
				CreatedBy:  "bob",
			})
			noExiste("CreateShare sobre fichero de alice por bob", err)

			// Si Alice comparte explícitamente su fichero con Bob, aparece en «Compartido conmigo» de Bob.
			sh, err := st.CreateShare(ctxAlice, store.NewShareRecord{
				FileID:     &ficheroAlice.ID,
				Kind:       domain.ShareUser,
				Permission: domain.PermRead,
				Subject:    "bob",
				CreatedBy:  "alice",
			})
			if err != nil {
				t.Fatalf("CreateShare de alice para bob: %v", err)
			}
			compartidos, err := st.ListSharedWithMe(ctxBob, "bob", nil)
			if err != nil || len(compartidos) != 1 || compartidos[0].ID != ficheroAlice.ID {
				t.Fatalf("ListSharedWithMe de bob = %+v (%v), esperaba el fichero compartido por alice", compartidos, err)
			}
			// Bob no puede revocar el share creado por Alice sobre el fichero de Alice.
			if err := st.RevokeShare(ctxBob, sh.ID); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("RevokeShare por bob sobre share de alice: %v, esperaba ErrNotFound", err)
			}
		})
	}
}

