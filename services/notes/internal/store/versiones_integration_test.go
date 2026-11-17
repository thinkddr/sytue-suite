// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Editar una nota deja cómo estaba antes; la misma persona escribiendo seguido no
// abre otra versión; otra persona sí; quien no puede escribirla no ve su historial; y
// lo que pasa de 50 se poda.
func TestHistorialDeVersionesDeUnaNota(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Casa", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Plan", Content: "<p>uno</p>"})
	if err != nil {
		t.Fatal(err)
	}
	editar := func(sub, texto string) {
		t.Helper()
		if _, err := st.UpdateNote(ctx, n.ID, sub, domain.NoteInput{Title: "Plan", Content: texto}); err != nil {
			t.Fatal(err)
		}
	}
	editar("ada", "<p>dos</p>")
	editar("ada", "<p>tres</p>") // misma tanda
	editar("bob", "<p>cuatro</p>")

	vs, err := st.Versiones(ctx, n.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("versiones = %d, quiere 2", len(vs))
	}
	if vs[0].ReplacedBy != "bob" || vs[1].ReplacedBy != "ada" {
		t.Fatalf("orden o autoría: %+v", vs)
	}
	if vs[0].Content != "" {
		t.Fatal("la lista no debe llevar el texto")
	}
	v, err := st.Version(ctx, n.ID, vs[0].ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "<p>tres</p>" {
		t.Fatalf("antes de bob había %q", v.Content)
	}
	v, err = st.Version(ctx, n.ID, vs[1].ID, "ada")
	if err != nil || v.Content != "<p>uno</p>" {
		t.Fatalf("la primera versión es %q (%v)", v.Content, err)
	}

	// Una nota privada de otra persona: su historial no existe para quien no la escribe.
	privado, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Mío", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateNote(ctx, privado.ID, "ada", domain.NoteInput{Title: "x", Content: "<p>a</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Versiones(ctx, p.ID, "eve"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("historial ajeno: %v", err)
	}

	// Poda: con 60 personas distintas editando, quedan 50.
	for i := range 60 {
		editar(string(rune('A'+i%26))+string(rune('a'+i/26)), "<p>v"+string(rune('a'+i%26))+string(rune('a'+i/26))+"</p>")
	}
	var total int
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM note_versions WHERE note_id = $1`, n.ID).Scan(&total)
	}); err != nil {
		t.Fatal(err)
	}
	if total != domain.MaxVersiones {
		t.Fatalf("tras la poda quedan %d, quiere %d", total, domain.MaxVersiones)
	}
}
