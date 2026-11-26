// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
)

// dnsDePrueba contesta como un DNS y apunta lo que le preguntan y lo que el portero
// enseñaba mientras.
type dnsDePrueba struct {
	p         *Portero
	preguntas []string
	durante   []Destino
}

func (d *dnsDePrueba) LookupSRV(_ context.Context, servicio, proto, nombre string) (string, []*net.SRV, error) {
	d.preguntas = append(d.preguntas, "SRV _"+servicio+"._"+proto+"."+nombre)
	d.durante = d.p.Destinos()
	return "", []*net.SRV{{Target: "imap.casa.eu.", Port: 993}}, nil
}

func (d *dnsDePrueba) LookupTXT(_ context.Context, nombre string) ([]string, error) {
	d.preguntas = append(d.preguntas, "TXT "+nombre)
	return []string{"path=/dav"}, nil
}

// Las consultas DNS del alta de una cuenta pasan por el portero: con «Sin conexión» no
// se pregunta nada, ni por algo que no es un dominio; la prueba de arranque ve cada una;
// y mientras dura, Ajustes → Red la enseña como temporal (con el puerto del DNS: no abre
// el dominio a nada más), y al acabar se cierra.
//
// VISTO EN ROJO (2026-11-25): sin mirar SinConexion en abrir, la consulta llegó al DNS.
func TestElDNSDelAltaPasaPorElPortero(t *testing.T) {
	p := Abrir("")
	d := &dnsDePrueba{p: p}
	p.dns = d
	var intentos []Intento
	t.Cleanup(VigilarDePrueba(func(i Intento) { intentos = append(intentos, i) }))
	r := p.Resolvedor(Descubrimiento, "casa.eu")

	_, ss, err := r.LookupSRV(t.Context(), "imaps", "tcp", "casa.eu")
	if err != nil || len(ss) != 1 || ss[0].Target != "imap.casa.eu." {
		t.Fatal("SRV:", ss, err)
	}
	if txt, err := r.LookupTXT(t.Context(), "_caldavs._tcp.casa.eu"); err != nil || !slices.Equal(txt, []string{"path=/dav"}) {
		t.Fatal("TXT:", txt, err)
	}
	if len(d.durante) != 1 || d.durante[0] != (Destino{Proposito: Descubrimiento, Host: "casa.eu", Puerto: 53, Motivo: "casa.eu", Temporal: true}) {
		t.Errorf("durante la consulta: %+v", d.durante)
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Errorf("después sigue abierto: %+v", ds)
	}
	if !slices.Equal(intentos, []Intento{{Descubrimiento, "dns", "_imaps._tcp.casa.eu"}, {Descubrimiento, "dns", "_caldavs._tcp.casa.eu"}}) {
		t.Errorf("la prueba de arranque vio %+v", intentos)
	}

	// Lo que no es un dominio no se consulta.
	for _, nombre := range []string{"", "localhost", "127.0.0.1", "casa", "a/b.eu", "[::1]"} {
		if _, _, err := r.LookupSRV(t.Context(), "imaps", "tcp", nombre); !errors.Is(err, ErrDenegado) {
			t.Errorf("%q: %v; quiero ErrDenegado", nombre, err)
		}
	}
	// «Sin conexión»: nada.
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	d.preguntas = nil
	if _, _, err := r.LookupSRV(t.Context(), "imaps", "tcp", "casa.eu"); !errors.Is(err, ErrDenegado) {
		t.Errorf("SRV sin conexión: %v; quiero ErrDenegado", err)
	}
	if _, err := r.LookupTXT(t.Context(), "casa.eu"); !errors.Is(err, ErrDenegado) {
		t.Errorf("TXT sin conexión: %v; quiero ErrDenegado", err)
	}
	if len(d.preguntas) != 0 {
		t.Errorf("con «Sin conexión» se preguntó %v", d.preguntas)
	}
}
