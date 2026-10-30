// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log/slog"
	"net"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/auth"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// dnsFalso es el DNS de la prueba: solo registros TXT (claves DKIM y políticas DMARC).
type dnsFalso map[string][]string

func (d dnsFalso) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := d[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no TXT", Name: name, IsNotFound: true}
}

func (dnsFalso) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return nil, nil }
func (dnsFalso) LookupMX(context.Context, string) ([]*net.MX, error)        { return nil, nil }

// firmaDe firma `raw` con una clave recién creada de `dominio` y la publica en el DNS:
// lo que hace cualquiera con un dominio suyo.
func firmaDe(t *testing.T, dns dnsFalso, dominio string, raw []byte) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewSigner(dominio, "sel", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	if err != nil {
		t.Fatal(err)
	}
	dns["sel._domainkey."+dominio] = []string{"v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub)}
	firmado, err := s.Sign(raw)
	if err != nil {
		t.Fatal(err)
	}
	return firmado
}

// COM-05, de punta a punta en el receptor. Un correo firmado por `atacante.example` que
// dice venir de otro: la firma es válida, pero ni DMARC pasa ni el remitente cuenta como
// autenticado. Vale igual para el MX que para el webhook (allí SPF va apagado y DKIM era
// la única barrera).
func TestUnCorreoFirmadoPorOtroNoPasaPorElRemitente(t *testing.T) {
	dns := dnsFalso{
		"_dmarc.gmail.com": {"v=DMARC1; p=none; sp=quarantine"},
		"_dmarc.sytue.eu":  {"v=DMARC1; p=quarantine"},
	}
	in := &Inbound{resolver: dns, verifyDKIM: true, enforceDMARC: true, logger: slog.New(slog.DiscardHandler)}
	sobre := InboundMessage{MailFrom: "rebotes@atacante.example"}

	t.Run("el dominio propio va a Spam", func(t *testing.T) {
		raw := firmaDe(t, dns, "atacante.example", []byte("From: Direccion <direccion@corporate.sytue.eu>\r\n"+
			"To: ana@corporate.sytue.eu\r\nSubject: Cambio de cuenta\r\nDate: Mon, 05 Oct 2026 10:00:00 +0000\r\n"+
			"Message-Id: <1@x>\r\n\r\nNueva cuenta bancaria.\r\n"))
		_, dkim, dmarc := in.authenticate(t.Context(), sobre, bytes.NewReader(raw), "direccion@corporate.sytue.eu")
		if dkim != auth.ResultPass {
			t.Fatalf("dkim = %q: la firma del atacante sí es válida, y por eso no basta", dkim)
		}
		if dmarc.Result != auth.ResultFail || dmarc.Disposition != "quarantine" || dmarc.Alineado {
			t.Fatalf("dmarc = %+v, quiero fail/quarantine sin alinear", dmarc)
		}
	})

	t.Run("el REPLY de una invitación no llega a Calendar", func(t *testing.T) {
		llegan, _, puente := calendarioDePrueba(t)
		env := domain.Message{FromAddr: "luis@gmail.com"}

		falso := firmaDe(t, dns, "atacante.example", replyDeFuera())
		_, dkim, dmarc := in.authenticate(t.Context(), sobre, bytes.NewReader(falso), env.FromAddr)
		if dkim != auth.ResultPass || dmarc.Alineado {
			t.Fatalf("dkim=%q dmarc=%+v, quiero firma válida y remitente SIN autenticar", dkim, dmarc)
		}
		env.DKIMResult = dkim.ToDomain()
		puente.Pasar(t.Context(), "acme", env, falso, dmarc.Alineado)
		noLlega(t, llegan, "el REPLY firmado por otro dominio ha cambiado la respuesta de luis@gmail.com")

		bueno := firmaDe(t, dns, "gmail.com", replyDeFuera())
		_, _, dmarc = in.authenticate(t.Context(), InboundMessage{MailFrom: "luis@gmail.com"}, bytes.NewReader(bueno), env.FromAddr)
		if !dmarc.Alineado {
			t.Fatalf("dmarc = %+v: firmado por gmail.com, el remitente está autenticado", dmarc)
		}
		puente.Pasar(t.Context(), "acme", env, bueno, dmarc.Alineado)
		select {
		case <-llegan:
		case <-time.After(5 * time.Second):
			t.Fatal("el REPLY legítimo no ha llegado a Calendar")
		}
	})

	t.Run("dos cabeceras From no pasan por la primera", func(t *testing.T) {
		// El atacante pone primero su propio From (que alinea con su DKIM y su SPF)
		// y añade una segunda cabecera From suplantando a dirección@corporate.sytue.eu.
		raw := firmaDe(t, dns, "atacante.example", []byte(
			"From: Atacante <rebotes@atacante.example>\r\n"+
				"From: Direccion <direccion@corporate.sytue.eu>\r\n"+
				"To: ana@corporate.sytue.eu\r\nSubject: Cambio de cuenta\r\nDate: Mon, 05 Oct 2026 10:00:00 +0000\r\n"+
				"Message-Id: <2@x>\r\n\r\nNueva cuenta bancaria.\r\n"))
		env, err := mime.ParseSobre(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		_, dkim, dmarc := in.authenticate(t.Context(), sobre, bytes.NewReader(raw), env.FromAddr)
		if dkim != auth.ResultPass {
			t.Fatalf("dkim = %q: la firma del atacante sí es válida", dkim)
		}
		if dmarc.Result != auth.ResultFail || dmarc.Disposition != "quarantine" || dmarc.Alineado {
			t.Fatalf("dmarc con dos cabeceras From = %+v, quiero fail/quarantine sin alinear", dmarc)
		}
	})
}
