// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
)

// MKCALENDAR: el calendario nuevo sale en la lista con su nombre, y se puede escribir.
func TestCrearCalendario(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	ctx := context.Background()
	c := cliente(t, s, "/cal/")
	_, casa, _, err := c.Principal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.CrearCalendario(ctx, casa, "Viajes & ocio", "#aa3300")
	if err != nil || !strings.HasPrefix(u, casa) || !strings.HasSuffix(u, "/") {
		t.Fatalf("crear: %q %v", u, err)
	}
	cs, err := c.Colecciones(ctx, casa, dav.Calendario)
	if err != nil || len(cs) != 1 || cs[0].Nombre != "Viajes &amp; ocio" && cs[0].Nombre != "Viajes & ocio" || cs[0].URL != u {
		t.Fatalf("lista: %+v %v", cs, err)
	}
	if _, err := c.Poner(ctx, dav.URLNueva(u, "a", dav.Calendario), "", evento("a", "Tren", time.Now()), dav.Calendario); err != nil {
		t.Fatalf("escribir en el nuevo: %v", err)
	}
	if _, err := c.CrearCalendario(ctx, casa, "  ", ""); err == nil {
		t.Fatal("creó un calendario sin nombre")
	}
	// Fuera de la casa, el servidor dice que no, y se nota.
	if _, err := c.CrearCalendario(ctx, s.URL+"/cal/u/", "x", ""); !errors.Is(err, dav.ErrAcceso) {
		t.Fatalf("fuera de la casa: %v", err)
	}
	var metodo string
	for _, p := range s.Peticion {
		if strings.HasPrefix(p, "MKCALENDAR ") {
			metodo = p
		}
	}
	if metodo == "" {
		t.Fatalf("sin MKCALENDAR: %v", s.Peticion)
	}
}

// Un servidor que no quiere vCard 4.0 lo dice con ErrFormato; la 3.0 entra.
func TestFormatoRechazado(t *testing.T) {
	s := davprueba.Nuevo()
	s.Solo3 = true
	defer s.Close()
	col := davprueba.CasaCard + "libreta/"
	s.Crear(col, "Libreta", false)
	c := cliente(t, s, "/card/")
	ctx := context.Background()
	v4 := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:a\r\nFN:Ana\r\nEND:VCARD\r\n")
	if _, err := c.Poner(ctx, s.URL+col+"a.vcf", "", v4, dav.Libreta); !errors.Is(err, dav.ErrFormato) {
		t.Fatalf("4.0: %v", err)
	}
	if _, err := c.Poner(ctx, s.URL+col+"a.vcf", "", tarjeta("a", "Ana"), dav.Libreta); err != nil {
		t.Fatalf("3.0: %v", err)
	}
	// La precondición de CardDAV en un 403 también es de formato, no de permiso.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<d:error xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav"><card:supported-address-data/></d:error>`))
	}))
	defer srv.Close()
	k, _ := dav.Nuevo(dav.Cuenta{Direccion: srv.URL + "/"}, srv.Client())
	if _, err := k.Poner(ctx, srv.URL+"/l/a.vcf", "", v4, dav.Libreta); !errors.Is(err, dav.ErrFormato) || errors.Is(err, dav.ErrAcceso) {
		t.Fatalf("precondición: %v", err)
	}
}

// La consulta por rango y la lectura con su tipo.
func TestConsultarEventosYLeerComo(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	col := davprueba.CasaCal + "trabajo/"
	s.Crear(col, "Trabajo", false)
	ahora := time.Now().UTC().Truncate(time.Hour)
	s.Poner(col+"dentro.ics", evento("dentro", "Dentro", ahora.Add(24*time.Hour)))
	s.Poner(col+"fuera.ics", evento("fuera", "Fuera", ahora.AddDate(0, 2, 0)))
	c := cliente(t, s, "/cal/")
	ctx := context.Background()
	objs, err := c.ConsultarEventos(ctx, s.URL+col, ahora, ahora.AddDate(0, 0, 7), false)
	if err != nil || len(objs) != 1 || !strings.HasSuffix(objs[0].URL, "dentro.ics") || objs[0].ETag == "" {
		t.Fatalf("consulta: %+v %v", objs, err)
	}
	o, err := c.LeerComo(ctx, objs[0].URL, dav.Calendario)
	if err != nil || !strings.HasPrefix(o.Tipo, "text/calendar") || !strings.Contains(string(o.Datos), "Dentro") {
		t.Fatalf("leer: %+v %v", o, err)
	}
	if ruta, err := c.RutaDe(objs[0].URL); err != nil || ruta != col+"dentro.ics" {
		t.Fatalf("ruta: %q %v", ruta, err)
	}
	if _, err := c.RutaDe("https://otro.example" + col); err == nil {
		t.Fatal("dio la ruta de otro host")
	}
}
