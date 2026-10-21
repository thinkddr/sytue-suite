// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestPermisosTareasYDocumentos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	// Permisos: solo se guarda lo distinto del defecto, y los destinatarios de un
	// módulo cerrado son quien lo tiene puesto y los owner/admin que han entrado.
	if err := st.GuardarPermisos(ctx, "ana", "buzon@fuera.example", "Ana", map[string]string{"obligaciones": "lectura", "crm": "ninguno"}, nil, "jefa"); err != nil {
		t.Fatal(err)
	}
	// NEG-16: el correo que escribe la administración no es destino de nada; Ana
	// recibe avisos en el suyo cuando entra.
	if para, err := st.DestinatariosDe(ctx, "obligaciones", false); err != nil || len(para) != 0 {
		t.Fatalf("antes de que nadie entre, no se avisa a nadie: %+v %v", para, err)
	}
	if err := st.VerUsuario(ctx, "ana", "ana@acme.es", "Ana", []string{"member"}); err != nil {
		t.Fatal(err)
	}
	if err := st.VerUsuario(ctx, "jefa", "jefa@acme.es", "Jefa", []string{"owner"}); err != nil {
		t.Fatal(err)
	}
	if err := st.VerUsuario(ctx, "luis", "luis@acme.es", "Luis", []string{"member"}); err != nil {
		t.Fatal(err)
	}
	if p, _, err := st.PermisosDe(ctx, "ana"); err != nil || p["obligaciones"] != "lectura" || p["crm"] != "ninguno" {
		t.Fatalf("permisos de Ana: %v %v", p, err)
	}
	para, err := st.DestinatariosDe(ctx, "obligaciones", false)
	if err != nil || len(para) != 2 || para[0].Email != "ana@acme.es" || para[1].Email != "jefa@acme.es" {
		t.Fatalf("destinatarios de obligaciones: %+v %v", para, err)
	}
	if abiertos, _ := st.DestinatariosDe(ctx, "crm", true); len(abiertos) != 2 {
		t.Fatalf("un módulo abierto avisa a quien entró salvo que lo tenga cerrado (Ana): %+v", abiertos)
	}
	if personas, err := st.Personas(ctx); err != nil || len(personas) != 3 {
		t.Fatalf("personas: %+v %v", personas, err)
	}
	if otra, _, _ := st.PermisosDe(tenantCtx("otra"), "ana"); len(otra) != 0 {
		t.Fatal("los permisos son de cada organización")
	}

	// Varios registros de una vez: o todos o ninguno.
	contacto := modelo(t, c, "contacto")
	compra := modelo(t, c, "factura_compra")
	dataContacto, etiqueta, err := domain.Validar(contacto, []byte(`{"nombre":"Anthropic Ireland, Limited","tipo":"organizacion"}`))
	if err != nil {
		t.Fatal(err)
	}
	dataFactura, etiquetaFactura, err := domain.Validar(compra, []byte(`{"proveedor":"00000000-0000-4000-8000-000000000000","fecha":"2026-09-15","base":18000,"iva":0,"total":18000}`))
	if err != nil {
		t.Fatal(err)
	}
	roto := []store.Nuevo{
		{Modelo: contacto, Data: dataContacto, Etiqueta: etiqueta},
		{Modelo: compra, Data: dataFactura, Etiqueta: etiquetaFactura}, // relación a un id que no existe
	}
	if _, _, err := st.CrearVarios(ctx, roto, nil, "ana"); err == nil {
		t.Fatal("una factura con un proveedor inexistente no entra")
	}
	if pag, _ := st.Listar(ctx, store.Filtro{Modelo: "contacto", Texto: "Anthropic", Limite: 5}); len(pag.Registros) != 0 {
		t.Fatal("y el proveedor tampoco se queda suelto")
	}
	roto[1].Enlaces = map[string]int{"proveedor": 0}
	creados, tareas, err := st.CrearVarios(ctx, roto, []store.ActividadDe{{Indice: 1, Actividad: store.Actividad{
		Tipo: "subir_documento", Resumen: "Sube la factura en PDF", Vence: "2026-09-15", ResponsableSub: "ana",
	}}}, "ana")
	if err != nil || len(creados) != 2 || creados[1].Data["proveedor"] != creados[0].ID || len(tareas) != 1 || tareas[0].Registro != creados[1].ID {
		t.Fatalf("proveedor, factura y su tarea: %+v %+v %v", creados, tareas, err)
	}

	pendientes, err := st.Actividades(ctx, store.FiltroActividades{Estado: "pendiente", ResponsableSub: "ana", HastaVencimiento: "2026-09-30"})
	if err != nil || len(pendientes) != 1 {
		t.Fatalf("tareas de Ana: %+v %v", pendientes, err)
	}
	if todas, err := st.Actividades(ctx, store.FiltroActividades{}); err != nil || len(todas) != 1 {
		t.Fatalf("sin filtro de fecha no se rompe el cast: %+v %v", todas, err)
	}

	// Subir la factura ES la tarea: se cierra sola.
	pdf := []byte("%PDF-1.7\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF")
	adj, cerradas, err := st.GuardarAdjunto(ctx, "factura_compra", creados[1].ID, "anthropic.pdf", "application/pdf", pdf, "ana")
	if err != nil || cerradas != 1 || adj.Tamano != len(pdf) || len(adj.SHA256) != 64 {
		t.Fatalf("adjunto: %+v cerradas=%d %v", adj, cerradas, err)
	}
	if lista, _ := st.Adjuntos(ctx, "factura_compra", creados[1].ID); len(lista) != 1 {
		t.Fatalf("adjuntos del registro: %+v", lista)
	}
	if _, contenido, err := st.AdjuntoConContenido(ctx, adj.ID); err != nil || !bytes.Equal(contenido, pdf) {
		t.Fatalf("contenido: %v", err)
	}
	if _, _, err := st.AdjuntoConContenido(tenantCtx("otra"), adj.ID); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("otra organización no lee el documento: %v", err)
	}
	if hecha, err := st.Actividad(ctx, tareas[0].ID); err != nil || hecha.Estado != "hecha" {
		t.Fatalf("la tarea quedó hecha: %+v %v", hecha, err)
	}
	if _, err := st.CerrarActividad(ctx, tareas[0].ID, "cancelada"); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("una tarea hecha no se cancela: %v", err)
	}
	grande := make([]byte, store.MaxAdjunto+1)
	if _, _, err := st.GuardarAdjunto(ctx, "factura_compra", creados[1].ID, "x.pdf", "application/pdf", grande, "ana"); err == nil {
		t.Fatal("la tabla rechaza un documento de más de 10 MiB")
	}
	if err := st.BorrarAdjunto(ctx, adj.ID); err != nil {
		t.Fatal(err)
	}

	// Avisos: una vez por obligación y periodo.
	if ok, err := st.ReclamarAviso(ctx, "es_303", "2026-T3", "10d"); !ok || err != nil {
		t.Fatalf("primer aviso: %v %v", ok, err)
	}
	if ok, _ := st.ReclamarAviso(ctx, "es_303", "2026-T3", "10d"); ok {
		t.Fatal("el mismo aviso no se reclama dos veces")
	}
	if err := st.LiberarAviso(ctx, "es_303", "2026-T3", "10d"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.ReclamarAviso(ctx, "es_303", "2026-T3", "10d"); !ok {
		t.Fatal("liberado, se puede volver a mandar")
	}

	// Las organizaciones con perfil se leen desde fuera de cualquiera de ellas.
	for _, slug := range []string{"acme", "otra"} {
		if _, err := st.GuardarOrganizacion(tenantCtx(slug), store.Organizacion{Nombre: strings.ToUpper(slug), Pais: "ES", Forma: "sl"}, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if tenants, err := st.TenantsConPerfil(tenantCtx("")); err != nil || strings.Join(tenants, ",") != "acme,otra" {
		t.Fatalf("organizaciones con perfil: %v %v", tenants, err)
	}
}
