// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSubirEImportarUsanElSelectorDelSistema: «Subir» manda el primer fichero que el
// `accept` de la web admite, con su tipo, y «Importar CSV» solo los CSV. Sin selector,
// la lista no pinta «Importar CSV».
//
// VISTO EN ROJO (frontal, 2026-09-23): sin filtrar por extensión, «Subir» manda la
// `nota.txt` que se eligió primero, que el servicio rechaza por su contenido.
func TestSubirEImportarUsanElSelectorDelSistema(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost {
			mu.Lock()
			vistas = append(vistas, r.URL.Path+"?"+r.URL.RawQuery+" "+r.Header.Get("Content-Type")+" "+string(cuerpo))
			mu.Unlock()
		}
		switch r.URL.Path {
		case "/api/v1/adjuntos":
			_, _ = io.WriteString(w, `{"tareas_hechas":1}`)
		case "/api/v1/factura_venta/importar":
			_, _ = io.WriteString(w, `{"filas":1,"creados":1,"actualizados":0,"fallos":[],"ignoradas":[]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	tr := traductor(t)
	p := NuevaPantalla(tr, api.NewERP(srv.URL, nil, nil, "es"), nil)
	p.fondo = func(f func()) { f() }
	p.Catalogo = catalogoEjemplo()

	l := p.lista
	l.Modelo, l.Pagina, l.Ventana, l.Fase = facturas(), paginaEjemplo(), 1280, Exito
	l.AlNueva = func() {}
	p.conFicheros()
	if contiene(textosDe(cabeceraDe(l)), tr.T("list.import", nil)) {
		t.Error("sin selector de ficheros no se pinta «Importar CSV»")
	}

	dir := t.TempDir()
	escribir := func(nombre, contenido string) string {
		r := filepath.Join(dir, nombre)
		if err := os.WriteFile(r, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
		return r
	}
	rutas := []string{escribir("nota.txt", "hola"), escribir("factura.pdf", "%PDF"), escribir("lista.csv", "ref,total\n")}
	p.ElegirFicheros = func(listo func([]string)) { listo(rutas) }
	p.conFicheros()
	c, alto := l.cabecera(tema.Claro, anchoContenido)
	if !contiene(textosDe(c), tr.T("list.import", nil)) || alto != cabeceraMedida {
		t.Errorf("con selector, «Importar CSV» está y la cabecera mide %.2f (la web %.2f)", alto, cabeceraMedida)
	}
	p.subirDocumento("factura_venta", "f1", func(bool) {})
	p.importar()
	mu.Lock()
	defer mu.Unlock()
	quiero := []string{
		"/api/v1/adjuntos?modelo=factura_venta&nombre=factura.pdf&registro=f1 application/pdf %PDF",
		"/api/v1/factura_venta/importar? text/csv ref,total\n",
	}
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}
