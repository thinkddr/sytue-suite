// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package clientip

import "testing"

// TestTrustedProxiesClientIP fija la regla completa: la cabecera solo cuenta
// cuando quien conecta es un proxy de la lista, y dentro de ella se lee de
// derecha a izquierda. Lo que el cliente pueda inventar queda siempre a la
// izquierda de lo que añade el primer salto, así que nunca gana.
func TestTrustedProxiesClientIP(t *testing.T) {
	casos := []struct {
		nombre string
		tp     TrustedProxies
		remote string
		xff    string
		quiero string
	}{
		{
			"sin proxy delante, cabecera ignorada",
			DefaultTrustedProxies(), "198.51.100.77:5555", "1.2.3.4", "198.51.100.77",
		},
		{
			"sin cabecera, el peer",
			DefaultTrustedProxies(), "198.51.100.77:5555", "", "198.51.100.77",
		},
		{
			"tras el proxy, el cliente real",
			DefaultTrustedProxies(), "10.42.0.7:44321", "203.0.113.9", "203.0.113.9",
		},
		{
			"tras el proxy, la inyección del cliente se descarta",
			DefaultTrustedProxies(), "10.42.0.7:44321", "1.2.3.4, 203.0.113.9", "203.0.113.9",
		},
		{
			"varios saltos internos",
			DefaultTrustedProxies(), "10.42.0.7:44321", "203.0.113.9, 10.0.0.1, 172.16.4.4", "203.0.113.9",
		},
		{
			"cadena entera interna, el peer",
			DefaultTrustedProxies(), "10.42.0.7:44321", "10.0.0.1, 10.0.0.2", "10.42.0.7",
		},
		{
			"cabecera ilegible, el peer",
			DefaultTrustedProxies(), "10.42.0.7:44321", "no-es-una-ip", "10.42.0.7",
		},
		{
			"IPv6 tras el proxy",
			DefaultTrustedProxies(), "[::1]:44321", "2001:db8::5", "2001:db8::5",
		},
		{
			"none no cree a nadie",
			NoTrustedProxies(), "10.42.0.7:44321", "203.0.113.9", "10.42.0.7",
		},
		{
			"el valor cero se comporta como el defecto",
			TrustedProxies{},
			"10.42.0.7:44321", "203.0.113.9", "203.0.113.9",
		},
		{
			"peer ilegible, sin IP (fail-closed)",
			DefaultTrustedProxies(), "pipe", "203.0.113.9", "",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.tp.ClientIP(c.remote, c.xff); got != c.quiero {
				t.Errorf("ClientIP(%q, %q) = %q, quiero %q", c.remote, c.xff, got, c.quiero)
			}
		})
	}
}

// TestIdentifica: tras una cadena entera de proxies nuestros, la IP que queda es la de
// un proxy y no identifica a nadie (el túnel del piloto); una pública, sí.
func TestIdentifica(t *testing.T) {
	tp := DefaultTrustedProxies()
	for ip, quiero := range map[string]bool{
		"203.0.113.9": true, "2001:db8::5": true,
		"10.42.0.7": false, "127.0.0.1": false, "100.89.128.4": false, "::1": false, "": false, "pipe": false,
	} {
		if got := tp.Identifica(ip); got != quiero {
			t.Errorf("Identifica(%q) = %v, quiero %v", ip, got, quiero)
		}
	}
}

// TestParseTrustedProxies acepta CIDR y direcciones sueltas, y RECHAZA lo que no
// entiende en vez de quedarse con una lista a medias: una lista mal escrita que
// se cargara en silencio dejaría el servicio creyendo a menos proxies de los que
// el operador puso —o, peor, a otros—.
func TestParseTrustedProxies(t *testing.T) {
	tp, err := ParseTrustedProxies(" 192.0.2.0/24 , 203.0.113.7 ")
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}
	if got := tp.ClientIP("192.0.2.9:1", "198.51.100.4"); got != "198.51.100.4" {
		t.Errorf("el CIDR configurado no se cree: %q", got)
	}
	if got := tp.ClientIP("203.0.113.7:1", "198.51.100.4"); got != "198.51.100.4" {
		t.Errorf("la dirección suelta no se cree: %q", got)
	}
	// Una red privada NO está en la lista si el operador dio la suya.
	if got := tp.ClientIP("10.0.0.1:1", "198.51.100.4"); got != "10.0.0.1" {
		t.Errorf("se creyó a un proxy fuera de la lista: %q", got)
	}
	if _, err := ParseTrustedProxies("10.0.0.0/8, no-es-un-cidr"); err == nil {
		t.Error("una entrada ilegible debe abortar la carga, no ignorarse")
	}
	sinNadie, err := ParseTrustedProxies("none")
	if err != nil {
		t.Fatalf("ParseTrustedProxies(none): %v", err)
	}
	if got := sinNadie.ClientIP("10.0.0.1:1", "198.51.100.4"); got != "10.0.0.1" {
		t.Errorf("`none` debe ignorar la cabecera: %q", got)
	}
}
