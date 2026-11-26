// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
)

// iaDePrueba contesta por acción y apunta lo que le llega.
type iaDePrueba struct {
	mu        sync.Mutex
	respuesta map[string]string
	contextos map[string]string
}

func (f *iaDePrueba) pedir(_ context.Context, p api.PeticionIA) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.contextos == nil {
		f.contextos = map[string]string{}
	}
	f.contextos[p.Accion] = p.Contexto
	return f.respuesta[p.Accion], nil
}

// Una frase → un plan que encuentra lo que ya existe y enlaza lo nuevo; crearlo es
// todo o nada y deja la tarea de subir la factura del gasto.
func TestPlanDeRegistrosPorIA(t *testing.T) {
	ctx := context.Background()
	f := &iaDePrueba{respuesta: map[string]string{"acciones": `{"resumen": "Un proveedor y su factura",
		"pasos": [
		  {"ref": "p1", "modelo": "contacto", "buscar": "Anthropic", "datos": {"nombre": "Anthropic", "tipo": "organizacion"}},
		  {"ref": "p2", "modelo": "contacto", "datos": {"nombre": "Imprenta Sur", "tipo": "organizacion"}},
		  {"ref": "p3", "modelo": "factura_compra", "datos": {"proveedor": "$p2", "fecha": "2026-03-02", "base": 10000, "iva": 2100, "total": 12100}},
		  {"ref": "p4", "modelo": "no_existe", "datos": {}}
		]}`}}
	e, _, _ := abrirConIA(t, f.pedir)
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	ant, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Anthropic Ireland, Limited", "tipo": "organizacion"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.ProponerAcciones(ctx, "Da de alta a Imprenta Sur y su factura de 121 € de hoy; Anthropic ya lo tengo")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pasos) != 3 || plan.Pasos[0].Existente == nil || plan.Pasos[0].Existente.ID != ant.ID {
		t.Fatalf("plan: %+v", plan)
	}
	fc := plan.Pasos[2]
	if fc.Enlaces["proveedor"] != "p2" || fc.Tarea == "" || plan.Incompleto() {
		t.Fatalf("paso de la factura: %+v", fc)
	}
	if !strings.Contains(f.contextos["acciones"], "factura_compra") {
		t.Fatal("el contexto describe los modelos")
	}
	creados, err := e.CrearAcciones(ctx, plan.Pasos)
	if err != nil {
		t.Fatal(err)
	}
	if len(creados.Creados) != 2 || len(creados.Actividades) != 1 || creados.Creados[1].Data["proveedor"] != creados.Creados[0].ID {
		t.Fatalf("creados: %+v", creados)
	}
	// Un paso que no valida no deja nada a medias.
	malos := []api.ERPPasoPlan{
		{Ref: "a", Modelo: "contacto", Data: map[string]any{"nombre": "Nuevo", "tipo": "organizacion"}},
		{Ref: "b", Modelo: "factura_compra", Data: map[string]any{"base": "no es dinero"}},
	}
	if _, err := e.CrearAcciones(ctx, malos); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("plan inválido: %v", err)
	}
	if pag, err := e.ListarCon(ctx, "contacto", api.ERPFiltro{Q: "Nuevo"}); err != nil || len(pag.Registros) != 0 {
		t.Fatalf("todo o nada: %v %d", err, len(pag.Registros))
	}
}

// Las pistas del banco van a los cargos sin pista; lo que la IA no contesta queda
// mirado, y no se vuelve a preguntar.
func TestPistasDelBanco(t *testing.T) {
	ctx := context.Background()
	f := &iaDePrueba{respuesta: map[string]string{"pistas_factura": `{"movimientos": [
		{"id": "m1", "empresa": "Amazon", "tipo": "online", "web": "https://www.amazon.es", "url_factura": "https://www.amazon.es/pedidos"}]}`}}
	e, alm, _ := abrirConIA(t, f.pedir)
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Taller", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ImportarExtracto(ctx, n43Cargo()); err != nil {
		t.Fatal(err)
	}
	n, err := e.PistasBanco(ctx)
	if err != nil || n == 0 {
		t.Fatalf("pistas: %v %d", err, n)
	}
	lineas, _, err := alm.LineasBanco(ctx, "pendiente", 100)
	if err != nil {
		t.Fatal(err)
	}
	conPista := 0
	for _, l := range lineas {
		if l.Empresa == "Amazon" && l.URLFactura != "" {
			conPista++
		}
	}
	if conPista != 1 {
		t.Fatalf("una pista guardada: %+v", lineas)
	}
	if n, err := e.PistasBanco(ctx); err != nil || n != 0 {
		t.Fatalf("lo mirado no se repite: %v %d", err, n)
	}
}

// n43Cargo es un extracto con un cargo de una tienda en línea, sin nada que lo explique.
func n43Cargo() []byte {
	var b strings.Builder
	linea := func(s string) { b.WriteString(fmt.Sprintf("%-80s", s) + "\r\n") }
	linea("11" + "2100" + "0001" + "0123456789" + "260901" + "260930" + "2" + "00000000100000" + "978" + "3" + "TALLER RUIZ SL")
	linea("22" + "    " + "0001" + "260912" + "260912" + "12" + "000" + "1" + "00000000004599" + "0000000000" + "000000000000" + "")
	linea("23" + "01" + fmt.Sprintf("%-38s", "AMAZON MKTPLACE PMTS") + fmt.Sprintf("%-38s", ""))
	linea("33" + "2100" + "0001" + "0123456789" + "00001" + "00000000004599" + "00000" + "00000000000000" + "2" + "00000000095401" + "978")
	linea("88" + "999999999999999999" + "000004")
	return []byte(b.String())
}

// Rellenar una ficha desde el PDF de una factura: su texto sale del motor PDF local.
func TestRellenarFichaDesdePDF(t *testing.T) {
	ctx := context.Background()
	f := &iaDePrueba{respuesta: map[string]string{"rellenar": `{"nombre": "Acme Ibérica SL", "tipo": "organizacion", "nif": "B12345678"}`}}
	e, _, _ := abrirConIA(t, f.pedir)
	impreso := efactura.Imprimible{
		Idioma: "es", Titulo: "Factura", Numero: "F-7",
		Emisor: efactura.ParteImprimible{Nombre: "Acme Ibérica SL", NIF: "B12345678"}, Base: "10,00 €", IVA: "2,10 €", Total: "12,10 €",
		RotuloBase: "Base", RotuloIVA: "IVA", RotTotal: "Total", Concepto: "Servicio",
	}
	pdf, err := impreso.PDF()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := e.RellenarFichaPDF(ctx, "contacto", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Data["nombre"] != "Acme Ibérica SL" || !strings.Contains(f.contextos["rellenar"], "Acme Ibérica SL") || !strings.Contains(f.contextos["rellenar"], "Factura F-7") {
		t.Fatalf("relleno: %+v\ncontexto: %q", rel, f.contextos["rellenar"])
	}
	if _, err := e.RellenarFichaPDF(ctx, "contacto", []byte("%PDF-1.4\n%%EOF")); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("un PDF sin texto: %v", err)
	}
}
