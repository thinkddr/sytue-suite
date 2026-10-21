// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestImagenesProductosSeleccionYAislamiento(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	m := modelo(t, c, "producto")
	bien := crear(ctx, t, st, m, `{"nombre":"Café","tipo":"bien","precio":280}`)
	servicio := crear(ctx, t, st, m, `{"nombre":"Consulta","tipo":"servicio","precio":9000}`)
	sinFoto := crear(ctx, t, st, m, `{"nombre":"Sin foto","tipo":"bien","precio":100}`)
	otra := crear(tenantCtx("otra"), t, st, m, `{"nombre":"Privado","tipo":"bien","precio":300}`)
	contacto := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Contacto","tipo":"persona"}`)

	guardar := func(ctx context.Context, modelo, registro, tipo string) store.Adjunto {
		t.Helper()
		a, _, err := st.GuardarAdjunto(ctx, modelo, registro, "prueba", tipo, []byte("datos"), "ada")
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	guardar(ctx, "producto", bien.ID, "image/jpeg")
	ultima := guardar(ctx, "producto", bien.ID, "image/webp")
	// Un documento posterior no sustituye la foto, ni un adjunto de otro modelo.
	guardar(ctx, "producto", bien.ID, "application/pdf")
	guardar(ctx, "contacto", bien.ID, "image/png")
	fotoServicio := guardar(ctx, "producto", servicio.ID, "image/png")
	guardar(tenantCtx("otra"), "producto", otra.ID, "image/png")
	guardar(tenantCtx("otra"), "producto", bien.ID, "image/png")
	// Aunque haya adjuntos sueltos, solo salen productos vivos del tenant actual.
	huerfano := uuid.NewString()
	guardar(ctx, "producto", huerfano, "image/png")
	guardar(ctx, "producto", contacto.ID, "image/png")
	ids := []string{bien.ID, servicio.ID, sinFoto.ID, otra.ID, huerfano, contacto.ID}
	fotos, err := st.ImagenesProductos(ctx, ids)
	if err != nil || len(fotos) != 2 || fotos[bien.ID] != ultima.ID || fotos[servicio.ID] != fotoServicio.ID {
		t.Fatalf("solo la última imagen de bien y servicio propios: %v, %v", fotos, err)
	}
	if err := st.Borrar(ctx, "producto", bien.ID); err != nil {
		t.Fatal(err)
	}
	fotos, err = st.ImagenesProductos(ctx, ids)
	if err != nil || len(fotos) != 1 || fotos[servicio.ID] != fotoServicio.ID {
		t.Fatalf("un producto en la papelera deja de ofrecer foto: %v, %v", fotos, err)
	}
	fotos, err = st.ImagenesProductos(ctx, nil)
	if err != nil || fotos == nil || len(fotos) != 0 {
		t.Fatalf("el resultado vacío es un objeto JSON, no null: %v, %v", fotos, err)
	}
}
