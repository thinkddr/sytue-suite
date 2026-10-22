// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Test INTERNO (package policy) de redirectPolicy: la función que decide, salto a
// salto, si el cliente de egress sigue una redirección.
//
// Se llama a la función DIRECTAMENTE, sin cliente ni red, y es deliberado: es la
// lección de platform/pkg/ssrfhttp/redirect_test.go. Un test que solo mire el
// resultado final sigue pasando con ESTA capa rota, porque la barrera SSRF ataja el
// salto por otro lado y el error se parece. Aquí no hay ninguna otra capa que pueda
// tapar el fallo.
//
// Qué se rompe si esto falla: la pasarela inyecta el Bearer del vault en CADA salto
// (transport.RoundTrip lo pone en cada RoundTrip, no una vez por petición), así que un
// redirect que salga del host original —o que baje de https a http— entrega la
// credencial de un cliente a un destino que no es el suyo. No es una fuga teórica: es
// el token con el que se lee el SaaS de esa organización.
package policy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// soloLoopback permite loopback (127.0.0.0/8, que es donde escucha httptest y de
// donde salen los literales de esta tabla) y deja el RESTO de la política tal como en
// producción. Relajar la barrera entera haría que estos tests pasaran también con la
// defensa rota, que es exactamente el test que no sirve para nada.
func soloLoopback(t *testing.T) {
	t.Helper()
	restore := ssrfhttp.SetIPPolicyForTest(func(ip net.IP) bool {
		if ip != nil && ip.IsLoopback() {
			return false // permitida: es el destino del test
		}
		return ssrfhttp.IsBlockedIP(ip) // el resto, como en producción
	})
	t.Cleanup(restore)
}

// salto arma los argumentos de CheckRedirect: la petición del salto siguiente y la
// cadena recorrida (via[0] es SIEMPRE la petición original, contra la que se compara).
func salto(t *testing.T, origen, destino string, saltosPrevios int) (*http.Request, []*http.Request) {
	t.Helper()
	mk := func(u string) *http.Request {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, http.NoBody)
		if err != nil {
			t.Fatalf("NewRequest(%q): %v", u, err)
		}
		return req
	}
	via := make([]*http.Request, 0, saltosPrevios)
	for i := 0; i < saltosPrevios; i++ {
		via = append(via, mk(origen))
	}
	return mk(destino), via
}

func TestRedirectPolicy(t *testing.T) {
	soloLoopback(t)

	casos := []struct {
		nombre   string
		origen   string
		destino  string
		previos  int
		internal bool
		quiero   string // substring del error; "" = debe permitirse
	}{{
		nombre: "mismo host sigue",
		// El caso simétrico NO es de adorno: una política que cortara TODOS los saltos
		// pasaría el resto de la tabla y rompería media integración (los proveedores
		// redirigen constantemente).
		origen: "http://127.0.0.1/a", destino: "http://127.0.0.1/b", previos: 1,
		quiero: "",
	}, {
		nombre: "cambio de host se corta",
		// 127.0.0.2 pasa la política de IP igual que 127.0.0.1: lo que corta aquí es la
		// regla de same-host, no la barrera SSRF. Si no cortara, el salto se llevaría el
		// Bearer de esta organización a otro destino.
		origen: "http://127.0.0.1/a", destino: "http://127.0.0.2/a", previos: 1,
		quiero: "cross-host",
	}, {
		nombre: "cambio de host se corta también en interno",
		// Internal omite las barreras SSRF (el destino es un servicio de la suite, en IP
		// privada) pero NO el same-host: es lo único que impide que un redirect saque de
		// la suite el token del client delegable de la organización.
		origen: "http://127.0.0.1/a", destino: "http://127.0.0.2/a", previos: 1, internal: true,
		quiero: "cross-host",
	}, {
		nombre: "bajar de https a http se corta",
		// Mismo host, pero el siguiente salto viaja en claro: el Bearer inyectado saldría
		// legible por la red. ssrfhttp.ValidateURL admite http (es un esquema válido), así
		// que esto no lo para nadie más.
		origen: "https://127.0.0.1/a", destino: "http://127.0.0.1/a", previos: 1,
		quiero: "https",
	}, {
		nombre: "subir de http a https sigue",
		// El sentido contrario es la redirección más común que existe: no se toca.
		origen: "http://127.0.0.1/a", destino: "https://127.0.0.1/a", previos: 1,
		quiero: "",
	}, {
		nombre: "tope de saltos",
		origen: "http://127.0.0.1/a", destino: "http://127.0.0.1/b", previos: 5,
		quiero: "demasiados redirects",
	}, {
		nombre: "destino privado se corta aunque no cambie de host",
		// La revalidación SSRF de cada salto sigue en la cadena: el clásico 302 hacia la
		// red interna no se salva por venir del mismo host.
		origen: "http://10.0.0.5/a", destino: "http://10.0.0.5/b", previos: 1,
		quiero: ssrfhttp.ErrBlocked.Error(),
	}, {
		nombre: "destino privado se permite en interno",
		// La contrapartida exacta de lo anterior, y el motivo de que Internal exista: los
		// servicios de la propia suite viven justo en esas IPs.
		origen: "http://10.0.0.5/a", destino: "http://10.0.0.5/b", previos: 1, internal: true,
		quiero: "",
	}}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req, via := salto(t, c.origen, c.destino, c.previos)
			err := redirectPolicy(5, c.internal, nil)(req, via)
			if c.quiero == "" {
				if err != nil {
					t.Fatalf("el salto debía permitirse; err = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("el salto %s → %s debía cortarse; no dio error", c.origen, c.destino)
			}
			if !strings.Contains(err.Error(), c.quiero) {
				t.Errorf("err = %v; se esperaba que mencionara %q", err, c.quiero)
			}
		})
	}
}

// TestRedirectPolicy_maxRedirectsSeAplicaAlPrimerSalto fija el borde del tope: con un
// máximo de 1, el PRIMER salto ya es uno de más. Un `>` en vez de `>=` daría un salto
// gratis, que es justo por donde se cuela un bucle de redirecciones.
func TestRedirectPolicy_maxRedirectsSeAplicaAlPrimerSalto(t *testing.T) {
	soloLoopback(t)
	req, via := salto(t, "http://127.0.0.1/a", "http://127.0.0.1/b", 1)
	err := redirectPolicy(1, false, nil)(req, via)
	if err == nil || !strings.Contains(err.Error(), "demasiados redirects") {
		t.Fatalf("err = %v; con maxRedirects=1 el primer salto ya sobra", err)
	}
}

// TestRedirectPolicy_esquemaNoHTTPSeCorta: un esquema que ninguna comprobación de IP
// atraparía. Se usa el MISMO host a propósito, para que no lo ataje antes la regla de
// same-host y quede aislada la revalidación de esquema (con `file:///…` el Hostname
// queda vacío y lo corta el same-host, que prueba otra cosa).
func TestRedirectPolicy_esquemaNoHTTPSeCorta(t *testing.T) {
	soloLoopback(t)
	req, via := salto(t, "http://127.0.0.1/a", "gopher://127.0.0.1/x", 1)
	if err := redirectPolicy(5, false, nil)(req, via); !errors.Is(err, ssrfhttp.ErrInvalidURL) {
		t.Fatalf("err = %v; se esperaba ErrInvalidURL", err)
	}
}
