// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package carddav

import "testing"

// Sin cache, listar libretas se comporta como antes de que la cache existiera.
//
// La degradación tiene que ser total: si el backend dependiera de Dragonfly —una
// réplica, sin persistencia, con evicción— una cache caída dejaría a todos los
// clientes CardDAV sin poder sincronizar sus contactos.
func TestSinCacheLasLibretasSeListanComoSiempre(t *testing.T) {
	b := &Backend{} // cache nil: el estado por defecto
	if b.cache.Enabled() {
		t.Fatal("un Backend sin cache la declara disponible")
	}
	if _, ok := b.libsCacheadas(t.Context(), "acme", "usuario"); ok {
		t.Error("sin cache dice haber encontrado algo")
	}
	// No-ops silenciosos, no panics: se llaman desde el camino de crear libretas.
	b.cachearLibs(t.Context(), "acme", "usuario", nil)
	b.invalidarLibs(t.Context(), "usuario")
}

// La clave separa por TENANT y por usuario: servir las libretas de una
// organización a otra sería un fallo de aislamiento, no una imprecisión.
func TestLaClaveDeLibretasSeparaTenantYUsuario(t *testing.T) {
	vistas := map[string]bool{}
	for _, c := range [][2]string{{"acme", "ana"}, {"acme", "beto"}, {"otra", "ana"}} {
		k := libKey(c[0], c[1])
		if vistas[k] {
			t.Fatalf("clave repetida para tenant=%s sub=%s: %q", c[0], c[1], k)
		}
		vistas[k] = true
	}
	if libKey("acme", "ana") != "carddav:libs:acme:ana" {
		t.Errorf("clave inesperada: %q", libKey("acme", "ana"))
	}
}

// Sin tenant NO se cachea: mejor perder el ahorro que construir una clave
// incompleta que dos organizaciones podrían compartir.
func TestSinTenantNoSeCacheanLibretas(t *testing.T) {
	b := &Backend{}
	if _, ok := b.libsCacheadas(t.Context(), "", "usuario"); ok {
		t.Error("se sirvió una entrada con el tenant vacío")
	}
}

// Un minuto: lo que se paga al equivocarse es que alguien no vea la libreta que
// acaba de crear.
func TestLasLibretasNoSeCacheanMasDeUnMinuto(t *testing.T) {
	if libTTL.Minutes() > 1 {
		t.Errorf("las libretas se cachean %v: demasiado para una creación reciente", libTTL)
	}
}
