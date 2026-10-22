// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El barrido de cierre de sesión sobre los clients REALES (los que siembran las
// migraciones), que es donde se ve lo que ninguna prueba unitaria puede ver: a
// quién se le pide de verdad que cierre sesión en una instalación de Sytue.
package http

import (
	"io"
	"log/slog"
	"testing"
)

func TestBarridoSobreLosClientsSembrados(t *testing.T) {
	l := newLiveServer(t)
	// Server con el issuer de PRODUCCIÓN: el barrido se acota al dominio de la
	// suite del entorno, y el harness usa id.test.
	srv := NewServer(Config{Issuer: "https://id.sytue.eu"},
		slog.New(slog.NewTextHandler(io.Discard, nil)), l.st, nil, nil)

	uris := srv.suiteLogoutURIs(l.ctx)
	índice := make(map[string]bool, len(uris))
	for _, u := range uris {
		índice[u] = true
	}

	// LAS SUPERFICIES, EN UN SOLO SITIO. Antes eran tres listas escritas a mano
	// —los subdominios, los prefijos del host de organización— más una cuenta
	// exacta al final, y las cuatro cosas se desincronizaban por separado: al
	// escribir esto, las listas no nombraban ni `notes` ni `photos` ni `account`
	// y lo único que cazaba el fallo era el número.
	//
	// Es la TERCERA vez que este test se queda atrás por una superficie nueva: ya
	// pasó con Sytue Notepads y con el barrido del host de organización. Con una
	// sola lista, dar de alta una aplicación es tocar una línea, y el fallo dice
	// qué sobra y qué falta en vez de un número contra otro.
	//
	// El valor dice si además vive bajo el host de ORGANIZACIÓN con su prefijo
	// (`corporate.sytue.eu/mail`). `platform` no tiene prefijo propio ahí.
	superficies := map[string]bool{
		"account": true, "admin": true, "bases": true, "calendar": true, "cast": true, "cloud": true, "pages": true,
		"contacts": true, "drive": true, "erp": true, "mail": true,
		"notes": true, "one": true, "photos": true, "rooms": true,
		// La ofimática, una app por producto desde el 2026-10-12 (migración 0111); el
		// client de la app conjunta, `sytue-docs`, lo borra la 0112.
		"docus": true, "calcus": true, "diapos": true, "certipdf": true,
		"platform": false,
	}

	// LA CUENTA ES EXACTA, y no un mínimo. El barrido es lo que cierra la sesión
	// en TODAS partes: una superficie que no esté conserva la sesión abierta
	// después de pulsar «salir», sin ningún error a la vista. Y sobrar es tan
	// sospechoso como faltar —cada entrada de más es una petición por cierre de
	// sesión—: el Hub es una API sin flujo de navegador y su callback responde
	// 404 (migración 0040), y `chat`, `meet` y `console` son hosts legados que
	// solo devuelven un 301 al canónico, que ya está en la lista (migración 0041).
	quiero := map[string]bool{
		// La forja cierra por el suyo, que impone Gitea.
		"https://git.sytue.eu/user/logout": true,
		// La cuenta global se sirve desde el propio IdP (migración 0097).
		"https://id.sytue.eu/account/auth/logout?scope=local": true,
		// `gio.sytue.eu` estuvo aquí hasta la baja de `sytue-gio` (migración 0096).
		// Sytue Web: su dirección es `web.sytue.eu/web` desde las migraciones 0104
		// (host) y 0105 (prefijo), y su clave sigue siendo `pages`, así que no sale
		// del bucle de abajo, que compone `<host>.sytue.eu/<host>`. Lo anterior SÍ
		// sale de ahí —`pages.` y el prefijo `/pages`— y se queda a propósito: el host
		// sigue sirviendo la app con su propia cookie de sesión, y bajo el prefijo
		// viejo el cierre llega por la redirección a `/web`.
		"https://web.sytue.eu/auth/logout?scope=local":           true,
		"https://web.sytue.eu/web/auth/logout?scope=local":       true,
		"https://web.sytue.eu/pages/auth/logout?scope=local":     true,
		"https://corporate.sytue.eu/web/auth/logout?scope=local": true,
	}
	for host, enOrg := range superficies {
		quiero["https://"+host+".sytue.eu/auth/logout?scope=local"] = true
		// La migración 0099 mantiene el callback raíz y añade el que lleva
		// prefijo en el host propio: el mismo build atiende ambas formas durante
		// la transición al segmento de organización. Las dos cookies host-only
		// tienen que caer al cerrar sesión.
		if host != "account" && host != "platform" {
			quiero["https://"+host+".sytue.eu/"+host+"/auth/logout?scope=local"] = true
		}
		if enOrg {
			quiero["https://corporate.sytue.eu/"+host+"/auth/logout?scope=local"] = true
		}
	}

	for u := range quiero {
		if !índice[u] {
			t.Errorf("el barrido NO alcanza a %s: esa sesión se queda abierta al salir", u)
		}
	}
	for _, u := range uris {
		if !quiero[u] {
			t.Errorf("el barrido pide de más a %s: una petición por cada cierre de sesión", u)
		}
	}
}
