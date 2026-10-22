// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// Sin cache, resolver un token se comporta como antes de que la cache existiera.
//
// Importa más aquí que en otros sitios: `ResolveAPIToken` decide si una petición
// entra o no. Si dependiera de Dragonfly —una réplica, sin persistencia, con
// evicción— una cache caída dejaría fuera a todos los clientes de la API.
func TestSinCacheLosTokensSeResuelvenComoSiempre(t *testing.T) {
	s := &Service{} // cache nil: el estado por defecto y el de cualquier dev
	if s.cache.Enabled() {
		t.Fatal("un Service sin cache la declara disponible")
	}
	if _, ok := s.tokenCacheado(t.Context(), "hash-cualquiera"); ok {
		t.Error("sin cache dice haber encontrado algo")
	}
	// Guardar e invalidar tienen que ser no-ops silenciosos, no panics: se llaman
	// desde los caminos de crear y revocar, que deben funcionar sin Dragonfly.
	s.cachearToken(t.Context(), "hash", store.ResolvedToken{})
	s.invalidarToken(t.Context(), uuid.New())
}

// La clave lleva el hash COMPLETO del token, no un prefijo.
//
// Con un prefijo, dos tokens distintos podrían caer en la misma entrada y un
// cliente hablaría en nombre de otra organización: no sería una cache lenta, sería
// un fallo de aislamiento. Y nunca el token en claro, que viaja a la cache y a sus
// registros.
func TestLaClaveUsaElHashCompleto(t *testing.T) {
	a := tokenCacheKey("aaaaaaaabbbbbbbbcccccccc")
	b := tokenCacheKey("aaaaaaaabbbbbbbbdddddddd")
	if a == b {
		t.Fatal("dos hashes distintos comparten clave")
	}
	if a != "cloud:token:aaaaaaaabbbbbbbbcccccccc" {
		t.Errorf("clave inesperada: %q", a)
	}
}

// El rastro id→hash y la entrada principal son claves DISTINTAS: si coincidieran,
// invalidar una borraría la otra a medias y quedaría un token resoluble sin su
// rastro, imposible de revocar hasta que venciera.
func TestElRastroInversoNoPisaLaEntradaPrincipal(t *testing.T) {
	id := uuid.New()
	if tokenIDKey(id) == tokenCacheKey(id.String()) {
		t.Fatal("el rastro inverso comparte clave con la entrada principal")
	}
}

// El TTL es de un minuto: es el retraso máximo con el que un token caducado deja
// de admitirse. Quien lo suba tiene que pasar por aquí y justificarlo.
func TestElTokenNoSeCacheaMasDeUnMinuto(t *testing.T) {
	if tokenTTL.Minutes() > 1 {
		t.Errorf("los tokens se cachean %v: es tiempo de acceso concedido de más", tokenTTL)
	}
}
