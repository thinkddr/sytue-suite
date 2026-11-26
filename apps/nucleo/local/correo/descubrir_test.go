// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// srvFalso contesta los SRV de una tabla.
type srvFalso map[string][]*net.SRV

func (f srvFalso) LookupSRV(_ context.Context, servicio, proto, nombre string) (string, []*net.SRV, error) {
	if ss, ok := f["_"+servicio+"._"+proto+"."+nombre]; ok {
		return "", ss, nil
	}
	return "", nil, errors.New("no existe")
}

// transporteFalso contesta las URL de una tabla y apunta lo pedido.
type transporteFalso struct {
	cuerpos map[string]string
	pedidas *[]string
}

func (t transporteFalso) RoundTrip(r *http.Request) (*http.Response, error) {
	*t.pedidas = append(*t.pedidas, r.URL.String())
	if c, ok := t.cuerpos[r.URL.Scheme+"://"+r.URL.Host+r.URL.Path]; ok {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(c)), Request: r}, nil
	}
	return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Request: r}, nil
}

const autoconfigEjemplo = `<?xml version="1.0"?>
<clientConfig version="1.1"><emailProvider id="ejemplo.eu">
<incomingServer type="pop3"><hostname>pop.ejemplo.eu</hostname><port>995</port><socketType>SSL</socketType></incomingServer>
<incomingServer type="imap"><hostname>imap.ejemplo.eu</hostname><port>143</port><socketType>plain</socketType></incomingServer>
<incomingServer type="imap"><hostname>IMAP.Ejemplo.eu</hostname><port>993</port><socketType>SSL</socketType></incomingServer>
<outgoingServer type="smtp"><hostname>smtp.ejemplo.eu</hostname><port>587</port><socketType>STARTTLS</socketType></outgoingServer>
</emailProvider></clientConfig>`

// El fichero del dominio manda; de él se queda el IMAP cifrado (nunca el «plain»).
func TestDescubrirPorAutoconfig(t *testing.T) {
	var pedidas []string
	d := Descubridor{HTTP: &http.Client{Transport: transporteFalso{cuerpos: map[string]string{
		"https://ejemplo.eu/.well-known/autoconfig/mail/config-v1.1.xml": autoconfigEjemplo,
	}, pedidas: &pedidas}}, Resolver: srvFalso{}}
	r, err := d.Descubrir(context.Background(), "ana@ejemplo.eu")
	if err != nil {
		t.Fatal(err)
	}
	if r.Origen != "autoconfig" || r.IMAP != (Servidor{Host: "imap.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS}) ||
		r.SMTP != (Servidor{Host: "smtp.ejemplo.eu", Puerto: 587, Seguridad: SeguridadSTARTTLS}) {
		t.Fatalf("%+v", r)
	}
	for _, p := range pedidas {
		if !strings.HasPrefix(p, "https://") {
			t.Errorf("pedido sin TLS: %s", p)
		}
	}
}

// Sin fichero, los SRV: primero los de TLS implícito; «.» es que no hay servicio.
func TestDescubrirPorSRV(t *testing.T) {
	res := srvFalso{
		"_imaps._tcp.casa.eu":       {{Target: ".", Port: 0}},
		"_imap._tcp.casa.eu":        {{Target: "correo.casa.eu.", Port: 143}},
		"_submissions._tcp.casa.eu": {{Target: "smtp.casa.eu.", Port: 465}},
	}
	r, err := Descubridor{Resolver: res}.Descubrir(context.Background(), "yo@casa.eu")
	if err != nil {
		t.Fatal(err)
	}
	if r.Origen != "srv" || r.IMAP != (Servidor{Host: "correo.casa.eu", Puerto: 143, Seguridad: SeguridadSTARTTLS}) ||
		r.SMTP != (Servidor{Host: "smtp.casa.eu", Puerto: 465, Seguridad: SeguridadTLS}) {
		t.Fatalf("%+v", r)
	}
	if _, err := (Descubridor{Resolver: srvFalso{}}).Descubrir(context.Background(), "yo@nada.eu"); !errors.Is(err, ErrSinDescubrir) {
		t.Fatalf("sin nada: %v", err)
	}
	if _, err := (Descubridor{Resolver: srvFalso{}}).Descubrir(context.Background(), "yo@[127.0.0.1]"); err == nil {
		t.Fatal("una dirección sin dominio se ha buscado")
	}
	// Sin el resolvedor del portero no se pregunta al DNS del sistema: no hay SRV.
	if _, err := (Descubridor{}).Descubrir(context.Background(), "yo@casa.eu"); !errors.Is(err, ErrSinDescubrir) {
		t.Fatalf("sin resolvedor: %v", err)
	}
}
