// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"
)

// «Restaurar todo» saca de la papelera lo de quien llama, en una sentencia, y dice
// cuántas: lo de otra persona se queda donde estaba, y lo que no estaba en la papelera
// no cuenta.
func TestPapelera_restaurarTodoSoloLoPropio(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("a.jpg", "da", nil))
	b, _ := st.Ingerir(ctx, foto("b.jpg", "db", nil))
	// Una que no está en la papelera: no cuenta.
	_, _ = st.Ingerir(ctx, foto("c.jpg", "dc", nil))
	ajena := foto("d.jpg", "dd", nil)
	ajena.Duenyo = "u2"
	otra, _ := st.Ingerir(ctx, ajena)
	if err := st.ALaPapelera(ctx, a.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, b.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, otra.ID, "u2"); err != nil {
		t.Fatal(err)
	}
	n, err := st.RestaurarTodo(ctx, "u1")
	if err != nil || n != 2 {
		t.Fatalf("restauradas = %d (%v), quiero 2", n, err)
	}
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 0 {
		t.Fatalf("papelera de u1 tras restaurar todo: %v", lista)
	}
	if lista, _ := st.Papelera(ctx, "u2"); len(lista) != 1 || lista[0].ID != otra.ID {
		t.Fatalf("se tocó la papelera de otra persona: %v", lista)
	}
	if n, err := st.RestaurarTodo(ctx, "u1"); err != nil || n != 0 {
		t.Fatalf("con la papelera vacía: %d (%v), quiero 0", n, err)
	}
}
