// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"context"
	"errors"
	"net"
	"testing"
)

// El permiso puntual (imágenes remotas, baja de lista): solo https, nunca a este equipo
// ni a la red de casa, nada con «Sin conexión»; y mientras dura la petición, Ajustes →
// Red la enseña como temporal con su motivo, y al acabar se cierra.
func TestPermisoPuntual(t *testing.T) {
	p := Abrir("")
	c := p.Puntual(ImagenRemota, "Imágenes de un correo")
	if _, err := c.Get("http://imagenes.ejemplo.eu/a.png"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("http: %v; quiero ErrDenegado", err)
	}
	if _, err := c.Get("https://127.0.0.1/a.png"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("este equipo: %v; quiero ErrDenegado", err)
	}
	var vistos []Destino
	p.marcarPublica = func(context.Context, string, string) (net.Conn, error) {
		vistos = p.Destinos()
		return nil, errors.New("sin red en la prueba")
	}
	_, _ = c.Get("https://imagenes.ejemplo.eu/a.png")
	if len(vistos) != 1 || vistos[0].Host != "imagenes.ejemplo.eu" || !vistos[0].Temporal || vistos[0].Proposito != ImagenRemota {
		t.Fatalf("durante la petición: %+v", vistos)
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Fatalf("después de la petición sigue abierto: %+v", ds)
	}
	// Y no vale para nada más: el permiso era de esa petición.
	if _, err := p.Cliente(ImagenRemota).Get("https://imagenes.ejemplo.eu/a.png"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("cliente normal: %v; quiero ErrDenegado", err)
	}
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	vistos = nil
	if _, err := c.Get("https://imagenes.ejemplo.eu/a.png"); !errors.Is(err, ErrDenegado) || vistos != nil {
		t.Fatalf("sin conexión: %v", err)
	}
	for ip, quiero := range map[string]bool{"8.8.8.8": true, "192.168.1.2": false, "10.0.0.1": false, "100.64.1.1": false, "::1": false, "fe80::1": false, "2a00:1450::1": true} {
		if Publica(net.ParseIP(ip)) != quiero {
			t.Errorf("Publica(%s) != %v", ip, quiero)
		}
	}
}

// VIES (comprobar un NIF-IVA desde el ERP) no lo abre ninguna configuración: solo la
// consulta que pide la persona, por el permiso puntual, y mientras dura.
func TestVIESSoloMientrasDuraLaConsulta(t *testing.T) {
	p := Abrir("")
	const vies = "https://ec.europa.eu/taxation_customs/vies/rest-api/check-vat-number"
	if _, err := p.Cliente(VIES).Post(vies, "application/json", nil); !errors.Is(err, ErrDenegado) {
		t.Fatalf("sin consulta, el cliente de VIES sale: %v", err)
	}
	var vistos []Destino
	p.marcarPublica = func(context.Context, string, string) (net.Conn, error) {
		vistos = p.Destinos()
		return nil, errors.New("sin red en la prueba")
	}
	_, _ = p.Puntual(VIES, "").Post(vies, "application/json", nil)
	if len(vistos) != 1 || vistos[0] != (Destino{Proposito: VIES, Host: "ec.europa.eu", Puerto: 443, Temporal: true}) {
		t.Fatalf("durante la consulta: %+v", vistos)
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Fatalf("acabada la consulta sigue abierto: %+v", ds)
	}
}
