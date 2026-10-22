// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Cada enlace de aviso del Hub lo entiende el receptor de su app nativa. Los ejemplos
// son los de `platform/pkg/hubnotify/testdata/enlaces.json`, que su propio guardián ata
// al código que los escribe: si un emisor cambia la forma de un enlace, o una app deja
// de atenderla, esto se pone en rojo en vez de abrir la app en blanco.
//
// Cada ejemplo se prueba en las DOS formas que circulan: la que sale del código, con la
// organización detrás del prefijo (ADR 0056), y la transicional sin org, que siguen
// trayendo enlaces guardados antes de la ola. La que falla es la que rompe.
//
// Así estaban el 2026-09-24: Mail solo atendía «redactar» y un correo nuevo abría la
// bandeja; Rooms ignoraba el canal y el mensaje; Drive enseñaba «no encontrado».
func TestLosAvisosAbrenSuElementoEnLaAppNativa(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "platform", "pkg", "hubnotify", "testdata", "enlaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ejemplos []struct{ Tipo, Producto, URL string }
	if err := json.Unmarshal(b, &ejemplos); err != nil || len(ejemplos) == 0 {
		t.Fatalf("ejemplos: %v (%d)", err, len(ejemplos))
	}
	for _, e := range ejemplos {
		u, err := url.Parse(e.URL)
		if err != nil {
			t.Fatal(err)
		}
		sinOrg, conOrg := sinOrganizacion(u)
		if !conOrg {
			t.Errorf("%s: %q no lleva la organización tras el prefijo", e.Tipo, e.URL)
			continue
		}
		// La forma del código y la transicional sin org se entienden las dos.
		pruebaAviso(t, e.Tipo, e.Producto, u)
		pruebaAviso(t, e.Tipo+" (sin org)", e.Producto, sinOrg)
	}
}

// sinOrganizacion quita el segmento de organización de la ruta de un enlace de aviso
// (`/mail/<org>/` → `/mail/`) y devuelve también la URL sin él. Dice si lo había.
func sinOrganizacion(u *url.URL) (*url.URL, bool) {
	partes := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	if len(partes) < 3 || !esSlugDeOrg(partes[1]) {
		return nil, false
	}
	sin := *u
	sin.Path = "/" + partes[0] + "/" + partes[2]
	return &sin, true
}

// pruebaAviso abre el enlace como lo hace la barra (primero la app nativa) y mira que el
// receptor del producto lo entienda.
func pruebaAviso(t *testing.T, tipo, producto string, destino *url.URL) {
	t.Helper()
	a := &abridorDoble{}
	_ = AbrirURL(a, producto, destino.String())
	nativo, err := url.Parse(a.urls[0])
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	switch producto {
	case "mail":
		_, ok = LeerHilo(nativo)
		if !ok {
			_, ok = LeerBuzon(nativo)
		}
	case "calendar":
		_, ok = LeerEvento(nativo)
		ok = ok || LeerInvitaciones(nativo)
		if !ok {
			_, ok = LeerCalendario(nativo)
		}
	case "rooms":
		_, _, ok = LeerCanal(nativo)
		ok = ok || LeerDenuncias(nativo) || LeerInvitacionesRooms(nativo)
	case "drive":
		_, ok = LeerSeccion(nativo)
		if _, hay := nativo.Query()["shared"]; hay {
			_, ok = LeerCompartido(nativo)
		}
		if _, hay := nativo.Query()["request"]; hay {
			_, ok = LeerPeticionDrive(nativo)
		}
	case "notes":
		_, ok = LeerCuaderno(nativo)
		if !ok {
			_, ok = LeerNotaCompartida(nativo)
		}
	case "photos":
		_, ok = LeerAlbumCompartido(nativo)
		if !ok {
			_, ok = LeerAlbum(nativo)
		}
	case "erp":
		_, ok = LeerVista(nativo)
		if !ok {
			_, ok = LeerPeticionERP(nativo)
		}
	case "docs", "sheets", "slides":
		ok = IDDeEdicion(producto, nativo) != ""
	case "pdf":
		_, ok = LeerApertura(producto, nativo)
		ok = ok || IDDeEdicion(producto, nativo) != ""
	default:
		t.Errorf("%s: no hay receptor conocido para %q", tipo, producto)
		return
	}
	if !ok {
		t.Errorf("%s: la app de %q no entiende %v", tipo, producto, nativo)
	}
}
