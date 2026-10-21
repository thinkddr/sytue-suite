// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// direcciones es el store de la prueba: de qué cuenta es cada dirección.
type direcciones map[string]uuid.UUID

func (d direcciones) LookupAddress(_ context.Context, addr string) (store.ResolvedAddress, error) {
	id, ok := d[strings.ToLower(addr)]
	if !ok {
		return store.ResolvedAddress{}, domain.ErrNoSuchUser
	}
	return store.ResolvedAddress{AccountID: id, Address: addr}, nil
}

func correoCon(cabecerasFrom ...string) []byte {
	var b strings.Builder
	for _, f := range cabecerasFrom {
		b.WriteString("From: " + f + "\r\n")
	}
	b.WriteString("To: cliente@fuera.example\r\nSubject: Factura\r\n\r\nhola\r\n")
	return []byte(b.String())
}

// COM-04. El remitente de SOBRE ya llega autorizado; lo que el cliente escribe en la
// cabecera `From:` —lo que se enseña y lo que firma el relé— tiene que ser una
// dirección de esa misma cuenta.
func TestElFromDeCabeceraTieneQueSerDeLaCuentaQueEnvia(t *testing.T) {
	ana, bea := uuid.New(), uuid.New()
	st := direcciones{
		"ana@corporate.sytue.eu":        ana,
		"ana.garcia@corporate.sytue.eu": ana, // su alias
		"bea@corporate.sytue.eu":        bea,
		"soporte@sytue.eu":              bea,
	}
	const sobre = "ana@corporate.sytue.eu"

	for _, c := range []struct {
		nombre string
		raw    []byte
		vale   bool
	}{
		{"su dirección", correoCon("Ana <ana@corporate.sytue.eu>"), true},
		{"su dirección, sin nombre y en otra caja", correoCon("ANA@Corporate.Sytue.eu"), true},
		{"un alias suyo", correoCon(`"Ana García" <ana.garcia@corporate.sytue.eu>`), true},
		{"el nombre visible en otro juego de caracteres", correoCon("=?windows-1252?Q?Ana_Garc=EDa?= <ana@corporate.sytue.eu>"), true},
		{"una compañera", correoCon("Bea <bea@corporate.sytue.eu>"), false},
		{"la plataforma", correoCon("Soporte de Sytue <soporte@sytue.eu>"), false},
		{"una dirección de fuera", correoCon("jefa@otra-empresa.example"), false},
		{"dos direcciones en la cabecera", correoCon("ana@corporate.sytue.eu, bea@corporate.sytue.eu"), false},
		{"dos cabeceras From", correoCon("ana@corporate.sytue.eu", "bea@corporate.sytue.eu"), false},
		{"sin cabecera From", correoCon(), false},
		{"una cabecera que no se puede leer", correoCon("bea@corporate.sytue.eu <ana@corporate.sytue.eu"), false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			err := exigirFromDeLaCuenta(t.Context(), st, c.raw, sobre, ana)
			if c.vale && err != nil {
				t.Fatalf("rechazado: %v", err)
			}
			if !c.vale && !errors.Is(err, domain.ErrFromNoAutorizado) {
				t.Fatalf("err = %v, quiero ErrFromNoAutorizado", err)
			}
		})
	}

	t.Run("un fallo del store no se confunde con un From ajeno", func(t *testing.T) {
		err := exigirFromDeLaCuenta(t.Context(), storeCaido{}, correoCon("bea@corporate.sytue.eu"), sobre, ana)
		if err == nil || errors.Is(err, domain.ErrFromNoAutorizado) {
			t.Fatalf("err = %v, quiero el error del store", err)
		}
	})
}

type storeCaido struct{}

func (storeCaido) LookupAddress(context.Context, string) (store.ResolvedAddress, error) {
	return store.ResolvedAddress{}, errors.New("postgres no contesta")
}
