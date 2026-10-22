// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

func minter(t *testing.T) *Minter {
	t.Helper()
	c, err := keycrypto.New([]byte("0123456789abcdef0123456789abcdef")) // 32 B
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}
	return NewMinter(c, time.Hour)
}

// TestTokenIdaYVuelta: el camino feliz. Un token acuñado para un fichero se abre y
// devuelve exactamente a quién representa.
func TestTokenIdaYVuelta(t *testing.T) {
	m := minter(t)
	id := uuid.New()
	tok, err := m.Mint(Claims{Tenant: "acme", Subject: "u1", Name: "Ana", FileID: id, Write: true})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	got, err := m.Verify(tok, id)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Tenant != "acme" || got.Subject != "u1" || !got.Write || got.Name != "Ana" {
		t.Fatalf("claims = %+v", got)
	}
}

// TestTokenNoSirveParaOtroFichero es el test que de verdad importa.
//
// El editor recibe un token por fichero y lo reenvía en la query string. Si valiera para
// cualquier otro, bastaría con abrir UN fichero al que sí se tiene acceso y cambiar el id
// de la URL para leer cualquier otro del almacén.
func TestTokenNoSirveParaOtroFichero(t *testing.T) {
	m := minter(t)
	mio, ajeno := uuid.New(), uuid.New()
	tok, err := m.Mint(Claims{Tenant: "acme", Subject: "u1", FileID: mio})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, err := m.Verify(tok, ajeno); err == nil {
		t.Fatal("un token de otro fichero se aceptó: cambiar el id de la URL bastaría")
	}
}

// TestTokenCaduca: sin vencimiento, una fuga valdría para siempre.
func TestTokenCaduca(t *testing.T) {
	m := minter(t)
	id := uuid.New()
	tok, err := m.Mint(Claims{Tenant: "acme", Subject: "u1", FileID: id})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	// El reloj del acuñador avanza más allá del TTL.
	m.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := m.Verify(tok, id); err == nil {
		t.Fatal("un token caducado se aceptó")
	}
}

// TestTokenRechazaLoManipulado: el token es AEAD, así que cualquier cambio rompe el tag.
// Y un token en claro —sin el prefijo del cifrado— no se acepta aunque el JSON sea válido.
func TestTokenRechazaLoManipulado(t *testing.T) {
	m := minter(t)
	id := uuid.New()
	tok, err := m.Mint(Claims{Tenant: "acme", Subject: "u1", FileID: id})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	// Un byte cambiado en el sellado.
	manipulado := "A" + tok[1:]
	if manipulado == tok {
		manipulado = "B" + tok[1:]
	}
	if _, err := m.Verify(manipulado, id); err == nil {
		t.Fatal("un token manipulado se aceptó")
	}

	for nombre, malo := range map[string]string{
		"vacío":        "",
		"no base64":    "!!!!",
		"otro cifrado": "ZW5jdjE6bm8tZXMtdW4tdG9rZW4", // base64 de algo que no descifra
	} {
		t.Run(nombre, func(t *testing.T) {
			if _, err := m.Verify(malo, id); err == nil {
				t.Fatalf("%q se aceptó", malo)
			}
		})
	}
}

// TestSinClaveNoSeAcuña: sin KEK configurada la capa WOPI debe quedarse APAGADA, no
// emitir tokens sin cifrar.
func TestSinClaveNoSeAcuna(t *testing.T) {
	m := NewMinter(nil, time.Hour)
	if m.Enabled() {
		t.Fatal("sin clave, Enabled() debe ser falso")
	}
	if _, err := m.Mint(Claims{Tenant: "acme", Subject: "u1", FileID: uuid.New()}); err == nil {
		t.Fatal("sin clave se acuñó un token")
	}
}
