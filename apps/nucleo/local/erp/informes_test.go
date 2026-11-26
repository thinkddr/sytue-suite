// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// abrirConIA es abrir con una IA falsa que contesta según la acción.
func abrirConIA(t *testing.T, ia Asistente) (*api.ERP, *Almacen, *Servidor) {
	t.Helper()
	cat, obl, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := Abrir(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	s := NuevoServidor(cat, obl, alm)
	s.PonerIA(ia)
	return api.NewERP(Base, nil, s.Cliente(), "es"), alm, s
}

func TestInformesTablerosEIA(t *testing.T) {
	ctx := context.Background()
	var pedidas []api.PeticionIA
	e, alm, s := abrirConIA(t, func(_ context.Context, p api.PeticionIA) (string, error) {
		pedidas = append(pedidas, p)
		switch p.Accion {
		case "informe":
			return "Aquí tienes:\n```json\n" + `{"titulo":"Ventas","partes":[
				{"titulo":"Facturado","vista":"cifra","consulta":{"fuente":"factura_venta","medida":"suma:total"}},
				{"titulo":"Inventada","vista":"tabla","consulta":{"fuente":"no_existe"}}]}` + "\n```", nil
		case "rellenar":
			return `{"Nombre":"Bolt España SL","correo":"hola@bolt.example","tipo":"organizacion"}`, nil
		}
		return "", errors.New("acción inesperada")
	})
	defer func() { _ = alm.Cerrar() }()
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, _ := e.Crear(ctx, "contacto", map[string]any{"nombre": "Ana", "tipo": "persona"})
	for _, total := range []int{12100, 5000} {
		if _, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-03-04", "base": total, "total": total}); err != nil {
			t.Fatal(err)
		}
	}

	// Una frase → un informe: la parte inventada se descarta.
	inf, err := e.ProponerInforme(ctx, "¿cuánto he facturado?", api.ERPParaTablero, "factura_venta")
	if err != nil || len(inf.Partes) != 1 || inf.Titulo != "Ventas" {
		t.Fatalf("proponer: %v %+v", err, inf)
	}
	if len(pedidas) != 1 || !strings.Contains(pedidas[0].Contexto, "factura_venta") || pedidas[0].Idioma != "es" {
		t.Fatalf("lo que se le pide a la IA: %+v", pedidas)
	}
	tablas, err := e.DatosInforme(ctx, inf)
	if err != nil || len(tablas) != 1 || tablas[0].Error != "" || len(tablas[0].Filas) != 1 {
		t.Fatalf("datos: %v %+v", err, tablas)
	}
	if v := tablas[0].Filas[0][0]; !strings.Contains(strings.TrimSpace(toString(v)), "171") {
		t.Fatalf("la suma de los totales (171,00 €): %v", v)
	}
	rc, err := e.ExcelInforme(ctx, inf)
	if err != nil {
		t.Fatal(err)
	}
	xlsx, _ := io.ReadAll(rc)
	_ = rc.Close()
	if _, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx))); err != nil {
		t.Fatalf("el Excel es un .xlsx: %v", err)
	}

	tab, err := e.GuardarTablero(ctx, inf)
	if err != nil || tab.ID == "" || tab.Titulo != "Ventas" {
		t.Fatalf("guardar tablero: %v %+v", err, tab)
	}
	if ts, err := e.Tableros(ctx); err != nil || len(ts) != 1 || len(ts[0].Informe.Partes) != 1 {
		t.Fatalf("tableros: %v %+v", err, ts)
	}
	if err := e.BorrarTablero(ctx, tab.ID); err != nil {
		t.Fatal(err)
	}

	// Rellenar una ficha desde un texto pegado.
	rel, err := e.RellenarFicha(ctx, "contacto", "Bolt España SL, hola@bolt.example")
	if err != nil || rel.Data["nombre"] != "Bolt España SL" || rel.Data["correo"] != "hola@bolt.example" {
		t.Fatalf("rellenar: %v %+v", err, rel)
	}

	// Sin IA (apagada en Ajustes): 503 como un servidor sin Sytue AI.
	s.PonerIA(nil)
	if _, err := e.ProponerInforme(ctx, "x", "", ""); codigo(err) != "ia_no_disponible" {
		t.Fatalf("sin IA: %v", err)
	}
}

func toString(v any) string { return fmt.Sprint(v) }
