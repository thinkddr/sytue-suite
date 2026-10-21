// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios de la autenticación de remitente (SPF/DMARC) con un resolver
// DNS falso: lógica pura, sin red.
package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"testing"
)

// fakeResolver implementa Resolver a partir de mapas en memoria.
type fakeResolver struct {
	txt map[string][]string
	ip  map[string][]net.IPAddr
	mx  map[string][]*net.MX
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no TXT", Name: name, IsNotFound: true}
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if v, ok := f.ip[host]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no A", Name: host, IsNotFound: true}
}

func (f *fakeResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no MX", Name: name, IsNotFound: true}
}

func TestVerifySPF(t *testing.T) {
	ctx := context.Background()

	t.Run("ip4 pass", func(t *testing.T) {
		res := &fakeResolver{txt: map[string][]string{
			"sytue.eu": {"v=spf1 ip4:192.0.2.0/24 -all"},
		}}
		if got := VerifySPF(ctx, res, net.ParseIP("192.0.2.10"), "sytue.eu"); got != ResultPass {
			t.Fatalf("SPF = %q, quiero pass", got)
		}
	})

	t.Run("ip4 fail (-all)", func(t *testing.T) {
		res := &fakeResolver{txt: map[string][]string{
			"sytue.eu": {"v=spf1 ip4:192.0.2.0/24 -all"},
		}}
		if got := VerifySPF(ctx, res, net.ParseIP("198.51.100.1"), "sytue.eu"); got != ResultFail {
			t.Fatalf("SPF = %q, quiero fail", got)
		}
	})

	t.Run("softfail (~all)", func(t *testing.T) {
		res := &fakeResolver{txt: map[string][]string{
			"sytue.eu": {"v=spf1 ip4:192.0.2.0/24 ~all"},
		}}
		if got := VerifySPF(ctx, res, net.ParseIP("203.0.113.7"), "sytue.eu"); got != ResultSoftFail {
			t.Fatalf("SPF = %q, quiero softfail", got)
		}
	})

	t.Run("no record", func(t *testing.T) {
		res := &fakeResolver{}
		if got := VerifySPF(ctx, res, net.ParseIP("192.0.2.10"), "nosuchdomain.eu"); got != ResultNone {
			t.Fatalf("SPF = %q, quiero none", got)
		}
	})

	t.Run("include resuelve al dominio incluido", func(t *testing.T) {
		res := &fakeResolver{txt: map[string][]string{
			"sytue.eu":         {"v=spf1 include:_spf.provider.eu -all"},
			"_spf.provider.eu": {"v=spf1 ip4:192.0.2.0/24 -all"},
		}}
		if got := VerifySPF(ctx, res, net.ParseIP("192.0.2.10"), "sytue.eu"); got != ResultPass {
			t.Fatalf("SPF include = %q, quiero pass", got)
		}
	})

	t.Run("empty inputs -> none", func(t *testing.T) {
		if got := VerifySPF(ctx, &fakeResolver{}, nil, "sytue.eu"); got != ResultNone {
			t.Fatalf("SPF sin IP = %q, quiero none", got)
		}
	})
}

func TestVerifyDMARCAlignment(t *testing.T) {
	ctx := context.Background()
	res := &fakeResolver{txt: map[string][]string{
		"_dmarc.sytue.eu": {"v=DMARC1; p=reject; rua=mailto:dmarc@sytue.eu"},
	}}

	t.Run("spf alineado pasa", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "sytue.eu", "sytue.eu", ResultPass, nil, ResultNone)
		if d.Result != ResultPass || d.Disposition != "none" {
			t.Fatalf("DMARC = %+v, quiero pass/none", d)
		}
	})

	t.Run("dkim alineado pasa", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "sytue.eu", "otro.eu", ResultFail, []string{"sytue.eu"}, ResultPass)
		if d.Result != ResultPass {
			t.Fatalf("DMARC dkim = %+v, quiero pass", d)
		}
	})

	t.Run("desalineado usa política publicada (reject)", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "sytue.eu", "spoofer.eu", ResultFail, []string{"spoofer.eu"}, ResultFail)
		if d.Result != ResultFail || d.Disposition != "reject" {
			t.Fatalf("DMARC = %+v, quiero fail/reject", d)
		}
	})

	t.Run("sin política -> none", func(t *testing.T) {
		d := VerifyDMARC(ctx, &fakeResolver{}, "nopolicy.eu", "nopolicy.eu", ResultFail, nil, ResultFail)
		if d.Result != ResultNone {
			t.Fatalf("DMARC sin política = %+v, quiero none", d)
		}
	})
}

// firmante publica en el DNS falso la clave DKIM de `dominio` y devuelve quien
// firma con ella: lo que hace cualquiera con un dominio propio.
func firmante(t *testing.T, res *fakeResolver, dominio string) *Signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSigner(dominio, "sel", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	if err != nil {
		t.Fatal(err)
	}
	if res.txt == nil {
		res.txt = map[string][]string{}
	}
	res.txt["sel._domainkey."+dominio] = []string{"v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub)}
	return s
}

func correoDe(from string) []byte {
	return []byte("From: " + from + "\r\nTo: ana@corporate.sytue.eu\r\nSubject: Factura pendiente\r\n" +
		"Date: Mon, 05 Oct 2026 10:00:00 +0000\r\nMessage-Id: <1@x>\r\n\r\nPaga aqui.\r\n")
}

// EL HALLAZGO COM-05. Firmar con el dominio propio un correo que dice venir de otro
// da una firma DKIM VÁLIDA; lo que no da es alineación. Antes el llamante metía el
// dominio del From entre los «firmantes» en cuanto había una firma válida, y DMARC
// pasaba para cualquier remitente, también con `p=reject`.
func TestUnaFirmaDeOtroDominioNoAutenticaElFrom(t *testing.T) {
	ctx := context.Background()
	res := &fakeResolver{txt: map[string][]string{
		"_dmarc.victima.example": {"v=DMARC1; p=reject"},
		// El dominio propio: la política vive en el organizativo, y es la que manda
		// para `corporate.sytue.eu`, que no publica la suya.
		"_dmarc.sytue.eu": {"v=DMARC1; p=quarantine"},
	}}
	atacante := firmante(t, res, "atacante.example")

	for _, c := range []struct{ nombre, from, dominio, quiere string }{
		{"dominio ajeno con p=reject", "jefa@victima.example", "victima.example", "reject"},
		{"el dominio propio", "direccion@corporate.sytue.eu", "corporate.sytue.eu", "quarantine"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			firmado, err := atacante.Sign(correoDe(c.from))
			if err != nil {
				t.Fatal(err)
			}
			dkim, dominios, err := VerifyDKIM(ctx, res, firmado)
			if err != nil || dkim != ResultPass {
				t.Fatalf("la firma del atacante es válida para SU dominio: %q %v", dkim, err)
			}
			if len(dominios) != 1 || dominios[0] != "atacante.example" {
				t.Fatalf("firmantes = %v, quiero solo atacante.example", dominios)
			}
			// SPF también le pasa: manda desde su servidor con su dominio de sobre.
			d := VerifyDMARC(ctx, res, c.dominio, "atacante.example", ResultPass, dominios, dkim)
			if d.Result != ResultFail || d.Disposition != c.quiere || d.Alineado {
				t.Fatalf("DMARC = %+v, quiero fail/%s sin alinear", d, c.quiere)
			}
		})
	}

	t.Run("quien firma con el dominio del From sí pasa", func(t *testing.T) {
		firmado, err := firmante(t, res, "victima.example").Sign(correoDe("jefa@victima.example"))
		if err != nil {
			t.Fatal(err)
		}
		dkim, dominios, _ := VerifyDKIM(ctx, res, firmado)
		d := VerifyDMARC(ctx, res, "victima.example", "rebotes.proveedor.example", ResultPass, dominios, dkim)
		if d.Result != ResultPass || !d.Alineado {
			t.Fatalf("DMARC = %+v (firmantes %v), quiero pass alineado", d, dominios)
		}
	})

	t.Run("una firma rota no cuenta como firmante", func(t *testing.T) {
		firmado, err := firmante(t, res, "victima.example").Sign(correoDe("jefa@victima.example"))
		if err != nil {
			t.Fatal(err)
		}
		tocado := bytes.Replace(firmado, []byte("Paga aqui."), []byte("Paga alli."), 1)
		dkim, dominios, _ := VerifyDKIM(ctx, res, tocado)
		if dkim == ResultPass || len(dominios) != 0 {
			t.Fatalf("cuerpo alterado: dkim=%q firmantes=%v", dkim, dominios)
		}
	})
}

// La alineación relajada es por dominio ORGANIZATIVO, no «uno acaba en el otro»:
// con el sufijo a secas, `co.uk` alineaba con cualquier `algo.co.uk`.
func TestLaAlineacionEsPorDominioOrganizativo(t *testing.T) {
	for _, c := range []struct {
		from, otro string
		estricta   bool
		quiere     bool
	}{
		{"victima.example", "victima.example", true, true},
		{"victima.example", "correo.victima.example", false, true},
		{"correo.victima.example", "victima.example", false, true},
		{"a.victima.example", "b.victima.example", false, true},
		{"victima.example", "correo.victima.example", true, false},
		{"victima.co.uk", "co.uk", false, false},
		{"victima.co.uk", "otra.co.uk", false, false},
		{"a.github.io", "b.github.io", false, false},
		{"victima.example", "victima.example.atacante.example", false, false},
		{"VICTIMA.example.", "victima.EXAMPLE", true, true},
		{"victima.example", "", false, false},
	} {
		if got := aligned(c.from, c.otro, c.estricta); got != c.quiere {
			t.Errorf("aligned(%q, %q, estricta=%v) = %v, quiero %v", c.from, c.otro, c.estricta, got, c.quiere)
		}
	}
}

func TestLaPoliticaDMARCMandaSobreLaAlineacion(t *testing.T) {
	ctx := context.Background()
	res := &fakeResolver{txt: map[string][]string{
		"_dmarc.estricto.example": {"v=DMARC1; p=reject; adkim=s; aspf=s"},
		"_dmarc.matriz.example":   {"v=DMARC1; p=none; sp=reject"},
		"_dmarc.errata.example":   {"v=DMARC1; p=rejectt"},
	}}

	t.Run("adkim=s exige el dominio exacto", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "estricto.example", "x.example", ResultFail, []string{"correo.estricto.example"}, ResultPass)
		if d.Result != ResultFail || d.Disposition != "reject" {
			t.Fatalf("DMARC = %+v, quiero fail/reject", d)
		}
		d = VerifyDMARC(ctx, res, "estricto.example", "x.example", ResultFail, []string{"estricto.example"}, ResultPass)
		if d.Result != ResultPass {
			t.Fatalf("DMARC = %+v, quiero pass", d)
		}
	})

	t.Run("aspf=s exige el dominio de sobre exacto", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "estricto.example", "rebotes.estricto.example", ResultPass, nil, ResultNone)
		if d.Result != ResultFail {
			t.Fatalf("DMARC = %+v, quiero fail", d)
		}
	})

	t.Run("un subdominio sin registro hereda sp= del organizativo", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "nominas.matriz.example", "atacante.example", ResultPass, []string{"atacante.example"}, ResultPass)
		if d.Result != ResultFail || d.Disposition != "reject" {
			t.Fatalf("subdominio: DMARC = %+v, quiero fail/reject", d)
		}
		d = VerifyDMARC(ctx, res, "matriz.example", "atacante.example", ResultPass, []string{"atacante.example"}, ResultPass)
		if d.Result != ResultFail || d.Disposition != "none" {
			t.Fatalf("organizativo: DMARC = %+v, quiero fail/none (p=none)", d)
		}
	})

	t.Run("una política con errata no rechaza correo", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "errata.example", "otro.example", ResultFail, nil, ResultNone)
		if d.Disposition != "none" {
			t.Fatalf("DMARC = %+v, quiero disposición none", d)
		}
	})

	t.Run("sin política también se sabe si el From está autenticado", func(t *testing.T) {
		d := VerifyDMARC(ctx, res, "sinregistro.example", "sinregistro.example", ResultPass, nil, ResultNone)
		if d.Result != ResultNone || !d.Alineado {
			t.Fatalf("DMARC = %+v, quiero none y alineado", d)
		}
		d = VerifyDMARC(ctx, res, "sinregistro.example", "atacante.example", ResultPass, []string{"atacante.example"}, ResultPass)
		if d.Alineado {
			t.Fatalf("DMARC = %+v: firmar con otro dominio no autentica el From", d)
		}
	})
}

func TestResultToDomain(t *testing.T) {
	if ResultPass.ToDomain() != "pass" || ResultSoftFail.ToDomain() != "softfail" {
		t.Fatal("ToDomain mapea mal")
	}
}
