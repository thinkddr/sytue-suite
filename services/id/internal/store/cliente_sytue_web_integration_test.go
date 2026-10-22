// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"io/fs"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/id/db"
)

// TestElClientDeSytueWebTrasLasMigraciones levanta la base con TODAS las migraciones
// —lo mismo que hace Sytue ID al arrancar— y mira cómo queda el client `sytue-pages`
// tras la 0104 (el host) y la 0105 (el prefijo): se llama «Sytue Web», tiene los
// callbacks de `web.sytue.eu` con `/web` y conserva los anteriores —`pages.sytue.eu`
// y el prefijo `/pages`—, que siguen sirviendo o redirigiendo.
//
// Es también la prueba de que el IdP arranca con ella: un SQL malo en una migración
// no falla al desplegar, falla al levantar (0094), y aquí `pgtest.New` no llega a
// devolver el store.
//
// Verlo en rojo: quita una URI de la 0104, o cambia su `WHERE NOT (u = ANY (…))` por
// un `||` a secas y la segunda pasada duplica las entradas.
func TestElClientDeSytueWebTrasLasMigraciones(t *testing.T) {
	st, ctx := newStore(t)

	comprobar := func(cuando string) {
		t.Helper()
		c, err := st.GetClient(ctx, "sytue-pages")
		if err != nil {
			t.Fatalf("%s: GetClient(sytue-pages): %v", cuando, err)
		}
		if c.Name != "Sytue Web" {
			t.Errorf("%s: el client se llama %q, quiero «Sytue Web»", cuando, c.Name)
		}
		for _, uri := range []string{
			// La canónica que anuncia el chart y la del host de organización (0105).
			"https://web.sytue.eu/web/auth/callback",
			"https://corporate.sytue.eu/web/auth/callback",
			// El host nuevo con el prefijo anterior, la raíz y dev (0104).
			"https://web.sytue.eu/pages/auth/callback",
			"https://web.sytue.eu/auth/callback",
			"https://web.sytue.localhost/auth/callback",
			// El anterior y el de organización NO se quitan (0102).
			"https://pages.sytue.eu/pages/auth/callback",
			"https://pages.sytue.eu/auth/callback",
			"https://corporate.sytue.eu/pages/auth/callback",
		} {
			if !c.ValidRedirectURI(uri) {
				t.Errorf("%s: falta el redirect_uri %s", cuando, uri)
			}
		}
		for _, uri := range []string{"https://web.sytue.eu/", "https://pages.sytue.eu/"} {
			if !slices.Contains(c.PostLogoutRedirectURIs, uri) {
				t.Errorf("%s: falta el post_logout_redirect_uri %s", cuando, uri)
			}
		}
		for nombre, lista := range map[string][]string{
			"redirect_uris": c.RedirectURIs, "post_logout_redirect_uris": c.PostLogoutRedirectURIs,
		} {
			if unicas := slices.Compact(slices.Sorted(slices.Values(lista))); len(unicas) != len(lista) {
				t.Errorf("%s: %s tiene entradas repetidas: %v", cuando, nombre, lista)
			}
		}
	}
	comprobar("tras migrar")

	// Repetirlas no cambia nada: es lo que le permite a un despliegue reaplicarlas.
	for _, fichero := range []string{"0104_sytue_web_nombre_y_host.up.sql", "0105_sytue_web_prefijo_web.up.sql"} {
		sql, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/"+fichero)
		if err != nil {
			t.Fatalf("leer %s: %v", fichero, err)
		}
		if _, err := st.DB().Exec(ctx, string(sql)); err != nil {
			t.Fatalf("repetir %s: %v", fichero, err)
		}
	}
	comprobar("tras repetir la 0104 y la 0105")
}
