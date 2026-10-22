// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
)

// Los avisos de Docs que se reconstruyen: «compartido contigo» solo si la concesión
// sigue en pie y se dio a mano; un comentario al dueño y a quien ya escribía en el
// hilo y SIGUE teniendo permiso propio, nunca a quien lo escribe; una mención solo a
// quien la dirección es HOY suya y HOY puede abrir el documento, también de fuera. Y
// todo solo a quien sigue en la organización o tiene un permiso de los de fuera.
func TestAvisosHistoricosDeDocs(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	// Dentro de la ventana de 90 días pase lo que pase con el reloj, a las 08:00 UTC.
	t0 := time.Now().UTC().Truncate(24 * time.Hour).Add(-20*24*time.Hour + 8*time.Hour)

	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var doc, borrado string
		if err := tx.QueryRow(ctx, `INSERT INTO documents (tenant, kind, title, owner_sub) VALUES ('acme', 'spreadsheet', 'Cuentas', 'ana') RETURNING id::text`).Scan(&doc); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO documents (tenant, kind, title, owner_sub, deleted_at) VALUES ('acme', 'document', 'Borrado', 'ana', now()) RETURNING id::text`).Scan(&borrado); err != nil {
			return err
		}
		permiso := func(d, quien, extra string) error {
			_, err := tx.Exec(ctx, `INSERT INTO doc_permissions (document_id, tenant, principal_kind, principal_id, role, granted_by, created_at, bloqueado_desde, externo)
				VALUES ($1, 'acme', 'user', $2, 'commenter', 'ana', $3, CASE WHEN $4 = 'bloqueado' THEN now() END, $4 = 'externo')`, d, quien, t0, extra)
			return err
		}
		for _, p := range [][3]string{{doc, "bea", ""}, {doc, "dani@acme.es", ""}, {doc, "eva", "bloqueado"}, {borrado, "bea", ""}, {borrado, "gus", ""}} {
			if err := permiso(p[0], p[1], p[2]); err != nil {
				return err
			}
		}
		// Abierto a toda la organización: eso no dice que Carla, que escribió en el hilo
		// y ya no tiene permiso propio, sea de ella.
		if _, err := tx.Exec(ctx, `INSERT INTO doc_permissions (document_id, tenant, principal_kind, principal_id, role, granted_by)
			VALUES ($1, 'acme', 'tenant', 'acme', 'viewer', 'ana')`, doc); err != nil {
			return err
		}
		comentario := func(autor, padre, cuerpo string, cuando time.Time) (string, error) {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO doc_comments (document_id, tenant, parent_id, author_sub, body, created_at)
				VALUES ($1, 'acme', nullif($2, '')::uuid, $3, $4, $5) RETURNING id::text`, doc, padre, autor, cuerpo, cuando).Scan(&id)
			return id, err
		}
		// Carla escribió en el hilo pero ya no tiene acceso; Eva lo tiene bloqueado.
		raiz, err := comentario("bea", "", "¿Cuadra?", t0.Add(time.Hour))
		if err != nil {
			return err
		}
		for _, c := range []struct {
			autor string
			min   int
		}{{"carla", 2}, {"eva", 3}} {
			if _, err := comentario(c.autor, raiz, "ojo", t0.Add(time.Duration(c.min)*time.Hour)); err != nil {
				return err
			}
		}
		if _, err = comentario("ana", raiz, "Sí, @fer@acme.es lo revisa (o @nadie@acme.es, que no existe)", t0.Add(4*time.Hour)); err != nil {
			return err
		}
		// Un documento privado: Gus tiene permiso; Iván no; Eva lo tiene bloqueado;
		// nadie@x.es no es de nadie; y Hana se menciona a sí misma.
		var privado string
		if err := tx.QueryRow(ctx, `INSERT INTO documents (tenant, kind, title, owner_sub) VALUES ('acme', 'spreadsheet', 'Privado', 'ana') RETURNING id::text`).Scan(&privado); err != nil {
			return err
		}
		for _, p := range [][3]string{{privado, "gus", ""}, {privado, "eva", "bloqueado"}, {privado, "hana", ""}} {
			if err := permiso(p[0], p[1], p[2]); err != nil {
				return err
			}
		}
		// Iván entró por un enlace que después se revocó: ya no puede abrirlo.
		if _, err := tx.Exec(ctx, `
			WITH l AS (INSERT INTO doc_share_links (document_id, tenant, token_hash, created_by, revoked_at)
			           VALUES ($1, 'acme', 'h-ivan', 'ana', now()) RETURNING id)
			INSERT INTO doc_permissions (document_id, tenant, principal_kind, principal_id, role, granted_by, via_link)
			SELECT $1, 'acme', 'user', 'ivan', 'viewer', 'ana', id FROM l`, privado); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'hana', '@gus@acme.es @ivan@acme.es @eva@acme.es @nadie@x.es @hana@acme.es', $2)`,
			privado, t0.Add(5*time.Hour)); err != nil {
			return err
		}
		// La dueña mencionada en su documento: el aviso del comentario Y el de la
		// mención, como en vivo.
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'gus', 'mira @ana@acme.es', $2)`, privado, t0.Add(7*time.Hour)); err != nil {
			return err
		}
		// Gente de OTRA organización (ADR 0040): Zoe con el documento compartido, Vera
		// por el canje de un enlace vivo (ADR 0052), Yago con permiso en OTRO documento.
		// Xavi tiene permiso en el privado, no en el de toda acme.
		for _, p := range [][3]string{{privado, "zoe", "externo"}, {doc, "yago", "externo"}, {privado, "xavi", "externo"}} {
			if err := permiso(p[0], p[1], p[2]); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			WITH l AS (INSERT INTO doc_share_links (document_id, tenant, token_hash, created_by)
			           VALUES ($1, 'acme', 'h-vera', 'ana') RETURNING id)
			INSERT INTO doc_permissions (document_id, tenant, principal_kind, principal_id, role, granted_by, via_link, externo)
			SELECT $1, 'acme', 'user', 'vera', 'viewer', 'ana', id, true FROM l`, privado); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'hana', '@zoe@globex.es @yago@globex.es @vera@globex.es @xavi@globex.es', $2)`,
			privado, t0.Add(8*time.Hour)); err != nil {
			return err
		}
		// Y Xavi, de fuera y sin permiso propio, en el documento abierto a toda acme: esa
		// apertura es para la gente de acme, no para él.
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'bea', '@xavi@globex.es', $2)`, doc, t0.Add(9*time.Hour)); err != nil {
			return err
		}
		// Yago mencionado donde SÍ tiene permiso: ahí le llega (y en el privado, no).
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'bea', '@yago@globex.es', $2)`, doc, t0.Add(10*time.Hour)); err != nil {
			return err
		}
		// Rui entró por un enlace siendo de fuera y hoy es de acme: su fila sigue con
		// `externo`, y la mención le llega igual.
		if err := permiso(privado, "rui", "externo"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'hana', '@rui@acme.es', $2)`, privado, t0.Add(13*time.Hour)); err != nil {
			return err
		}
		// Exa se fue de acme y conserva su fila de permiso (no externa): ya no abre nada
		// de acme, así que ni «compartido», ni el aviso del hilo, ni la mención.
		if err := permiso(privado, "exa", ""); err != nil {
			return err
		}
		var raizExa string
		if err := tx.QueryRow(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'exa', '¿Y esto?', $2) RETURNING id::text`, privado, t0.Add(11*time.Hour)).Scan(&raizExa); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, parent_id, author_sub, body, created_at)
			VALUES ($1, 'acme', $2::uuid, 'hana', 'Pregunta a @exa@acme.es', $3)`, privado, raizExa, t0.Add(12*time.Hour)); err != nil {
			return err
		}
		// Un comentario de hace más de 90 días: fuera de la ventana, ni se lee.
		if _, err := tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'bea', 'viejo @fer@acme.es', now() - interval '100 days')`, doc); err != nil {
			return err
		}
		// Y una mención en un documento borrado, a alguien con permiso: nada.
		_, err = tx.Exec(ctx, `INSERT INTO doc_comments (document_id, tenant, author_sub, body, created_at)
			VALUES ($1, 'acme', 'ana', '@gus@acme.es', $2)`, borrado, t0.Add(6*time.Hour))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Un Sytue ID de mentira que se porta como el de verdad: `Emails` solo resuelve gente
	// de acme; `Subs` dice quién sigue en acme; `Candidatos` mira el directorio global y
	// solo contesta con uno de los candidatos dados que no sea de acme.
	miembros := map[string]string{"fer@acme.es": "fer", "gus@acme.es": "gus", "ivan@acme.es": "ivan", "eva@acme.es": "eva", "hana@acme.es": "hana", "ana@acme.es": "ana", "rui@acme.es": "rui"}
	deAcme := []string{"ana", "bea", "carla", "eva", "fer", "gus", "hana", "ivan", "rui"}
	global := map[string]string{"zoe@globex.es": "zoe", "yago@globex.es": "yago", "vera@globex.es": "vera", "xavi@globex.es": "xavi"}
	maps.Copy(global, miembros)
	var preguntados []string        // los candidatos que Docs llegó a dar
	var direccionesDeFuera []string // y las direcciones por las que preguntó así
	resolver := func(_ context.Context, c peticiones.ConsultaDirectorio) (peticiones.Resueltas, error) {
		r := peticiones.Resueltas{Miembros: map[string]string{}, ConPermiso: map[string]string{}}
		for _, e := range c.Emails {
			if p, ok := miembros[e]; ok {
				r.Miembros[e] = p
			}
		}
		for _, s := range c.Subs {
			if slices.Contains(deAcme, s) {
				r.Activos = append(r.Activos, s)
			}
		}
		for e, subs := range c.Candidatos {
			preguntados = append(preguntados, subs...)
			direccionesDeFuera = append(direccionesDeFuera, e)
			if p, ok := global[e]; ok && slices.Contains(subs, p) && !slices.Contains(deAcme, p) {
				r.ConPermiso[e] = p
			}
		}
		return r, nil
	}
	// Sin Sytue ID no se sabe quién sigue en acme: nada.
	if nada, err := AvisosHistoricos(ctx, h.Pool, "https://docs.sytue.eu", nil); err == nil || len(nada) != 0 {
		t.Fatalf("sin Sytue ID: %d avisos y %v; quiero ninguno y un error", len(nada), err)
	}
	// Si Sytue ID cae del todo, tampoco: no se puede comprobar a nadie.
	caido := func(context.Context, peticiones.ConsultaDirectorio) (peticiones.Resueltas, error) {
		return peticiones.Resueltas{}, errors.New("429")
	}
	if nada, err := AvisosHistoricos(ctx, h.Pool, "https://docs.sytue.eu", caido); err == nil || len(nada) != 0 {
		t.Fatalf("con Sytue ID caído: %d avisos y %v; quiero ninguno y un error", len(nada), err)
	}
	// Si solo falla la vuelta de los de fuera, lo demás se entrega, con el error.
	sinFuera := func(ctx context.Context, c peticiones.ConsultaDirectorio) (peticiones.Resueltas, error) {
		if len(c.Candidatos) > 0 {
			return peticiones.Resueltas{}, errors.New("429")
		}
		return resolver(ctx, c)
	}
	parcial, err := AvisosHistoricos(ctx, h.Pool, "https://docs.sytue.eu", sinFuera)
	if err == nil {
		t.Fatal("sin la vuelta de los de fuera, el error no llega")
	}
	for _, a := range parcial {
		if a.Type == "docs.mention" && slices.Contains([]string{"zoe", "vera", "xavi", "yago"}, a.Subject) {
			t.Fatalf("sin la vuelta de los de fuera salió una mención de fuera: %+v", a)
		}
	}
	preguntados, direccionesDeFuera = nil, nil
	avisos, err := AvisosHistoricos(ctx, h.Pool, "https://docs.sytue.eu", resolver)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(avisos))
	for _, a := range avisos {
		got = append(got, a.Subject+" "+a.Type+" "+a.Fecha.UTC().Format("15"))
		if a.Source != "sheets" {
			t.Errorf("una hoja de cálculo la abre Calcus: %+v", a)
		}
	}
	slices.Sort(got)
	quiero := []string{
		"ana docs.comment 09",  // el de Bea, al dueño
		"ana docs.comment 10",  // el de Carla
		"ana docs.comment 11",  // el de Eva
		"bea docs.comment 10",  // el de Carla, a quien abrió el hilo
		"bea docs.comment 11",  // el de Eva
		"bea docs.comment 12",  // la respuesta de Ana
		"bea docs.shared 08",   // lo compartido en pie
		"fer docs.mention 12",  // mencionado en un documento abierto a toda la organización
		"ana docs.comment 13",  // el de Hana en el privado, a la dueña
		"gus docs.mention 13",  // mencionado con permiso propio; Iván, Eva, nadie y Hana no
		"ana docs.comment 15",  // el de Gus, a la dueña…
		"ana docs.mention 15",  // …y además la mención, como en vivo
		"ana docs.comment 16",  // el de Hana a los de fuera, a la dueña
		"zoe docs.mention 16",  // de fuera, con el documento compartido
		"vera docs.mention 16", // de fuera, por el canje de un enlace; Yago no (su permiso es de otro)
		"xavi docs.mention 16", // de fuera, con permiso en ESTE
		"ana docs.comment 17",  // el de Bea a Xavi, a la dueña; a Xavi nada
		"zoe docs.shared 08",
		"yago docs.shared 08",
		"xavi docs.shared 08",
		"ana docs.comment 18",  // el de Bea a Yago, a la dueña…
		"yago docs.mention 18", // …y a Yago, que en ESTE sí tiene permiso
		"gus docs.shared 08",
		"hana docs.shared 08",
		"ana docs.comment 19", // el de Exa, a la dueña (Exa escribió cuando era de acme)
		"ana docs.comment 20", // la respuesta de Hana, a la dueña; a Exa nada
		"ana docs.comment 21", // el de Hana a Rui, a la dueña…
		"rui docs.mention 21", // …y a Rui, de acme con su fila `externo`
		"rui docs.shared 08",
	}
	slices.Sort(quiero)
	if !slices.Equal(got, quiero) {
		t.Fatalf("avisos\n%v\nquiero\n%v", got, quiero)
	}
	// A Sytue ID solo se le dan como candidatos quienes tienen un permiso vivo de los que
	// abren a los de fuera; nunca gente de acme ni quien se fue.
	for _, sub := range preguntados {
		if !slices.Contains([]string{"zoe", "yago", "vera", "xavi"}, sub) {
			t.Errorf("se le dio a Sytue ID como candidato a %q, que no tiene un permiso de los de fuera", sub)
		}
	}
	// Quien ya es de dentro y puede abrirlo no se pregunta por la vía de fuera.
	for _, d := range []string{"gus@acme.es", "ana@acme.es", "hana@acme.es"} {
		if slices.Contains(direccionesDeFuera, d) {
			t.Errorf("se preguntó por la vía de fuera por %s, que ya es de acme y puede abrirlo", d)
		}
	}
	if len(parcial) != len(avisos)-4 {
		t.Errorf("sin la vuelta de los de fuera se entregaron %d, quiero todos menos las 4 menciones de fuera (%d)", len(parcial), len(avisos)-4)
	}
}
