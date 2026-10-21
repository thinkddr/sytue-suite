// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El catálogo de conectores se lee SIN organización, y esto lo fija.
//
// `app_templates` es de la PLATAFORMA: no tiene columna de organización ni
// política RLS, y quien lo pide es la consola global, que no trae ninguna. Cuando
// la consulta iba por `tenantTx` —que exige un tenant en el contexto— el endpoint
// respondía 500 con «tenancy: no hay tenant en el contexto», y la pantalla de
// conexiones se lo tragaba entero: el catálogo llegaba vacío y el botón «añadir
// proveedor» salía DESACTIVADO, sin decir por qué. Un error de servidor
// convertido en un botón gris, que es de los que nadie sabe atribuir.
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestListTemplatesSinOrganizacion(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}
	st := store.New(h.Pool, cipher)

	// Contexto PELADO, sin tenant: es exactamente el que llega desde la consola
	// de plataforma. Antes esto devolvía error en vez de filas.
	if _, err := st.ListTemplates(context.Background()); err != nil {
		t.Fatalf("ListTemplates sin organización: %v", err)
	}
}
