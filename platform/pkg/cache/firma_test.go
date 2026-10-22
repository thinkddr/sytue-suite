// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cache

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func clienteFirmado(t *testing.T, dominio string) (*Client, *miniredis.Miniredis) {
	t.Helper()
	c, s := newTestClient(t)
	c.UsarFirma(ClaveDeFirma([]byte("secreto-maestro-de-prueba"), dominio))
	return c, s
}

// El caso que motiva todo el sello: un valor puesto A MANO en Dragonfly —que es
// lo que puede hacer cualquiera que alcance el puerto— no se acepta.
//
// Sin esto, `cloud:token:<hash>` es una credencial que emite quien quiera: el
// servicio deserializa el JSON y lo devuelve como identidad autenticada sin
// volver a preguntar a PostgreSQL.
func TestValorSinFirmarNoSeAcepta(t *testing.T) {
	c, _ := clienteFirmado(t, "cloud")
	ctx := context.Background()

	if err := c.SetTTL(ctx, "cloud:token:abc", `{"Tenant":"victima","Scope":"deploy"}`, time.Minute); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}
	if _, found, err := c.GetSigned(ctx, "cloud:token:abc"); err != nil || found {
		t.Fatalf("se aceptó un valor sin firmar: found=%v err=%v", found, err)
	}
}

// El sello cubre el NOMBRE DE LA CLAVE, no solo el valor.
//
// Si no lo cubriera, bastaría con copiar una entrada legítima a otra clave para
// que verificara: la resolución del token de uno bajo el hash del token de otro,
// o el derecho de una organización bajo el de la de al lado. Sería un fallo de
// aislamiento firmado y todo.
func TestElSelloNoSirveEnOtraClave(t *testing.T) {
	c, s := clienteFirmado(t, "cloud")
	ctx := context.Background()

	if err := c.SetSignedTTL(ctx, "cloud:token:mio", `{"Tenant":"mio"}`, time.Minute); err != nil {
		t.Fatalf("SetSignedTTL: %v", err)
	}
	// Se copia el valor sellado TAL CUAL a la clave de otro token.
	sellado, err := s.Get("cloud:token:mio")
	if err != nil {
		t.Fatalf("leyendo el valor sellado: %v", err)
	}
	if err := c.SetTTL(ctx, "cloud:token:ajeno", sellado, time.Minute); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}
	if _, found, _ := c.GetSigned(ctx, "cloud:token:ajeno"); found {
		t.Fatal("un valor sellado para otra clave verificó al copiarlo")
	}
	// Y en su sitio sigue valiendo, que es la otra mitad de la prueba.
	if v, found, _ := c.GetSigned(ctx, "cloud:token:mio"); !found || v != `{"Tenant":"mio"}` {
		t.Fatalf("el valor legítimo dejó de verificar: %q found=%v", v, found)
	}
}

func TestUnValorManipuladoSeRechaza(t *testing.T) {
	c, s := clienteFirmado(t, "cloud")
	ctx := context.Background()

	if err := c.SetSignedTTL(ctx, "k", "original", time.Minute); err != nil {
		t.Fatalf("SetSignedTTL: %v", err)
	}
	sellado, err := s.Get("k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Se conserva el sello y se cambia el valor.
	if err := c.SetTTL(ctx, "k", sellado[:len(sellado)-len("original")]+"alterado", time.Minute); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}
	if _, found, _ := c.GetSigned(ctx, "k"); found {
		t.Fatal("se aceptó un valor alterado bajo un sello viejo")
	}
}

// Sin clave NO se escribe, y es lo que hace segura una mala configuración: un
// despliegue al que se le olvidó el secreto va siempre a PostgreSQL en vez de
// cachear decisiones de autenticación que cualquiera puede reescribir.
func TestSinClaveNiSeEscribeNiSeLee(t *testing.T) {
	c, s := newTestClient(t) // sin UsarFirma
	ctx := context.Background()

	if c.FirmaActiva() {
		t.Fatal("dice tener firma sin habérsela dado")
	}
	if err := c.SetSignedTTL(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("SetSignedTTL: %v", err)
	}
	if s.Exists("k") {
		t.Fatal("escribió sin clave de firma")
	}
	if _, found, _ := c.GetSigned(ctx, "k"); found {
		t.Fatal("leyó sin clave de firma")
	}
}

// Cada servicio deriva su propia clave del mismo secreto: cloud no puede sellar
// lo que lee id, ni al revés. Si algún día uno de los dos se compromete, no
// arrastra al otro.
func TestCadaDominioTieneSuClave(t *testing.T) {
	maestra := []byte("secreto-maestro-de-prueba")
	if bytes.Equal(ClaveDeFirma(maestra, "cloud"), ClaveDeFirma(maestra, "id")) {
		t.Fatal("dos dominios comparten clave")
	}

	c, s := clienteFirmado(t, "id")
	ctx := context.Background()
	if err := c.SetSignedTTL(ctx, "id:entitled:org", "1", time.Minute); err != nil {
		t.Fatalf("SetSignedTTL: %v", err)
	}
	sellado, err := s.Get("id:entitled:org")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	otro, _ := newTestClient(t)
	otro.UsarFirma(ClaveDeFirma(maestra, "cloud"))
	if _, ok := comprobarSello(otro.firma, "id:entitled:org", sellado); ok {
		t.Fatal("la clave de cloud verificó un sello de id")
	}
}

// LA REGRESIÓN QUE COSTÓ EL INCIDENTE. Antes, si Dragonfly no contestaba al
// arrancar, New devolvía nil y el servicio se quedaba SIN CACHE PARA SIEMPRE:
// no había reconexión y solo un reinicio del pod la devolvía.
//
// Ahora el cliente vuelve igualmente —con el error al lado, para que quede
// registrado que se arrancó degradado— y la sonda lo recupera.
func TestConDragonflyCaidoDevuelveClienteUsable(t *testing.T) {
	// Puerto 1: nadie escucha, la conexión se rechaza en el acto.
	c, err := New(context.Background(), "redis://127.0.0.1:1")
	if err == nil {
		t.Fatal("no informó de que no había conexión")
	}
	if c == nil {
		t.Fatal("devolvió nil: el servicio se quedaría sin cache para siempre")
	}
	t.Cleanup(func() { _ = c.Close() })

	if c.Enabled() {
		t.Error("se declara utilizable con Dragonfly caído")
	}
	// Configurada distingue «no hay Dragonfly» de «Dragonfly está caído».
	if !c.Configurada() {
		t.Error("perdió la configuración: la sonda no tendría a qué reconectar")
	}
	// Y las operaciones no panican mientras tanto.
	if _, found, err := c.Get(context.Background(), "k"); found || err == nil {
		t.Errorf("Get con la cache caída: found=%v err=%v", found, err)
	}
}

// Una URL inválida sí es definitiva: esperar no la arregla, así que ahí sigue
// devolviéndose nil y el llamante se queda sin cache a propósito.
func TestUnaURLInvalidaSigueSiendoDefinitiva(t *testing.T) {
	c, err := New(context.Background(), "esto-no-es-una-url")
	if err == nil {
		t.Fatal("aceptó una URL inválida")
	}
	if c != nil {
		t.Fatal("devolvió cliente para una URL que nunca va a funcionar")
	}
	// Close tolera el nil que acaba de devolver.
	if cerr := c.Close(); cerr != nil {
		t.Errorf("Close sobre nil: %v", cerr)
	}
}

// Enabled deja de ser «está configurado» y pasa a ser «tiro por la cache AHORA»:
// con Dragonfly caído los llamantes se van a PostgreSQL sin pagar el viaje.
func TestEnabledSigueLaSalud(t *testing.T) {
	c, s := newTestClient(t)
	ctx := context.Background()
	if !c.Enabled() {
		t.Fatal("no se declara utilizable con Dragonfly en pie")
	}

	s.Close()
	// La primera operación fallida es la que lo detecta.
	_ = c.SetTTL(ctx, "k", "v", time.Minute)
	if c.Enabled() {
		t.Error("sigue declarándose utilizable después de fallar")
	}
	if !c.Configurada() {
		t.Error("perdió la configuración al caerse Dragonfly")
	}
}
