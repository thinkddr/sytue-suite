// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const instanciaPrueba = "6f1c2a4e-1b2c-4d5e-8f90-123456789abc"

// Los objetos de cada programa son los de la web (`PROVEEDORES` de ImportarDesde.tsx),
// en el mismo orden y con el mismo rótulo, y sus claves existen en los mapeos del
// servicio: una que no existe hace que el servicio rechace la importación entera.
//
// VISTO EN ROJO (2026-11-25): con «invoice» en vez de «invoices» en Holded, falla contra
// holded.go y contra la web.
func TestLosProgramasSonLosDeLaWebYLosDelServicio(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	src, err := os.ReadFile(filepath.Join(raiz, "apps", "frontend", "apps", "erp", "src", "components", "ImportarDesde.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	// El bloque de `PROVEEDORES` y, en él, cada proveedor con sus { clave, rotulo }.
	web := string(src)
	inicio := strings.Index(web, "export const PROVEEDORES")
	if inicio < 0 {
		t.Fatal("la web ya no exporta PROVEEDORES")
	}
	web = web[inicio:]
	fin := strings.Index(web, "\n];")
	if fin < 0 {
		t.Fatal("no se encuentra el final de PROVEEDORES")
	}
	web = web[:fin]
	reProveedor := regexp.MustCompile(`id: '([a-z-]+)',`)
	reObjeto := regexp.MustCompile(`\{ clave: '([^']+)', rotulo: '([^']+)' \}`)
	cortes := reProveedor.FindAllStringSubmatchIndex(web, -1)
	if len(cortes) != len(ProveedoresImportacion) {
		t.Fatalf("la web tiene %d programas y Fyne %d", len(cortes), len(ProveedoresImportacion))
	}
	fichero := map[string]string{"holded": "holded.go", "odoo": "odoo.go", "salesforce": "salesforce.go", "business-central": "businesscentral.go"}
	reClave := regexp.MustCompile(`\b(?:Nombre|Objeto):\s*"([^"]+)"`)
	for i, c := range cortes {
		id := web[c[2]:c[3]]
		fin := len(web)
		if i+1 < len(cortes) {
			fin = cortes[i+1][0]
		}
		p := ProveedoresImportacion[i]
		if p.ID != id {
			t.Fatalf("el programa %d es %q en la web y %q en Fyne", i, id, p.ID)
		}
		var objetos []ObjetoImportacion
		for _, m := range reObjeto.FindAllStringSubmatch(web[c[0]:fin], -1) {
			objetos = append(objetos, ObjetoImportacion{m[1], m[2]})
		}
		if !slices.Equal(objetos, p.Objetos) {
			t.Errorf("%s: la web trae %v y Fyne %v", id, objetos, p.Objetos)
		}
		goSrc, err := os.ReadFile(filepath.Join(raiz, "services", "syerp", "internal", "conectores", fichero[id]))
		if err != nil {
			t.Fatal(err)
		}
		var claves []string
		for _, m := range reClave.FindAllSubmatch(goSrc, -1) {
			claves = append(claves, string(m[1]))
		}
		for _, o := range p.Objetos {
			if !slices.Contains(claves, o.Clave) {
				t.Errorf("%s: el servicio no conoce «%s»", id, o.Clave)
			}
		}
	}
}

// pantallaImportar es la pantalla de quien administra, en una ventana (el asistente es
// un diálogo) y con el trabajo de red en la misma rutina.
func pantallaImportar(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) (*Pantalla, *servicioFalso) {
	t.Helper()
	p, sf := pantallaConServicio(t, responder)
	p.SabePermisos = true
	p.Permisos = api.ERPPermisos{Administrador: true, Modulos: map[string]string{}}
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.Ventana = fyne.NewSize(1280, 800)
	return p, sf
}

// TestImportarDesdeHolded es el recorrido de la web (ImportarDesde.test.tsx): la tarjeta
// de la portada abre el asistente, se elige Holded, una instancia que no conecta no deja
// seguir, la buena se prueba y se guarda, se desmarca un objeto, se importa y el resumen
// dice lo que entró y lo que falló. Quien no administra no ve la tarjeta.
func TestImportarDesdeHolded(t *testing.T) {
	var importar map[string][]string
	var guardada map[string]any
	pruebaFalla := true
	p, sf := pantallaImportar(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/conexiones":
			_, _ = io.WriteString(w, `{"conexiones":[{"proveedor":"odoo","instancia":"`+instanciaPrueba+`","activo":true}]}`)
		case "/api/v1/conexiones/holded/probar":
			if pruebaFalla {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, `{"error":{"code":"unprocessable","message":"gateway (401)"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"objeto":"contacts","filas":1}`)
		case "/api/v1/conexiones/holded":
			_ = json.NewDecoder(r.Body).Decode(&guardada)
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/importar/holded":
			_ = json.NewDecoder(r.Body).Decode(&importar)
			_, _ = io.WriteString(w, `{"importado":[{"objeto":"contacts","modelo":"contacto","creados":12,"actualizados":3},{"objeto":"invoices","modelo":"factura_venta","creados":4,"actualizados":0,"fallos":["F-1: sin cliente"]}]}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	p.inicio.Fase = Exito
	textos := textosDe(p.inicio.Objeto(tema.Claro))
	if !contiene(textos, "Importar desde otro programa") {
		t.Fatalf("la portada de quien administra no ofrece importar: %v", textos)
	}
	if p.AbrirImportarDesde() == nil || p.importando == nil {
		t.Fatal("no se abrió el asistente")
	}
	x := p.importando
	// Las conexiones guardadas ya están: Odoo rellenaría su instancia.
	if x.Guardadas["odoo"] != instanciaPrueba {
		t.Errorf("no se leyeron las conexiones guardadas: %v", x.Guardadas)
	}
	x.Elegir("holded")
	if x.Instancia != "" || len(x.ObjetosElegidos()) != 4 {
		t.Fatalf("Holded: instancia %q, objetos %v", x.Instancia, x.ObjetosElegidos())
	}
	x.asistente.IrA(PasoImportarConexion)
	x.campo.PonerTexto("no-es-un-id")
	x.Probar()
	if x.ErrorConexion == "" || x.Probada {
		t.Fatalf("un identificador sin forma de UUID se prueba: %q", x.ErrorConexion)
	}
	x.campo.PonerTexto(instanciaPrueba)
	x.Probar()
	if x.Probada || x.ErrorConexion == "" || x.asistente.PuedeSeguir() {
		t.Fatalf("una instancia que no conecta deja seguir: probada %v, %q", x.Probada, x.ErrorConexion)
	}
	pruebaFalla = false
	x.Probar()
	if !x.Probada || !x.asistente.PuedeSeguir() {
		t.Fatalf("la buena no deja seguir: %q", x.ErrorConexion)
	}
	x.guardar()
	if guardada["instancia"] != instanciaPrueba || guardada["activo"] != true || x.Paso != PasoImportarQue {
		t.Fatalf("guardada %v, paso %d", guardada, x.Paso)
	}
	x.Elegidos["products"] = false
	x.importar()
	if !slices.Equal(importar["objetos"], []string{"contacts", "invoices", "purchases"}) {
		t.Fatalf("se importa %v", importar)
	}
	if x.Paso != PasoImportarListo || len(x.Resultado) != 2 {
		t.Fatalf("paso %d, resultado %v", x.Paso, x.Resultado)
	}
	textos = textosDe(x.Objeto(tema.Claro, 600))
	for _, s := range []string{"12", "3", "1"} {
		if !contiene(textos, s) {
			t.Errorf("el resumen no dice %s: %v", s, textos)
		}
	}
	if !strings.Contains(strings.Join(sf.pedidas, "\n"), "POST /api/v1/importar/holded") {
		t.Error("no se importó")
	}

	// Quien no administra no ve la tarjeta ni puede abrirlo.
	p.Permisos.Administrador = false
	p.importando = nil
	if contiene(textosDe(p.inicio.Objeto(tema.Claro)), "Importar desde otro programa") || p.AbrirImportarDesde() != nil {
		t.Error("quien no administra puede importar")
	}
}
