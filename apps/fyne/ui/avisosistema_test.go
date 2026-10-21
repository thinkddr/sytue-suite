// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSoloInterrumpeLoQueDebe son las cuatro reglas del aviso del sistema, y las dos
// últimas son las que la web no necesita: con trece aplicaciones abiertas y la misma
// barra en todas, sin la tercera un mensaje de Sytue Rooms sonaría trece veces; y sin la
// cuarta, un correo con Mail cerrado no lo avisaría nadie. La cuarta se ha visto en rojo
// quitando el `!` de `!abierta`.
func TestSoloInterrumpeLoQueDebe(t *testing.T) {
	casos := []struct {
		que    string
		propio string
		n      api.HubNotification
		ya     bool
		quiero bool
	}{
		{
			"el de mi producto y sin leer, sale", "mail",
			api.HubNotification{ID: "1", Source: "mail"},
			false, true,
		},
		{
			"el de OTRO producto, no: lo anuncia su aplicación", "mail",
			api.HubNotification{ID: "2", Source: "rooms"},
			false, false,
		},
		{
			"el ya leído, no", "mail",
			api.HubNotification{ID: "3", Source: "mail", Read: true},
			false, false,
		},
		{
			"el ya anunciado, no: llega por dos caminos", "mail",
			api.HubNotification{ID: "4", Source: "mail"},
			true, false,
		},
		{"sin origen, no", "mail", api.HubNotification{ID: "5"}, false, false},
		{
			"sin metadatos de aplicación, nada sale", "",
			api.HubNotification{ID: "6", Source: "mail"},
			false, false,
		},
		{
			"y un aviso sin origen con la app sin metadatos tampoco", "",
			api.HubNotification{ID: "7"},
			false, false,
		},
		// La regla 4: `mail` está abierta en estos casos y `drive` no.
		{
			"el Hub avisa de lo de una app CERRADA", productoHub,
			api.HubNotification{ID: "8", Source: "drive"},
			false, true,
		},
		{
			"pero no de lo de una abierta: lo avisa ella", productoHub,
			api.HubNotification{ID: "9", Source: "mail"},
			false, false,
		},
		{
			"una app que no es el Hub no avisa por la cerrada", "rooms",
			api.HubNotification{ID: "10", Source: "drive"},
			false, false,
		},
		{
			"ni el Hub repite lo ya leído de una cerrada", productoHub,
			api.HubNotification{ID: "11", Source: "drive", Read: true},
			false, false,
		},
	}
	abierta := func(origen string) bool { return origen == "mail" }
	for _, c := range casos {
		if got := avisoDelSistema(c.propio, c.n, c.ya, abierta); got != c.quiero {
			t.Errorf("%s: avisoDelSistema = %v, se esperaba %v", c.que, got, c.quiero)
		}
	}
}

// TestLaClaveDelProductoSaleDelIdentificador: `eu.sytue.mail` es `mail`, que es el
// `source` con el que el Hub etiqueta sus avisos.
func TestLaClaveDelProductoSaleDelIdentificador(t *testing.T) {
	casos := map[string]string{
		"eu.sytue.mail": "mail", "eu.sytue.one": "one", "eu.sytue.certipdf": "pdf",
		"eu.sytue.docus": "docs", "eu.sytue.calcus": "sheets", "eu.sytue.diapos": "slides",
		"": "", "mail": "mail",
	}
	for id, quiero := range casos {
		if got := productoDeID(id); got != quiero {
			t.Errorf("productoDeID(%q) = %q, se esperaba %q", id, got, quiero)
		}
		// Y de vuelta: el Hub pregunta por la app de un origen con su identificador.
		if strings.HasPrefix(id, "eu.sytue.") {
			if got := idDeProducto(quiero); got != id {
				t.Errorf("idDeProducto(%q) = %q, se esperaba %q", quiero, got, id)
			}
		}
	}
}

// TestCadaIconoCuentaLoSuyo: el Dock con trece apps no enseña trece veces el mismo total.
// Mail cuenta sus avisos sin leer, y el Hub —la bandeja de la suite— el total del
// servicio. Se ha visto en rojo devolviendo `sinLeer` para todas.
func TestCadaIconoCuentaLoSuyo(t *testing.T) {
	items := []api.HubNotification{
		{ID: "1", Source: "mail"},
		{ID: "2", Source: "mail", Read: true},
		{ID: "3", Source: "rooms"},
		{ID: "4", Source: "mail"},
	}
	for propio, quiero := range map[string]int{"mail": 2, "rooms": 1, "drive": 0, productoHub: 40} {
		if got := insigniaDe(propio, items, 40); got != quiero {
			t.Errorf("insigniaDe(%q) = %d; quiero %d", propio, got, quiero)
		}
	}
}
