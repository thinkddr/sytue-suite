// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
)

func evento(uid, resumen string, inicio time.Time) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:" + inicio.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + inicio.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\nSUMMARY:" + resumen +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

func tarjeta(uid, nombre string) []byte {
	return []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + nombre + "\r\nN:" + nombre + ";;;;\r\nEND:VCARD\r\n")
}

func cliente(t *testing.T, s *davprueba.Servidor, ruta string) *dav.Cliente {
	t.Helper()
	c, err := dav.Nuevo(dav.Cuenta{Direccion: s.URL + ruta, Usuario: s.Usuario, Clave: s.Clave}, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPrincipalYColecciones(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	s.Crear(davprueba.CasaCal+"trabajo/", "Trabajo", false)
	s.Crear(davprueba.CasaCal+"festivos/", "Festivos", true)
	s.Crear(davprueba.CasaCard+"libreta/", "Mis contactos", false)
	ctx := context.Background()

	c := cliente(t, s, "/cal/")
	_, cals, _, err := c.Principal(ctx)
	if err != nil || !strings.HasSuffix(cals, davprueba.CasaCal) {
		t.Fatalf("principal: %q %v", cals, err)
	}
	cs, err := c.Colecciones(ctx, cals, dav.Calendario)
	if err != nil || len(cs) != 2 {
		t.Fatalf("colecciones: %+v %v", cs, err)
	}
	if cs[0].Nombre != "Festivos" || !cs[0].SoloLectura || cs[1].SoloLectura || cs[1].Color != "#33AA55" || cs[1].SyncToken == "" {
		t.Fatalf("propiedades: %+v", cs)
	}

	k := cliente(t, s, "/card/")
	_, _, libs, err := k.Principal(ctx)
	if err != nil || !strings.HasSuffix(libs, davprueba.CasaCard) {
		t.Fatalf("principal card: %q %v", libs, err)
	}
	ls, err := k.Colecciones(ctx, libs, dav.Libreta)
	if err != nil || len(ls) != 1 || ls[0].Nombre != "Mis contactos" {
		t.Fatalf("libretas: %+v %v", ls, err)
	}

	// Una contraseña mala se nota en la prueba.
	mala, _ := dav.Nuevo(dav.Cuenta{Direccion: s.URL + "/cal/", Usuario: "ana", Clave: "no"}, s.Client())
	if _, _, _, err := mala.Principal(ctx); !errors.Is(err, dav.ErrAcceso) {
		t.Fatalf("contraseña mala: %v", err)
	}
}

func TestCambiosPorSyncConVentana(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	col := davprueba.CasaCal + "trabajo/"
	s.Crear(col, "Trabajo", false)
	ahora := time.Now()
	s.Poner(col+"viejo.ics", evento("viejo", "Hace dos años", ahora.AddDate(-2, 0, 0)))
	s.Poner(col+"nuevo.ics", evento("nuevo", "La semana que viene", ahora.AddDate(0, 0, 7)))
	ctx := context.Background()
	c := cliente(t, s, "/cal/")
	cs, _ := c.Colecciones(ctx, s.URL+davprueba.CasaCal, dav.Calendario)
	if len(cs) != 1 {
		t.Fatalf("colecciones %+v", cs)
	}
	ca, err := c.Cambios(ctx, cs[0], "", nil, ahora.AddDate(-1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(ca.Puestos) != 1 || !strings.HasSuffix(ca.Puestos[0].URL, "nuevo.ics") || len(ca.Fuera) != 1 || !ca.Completo || !strings.HasPrefix(ca.Ficha, "sync:") {
		t.Fatalf("primera bajada: %+v", ca)
	}
	etags := map[string]string{ca.Puestos[0].URL: ca.Puestos[0].ETag}

	// Otro cliente cambia uno y borra el otro: solo eso llega.
	s.Poner(col+"nuevo.ics", evento("nuevo", "Cambiado", ahora.AddDate(0, 0, 7)))
	s.Quitar(col + "viejo.ics")
	ca, err = c.Cambios(ctx, cs[0], ca.Ficha, func(u string) (string, bool) { e, ok := etags[u]; return e, ok }, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ca.Puestos) != 1 || !strings.Contains(string(ca.Puestos[0].Datos), "Cambiado") || len(ca.Borrados) != 1 || ca.Completo {
		t.Fatalf("incremental: %+v", ca)
	}
	// Una ficha que el servidor no reconoce: se empieza de nuevo y la lista es completa.
	ca, err = c.Cambios(ctx, cs[0], "sync:basura", nil, time.Time{})
	if err != nil || !ca.Completo || len(ca.Puestos) != 1 {
		t.Fatalf("ficha caducada: %+v %v", ca, err)
	}
}

func TestCambiosPorETagSinSync(t *testing.T) {
	s := davprueba.Nuevo()
	s.SinSync = true
	defer s.Close()
	col := davprueba.CasaCard + "libreta/"
	s.Crear(col, "Libreta", false)
	s.Poner(col+"a.vcf", tarjeta("a", "Ana"))
	s.Poner(col+"b.vcf", tarjeta("b", "Bea"))
	ctx := context.Background()
	c := cliente(t, s, "/card/")
	ls, _ := c.Colecciones(ctx, s.URL+davprueba.CasaCard, dav.Libreta)
	if len(ls) != 1 || ls[0].SyncToken != "" || ls[0].CTag == "" {
		t.Fatalf("libreta %+v", ls)
	}
	ca, err := c.Cambios(ctx, ls[0], "", nil, time.Time{})
	if err != nil || len(ca.Puestos) != 2 || !ca.Completo || len(ca.Vistos) != 2 || !strings.HasPrefix(ca.Ficha, "ctag:") {
		t.Fatalf("primera: %+v %v", ca, err)
	}
	// Con el mismo ctag no se pregunta nada más.
	ca2, err := c.Cambios(ctx, ls[0], ca.Ficha, nil, time.Time{})
	if err != nil || ca2.Completo || len(ca2.Puestos) != 0 {
		t.Fatalf("sin cambios: %+v %v", ca2, err)
	}
	etags := map[string]string{}
	for _, o := range ca.Puestos {
		etags[o.URL] = o.ETag
	}
	s.Poner(col+"b.vcf", tarjeta("b", "Beatriz"))
	ls, _ = c.Colecciones(ctx, s.URL+davprueba.CasaCard, dav.Libreta)
	ca, err = c.Cambios(ctx, ls[0], ca.Ficha, func(u string) (string, bool) { e, ok := etags[u]; return e, ok }, time.Time{})
	if err != nil || len(ca.Puestos) != 1 || !strings.Contains(string(ca.Puestos[0].Datos), "Beatriz") || len(ca.Vistos) != 2 {
		t.Fatalf("cambio por etag: %+v %v", ca, err)
	}
}

func TestPonerYBorrarConETag(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	col := davprueba.CasaCal + "trabajo/"
	s.Crear(col, "Trabajo", false)
	s.Crear(davprueba.CasaCal+"festivos/", "Festivos", true)
	ctx := context.Background()
	c := cliente(t, s, "/cal/")
	u := dav.URLNueva(s.URL+col, "uid/raro 1", dav.Calendario)
	if !strings.HasSuffix(u, "/uid_raro_1.ics") {
		t.Fatalf("url nueva %q", u)
	}
	etag, err := c.Poner(ctx, u, "", evento("uid/raro 1", "Uno", time.Now()), dav.Calendario)
	if err != nil || etag == "" {
		t.Fatalf("crear: %q %v", etag, err)
	}
	// Crear otra vez la misma URL no pisa.
	if _, err := c.Poner(ctx, u, "", evento("uid/raro 1", "Dos", time.Now()), dav.Calendario); !errors.Is(err, dav.ErrConflicto) {
		t.Fatalf("crear dos veces: %v", err)
	}
	nueva, err := c.Poner(ctx, u, etag, evento("uid/raro 1", "Dos", time.Now()), dav.Calendario)
	if err != nil || nueva == etag {
		t.Fatalf("actualizar: %v", err)
	}
	// Con la ETag vieja, ni se reemplaza ni se borra.
	if _, err := c.Poner(ctx, u, etag, evento("uid/raro 1", "Tres", time.Now()), dav.Calendario); !errors.Is(err, dav.ErrConflicto) {
		t.Fatalf("etag vieja: %v", err)
	}
	if err := c.Borrar(ctx, u, etag); !errors.Is(err, dav.ErrConflicto) {
		t.Fatalf("borrar con etag vieja: %v", err)
	}
	o, err := c.Leer(ctx, u)
	if err != nil || !strings.Contains(string(o.Datos), "Dos") {
		t.Fatalf("leer: %v", err)
	}
	if err := c.Borrar(ctx, u, nueva); err != nil {
		t.Fatal(err)
	}
	if err := c.Borrar(ctx, u, nueva); err != nil {
		t.Fatalf("borrar lo que ya no está: %v", err)
	}
	// En una colección de solo lectura el servidor dice que no.
	if _, err := c.Poner(ctx, dav.URLNueva(s.URL+davprueba.CasaCal+"festivos/", "x", dav.Calendario), "", evento("x", "x", time.Now()), dav.Calendario); !errors.Is(err, dav.ErrAcceso) {
		t.Fatalf("solo lectura: %v", err)
	}
}

func TestPlanificaYHostsAjenos(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	col := davprueba.CasaCal + "trabajo/"
	s.Crear(col, "Trabajo", false)
	ctx := context.Background()
	c := cliente(t, s, "/cal/")
	if si, err := c.Planifica(ctx, s.URL+col); err != nil || si {
		t.Fatalf("sin planificación: %v %v", si, err)
	}
	s.Planifica = true
	if si, err := c.Planifica(ctx, s.URL+col); err != nil || !si {
		t.Fatalf("con planificación: %v %v", si, err)
	}
	// Una URL de otro host no se pide nunca (la contraseña no sale hacia él).
	if _, err := c.Leer(ctx, "https://otro.example/cal/x.ics"); err == nil {
		t.Fatal("pidió a otro host")
	}
}

type dnsFalso struct {
	srv map[string][]*net.SRV
	txt map[string][]string
}

func (d dnsFalso) LookupSRV(_ context.Context, servicio, proto, nombre string) (string, []*net.SRV, error) {
	if s, ok := d.srv["_"+servicio+"._"+proto+"."+nombre]; ok {
		return "", s, nil
	}
	return "", nil, errors.New("no hay")
}

func (d dnsFalso) LookupTXT(_ context.Context, nombre string) ([]string, error) {
	return d.txt[nombre], nil
}

func TestDescubrir(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	u, _ := url.Parse(s.URL)
	host, puerto, _ := net.SplitHostPort(u.Host)
	var p uint16
	for _, ch := range puerto {
		p = p*10 + uint16(ch-'0')
	}
	// SRV y TXT para CalDAV; CardDAV por .well-known, que redirige.
	wk := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/carddav" {
			http.Redirect(w, r, s.URL+"/card/", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	}))
	defer wk.Close()
	// El cliente de pruebas tiene que fiarse de los dos servidores, y el «dominio» de
	// la persona es el del segundo.
	pool := s.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	pool.AddCert(wk.Certificate())
	wu, _ := url.Parse(wk.URL)
	dns := dnsFalso{
		srv: map[string][]*net.SRV{"_caldavs._tcp." + wu.Host: {{Target: host + ".", Port: p}}},
		txt: map[string][]string{"_caldavs._tcp." + wu.Host: {"path=/cal/"}},
	}
	d, err := dav.Descubrir(context.Background(), "ana@"+wu.Host, dns, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	if d.Calendario != "https://"+net.JoinHostPort(host, puerto)+"/cal/" || d.Contactos != s.URL+"/card/" {
		t.Fatalf("descubierto: %+v", d)
	}
	if _, err := dav.Descubrir(context.Background(), "ana@"+wu.Host, nil, wk.Client()); err == nil {
		// Sin SRV y con el .well-known de calendario dando 404, el de contactos aún
		// lleva a otro servidor del que wk.Client no se fía: nada.
		t.Log("descubrió algo sin SRV")
	}
}
