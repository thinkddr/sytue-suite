// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesscrypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// TestSoloConSuToken es LA prueba de la promesa: lo cifrado con el token de una
// sesión no se abre con el de otra. Si esto dejara de cumplirse, existiría una
// forma de leer las IP ajenas y la garantía sería falsa.
func TestSoloConSuToken(t *testing.T) {
	const ip = "2.139.1.1"
	ct, err := Cifrar("token-de-la-sesion-A", ip)
	if err != nil {
		t.Fatalf("cifrar: %v", err)
	}

	claro, err := Descifrar("token-de-la-sesion-A", ct)
	if err != nil || claro != ip {
		t.Fatalf("con su propio token debería abrirse: %q, %v", claro, err)
	}

	if _, err := Descifrar("token-de-la-sesion-B", ct); !errors.Is(err, ErrNoDescifrable) {
		t.Fatalf("con OTRO token no debe abrirse, dio %v", err)
	}
	if _, err := Descifrar("", ct); !errors.Is(err, ErrNoDescifrable) {
		t.Fatalf("sin token no debe abrirse, dio %v", err)
	}
}

// TestElHashNoSirveParaAbrir es la otra mitad de la promesa. En la base de datos
// está el SHA-256 del token, no el token. Si el hash sirviera para descifrar,
// bastaría con leer la tabla para ver todas las IP y no habría ganado nada.
func TestElHashNoSirveParaAbrir(t *testing.T) {
	const token = "token-real-de-la-cookie"
	ct, err := Cifrar(token, "2.139.1.1")
	if err != nil {
		t.Fatal(err)
	}
	suma := sha256.Sum256([]byte(token))
	for _, loQueHayEnLaBD := range []string{
		hex.EncodeToString(suma[:]),
		string(suma[:]),
	} {
		if _, err := Descifrar(loQueHayEnLaBD, ct); !errors.Is(err, ErrNoDescifrable) {
			t.Fatalf("el hash almacenado NO puede abrir el texto cifrado, dio %v", err)
		}
	}
}

// TestNoDelataElContenido: dos cifrados del mismo valor tienen que salir
// distintos. Con un nonce fijo, filas iguales darían bytes iguales y se podría
// deducir «estas dos sesiones vienen del mismo sitio» sin descifrar nada.
func TestNoDelataElContenido(t *testing.T) {
	a, _ := Cifrar("t", "2.139.1.1")
	b, _ := Cifrar("t", "2.139.1.1")
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados del mismo valor no pueden coincidir")
	}
	if strings.Contains(string(a), "2.139") {
		t.Fatal("el texto cifrado contiene el valor original")
	}
}

// TestVacioNoDejaRastro: sin IP no se guarda nada. Cifrar la cadena vacía
// dejaría un bloque del tamaño de un dato real, y ver si una fila tiene bytes o
// no distinguiría «no había IP» de «hay una IP» sin descifrar.
func TestVacioNoDejaRastro(t *testing.T) {
	ct, err := Cifrar("t", "")
	if err != nil {
		t.Fatal(err)
	}
	if ct != nil {
		t.Fatalf("un valor vacío debe guardarse como nil, dio %d bytes", len(ct))
	}
	claro, err := Descifrar("t", nil)
	if err != nil || claro != "" {
		t.Fatalf("descifrar nil debe dar vacío sin error: %q, %v", claro, err)
	}
}

// TestManipulacionDetectada: GCM está autenticado, así que un byte cambiado en
// la base de datos se nota en vez de devolver basura que acabaría en pantalla.
func TestManipulacionDetectada(t *testing.T) {
	ct, _ := Cifrar("t", "2.139.1.1")
	ct[len(ct)-1] ^= 0x01
	if _, err := Descifrar("t", ct); !errors.Is(err, ErrNoDescifrable) {
		t.Fatalf("un texto manipulado debe rechazarse, dio %v", err)
	}
}
