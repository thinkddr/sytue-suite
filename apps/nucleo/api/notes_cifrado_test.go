// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"encoding/json"
	"errors"
	"testing"
)

// vectorWeb lo ha producido el cifrado de la WEB (crypto.ts, con WebCrypto) ejecutado con
// bun el 2026-09-23: los parámetros de un cuaderno recién cifrado y una nota empaquetada.
// Si este cliente no lo abre, un cuaderno cifrado en el navegador no se lee en nativo.
const vectorWeb = `{"pass":"contraseña de prueba 1","params":{"v":1,"kdf":"PBKDF2-SHA256","salt":"ySDRH6iwgFSPiv3PQb2vkw==","iterations":600000,"check":"encv1:wQ/ioeYD3cf2HeNQ81VF1gfmIXZ6FMc06+LdV0jVkaEY3LbhUcHACbla"},"cipher":"encv1:q+6xh5l/ZgPt0odIpYPdvDE5rjb0F+MjSbZb9Go0ov6pxuCf08IfRuFv657H/CfA1msg05uSLWKc5WRfjjqmN6c4ML309HeZq0GjsyzMvYRUPDEh"}`

// TestElCifradoDeLaWebSeAbreAqui: la contraseña buena abre el cuaderno y la nota; una
// mala da `ErrCifrado` sin intentar ninguna nota.
//
// VISTO EN ROJO (2026-09-23, en el frontal): derivando con SHA-1 en vez de SHA-256, la
// contraseña buena no pasa el testigo.
func TestElCifradoDeLaWebSeAbreAqui(t *testing.T) {
	var v struct {
		Pass   string          `json:"pass"`
		Params json.RawMessage `json:"params"`
		Cipher string          `json:"cipher"`
	}
	if err := json.Unmarshal([]byte(vectorWeb), &v); err != nil {
		t.Fatal(err)
	}
	c, ok := CryptoDe(v.Params)
	if !ok {
		t.Fatal("los parámetros de la web no se entienden")
	}
	if _, err := AbrirCifradoNotas(c, "otra"); !errors.Is(err, ErrCifrado) {
		t.Errorf("una contraseña mala da %v", err)
	}
	clave, err := AbrirCifradoNotas(c, v.Pass)
	if err != nil {
		t.Fatalf("la contraseña buena no abre: %v", err)
	}
	n, err := DesempaquetarNota(clave, v.Cipher)
	if err != nil || n.Title != "Diario" || n.Content != "<p>Hoy <b>llueve</b>.</p>" {
		t.Errorf("la nota de la web sale %+v, %v", n, err)
	}
}

// TestElCifradoDeAquiSeAbreConSuClave: la vuelta, con parámetros nuevos (y menos
// iteraciones para que el test no tarde: lo que se prueba es el formato).
func TestElCifradoDeAquiSeAbreConSuClave(t *testing.T) {
	c, clave, err := NuevoCifradoNotas("otra contraseña larga")
	if err != nil {
		t.Fatal(err)
	}
	if c.Iterations != IteracionesNotas || c.KDF != "PBKDF2-SHA256" || c.V != 1 {
		t.Errorf("los parámetros nuevos son %+v", c)
	}
	s, err := EmpaquetarNota(clave, NotaEnClaro{Title: "t", Content: "c"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := DesempaquetarNota(clave, s)
	if err != nil || n.Title != "t" || n.Content != "c" {
		t.Errorf("la vuelta da %+v, %v", n, err)
	}
	otra := make([]byte, 32)
	if _, err := DesempaquetarNota(otra, s); !errors.Is(err, ErrCifrado) {
		t.Errorf("con otra clave da %v", err)
	}
	// La clave recordada con Touch ID se comprueba sin derivar: la suya pasa, otra no
	// (la contraseña se cambió desde la web, y la guardada ya no vale).
	if err := ComprobarClaveNotas(c, clave); err != nil {
		t.Errorf("la clave del cuaderno no pasa su testigo: %v", err)
	}
	if err := ComprobarClaveNotas(c, otra); !errors.Is(err, ErrCifrado) {
		t.Errorf("otra clave pasa el testigo: %v", err)
	}
}
