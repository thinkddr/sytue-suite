// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Docs contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Tras el build-tag `integration`
// (no corren en el build/CI por defecto). Ejercen los flujos completos sobre BD:
// persistencia de documento, secuenciación atómica de operaciones CRDT con seq
// monótono + evento transaccional, snapshot + versión (historial), permisos/ACL,
// y el aislamiento multi-tenant reforzado por RLS (app.tenant) — un tenant no ve
// documentos de otro.
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// iniciaProteccion deja evaluado el estado de protección de un documento. Service.ApplyOps lo
// hace solo la primera vez; los tests que escriben por Store.AppendOps directamente, con
// operaciones por carácter, lo piden antes (las secciones de Docus las miran).
func iniciaProteccion(ctx context.Context, t *testing.T, st *store.Store, docID string) {
	t.Helper()
	if err := st.IniciarProteccion(ctx, docID, domain.EstadoProteccion{Claves: map[string]domain.ClaveHash{}}); err != nil {
		t.Fatal(err)
	}
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(harnessComoEnProduccion(t).Pool)
}

// harnessComoEnProduccion es el Postgres de los tests con las funciones SECURITY
// DEFINER en manos del rol de la aplicación, que no es superusuario ni tiene
// BYPASSRLS. Es como están en producción —mismo rol para migrar y para servir—, y sin
// esto el dueño de las pruebas (superusuario) se salta la RLS y tapa una función que
// en producción no ve ninguna fila (ADR 0059).
func harnessComoEnProduccion(t *testing.T) *pgtest.Harness {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	app, err := url.Parse(h.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	rol := app.User.Username()
	if strings.Trim(rol, "abcdefghijklmnopqrstuvwxyz_0123456789") != "" {
		t.Fatalf("rol de aplicación inesperado: %q", rol)
	}
	if _, err := conn.Exec(context.Background(), fmt.Sprintf(`DO $$
		DECLARE f record;
		BEGIN
			FOR f IN SELECT p.oid::regprocedure AS firma FROM pg_proc p
				JOIN pg_namespace n ON n.oid = p.pronamespace
				WHERE n.nspname = current_schema() AND p.prosecdef
			LOOP
				EXECUTE format('ALTER FUNCTION %%s OWNER TO %%I', f.firma, '%s');
			END LOOP;
		END $$`, rol)); err != nil {
		t.Fatal(err)
	}
	return h
}

func insertOp(after *domain.Dot, actor string, counter uint64, val string, lamport uint64) domain.Op {
	return domain.Op{
		Kind:    domain.OpInsert,
		Dot:     domain.Dot{Actor: actor, Counter: counter},
		After:   after,
		Value:   json.RawMessage(`"` + val + `"`),
		Lamport: lamport,
	}
}

// TestDocumentLifecycle cubre la persistencia de un documento: creación, lectura,
// renombrado, favorito y borrado lógico (papelera), con seq inicial 0.
func TestDocumentLifecycle(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Acta", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if doc.Seq != 0 || doc.Tenant != "acme" || doc.Kind != domain.KindDocument {
		t.Fatalf("documento inicial inesperado: %+v", doc)
	}

	got, err := st.GetDocument(ctx, doc.ID)
	if err != nil || got.ID != doc.ID {
		t.Fatalf("GetDocument: %v", err)
	}

	if err := st.RenameDocument(ctx, doc.ID, "Acta v2"); err != nil {
		t.Fatalf("RenameDocument: %v", err)
	}
	if err := st.SetStarred(ctx, doc.ID, "user-ada", true); err != nil {
		t.Fatalf("SetStarred: %v", err)
	}
	renamed, _ := st.GetDocument(ctx, doc.ID)
	if renamed.Title != "Acta v2" || len(renamed.StarredBy) != 1 {
		t.Fatalf("rename/star no persistió: %+v", renamed)
	}

	if err := st.SoftDeleteDocument(ctx, doc.ID); err != nil {
		t.Fatalf("SoftDeleteDocument: %v", err)
	}
	if _, err := st.GetDocument(ctx, doc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("documento borrado visible: %v", err)
	}
}

// TestAppendOpsSequencing cubre la secuencia CRDT: AppendOps asigna seq monótonos
// crecientes, sube documents.seq atómicamente y permite el replay incremental
// (OpsSince) para un cliente que reconecta. Rechaza documentos inexistentes.
func TestAppendOpsSequencing(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Doc", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	iniciaProteccion(ctx, t, st, doc.ID)

	batch1 := []domain.Op{
		insertOp(nil, "actorA", 1, "H", 1),
		insertOp(&domain.Dot{Actor: "actorA", Counter: 1}, "actorA", 2, "i", 2),
	}
	seqd, newSeq, err := st.AppendOps(ctx, doc.ID, domain.Subject{Sub: "user-ada"}, batch1, nil)
	if err != nil {
		t.Fatalf("AppendOps(1): %v", err)
	}
	if len(seqd) != 2 || seqd[0].Seq != 1 || seqd[1].Seq != 2 || newSeq != 2 {
		t.Fatalf("secuenciación incorrecta: seqs=%+v newSeq=%d", seqd, newSeq)
	}

	// Segundo lote continúa la secuencia monótona.
	batch2 := []domain.Op{insertOp(&domain.Dot{Actor: "actorA", Counter: 2}, "actorB", 3, "!", 5)}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: "user-bob", Role: domain.RoleEditor, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatal(err)
	}
	seqd2, newSeq2, err := st.AppendOps(ctx, doc.ID, domain.Subject{Sub: "user-bob"}, batch2, nil)
	if err != nil {
		t.Fatalf("AppendOps(2): %v", err)
	}
	if seqd2[0].Seq != 3 || newSeq2 != 3 {
		t.Fatalf("segunda tanda no continúa la seq: %+v", seqd2)
	}

	// documents.seq se actualizó a la última.
	after, _ := st.GetDocument(ctx, doc.ID)
	if after.Seq != 3 {
		t.Fatalf("documents.seq = %d, quiero 3", after.Seq)
	}

	// Replay incremental desde seq 1: devuelve solo ops posteriores, en orden.
	tail, err := st.OpsSince(ctx, doc.ID, 1, 100)
	if err != nil {
		t.Fatalf("OpsSince: %v", err)
	}
	if len(tail) != 2 || tail[0].Seq != 2 || tail[1].Seq != 3 {
		t.Fatalf("OpsSince(1) => %+v", tail)
	}
	if tail[1].AuthorSub != "user-bob" {
		t.Fatalf("author_sub no persistido: %+v", tail[1])
	}

	// Documento inexistente => ErrNotFound.
	if _, _, err := st.AppendOps(ctx, "00000000-0000-0000-0000-000000000000", domain.Subject{Sub: "x"}, batch1, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AppendOps(doc inexistente) = %v, quiero ErrNotFound", err)
	}
}

// TestAppendOpsBaseSeq cubre el guardado condicional de hojas y presentaciones:
// con base_seq, un `set` sobre una clave que otro escribió después se rechaza
// (ErrStaleSeq) sin persistir nada; una clave distinta, o la misma con la base al
// día, pasa. Es lo que impide que el modelo entero de uno pise al del otro.
func TestAppendOpsBaseSeq(t *testing.T) {
	st := newStore(t)
	// Por el servicio, que es quien prepara la protección de hojas antes de escribir.
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Hoja", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: "user-bob", Role: domain.RoleEditor, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatal(err)
	}
	set := func(actor, key, val string, lamport uint64) []domain.Op {
		return []domain.Op{{
			Kind: domain.OpSet, Dot: domain.Dot{Actor: actor, Counter: lamport},
			Key: key, Value: json.RawMessage(`"` + val + `"`), Lamport: lamport,
		}}
	}
	base := int64(0)
	if _, seq, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-ada"}, doc.ID, set("ada", "doc:sheet", "A", 1), &base); err != nil || seq != 1 {
		t.Fatalf("primer guardado con base 0: seq=%d err=%v", seq, err)
	}
	// Bob guarda con la base de antes de Ada: conflicto y nada escrito.
	if _, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-bob"}, doc.ID, set("bob", "doc:sheet", "B", 2), &base); !errors.Is(err, domain.ErrStaleSeq) {
		t.Fatalf("guardado con base obsoleta = %v, quiero ErrStaleSeq", err)
	}
	if d, _ := st.GetDocument(ctx, doc.ID); d.Seq != 1 {
		t.Fatalf("el rechazo escribió: documents.seq = %d, quiero 1", d.Seq)
	}
	// Otra clave no choca; la misma con la base al día, tampoco.
	if _, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-bob"}, doc.ID, set("bob", "otra", "x", 2), &base); err != nil {
		t.Fatalf("clave distinta con base vieja: %v", err)
	}
	alDia := int64(2)
	if _, seq, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-bob"}, doc.ID, set("bob", "doc:sheet", "AB", 3), &alDia); err != nil || seq != 3 {
		t.Fatalf("base al día: seq=%d err=%v", seq, err)
	}
	// Sin base, LWW incondicional como siempre (Fyne, documentos de texto).
	if _, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: "user-ada"}, doc.ID, set("ada", "doc:sheet", "Z", 4), nil); err != nil {
		t.Fatalf("sin base: %v", err)
	}
}

// Una autorización previa no sobrevive a una revocación antes de persistir.
// Las denegaciones no dejan operaciones, secuencia ni eventos parciales.
func TestAppendOpsReauthorizes(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	subj := domain.Subject{Sub: "editor", Tenant: "acme"}
	for _, kind := range []domain.Kind{domain.KindDocument, domain.KindSpreadsheet, domain.KindPresentation} {
		t.Run(string(kind), func(t *testing.T) {
			doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
				Kind: kind, Title: "ACL", OwnerSub: "owner",
			})
			if err != nil {
				t.Fatal(err)
			}
			iniciaProteccion(ctx, t, st, doc.ID)
			ops := []domain.Op{insertOp(nil, "editor", 1, "X", 1)}
			assertDeniedWrites(ctx, t, st, doc.ID, subj, ops)
			got, err := st.GetDocument(ctx, doc.ID)
			if err != nil || got.Seq != 0 {
				t.Fatalf("secuencia tras denegar: %+v, %v", got, err)
			}
			persisted, err := st.OpsSince(ctx, doc.ID, 0, 100)
			if err != nil || len(persisted) != 0 {
				t.Fatalf("operaciones tras denegar: %+v, %v", persisted, err)
			}
			var count int
			if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT count(*) FROM events_outbox WHERE type = $1 AND payload->>'document_id' = $2`, store.EventDocUpdated, doc.ID).Scan(&count)
			}); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("eventos tras denegar: %d", count)
			}
			if _, _, err := st.AppendOps(ctx, doc.ID, domain.Subject{Sub: "owner"}, ops, nil); err != nil {
				t.Fatal(err)
			}
			if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
				PrincipalKind: domain.PrincipalUser, PrincipalID: subj.Sub, Role: domain.RoleEditor, GrantedBy: "owner",
			}); err != nil {
				t.Fatal(err)
			}
			if _, seq, err := st.AppendOps(ctx, doc.ID, subj, []domain.Op{insertOp(nil, "editor", 2, "Y", 2)}, nil); err != nil || seq != 2 {
				t.Fatalf("editor vigente: seq=%d, %v", seq, err)
			}
		})
	}
}

func assertDeniedWrites(ctx context.Context, t *testing.T, st *store.Store, docID string, subj domain.Subject, ops []domain.Op) {
	t.Helper()
	for _, role := range []domain.Role{"", domain.RoleViewer, domain.RoleCommenter, domain.RoleEditor} {
		if role != "" {
			if err := st.UpsertPermission(ctx, docID, domain.Permission{
				PrincipalKind: domain.PrincipalUser, PrincipalID: subj.Sub, Role: role, GrantedBy: "owner",
			}); err != nil {
				t.Fatal(err)
			}
		}
		if role == domain.RoleEditor {
			if _, _, err := domain.NewService(st).Authorize(ctx, docID, subj, domain.RoleEditor); err != nil {
				t.Fatal(err)
			}
			if err := st.RevokePermission(ctx, docID, domain.PrincipalUser, subj.Sub); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := st.AppendOps(ctx, docID, subj, ops, nil); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("rol %q: escritura no denegada: %v", role, err)
		}
	}
}

// TestSnapshotAndVersion cubre la consolidación: guardar un snapshot inline,
// enlazarlo como snapshot actual del documento, y registrar una versión que lo
// referencia, recuperable en el historial.
func TestSnapshotAndVersion(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	doc, _, _ := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Doc", OwnerSub: "user-ada",
	})
	iniciaProteccion(ctx, t, st, doc.ID)
	// Aplicar unas ops para tener seq > 0.
	if _, _, err := st.AppendOps(ctx, doc.ID, domain.Subject{Sub: "user-ada"}, []domain.Op{
		insertOp(nil, "a", 1, "X", 1),
	}, nil); err != nil {
		t.Fatalf("AppendOps: %v", err)
	}

	stateJSON, _ := json.Marshal(domain.NewState())
	snapID, err := st.SaveSnapshot(ctx, domain.SaveSnapshotParams{
		DocumentID: doc.ID, Seq: 1, State: stateJSON, SizeBytes: int64(len(stateJSON)),
	})
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// El documento apunta ahora al snapshot.
	withSnap, _ := st.GetDocument(ctx, doc.ID)
	if withSnap.SnapshotID == nil || *withSnap.SnapshotID != snapID {
		t.Fatalf("snapshot_id no enlazado: %+v", withSnap.SnapshotID)
	}

	label := "Antes de revisión"
	ver, err := st.CreateVersion(ctx, doc.ID, snapID, "user-ada", 1, &label, false)
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if ver.SnapshotID != snapID || ver.Seq != 1 {
		t.Fatalf("versión incorrecta: %+v", ver)
	}

	versions, err := st.ListVersions(ctx, doc.ID, 50)
	if err != nil || len(versions) != 1 || versions[0].ID != ver.ID {
		t.Fatalf("ListVersions => %+v (%v)", versions, err)
	}

	got, err := st.GetVersion(ctx, doc.ID, ver.ID)
	if err != nil || got.Label == nil || *got.Label != label {
		t.Fatalf("GetVersion => %+v (%v)", got, err)
	}
}

// TestPermissionsAndListing cubre la ACL: conceder/actualizar/revocar permisos, y
// que ListDocuments incluya documentos compartidos vía entrada de tenant.
func TestPermissionsAndListing(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	doc, _, _ := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Compartido", OwnerSub: "user-ada",
	})

	// Conceder acceso de lectura a todo el tenant.
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalTenant, PrincipalID: "acme",
		Role: domain.RoleViewer, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatalf("UpsertPermission: %v", err)
	}
	// Upsert idempotente: promover a editor no duplica.
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalTenant, PrincipalID: "acme",
		Role: domain.RoleEditor, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatalf("UpsertPermission(promote): %v", err)
	}
	perms, err := st.ListPermissions(ctx, doc.ID)
	if err != nil || len(perms) != 1 || perms[0].Role != domain.RoleEditor {
		t.Fatalf("ListPermissions => %+v (%v)", perms, err)
	}

	// Un usuario del tenant (no owner) ve el documento por la entrada de tenant.
	subj := domain.Subject{Sub: "user-bob", Tenant: "acme"}
	docs, err := st.ListDocuments(ctx, subj, domain.ListFilter{})
	if err != nil || len(docs) != 1 || docs[0].ID != doc.ID {
		t.Fatalf("ListDocuments(compartido) => %+v (%v)", docs, err)
	}

	// Revocar el permiso: el usuario ajeno deja de verlo.
	if err := st.RevokePermission(ctx, doc.ID, domain.PrincipalTenant, "acme"); err != nil {
		t.Fatalf("RevokePermission: %v", err)
	}
	docs2, _ := st.ListDocuments(ctx, subj, domain.ListFilter{})
	if len(docs2) != 0 {
		t.Fatalf("tras revocar, el ajeno aún ve %d documentos", len(docs2))
	}
}

// TestTenantIsolationRLS verifica que las políticas RLS aíslan filas por tenant:
// un documento creado en el tenant "acme" no es visible ni operable desde "other".
func TestTenantIsolationRLS(t *testing.T) {
	st := newStore(t)

	doc, _, err := st.CreateDocument(tenantCtx("acme"), domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Privado", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}

	// Desde otro tenant, RLS oculta la fila => ErrNotFound.
	if _, err := st.GetDocument(tenantCtx("other"), doc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento RLS roto en GetDocument: %v", err)
	}
	// Y no puede secuenciar operaciones sobre él.
	if _, _, err := st.AppendOps(tenantCtx("other"), doc.ID, domain.Subject{Sub: "intruso"}, []domain.Op{
		insertOp(nil, "z", 1, "X", 1),
	}, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento RLS roto en AppendOps: %v", err)
	}
	// El tenant propietario sí lo ve.
	if _, err := st.GetDocument(tenantCtx("acme"), doc.ID); err != nil {
		t.Fatalf("GetDocument(propietario): %v", err)
	}
}

// Abrir dos veces el mismo fichero de Drive tiene que llevar al MISMO documento.
// Sin esto, cada «Abrir con Sytue Docs» dejaría otra copia con el mismo nombre y
// las ediciones acabarían repartidas entre todas sin que nadie lo note.
func TestDocumentoAncladoAlFicheroDeOrigen(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	const origen = "drive:11111111-1111-1111-1111-111111111111"

	primero, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Ventas.xlsx", OwnerSub: "user-ada", SourceRef: origen,
	})
	if err != nil || !creado {
		t.Fatalf("primera apertura: creado=%v err=%v", creado, err)
	}
	if primero.SourceRef != origen {
		t.Fatalf("SourceRef = %q", primero.SourceRef)
	}

	segundo, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Ventas.xlsx", OwnerSub: "user-ada", SourceRef: origen,
	})
	if err != nil {
		t.Fatalf("segunda apertura: %v", err)
	}
	if creado {
		t.Fatal("la segunda apertura creó otro documento en vez de devolver el suyo")
	}
	if segundo.ID != primero.ID {
		t.Fatalf("documentos distintos: %s vs %s", segundo.ID, primero.ID)
	}

	// Otra persona abriendo el MISMO fichero tiene el suyo: heredar el documento de
	// otro le daría acceso a lo que ese otro haya escrito encima.
	deOtro, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Ventas.xlsx", OwnerSub: "user-bob", SourceRef: origen,
	})
	if err != nil || !creado {
		t.Fatalf("apertura de otra persona: creado=%v err=%v", creado, err)
	}
	if deOtro.ID == primero.ID {
		t.Fatal("dos personas acabaron en el mismo documento")
	}

	// Y se encuentra por su origen, que es como se evita descargar el fichero para
	// descubrir que ya estaba abierto.
	encontrados, err := st.ListDocuments(ctx, domain.Subject{Sub: "user-ada", Tenant: "acme"},
		domain.ListFilter{SourceRef: origen})
	if err != nil {
		t.Fatalf("ListDocuments por origen: %v", err)
	}
	if len(encontrados) != 1 || encontrados[0].ID != primero.ID {
		t.Fatalf("buscar por origen devolvió %d documentos", len(encontrados))
	}

	// Sin origen (documento nacido aquí) no hay anclaje: dos creaciones son dos.
	a, _, _ := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Notas", OwnerSub: "user-ada",
	})
	b, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Notas", OwnerSub: "user-ada",
	})
	if err != nil || !creado || a.ID == b.ID {
		t.Fatalf("sin origen se dedujo un anclaje que no existe: %v", err)
	}
}

// El HTML del contenido se sanea al guardar, por la puerta de edición y como contenido
// inicial de un documento: lo que queda en el registro, lo que se difunde a los demás y lo
// que se lee después ya no lleva lo que ejecuta código. Lo legítimo se guarda como llegó.
func TestElContenidoSeSaneaAlGuardar(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	owner := domain.Subject{Sub: "owner"}
	set := func(n uint64, html string) domain.Op {
		v, _ := json.Marshal(html)
		return domain.Op{Kind: domain.OpSet, Key: "doc:html", Value: v, Dot: domain.Dot{Actor: "owner", Counter: n}, Lamport: n}
	}
	guardado := func(id string) string {
		t.Helper()
		estado, _, err := svc.LoadFull(ctx, owner, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		var html string
		_ = json.Unmarshal(estado.Registers["doc:html"].Value, &html)
		return html
	}
	const (
		legitimo = `<!--sytue-track--><p style="text-align: right;">Hola <strong>mundo</strong></p>`
		sucio    = `<p onclick="x()">Hola</p><script>x()</script>`
	)

	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Acta", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ApplyOps(ctx, owner, doc.ID, []domain.Op{set(1, legitimo)}, nil); err != nil {
		t.Fatal(err)
	}
	if got := guardado(doc.ID); got != legitimo {
		t.Errorf("lo legítimo no se guardó como llegó: %s", got)
	}
	difundido, _, err := svc.ApplyOps(ctx, owner, doc.ID, []domain.Op{set(2, sucio)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var html string
	_ = json.Unmarshal(difundido[0].Op.Value, &html)
	if html != `<p>Hola</p>` || guardado(doc.ID) != `<p>Hola</p>` {
		t.Errorf("al editar: se difunde %q y queda guardado %q", html, guardado(doc.ID))
	}

	otro, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Otra", OwnerSub: "owner", Seed: []domain.Op{set(1, sucio)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := guardado(otro.ID); got != `<p>Hola</p>` {
		t.Errorf("al crear con contenido: queda guardado %q", got)
	}
}
