// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests OFFLINE: la verificación del TXT con un resolver de prueba (ni una
// consulta real) y la normalización de lo que llega de una persona o de un borde.
package dominio

import (
	"context"
	"errors"
	"net"
	"testing"
)

// fakeResolver responde lo que le digamos: los tres desenlaces reales de una
// verificación DNS (registro correcto, aún sin registro, registro equivocado)
// más el DNS que no responde.
type fakeResolver struct {
	txts []string
	err  error
	// asked guarda el nombre consultado: comprobamos que se pregunta por el
	// registro EXACTO que se le pidió publicar al cliente.
	asked string
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.asked = name
	return f.txts, f.err
}

func TestCheckDomainTXTVerificado(t *testing.T) {
	const token = "0123456789abcdef"
	f := &fakeResolver{txts: []string{"v=spf1 -all", RecordValue(token)}}
	got := CheckDomainTXT(context.Background(), f, "acme.com", token)
	if got.Status != DomainVerified {
		t.Fatalf("status=%q, esperado verified", got.Status)
	}
	if f.asked != "_sytue.acme.com" {
		t.Fatalf("se consultó %q; debe consultarse el registro exacto que se pidió publicar", f.asked)
	}
}

func TestCheckDomainTXTPendienteDePropagar(t *testing.T) {
	// NXDOMAIN: el registro todavía no existe (o no ha propagado).
	f := &fakeResolver{err: &net.DNSError{Err: "no such host", Name: "_sytue.acme.com", IsNotFound: true}}
	if got := CheckDomainTXT(context.Background(), f, "acme.com", "tok"); got.Status != DomainPending {
		t.Fatalf("status=%q, esperado pending", got.Status)
	}
	// NODATA: el nombre existe pero sin TXT.
	f2 := &fakeResolver{txts: []string{"  ", `""`}}
	if got := CheckDomainTXT(context.Background(), f2, "acme.com", "tok"); got.Status != DomainPending {
		t.Fatalf("status=%q con TXT vacíos, esperado pending", got.Status)
	}
}

func TestCheckDomainTXTRegistroEquivocado(t *testing.T) {
	f := &fakeResolver{txts: []string{`"sytue-verification=otro-valor"`}}
	got := CheckDomainTXT(context.Background(), f, "acme.com", "el-bueno")
	if got.Status != DomainMismatch {
		t.Fatalf("status=%q, esperado mismatch", got.Status)
	}
	if len(got.Found) != 1 || got.Found[0] != "sytue-verification=otro-valor" {
		// Se devuelve lo encontrado (sin comillas) para poder enseñárselo al usuario.
		t.Fatalf("found=%v", got.Found)
	}
}

func TestCheckDomainTXTDNSInaccesible(t *testing.T) {
	f := &fakeResolver{err: errors.New("i/o timeout")}
	if got := CheckDomainTXT(context.Background(), f, "acme.com", "tok"); got.Status != DomainUnreachable {
		t.Fatalf("status=%q, esperado unreachable (no es culpa del registro del cliente)", got.Status)
	}
}

func TestNewDomainTokenEsAleatorio(t *testing.T) {
	a, errA := NewDomainToken()
	b, errB := NewDomainToken()
	if errA != nil || errB != nil || len(a) != 32 || a == b {
		t.Fatalf("tokens %q y %q (%v, %v): se esperan 32 hex distintos", a, b, errA, errB)
	}
}

// Normalizar NO quita «www.»: para una web, el ápex y www son dos hosts (el
// alta de Pages crea los dos, y el `ask` de Caddy pregunta por cada uno).
func TestNormalizarConservaWWW(t *testing.T) {
	ok := map[string]string{
		"www.Acme.com":                "www.acme.com",
		"https://www.acme.com/":       "www.acme.com",
		"  acme.com.  ":               "acme.com",
		"https://acme.com./":          "acme.com",
		"tienda.acme.co.uk":           "tienda.acme.co.uk",
		"ana@acme.co.uk":              "acme.co.uk",
		"https://acme.com:443/ab":     "acme.com",
		"https://acme.com/ruta?x=a@b": "acme.com",
		"https://ana@acme.com/":       "acme.com",
		"xn--panadera-g2a.es":         "xn--panadera-g2a.es",
		"1und1.de":                    "1und1.de",
	}
	for in, want := range ok {
		got, err := Normalizar(in)
		if err != nil || got != want {
			t.Errorf("Normalizar(%q) = %q, %v; esperado %q", in, got, err, want)
		}
	}
}

// Una IP literal no es un dominio: el `ask` de Caddy no debe emitir un
// certificado para ella, y un alta con IP no tiene zona donde poner el TXT.
func TestNormalizarRechaza(t *testing.T) {
	for _, bad := range []string{
		"", ".", "acme", "-acme.com", "acme-.com", "acme .com", "acme..com", "acmé.com", "_sytue.acme.com",
		"1.2.3.4", "https://1.2.3.4/", "127.1", "0x7f.1", "[::1]", "[::1]:443", "::1", "acme.123",
	} {
		got, err := Normalizar(bad)
		if err == nil {
			t.Errorf("Normalizar(%q) debía fallar; devolvió %q", bad, got)
			continue
		}
		if !errors.Is(err, ErrNoValido) {
			t.Errorf("Normalizar(%q): %v; debe envolver ErrNoValido", bad, err)
		}
	}
}
