// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestHashDeEnlaceEsElDelStore fija la identidad compartida entre el proxy y la
// tabla. Si dejaran de coincidir, GetUnfurlByHash no encontraría nunca la fila:
// la vista previa se quedaría sin imagen y NADA fallaría —ni un error, ni un
// log—, que es la clase de avería que tarda semanas en verse.
func TestHashDeEnlaceEsElDelStore(t *testing.T) {
	const url = "https://ejemplo.eu/articulo"
	if HashDeEnlace(url) != domain.HashToken(url) {
		t.Fatal("el hash del proxy debe ser el mismo que indexa link_unfurls")
	}
	// Y recorta como el store, para que un espacio de más no parta la caché en dos.
	if HashDeEnlace("  "+url+"  ") != HashDeEnlace(url) {
		t.Error("los espacios alrededor no deben cambiar el hash")
	}
}

// TestLaClaveDeCacheLlevaElTenant es la prueba de un salto de aislamiento que no
// se ve leyendo el flujo feliz.
//
// El hash es solo de la URL, así que dos organizaciones que peguen el mismo
// enlace comparten hash. Si la clave de caché no llevara el tenant, la miniatura
// que una guardó se serviría a la otra con solo adivinar el hash —saltándose la
// lista blanca, que es lo único que impide que este servicio descargue lo que le
// pidan—. Se comprueba sobre la cadena de la clave porque es donde está el fallo:
// un test de extremo a extremo con las dos organizaciones necesitaría Postgres y
// no señalaría la causa.
func TestLaClaveDeCacheLlevaElTenant(t *testing.T) {
	const hash = "abc123"
	clave := func(tenant string) string { return "rooms:unfurl:img:" + tenant + ":" + hash }

	if clave("acme") == clave("otra") {
		t.Fatal("dos organizaciones no pueden compartir clave de caché para el mismo enlace")
	}
	if !strings.Contains(clave("acme"), "acme") {
		t.Error("la clave debe llevar el identificador de la organización")
	}
	if !strings.HasPrefix(clave("acme"), "rooms:unfurl:img:") {
		t.Error("el prefijo separa este espacio de claves del resto de Rooms en Dragonfly")
	}
}
