// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La relación crea el cliente en otro panel y aplica la respuesta del servicio al
// formulario original. Antes, «Nuevo» obligaba a abandonar el documento sin guardar.
func TestElAltaRelacionadaConservaElDocumentoYSeleccionaLaRespuesta(t *testing.T) {
	var data map[string]any
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cliente/" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		data = body.Data
		_, _ = io.WriteString(w, `{"id":"cliente-creado","modelo":"cliente","etiqueta":"Cliente nuevo","data":{"nombre":"Cliente nuevo"}}`)
	})
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.lista.Modelo = facturas()
	padre := NuevaFicha(p.tr, facturas(), nil, nil, nil)
	padre.Valores["total"] = "12.34"
	p.conectarAltas(padre)
	p.ficha = padre
	r := padre.controles[0].relacion
	padre.CrearRelacion("cliente", r.elegir)
	if len(p.altas) != 1 || p.altas[0].dialogo.Colocacion != ui.DialogoDerecha {
		t.Fatal("la relación no abrió el panel lateral compartido")
	}
	alta := p.altas[0].ficha
	alta.fijar("nombre", "Cliente nuevo")
	var enviar func()
	p.fondo = func(f func()) { enviar = f }
	alta.AlGuardar()
	if !alta.Guardando {
		t.Fatal("guardar no inhabilita el formulario")
	}
	p.altas[0].dialogo.Descartar()
	if len(p.altas) != 1 {
		t.Fatal("el panel se cerró mientras guardaba")
	}
	alta.AlGuardar()
	enviar()
	if p.ficha != padre || p.lista.Modelo.Clave != "factura_venta" || padre.Valores["total"] != "12.34" {
		t.Fatal("el alta cambió el documento de origen")
	}
	if padre.Valores["cliente"] != "cliente-creado" || padre.Rotulos["cliente-creado"] != "Cliente nuevo" {
		t.Errorf("respuesta no seleccionada: %v/%v", padre.Valores, padre.Rotulos)
	}
	if len(p.altas) != 0 || data["nombre"] != "Cliente nuevo" || len(sf.vistas()) != 1 {
		t.Errorf("alta no única o cuerpo erróneo: %v/%v", data, sf.vistas())
	}
}

func TestElAltaRapidaRespetaPermisosYValidaRelacionObligatoria(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("no debía guardar %s", r.URL.Path) })
	w := test.NewWindow(p.raiz)
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.SabePermisos = true
	p.Permisos = api.ERPPermisos{Modulos: map[string]string{"contactos": api.ERPLectura}}
	cliente := p.Catalogo.Modulos[1].Modelos[0]
	if p.AltaRapida(cliente, nil, nil, nil) != nil {
		t.Fatal("ofrece alta con permiso sólo de lectura")
	}
	p.SabePermisos = false
	p.Catalogo.Activos = []string{"facturacion"}
	if p.PuedeCrearRelacion("cliente") {
		t.Fatal("ofrece crear en un módulo inactivo")
	}
	m := facturas()
	m.Campos[0].Requerido = true
	d := p.AltaRapida(m, nil, nil, nil)
	if d == nil {
		t.Fatal("no abrió el alta permitida")
	}
	f := p.altas[0].ficha
	f.AlGuardar()
	if f.Errores["cliente"] == "" || len(sf.vistas()) != 0 || f.Guardando {
		t.Errorf("la relación obligatoria no se validó antes de POST: %v", f.Errores)
	}
	d.Descartar()
}

func TestElAltaRapidaConservaErroresPorCampoDelServicio(t *testing.T) {
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"code":"datos_invalidos","message":"Revisa los datos","details":{"fields":[{"field":"nombre","message":"Ya existe"}]}}}`)
	})
	w := test.NewWindow(p.raiz)
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	d := p.AltaRapida(p.Catalogo.Modulos[1].Modelos[0], nil, nil, nil)
	f := p.altas[0].ficha
	f.fijar("nombre", "Duplicado")
	f.AlGuardar()
	if len(p.altas) != 1 || f.Guardando || f.Errores["nombre"] != "Ya existe" {
		t.Errorf("el error cerró el panel o no se conservó: %v/%q", f.Errores, f.Fallo)
	}
	if strings.TrimSpace(f.Valores["nombre"]) != "Duplicado" {
		t.Fatal("el error descartó lo escrito")
	}
	d.Descartar()
}
