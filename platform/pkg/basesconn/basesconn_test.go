// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package basesconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tokenBueno = "sbc_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func peticion(raiz string) Peticion {
	return Peticion{Raiz: raiz, Token: tokenBueno, Consumidor: ERP, Tenant: "acme", MaxBytes: 1 << 20, MaxFilas: 10, MaxCols: 5}
}

func servidor(t *testing.T, estado int, cuerpo string, visto *http.Header) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if visto != nil {
			*visto = r.Header.Clone()
		}
		if r.URL.Path != "/_bases/v1/connector/read" || r.Method != http.MethodGet {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(estado)
		_, _ = w.Write([]byte(cuerpo))
	}))
	t.Cleanup(s.Close)
	return s
}

const lecturaBuena = `{"columns":[{"name":"Nombre","type":"text"},{"name":"Importe","type":"number"}],
 "rows":[["Aurora",-12.5],["=HYPERLINK(\"http://x\")",3]],
 "provenance":{"source":"sytue-bases","consumer":"erp","connection_id":"c1","version":2,"base_name":"B","view_name":"V","granted_by":"ana"}}`

func TestLeerMandaTokenConsumidorYOrganizacion(t *testing.T) {
	var h http.Header
	s := servidor(t, 200, lecturaBuena, &h)
	l, err := Leer(context.Background(), s.Client(), peticion(s.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	if h.Get("Authorization") != "Bearer "+tokenBueno || h.Get("X-Sytue-Consumer") != "erp" || h.Get("X-Sytue-Tenant") != "acme" {
		t.Errorf("cabeceras: %v", h)
	}
	if len(l.Rows) != 2 || l.Provenance.Version != 2 || l.Provenance.GrantedBy != "ana" {
		t.Errorf("lectura: %+v", l)
	}
}

func TestLeerNoMandaNadaQueNoSeaUnTokenDeConexion(t *testing.T) {
	llamado := false
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { llamado = true }))
	defer s.Close()
	for _, tok := range []string{"", "eyJhbGciOi.jwt.de.una.persona", "sbc_corto", tokenBueno + "x", "sbf_" + tokenBueno[4:]} {
		p := peticion(s.URL)
		p.Token = tok
		if _, err := Leer(context.Background(), s.Client(), p); !errors.Is(err, ErrToken) {
			t.Errorf("token %q: %v", tok, err)
		}
	}
	p := peticion(s.URL)
	p.Tenant = ""
	if _, err := Leer(context.Background(), s.Client(), p); !errors.Is(err, ErrToken) {
		t.Errorf("sin organización: %v", err)
	}
	p = peticion("")
	if _, err := Leer(context.Background(), s.Client(), p); !errors.Is(err, ErrNoConfigurado) {
		t.Errorf("sin raíz: %v", err)
	}
	if llamado {
		t.Error("salió una petición hacia Bases con una credencial que no es de conexión")
	}
}

func TestLeerErrores(t *testing.T) {
	casos := map[string]struct {
		estado int
		cuerpo string
		quiero error
	}{
		"revocada":            {401, `{}`, ErrRevocada},
		"caída":               {500, `x`, ErrRemoto},
		"no es json":          {200, `<html>`, ErrRemoto},
		"otro consumidor":     {200, strings.Replace(lecturaBuena, `"consumer":"erp"`, `"consumer":"web"`, 1), ErrRemoto},
		"sin procedencia":     {200, `{"columns":[],"rows":[]}`, ErrRemoto},
		"fila desalineada":    {200, strings.Replace(lecturaBuena, `["Aurora",-12.5]`, `["Aurora"]`, 1), ErrRemoto},
		"demasiadas filas":    {200, strings.Replace(lecturaBuena, `["Aurora",-12.5]`, strings.Repeat(`["a",1],`, 10)+`["a",1]`, 1), ErrRemoto},
		"respuesta desmedida": {200, lecturaBuena + strings.Repeat(" ", 2<<20), ErrRemoto},
	}
	for nombre, c := range casos {
		s := servidor(t, c.estado, c.cuerpo, nil)
		if _, err := Leer(context.Background(), s.Client(), peticion(s.URL)); !errors.Is(err, c.quiero) {
			t.Errorf("%s: %v, esperado %v", nombre, err, c.quiero)
		}
	}
}

func TestCSVNeutralizaFormulasYDejaLosNumeros(t *testing.T) {
	s := servidor(t, 200, lecturaBuena, nil)
	l, err := Leer(context.Background(), s.Client(), peticion(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	l.Columns[0].Name = "@cabecera"
	b, err := CSV(l)
	if err != nil {
		t.Fatal(err)
	}
	quiero := "'@cabecera,Importe\nAurora,-12.5\n\"'=HYPERLINK(\"\"http://x\"\")\",3\n"
	if string(b) != quiero {
		t.Errorf("CSV:\n%s\nesperado:\n%s", b, quiero)
	}
}
