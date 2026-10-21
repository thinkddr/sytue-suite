// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import "testing"

// Sin cache, listar colecciones se comporta como antes de que la cache existiera.
//
// Aquí la degradación tiene que ser total: si el backend dependiera de Dragonfly
// —una réplica, sin persistencia, con evicción— una cache caída dejaría a todos los
// clientes CalDAV sin poder sincronizar.
func TestSinCacheLasColeccionesSeListanComoSiempre(t *testing.T) {
	b := &Backend{} // cache nil: el estado por defecto
	if b.cache.Enabled() {
		t.Fatal("un Backend sin cache la declara disponible")
	}
	if _, ok := b.colsCacheadas(t.Context(), "acme", "usuario"); ok {
		t.Error("sin cache dice haber encontrado algo")
	}
	// Guardar e invalidar son no-ops silenciosos, no panics: se llaman desde el
	// camino de crear colecciones, que debe funcionar sin Dragonfly.
	b.cachearCols(t.Context(), "acme", "usuario", nil)
	b.invalidarCols(t.Context(), "usuario")
}

// La clave separa por TENANT y por usuario.
//
// El tenant no es decorativo: servir la lista de calendarios de una organización a
// otra no sería una cache imprecisa, sería un fallo de aislamiento. Y sin el
// usuario, todos los de una misma organización compartirían calendarios.
func TestLaClaveSeparaTenantYUsuario(t *testing.T) {
	casos := [][2]string{
		{"acme", "ana"},
		{"acme", "beto"},
		{"otra", "ana"},
	}
	vistas := map[string]bool{}
	for _, c := range casos {
		k := colKey(c[0], c[1])
		if vistas[k] {
			t.Fatalf("clave repetida para tenant=%s sub=%s: %q", c[0], c[1], k)
		}
		vistas[k] = true
	}
	if colKey("acme", "ana") != "caldav:cols:acme:ana" {
		t.Errorf("clave inesperada: %q", colKey("acme", "ana"))
	}
}

// Sin tenant en el contexto NO se cachea. Es preferible perder el ahorro a
// construir una clave incompleta que dos organizaciones podrían compartir.
func TestSinTenantNoSeCachea(t *testing.T) {
	b := &Backend{}
	if _, ok := b.colsCacheadas(t.Context(), "", "usuario"); ok {
		t.Error("se sirvió una entrada con el tenant vacío")
	}
}

// Un minuto: el precio de equivocarse es que un cliente no vea una colección
// recién creada, y eso se percibe como que la aplicación no funciona.
func TestLasColeccionesNoSeCacheanMasDeUnMinuto(t *testing.T) {
	if colTTL.Minutes() > 1 {
		t.Errorf("las colecciones se cachean %v: demasiado para un MKCALENDAR reciente", colTTL)
	}
}
