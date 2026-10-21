// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cache

import (
	"strings"
	"testing"
)

// EL PREFIJO IDENTIFICA AL CULPABLE SIN REGISTRAR DE QUIÉN ERA LA ENTRADA.
//
// Las claves llevan cosas como el identificador de una persona o el texto que
// alguien tecleó en un buscador, y esto acaba en el diario del servicio. Con los
// dos primeros tramos se sabe qué código se está pasando de tamaño y no se sabe
// nada de nadie.
func TestPrefijoDeRecortaAlEspacioDeNombres(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		"id:agenda:org-123:a1b2c3":   "id:agenda",
		"rooms:img:https://x/y.png":  "rooms:img",
		"hub:presencia:acme:usuario": "hub:presencia",
		"sindospuntos":               "sindospuntos",
		"uno:solo":                   "uno:solo",
	}
	for clave, quiero := range casos {
		if got := prefijoDe(clave); got != quiero {
			t.Errorf("prefijoDe(%q) = %q, quiero %q", clave, got, quiero)
		}
	}
}

// Y NO PUEDE FILTRAR EL RESTO. Es la mitad que importa: si algún día alguien
// «arregla» prefijoDe devolviendo la clave entera, el identificador de una
// persona acabaría en el log de producción sin que nada avise.
func TestPrefijoDeNoFiltraElRestoDeLaClave(t *testing.T) {
	t.Parallel()
	clave := "id:agenda:org-secreta:consulta-que-alguien-tecleo"
	got := prefijoDe(clave)
	for _, prohibido := range []string{"org-secreta", "consulta-que-alguien-tecleo"} {
		if strings.Contains(got, prohibido) {
			t.Errorf("el prefijo %q lleva %q", got, prohibido)
		}
	}
}

// El tope es el que dice la constante, y se comprueba que no se mueva sin
// querer: subirlo es decidir cuánto puede evictar una sola entrada.
func TestElTopeEsElDeclarado(t *testing.T) {
	t.Parallel()
	if MaxValorBytes != 256*1024 {
		t.Errorf("MaxValorBytes = %d; si se cambia, dilo en el commit", MaxValorBytes)
	}
}

// Con el cliente deshabilitado, guardar algo enorme no revienta ni avisa: no hay
// dónde guardarlo, así que no hay nada que limitar.
func TestSinClienteNoSeQuejaDelTamano(t *testing.T) {
	t.Parallel()
	var c Client // rdb == nil
	if err := c.SetTTL(t.Context(), "k", strings.Repeat("x", MaxValorBytes+1), 0); err != nil {
		t.Errorf("SetTTL sin cliente devolvió %v", err)
	}
}
