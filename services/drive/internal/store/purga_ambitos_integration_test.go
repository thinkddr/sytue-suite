// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Purgar decide si el blob se borra del almacén, y esa pregunta es de TODA la
// organización: una copia por referencia en otro ámbito sigue necesitándolo.
//
// Con el rol dueño (que se salta la RLS) la consulta ya veía todos los ámbitos.
// Con el rol de la aplicación solo veía el de su transacción, y purgar la copia
// personal de un adjunto de canal lo daba por huérfano: el siguiente paso del
// servicio lo borraba del almacén y el adjunto del canal se quedaba sin contenido.
// Por eso cada caso corre con los dos roles.
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

// conLosDosRoles corre el caso con la RLS evaluada (rol de la aplicación) y sin
// ella (rol dueño): el resultado tiene que ser el mismo.
func conLosDosRoles(t *testing.T, caso func(t *testing.T, st *store.Store)) {
	t.Helper()
	t.Run("con RLS", func(t *testing.T) {
		caso(t, store.New(pgtest.New(t, db.Migrations, db.MigrationsDir).Pool))
	})
	t.Run("sin RLS", func(t *testing.T) { caso(t, montarSinRLS(t)) })
}

func TestPurgarNoDaPorHuerfanoUnBlobQueUsaOtroAmbito(t *testing.T) {
	conLosDosRoles(t, func(t *testing.T, st *store.Store) {
		ctx := store.WithOwner(tenantCtx("acme"), "bea")
		raiz, _, err := st.EnsureBase(ctx, 1<<30, false)
		if err != nil {
			t.Fatal(err)
		}
		pdf := func(clave string) store.NewFileRecord {
			return store.NewFileRecord{
				FolderID: raiz.ID, Name: uuid.NewString() + ".pdf", StorageKey: clave,
				SizeBytes: 7, ContentType: "application/pdf", OwnerSub: "ana",
			}
		}
		canal := uuid.New()
		adjunto, err := st.CreateChannelFile(ctx, canal, pdf("files/canal/1"))
		if err != nil {
			t.Fatal(err)
		}

		// Una copia personal del adjunto, purgada: el adjunto sigue usando el blob.
		copia, err := st.CopiarAdjuntoAMiUnidad(ctx, canal, adjunto.ID, raiz.ID, "bea")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.DeleteFile(ctx, copia.ID); err != nil {
			t.Fatal(err)
		}
		df, err := st.PurgeFile(ctx, copia.ID)
		if err != nil {
			t.Fatal(err)
		}
		if df.BlobOrphaned {
			t.Error("purgar la copia personal da por huérfano el blob del adjunto del canal")
		}

		// Lo mismo cuando la copia se purga con su carpeta.
		carpeta, err := st.CreateFolder(ctx, raiz.ID, "Guardado", "bea", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CopiarAdjuntoAMiUnidad(ctx, canal, adjunto.ID, carpeta.ID, "bea"); err != nil {
			t.Fatal(err)
		}
		if _, err := st.DeleteFolder(ctx, carpeta.ID); err != nil {
			t.Fatal(err)
		}
		dfs, err := st.PurgeFolder(ctx, carpeta.ID)
		if err != nil || len(dfs) != 1 {
			t.Fatalf("purgar la carpeta: %d ficheros (%v)", len(dfs), err)
		}
		if dfs[0].BlobOrphaned {
			t.Error("purgar la carpeta da por huérfano el blob del adjunto del canal")
		}
		if _, err := st.ChannelFile(ctx, canal, adjunto.ID); err != nil {
			t.Errorf("el adjunto ya no está en su canal: %v", err)
		}

		// Y al revés: se purga en una unidad de equipo lo que «Mi unidad» sigue usando.
		if _, err := st.CreateFile(ctx, pdf("files/comun/1")); err != nil {
			t.Fatal(err)
		}
		unidad, err := st.CreateTeamDrive(ctx, "Legal", "ana")
		if err != nil {
			t.Fatal(err)
		}
		rec := pdf("files/comun/1")
		rec.FolderID = unidad.RootFolderID
		enUnidad, err := st.TeamCreateFile(ctx, unidad.ID, rec)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.TeamDeleteFile(ctx, unidad.ID, enUnidad.ID); err != nil {
			t.Fatal(err)
		}
		if df, err = st.TeamPurgeFile(ctx, unidad.ID, enUnidad.ID); err != nil {
			t.Fatal(err)
		}
		if df.BlobOrphaned {
			t.Error("purgar en la unidad de equipo da por huérfano un blob que «Mi unidad» sigue usando")
		}

		// Controles. Un blob que ya no usa nadie SÍ queda huérfano, y la misma clave
		// en OTRA organización no cuenta como uso: la puerta de mantenimiento abre
		// todas, y lo que acota es el `tenant=$1` de la consulta.
		otra := tenantCtx("otra")
		raizOtra, _, err := st.EnsureBase(otra, 1<<30, false)
		if err != nil {
			t.Fatal(err)
		}
		ajeno := pdf("files/solo/1")
		ajeno.FolderID = raizOtra.ID
		if _, err := st.CreateFile(otra, ajeno); err != nil {
			t.Fatal(err)
		}
		solo, err := st.CreateFile(ctx, pdf("files/solo/1"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.DeleteFile(ctx, solo.ID); err != nil {
			t.Fatal(err)
		}
		if df, err = st.PurgeFile(ctx, solo.ID); err != nil {
			t.Fatal(err)
		}
		if !df.BlobOrphaned {
			t.Error("un blob que ya no usa nadie en la organización no queda huérfano")
		}
	})
}

// Los documentos de Docs viven en «Mi unidad» (mig 0027): un .odt de una unidad de
// equipo no es de Docs, con la RLS evaluada y sin ella. Lo que se fija es que la
// respuesta no dependa del rol.
func TestFicheroEnCualquierEstadoNoSaleDeMiUnidad(t *testing.T) {
	conLosDosRoles(t, func(t *testing.T, st *store.Store) {
		ctx := tenantCtx("acme")
		raiz, _, err := st.EnsureBase(ctx, 1<<30, false)
		if err != nil {
			t.Fatal(err)
		}
		odt := store.NewFileRecord{
			FolderID: raiz.ID, Name: "Informe.odt", StorageKey: "files/odt/1",
			SizeBytes: 7, ContentType: "application/vnd.oasis.opendocument.text", OwnerSub: "ana",
		}
		personal, err := st.CreateFile(ctx, odt)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.DeleteFile(ctx, personal.ID); err != nil {
			t.Fatal(err)
		}
		if f, err := st.FicheroEnCualquierEstado(ctx, personal.ID); err != nil || f.DeletedAt == nil {
			t.Errorf("el .odt de «Mi unidad» en la papelera: %+v (%v)", f.DeletedAt, err)
		}
		unidad, err := st.CreateTeamDrive(ctx, "Legal", "ana")
		if err != nil {
			t.Fatal(err)
		}
		odt.FolderID, odt.StorageKey = unidad.RootFolderID, "files/odt/2"
		enUnidad, err := st.TeamCreateFile(ctx, unidad.ID, odt)
		if err != nil {
			t.Fatal(err)
		}
		canal := uuid.New()
		odt.StorageKey = "files/odt/3"
		adjunto, err := st.CreateChannelFile(ctx, canal, odt)
		if err != nil {
			t.Fatal(err)
		}
		for de, id := range map[string]uuid.UUID{"unidad de equipo": enUnidad.ID, "canal": adjunto.ID} {
			if _, err := st.FicheroEnCualquierEstado(ctx, id); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Docs alcanza un .odt de %s: %v", de, err)
			}
		}
	})
}

// Desconectar el almacén de terceros borra sus credenciales: lo que quede escrito
// allí deja de poder leerse. «Queda algo» se pregunta a toda la organización, no a
// «Mi unidad», que es el ámbito de quien desconecta.
func TestDesconectarElAlmacenMiraTodaLaOrganizacion(t *testing.T) {
	conLosDosRoles(t, func(t *testing.T, st *store.Store) {
		ctx, otra := tenantCtx("acme"), tenantCtx("otra")
		conectar := func(ctx context.Context) {
			t.Helper()
			if _, err := st.SaveExternalStorage(ctx, store.ExternalStorage{
				Kind: "s3", Endpoint: "s3.example.eu",
				Bucket: "b", AccessKey: "k", SecretKeyEnc: "x", Enabled: true, AcceptedTermsAt: time.Now(),
			}, "ana"); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []context.Context{ctx, otra} {
			if _, _, err := st.EnsureBase(c, 1<<30, false); err != nil {
				t.Fatal(err)
			}
			conectar(c)
		}
		// Sin nada escrito allí, se desconecta.
		if err := st.DeleteExternalStorage(ctx); err != nil {
			t.Fatalf("desconectar un almacén vacío: %v", err)
		}
		conectar(ctx)
		unidad, err := st.CreateTeamDrive(ctx, "Legal", "ana")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.TeamCreateFile(ctx, unidad.ID, store.NewFileRecord{
			FolderID: unidad.RootFolderID, Name: "contrato.pdf",
			StorageKey: store.ExternalPrefix + "files/1", SizeBytes: 5, ContentType: "application/pdf", OwnerSub: "ana",
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.DeleteExternalStorage(ctx); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("se desconecta el almacén con un fichero de una unidad de equipo escrito allí: %v", err)
		}
		// Lo de una organización no retiene el almacén de otra.
		if err := st.DeleteExternalStorage(otra); err != nil {
			t.Errorf("los ficheros de otra organización impiden desconectar: %v", err)
		}
	})
}
