// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package media

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTokens(t *testing.T) {
	ahora := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	f := New("clave-de-prueba")
	f.ahora = func() time.Time { return ahora }
	id, otro := uuid.New(), uuid.New()

	tok := f.Emitir(id, AlcanceVer)
	if err := f.Verificar(id, tok, RutaHLS); err != nil {
		t.Fatalf("recién emitido: %v", err)
	}
	if err := f.Verificar(otro, tok, RutaHLS); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("el token de un vídeo vale para otro")
	}
	// Otra clave, otra firma.
	if err := New("otra").Verificar(id, tok, RutaHLS); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("una clave distinta acepta la firma")
	}
	// Caducado: seis horas y un segundo después.
	f.ahora = func() time.Time { return ahora.Add(Caducidad + time.Second) }
	if err := f.Verificar(id, tok, RutaHLS); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("caducado y aceptado")
	}
	f.ahora = func() time.Time { return ahora }
	// Cambiar la caducidad sin refirmar no cuela.
	firma, _, _ := strings.Cut(tok, ".")
	if err := f.Verificar(id, firma+".9999999999", RutaHLS); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("una caducidad retocada pasa")
	}
	for _, malo := range []string{"", "sinpunto", ".123", "??.123", firma + ".abc"} {
		if err := f.Verificar(id, malo, RutaHLS); !errors.Is(err, ErrTokenInvalido) {
			t.Errorf("%q aceptado", malo)
		}
	}
	// El token de reproducción no abre el original, ni el del original las derivadas.
	if err := f.Verificar(id, tok, RutaOriginal); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("un token de reproducción abre el original")
	}
	orig := f.Emitir(id, AlcanceOriginal)
	if err := f.Verificar(id, orig, RutaOriginal); err != nil {
		t.Fatalf("token del original: %v", err)
	}
	if err := f.Verificar(id, orig, RutaWebM); !errors.Is(err, ErrTokenInvalido) {
		t.Fatal("el token del original abre las derivadas")
	}
	u := f.URL(id, "/"+RutaPoster)
	if !strings.HasPrefix(u, "/media/"+id.String()+"/") || !strings.HasSuffix(u, "/"+RutaPoster) || strings.Contains(u, "//") {
		t.Fatalf("URL: %s", u)
	}
	if New("") != nil {
		t.Fatal("sin clave debería ser nil")
	}
}
