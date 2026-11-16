// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// dnsTabla es un resolvedor DNS de mentira por tablas: lo que no está, no existe.
type dnsTabla struct {
	txt   map[string][]string
	cname map[string]string
	hosts map[string][]string
}

func (d dnsTabla) LookupTXT(_ context.Context, h string) ([]string, error) {
	if v, ok := d.txt[h]; ok {
		return v, nil
	}
	return nil, errors.New("NXDOMAIN")
}

func (d dnsTabla) LookupCNAME(_ context.Context, h string) (string, error) {
	if v, ok := d.cname[h]; ok {
		return v, nil
	}
	if _, ok := d.hosts[h]; ok {
		return h + ".", nil // como net.Resolver: sin CNAME devuelve el propio nombre
	}
	return "", errors.New("NXDOMAIN")
}

func (d dnsTabla) LookupHost(_ context.Context, h string) ([]string, error) {
	if v, ok := d.hosts[h]; ok {
		return v, nil
	}
	return nil, errors.New("NXDOMAIN")
}

// El «Comprobar ahora» existe para decir QUÉ falta. Cada motivo es un registro
// distinto que el cliente tiene que tocar, así que confundirlos manda a la gente a
// arreglar lo que ya está bien.
func TestCheckDomainDNSDaElMotivo(t *testing.T) {
	const host, token, ingress = "www.example.com", "sytue-verify=abc", "ingress.sytue.eu"
	reto := "_sytue-challenge." + host
	ingressIPs := map[string][]string{ingress: {"198.51.100.10"}}

	casos := []struct {
		nombre   string
		dns      dnsTabla
		token    string
		verified bool
		reason   domain.DomainCheckReason
		found    string
	}{
		{
			nombre: "TXT y CNAME correctos", token: token, verified: true, reason: domain.DomainCheckOK,
			dns: dnsTabla{
				txt:   map[string][]string{reto: {token}},
				cname: map[string]string{host: "Ingress.Sytue.EU."},
				hosts: ingressIPs,
			},
		},
		{
			nombre: "sin TXT", token: token, reason: domain.DomainCheckTXTMissing,
			dns: dnsTabla{cname: map[string]string{host: ingress + "."}, hosts: ingressIPs},
		},
		{
			nombre: "TXT con otro token", token: token, reason: domain.DomainCheckTXTMismatch,
			dns: dnsTabla{txt: map[string][]string{reto: {"sytue-verify=viejo"}}},
		},
		{
			nombre: "sin token emitido nunca verifica", token: "", reason: domain.DomainCheckTXTMissing,
			dns: dnsTabla{txt: map[string][]string{reto: {""}}},
		},
		{
			nombre: "TXT bien y CNAME a otro sitio: verifica, pero avisa", token: token,
			verified: true, reason: domain.DomainCheckCNAMEMismatch, found: "otro.hosting.example",
			dns: dnsTabla{
				txt:   map[string][]string{reto: {token}},
				cname: map[string]string{host: "otro.hosting.example."},
				hosts: map[string][]string{host: {"203.0.113.9"}, ingress: {"198.51.100.10"}},
			},
		},
		{
			nombre: "TXT bien y sin CNAME (no resuelve)", token: token,
			verified: true, reason: domain.DomainCheckCNAMEMismatch, found: "",
			dns: dnsTabla{txt: map[string][]string{reto: {token}}, hosts: ingressIPs},
		},
		{
			nombre: "ápex con A a la IP del ingress cuenta como enrutado", token: token,
			verified: true, reason: domain.DomainCheckOK,
			dns: dnsTabla{
				txt:   map[string][]string{reto: {`"` + token + `"`}},
				hosts: map[string][]string{host: {"198.51.100.10"}, ingress: {"198.51.100.10"}},
			},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := checkDomainDNS(context.Background(), c.dns, host, c.token, ingress)
			if got.Verified != c.verified || got.Reason != c.reason {
				t.Fatalf("= {verified:%v reason:%q}, quiero {verified:%v reason:%q}",
					got.Verified, got.Reason, c.verified, c.reason)
			}
			if c.reason == domain.DomainCheckCNAMEMismatch {
				if got.Expected != ingress || got.Found != c.found {
					t.Errorf("expected/found = %q/%q, quiero %q/%q", got.Expected, got.Found, ingress, c.found)
				}
			}
		})
	}
}

// El verificador de la API y el bucle del controller tienen que contestar lo
// mismo para el mismo DNS: si no, el panel diría «ok» y el dominio seguiría
// pendiente (o al revés).
func TestDNSDomainVerifierEsLaMismaComprobacion(t *testing.T) {
	dns := dnsTabla{txt: map[string][]string{"_sytue-challenge.a.example.com": {"tok"}}}
	v := &DNSDomainVerifier{resolver: dns}
	p, _ := newTestPlatform()
	p.resolver = dns
	for _, tok := range []string{"tok", "otro", ""} {
		a, _ := v.VerifyDomain(context.Background(), "a.example.com", tok, "")
		b, _ := p.VerifyDomain(context.Background(), "a.example.com", tok, "")
		if a != b {
			t.Errorf("token %q: API %+v ≠ controller %+v", tok, a, b)
		}
	}
}

// StubPlatform en dev autoverifica, pero deja ensayar cada motivo por hostname.
func TestStubPlatformVerifyDomain(t *testing.T) {
	ctx := context.Background()
	got, err := StubPlatform{}.VerifyDomain(ctx, "x.example.com", "t", "ingress")
	if err != nil || !got.Verified || got.Reason != domain.DomainCheckOK {
		t.Fatalf("dev sin configurar = %+v, %v; quiero verificado y ok", got, err)
	}
	sin := domain.DomainCheck{Reason: domain.DomainCheckTXTMissing}
	p := StubPlatform{DomainChecks: map[string]domain.DomainCheck{"x.example.com": sin}}
	if got, _ := p.VerifyDomain(ctx, "x.example.com", "t", "ingress"); got != sin {
		t.Errorf("con motivo fijado = %+v, quiero %+v", got, sin)
	}
}
