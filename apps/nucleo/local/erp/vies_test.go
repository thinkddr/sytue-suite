// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// viesFalso es VIES con las respuestas de la API REST de la Comisión: un NIF-IVA válido
// con nombre y dirección (en varias líneas, como los da), uno español válido sin ellos
// («---»), uno no válido y los errores de la propia API.
type viesFalso struct {
	mu        sync.Mutex
	pedidos   []map[string]string
	metodo    string
	tipo      string
	respuesta map[string]string // vatNumber → JSON
}

func (f *viesFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var q map[string]string
	_ = json.NewDecoder(r.Body).Decode(&q)
	f.mu.Lock()
	f.pedidos = append(f.pedidos, q)
	f.metodo, f.tipo = r.Method, r.Header.Get("Content-Type")
	f.mu.Unlock()
	cuerpo, ok := f.respuesta[q["countryCode"]+q["vatNumber"]]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		cuerpo = `{"actionSucceed":false,"errorWrappers":[{"error":"INVALID_INPUT"}]}`
	}
	if q["countryCode"] == "DE" {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(cuerpo))
}

func nuevoVIESFalso(t *testing.T) (*viesFalso, VIES) {
	t.Helper()
	f := &viesFalso{respuesta: map[string]string{
		"FR12345678901": `{"countryCode":"FR","vatNumber":"12345678901","requestDate":"2026-11-25T10:00:00.000Z","valid":true,` +
			`"name":"EXEMPLE SARL","address":"1 RUE DE L’EXEMPLE\n75002 PARIS","traderNameMatch":"NOT_PROCESSED"}`,
		"ESB12345678":   `{"countryCode":"ES","vatNumber":"B12345678","requestDate":"2026-11-25T10:00:00.000Z","valid":true,"name":"---","address":"---"}`,
		"EL123456789":   `{"countryCode":"EL","vatNumber":"123456789","requestDate":"2026-11-25T10:00:00.000Z","valid":false,"name":"---","address":"---"}`,
		"DE123456789":   `{"actionSucceed":false,"errorWrappers":[{"error":"MS_UNAVAILABLE","message":"Member State service unavailable"}]}`,
		"IT01234567890": `<html>mantenimiento</html>`,
	}}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	return f, VIES{HTTP: srv.Client(), URL: srv.URL + "/taxation_customs/vies/rest-api/check-vat-number"}
}

func TestVIESDiceSiEsValidoYAQuienEsta(t *testing.T) {
	ctx := context.Background()
	f, v := nuevoVIESFalso(t)

	c, err := v.Comprobar(ctx, "fr 12 345 678 901", "")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Valido || c.Pais != "FR" || c.Numero != "12345678901" || c.Nombre != "EXEMPLE SARL" ||
		c.Direccion != "1 RUE DE L’EXEMPLE, 75002 PARIS" || c.Fecha.IsZero() {
		t.Fatalf("válido con nombre y dirección: %+v", c)
	}
	if f.metodo != http.MethodPost || f.tipo != "application/json" {
		t.Fatalf("la API es un POST con JSON: %s %s", f.metodo, f.tipo)
	}
	// Solo sale el país y el número: nada del contacto.
	if len(f.pedidos[0]) != 2 || f.pedidos[0]["countryCode"] != "FR" || f.pedidos[0]["vatNumber"] != "12345678901" {
		t.Fatalf("lo que sale: %v", f.pedidos[0])
	}

	// Sin prefijo, el del país del contacto; «---» es que el país no los publica.
	c, err = v.Comprobar(ctx, "B-12345678", "es")
	if err != nil || !c.Valido || c.Pais != "ES" || c.Nombre != "" || c.Direccion != "" {
		t.Fatalf("español sin nombre ni dirección: %+v %v", c, err)
	}
	// Grecia es EL en VIES, y uno no válido no es un error.
	c, err = v.Comprobar(ctx, "123456789", "GR")
	if err != nil || c.Valido || c.Pais != "EL" {
		t.Fatalf("no válido: %+v %v", c, err)
	}
}

func TestVIESFallaConSuMotivo(t *testing.T) {
	ctx := context.Background()
	f, v := nuevoVIESFalso(t)
	for _, c := range []struct {
		nif, pais string
		quiero    error
	}{
		{"DE123456789", "", ErrVIESNoResponde}, // MS_UNAVAILABLE
		{"IT01234567890", "", ErrVIESNoResponde},
		{"PT999999999", "", ErrNoEsNIFIVA}, // INVALID_INPUT
	} {
		if _, err := v.Comprobar(ctx, c.nif, c.pais); !errors.Is(err, c.quiero) {
			t.Errorf("%s: %v; quiero %v", c.nif, err, c.quiero)
		}
	}
	antes := len(f.pedidos)
	// Lo que no es un NIF-IVA de la UE ni se pregunta.
	for _, c := range []struct{ nif, pais string }{
		{"B12345678", ""}, {"B12345678", "US"}, {"US123456789", ""}, {"ES1", ""}, {"ES1234567890123", ""}, {"ES B1234¿", ""},
	} {
		if _, err := v.Comprobar(ctx, c.nif, c.pais); !errors.Is(err, ErrNoEsNIFIVA) {
			t.Errorf("%q (%q): %v; quiero ErrNoEsNIFIVA", c.nif, c.pais, err)
		}
	}
	if len(f.pedidos) != antes {
		t.Fatalf("se preguntó a VIES por lo que no es un NIF-IVA: %v", f.pedidos[antes:])
	}
	// Sin cliente no sale nada: la red es del portero.
	if _, err := (VIES{URL: v.URL}).Comprobar(ctx, "ESB12345678", ""); err == nil || len(f.pedidos) != antes {
		t.Fatalf("sin cliente: %v", err)
	}
}

func TestPartirNIFIVA(t *testing.T) {
	for _, c := range []struct{ nif, pais, pre, num string }{
		{"ES B-12345678", "", "ES", "B12345678"},
		{"esb12345678", "FR", "ES", "B12345678"}, // el prefijo manda sobre el país
		{"B12345678", "ES", "ES", "B12345678"},
		{"X1234567L", " es ", "ES", "X1234567L"},
		{"ATU12345678", "", "AT", "U12345678"},
		{"123456789", "GR", "EL", "123456789"},
		{"EL123456789", "", "EL", "123456789"},
		{"XI 123 4567 89", "", "XI", "123456789"},
		{"NL123456789B01", "", "NL", "123456789B01"},
	} {
		pre, num, err := PartirNIFIVA(c.nif, c.pais)
		if err != nil || pre != c.pre || num != c.num {
			t.Errorf("%q (%q): %q %q %v; quiero %q %q", c.nif, c.pais, pre, num, err, c.pre, c.num)
		}
	}
	if _, _, err := PartirNIFIVA("GR123456789", ""); !errors.Is(err, ErrNoEsNIFIVA) {
		t.Errorf("GR no es un prefijo de VIES (es EL): %v", err)
	}
}
