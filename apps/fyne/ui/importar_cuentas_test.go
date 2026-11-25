// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// primeraPalabra es el arranque de un texto traducido: lo que un árbol partido en líneas
// conserva entero.
func primeraPalabra(s string) string { return strings.Fields(s)[0] }

// TestImportarUnBotonPorCuenta: dos cuentas de Google llevan cada una su cuenta en el
// rótulo; la de Microsoft, que no puede leer, se ofrece apagada y dice por qué; y la
// cuenta que ya está trayendo no se ofrece otra vez.
func TestImportarUnBotonPorCuenta(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	g := &pasarelaDoble{
		origenes: `{"origins":[
			{"instance_id":"a","instance_ids":["a","a2"],"name":"Google","source":"google","account_key":"google:acme.com","account_label":"acme.com","state":"ready"},
			{"instance_id":"b","name":"Google 2","source":"google","account_key":"google:beta.com","account_label":"beta.com","state":"ready"},
			{"instance_id":"m","name":"M365","source":"m365","state":"unavailable"}]}`,
		migraciones: `{"migrations":[{"id":"j","instance_id":"a2","domain":"drive","source":"google","status":"running","account_key":"google:acme.com"}]}`,
	}
	srv := g.servidor(t)
	d := NuevoImportarDatos(tr, api.NewIntegraciones(srv.URL, nil, nil), "drive")
	d.aparte = func(f func()) { f() }
	d.Sondeo = 0
	d.CargarCon(context.Background())

	r := repartirImportaciones(d.origenes, d.trabajos, d.dominios)
	if len(r.libres) != 2 || r.libres[0].InstanceID != "b" || r.libres[1].Disponible() {
		t.Fatalf("libres = %+v; quiero beta (la de acme ya trae) y Microsoft apagada", r.libres)
	}
	repetidos := proveedoresRepetidos(d.origenes)
	if got := rotuloOrigen(tr, r.libres[0], repetidos); got != tr.T("import.fromGoogle", nil)+" · beta.com" {
		t.Fatalf("con dos cuentas de Google el botón debe llevar la suya: %q", got)
	}
	if got := rotuloOrigen(tr, r.libres[1], repetidos); got != tr.T("import.fromMicrosoft", nil) {
		t.Fatalf("con una sola de Microsoft basta el rótulo de siempre: %q", got)
	}
	texto := textosDeArbol(d.Objeto(400, tema.Claro))
	if !strings.Contains(texto, primeraPalabra(tr.T("import.unavailable", i18n.Vars{"source": "Microsoft 365"}))) {
		t.Fatalf("la conexión que no puede leer no dice por qué: %q", texto)
	}
}

// TestImportarResumenYReintento: una línea por contenido con lo acumulado y cuándo
// terminó, sin sumar dos importaciones de la misma cuenta; un fallo por conexión
// caducada se dice así, y «Reintentar» relanza lo que falló, que la pasarela reanuda.
func TestImportarResumenYReintento(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	g := &pasarelaDoble{
		origenes: `{"origins":[{"instance_id":"a","name":"Google","source":"google","account_key":"google:acme.com","state":"ready"}]}`,
		migraciones: `{"migrations":[
			{"id":"m","instance_id":"a","domain":"mail","source":"google","status":"done","account_key":"google:acme.com","counts":{"migrated":1250,"skipped":3},"created_at":"2026-11-02T10:00:00Z","finished_at":"2026-11-03T10:00:00Z"},
			{"id":"viejo","instance_id":"a","domain":"mail","source":"google","status":"done","account_key":"google:acme.com","counts":{"migrated":1200},"created_at":"2026-10-01T10:00:00Z"},
			{"id":"c","instance_id":"a","domain":"contacts","source":"google","status":"error","error_code":"connection_expired","account_key":"google:acme.com","created_at":"2026-11-02T10:00:00Z"}]}`,
	}
	srv := g.servidor(t)
	d := NuevoImportarDatos(tr, api.NewIntegraciones(srv.URL, nil, nil), "mail", "contacts")
	d.aparte = func(f func()) { f() }
	d.Sondeo = 0
	d.CargarCon(context.Background())

	r := repartirImportaciones(d.origenes, d.trabajos, d.dominios)
	if len(r.hechos) != 1 || r.hechos[0].ID != "m" || len(r.fallidos) != 1 {
		t.Fatalf("el resumen debe contar la importación vigente, no sumar las dos: %+v", r)
	}
	texto := textosDeArbol(d.Objeto(600, tema.Claro))
	if !strings.Contains(texto, tr.T("dataflow.item.mail", nil)) || !strings.Contains(texto, "2026") {
		t.Fatalf("la línea no dice qué trajo ni cuándo terminó: %q", texto)
	}
	caducada := primeraPalabra(tr.T("import.unavailable", i18n.Vars{"source": "Google"}))
	if !strings.Contains(texto, caducada) || strings.Contains(texto, primeraPalabra(tr.T("import.someFailed", nil))) {
		t.Fatalf("una conexión caducada debe decirse como tal: %q", texto)
	}
	d.reintentar(r.fallidos)
	if g.cuantas() != 1 {
		t.Fatalf("reintentar debía relanzar una vez: %d", g.cuantas())
	}

	// La pasarela rechaza el lanzamiento: el motivo con arreglo, no «inténtalo luego».
	g.poner("", "")
	g.mu.Lock()
	g.lanzar = `{"error":{"code":"import_connection_unavailable","message":"x"}}`
	g.mu.Unlock()
	d.traer(d.origenes[0])
	if d.fallo == nil || d.fallo.code != "import_connection_unavailable" {
		t.Fatalf("fallo = %+v", d.fallo)
	}
	if !strings.Contains(textosDeArbol(d.Objeto(600, tema.Claro)), caducada) {
		t.Fatal("el rechazo por conexión caducada no se dice así")
	}
}
