// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// abrir monta almacén, servidor y el cliente de siempre (`api.ERP`) sobre el transporte.
func abrir(t *testing.T, dir string) (*api.ERP, *Almacen) {
	t.Helper()
	cat, obl, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := Abrir(dir, cat)
	if err != nil {
		t.Fatal(err)
	}
	return api.NewERP(Base, nil, NuevoServidor(cat, obl, alm).Cliente(), "es"), alm
}

func estado(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

func TestClienteFacturaYReabrir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, alm := abrir(t, dir)

	// Primer arranque: catálogo real, nada encendido y sin perfil (abre el asistente).
	cat, err := e.Catalogo(ctx)
	if err != nil || len(cat.Modulos) == 0 || cat.Activos == nil || len(cat.Activos) != 0 {
		t.Fatalf("catálogo: %v %d %v", err, len(cat.Modulos), cat.Activos)
	}
	if _, err := e.Organizacion(ctx); !api.EsNoEncontrado(err) {
		t.Fatalf("organización sin hacer: %v", err)
	}
	if ps, err := e.Paises(ctx); err != nil || len(ps) == 0 {
		t.Fatalf("países: %v %d", err, len(ps))
	}
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Taller Ruiz", Pais: "ES", Forma: "autonomo"}); err != nil {
		t.Fatalf("perfil: %v", err)
	}
	if ob, err := e.Obligaciones(ctx, 2026); err != nil || len(ob.Obligaciones) == 0 {
		t.Fatalf("obligaciones: %v %d", err, len(ob.Obligaciones))
	}
	res, err := e.CambiarModulo(ctx, "facturacion", true)
	if err != nil || !strings.Contains(strings.Join(res.Activos, ","), "facturacion") || !strings.Contains(strings.Join(res.Activos, ","), "contactos") {
		t.Fatalf("encender facturación (y sus dependencias): %v %v", err, res)
	}
	perm, err := e.MisPermisos(ctx)
	if err != nil || !perm.PuedeAccion("facturacion", "crear") || perm.PuedeAccion("contabilidad", "aprobar") {
		t.Fatalf("permisos: %v %+v", err, perm)
	}

	// Cliente → factura → lista.
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Ferretería Óscar", "tipo": "organizacion", "correo": "hola@example.com"})
	if err != nil {
		t.Fatalf("crear contacto: %v", err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-03-04", "base": 10000, "iva": 2100, "total": 12100})
	if err != nil {
		t.Fatalf("crear factura: %v", err)
	}
	if fac.Ref != "FV-2026-00001" || fac.Estado != "borrador" {
		t.Fatalf("numeración y estado inicial: %+v", fac)
	}
	fac2, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-05-01", "base": 413, "total": 500})
	if err != nil || fac2.Ref != "FV-2026-00002" {
		t.Fatalf("segunda factura: %v %+v", err, fac2)
	}
	if _, err := e.CambiarEstado(ctx, "factura_venta", fac.ID, "confirmado"); err != nil {
		t.Fatalf("emitir: %v", err)
	}
	pag, err := e.ListarCon(ctx, "factura_venta", api.ERPFiltro{Limite: 50})
	if err != nil || len(pag.Registros) != 2 || pag.Registros[0].ID != fac2.ID {
		t.Fatalf("lista: %v %+v", err, pag)
	}
	if pag.Etiquetas[cli.ID] != "Ferretería Óscar" {
		t.Fatalf("etiquetas de las relaciones: %v", pag.Etiquetas)
	}
	if pag, err := e.ListarCon(ctx, "contacto", api.ERPFiltro{Q: "ferreteria"}); err != nil || len(pag.Registros) != 1 {
		t.Fatalf("buscar sin tildes: %v %d", err, len(pag.Registros))
	}
	st, err := e.Estadisticas(ctx, "factura_venta")
	if err != nil || st.Total != 2 || st.PorEstado["confirmado"] != 1 {
		t.Fatalf("estadísticas: %v %+v", err, st)
	}
	if r, err := e.Resumen(ctx); err != nil || r.Facturacion.TotalCentimos != 12600 || r.Facturacion.PendienteCobroCentimos != 12100 {
		t.Fatalf("resumen: %v %+v", err, r)
	}

	// Validación con los errores por campo, y relaciones que no existen.
	_, err = e.Crear(ctx, "factura_venta", map[string]any{"base": 12.5})
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusUnprocessableEntity || ae.Campos()["base"] == "" {
		t.Fatalf("validación: %v", err)
	}
	if _, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": "00000000-0000-0000-0000-000000000000"}); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("relación rota: %v", err)
	}

	// Historial: creado y el cambio de estado.
	hs, err := e.Historial(ctx, "factura_venta", fac.ID)
	if err != nil || len(hs) != 2 || hs[0].Tipo != "cambio" || hs[1].Tipo != "creado" {
		t.Fatalf("historial: %v %+v", err, hs)
	}

	// Facturae dice lo que le falta al perfil (aquí, el NIF y la dirección del emisor).
	if _, _, err := e.Facturae(ctx, "factura_venta", fac.ID); !errors.As(err, &ae) || ae.Status != http.StatusUnprocessableEntity || !strings.Contains(err.Error()+ae.Body, "emisor") {
		t.Fatalf("facturae: %v", err)
	}
	// Lo que no está en esta edición: 501 con su código, sin tumbar nada.
	if _, err := e.Aprobaciones(ctx, ""); !errors.As(err, &ae) || ae.Status != http.StatusNotImplemented || ae.Code() != CodigoNoDisponible {
		t.Fatalf("aprobaciones: %v", err)
	}
	if _, err := e.EstadoVerifactu(ctx); estado(err) != http.StatusNotImplemented {
		t.Fatalf("VERI*FACTU (D-29): %v", err)
	}

	// Reabrir: todo sigue ahí y la numeración continúa.
	if err := alm.Cerrar(); err != nil {
		t.Fatal(err)
	}
	e, alm = abrir(t, dir)
	defer func() { _ = alm.Cerrar() }()
	pag, err = e.ListarCon(ctx, "factura_venta", api.ERPFiltro{})
	if err != nil || len(pag.Registros) != 2 {
		t.Fatalf("tras reabrir: %v %+v", err, pag)
	}
	if r, err := e.Obtener(ctx, "factura_venta", fac.ID); err != nil || r.Estado != "confirmado" || r.Data["cliente"] != cli.ID {
		t.Fatalf("factura tras reabrir: %v %+v", err, r)
	}
	if cat, err := e.Catalogo(ctx); err != nil || len(cat.Activos) == 0 {
		t.Fatalf("módulos tras reabrir: %v %v", err, cat.Activos)
	}
	if o, err := e.Organizacion(ctx); err != nil || o.Nombre != "Taller Ruiz" {
		t.Fatalf("perfil tras reabrir: %v %+v", err, o)
	}
	f3, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-06-01", "base": 1})
	if err != nil || f3.Ref != "FV-2026-00003" {
		t.Fatalf("la numeración sigue: %v %+v", err, f3)
	}
}

func TestPapeleraDuplicarYConvertir(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()

	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Acme", "tipo": "organizacion"})
	if err != nil {
		t.Fatal(err)
	}
	pre, err := e.Crear(ctx, "presupuesto", map[string]any{"cliente": cli.ID, "fecha": "2026-02-01", "base": 1000, "iva": 210, "total": 1210})
	if err != nil {
		t.Fatal(err)
	}
	prod, err := e.Crear(ctx, "producto", map[string]any{"nombre": "Tornillos", "tipo": "bien", "precio": 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Crear(ctx, "linea_presupuesto", map[string]any{
		"presupuesto": pre.ID, "producto": prod.ID, "nombre": "Tornillos",
		"cantidad": "10", "precio_con_iva": 121, "tipo_iva": "21",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Convertir(ctx, "presupuesto", pre.ID); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("convertir un borrador: %v", err)
	}
	if _, err := e.CambiarEstado(ctx, "presupuesto", pre.ID, "aceptado"); err != nil {
		t.Fatal(err)
	}
	ped, err := e.Convertir(ctx, "presupuesto", pre.ID)
	if err != nil || ped.Modelo != "pedido_venta" || ped.Data["cliente"] != cli.ID || !strings.HasPrefix(ped.Ref, "PV-") {
		t.Fatalf("presupuesto → pedido: %v %+v", err, ped)
	}
	if again, err := e.Convertir(ctx, "presupuesto", pre.ID); err != nil || again.ID != ped.ID {
		t.Fatalf("convertir dos veces devuelve el mismo: %v", err)
	}
	if ls, err := e.ListarCon(ctx, "linea_venta", api.ERPFiltro{}); err != nil || len(ls.Registros) != 1 || ls.Registros[0].Data["pedido"] != ped.ID ||
		entero(ls.Registros[0].Data["precio"]) != 121 || ls.Registros[0].Data["tipo_iva"] != "21" {
		t.Fatalf("líneas copiadas: %v %+v", err, ls)
	}
	if _, err := e.CambiarEstado(ctx, "pedido_venta", ped.ID, "confirmado"); err != nil {
		t.Fatal(err)
	}
	if fac, err := e.Convertir(ctx, "pedido_venta", ped.ID); err != nil || fac.Modelo != "factura_venta" {
		t.Fatalf("pedido → factura: %v %+v", err, fac)
	}

	dup, err := e.Duplicar(ctx, "contacto", cli.ID)
	if err != nil || dup.Etiqueta != "Acme (copia)" {
		t.Fatalf("duplicar: %v %+v", err, dup)
	}
	if err := e.Borrar(ctx, "contacto", dup.ID); err != nil {
		t.Fatal(err)
	}
	if pag, _ := e.ListarCon(ctx, "contacto", api.ERPFiltro{Papelera: true}); len(pag.Registros) != 1 {
		t.Fatalf("papelera: %+v", pag)
	}
	if err := e.Restaurar(ctx, "contacto", dup.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := e.Lote(ctx, "contacto", "estado", []string{cli.ID, dup.ID}, "pausada"); err != nil || n != 2 {
		t.Fatalf("lote: %v %d", err, n)
	}
	if rs, err := e.Buscar(ctx, "acme", 10); err != nil || len(rs) != 2 {
		t.Fatalf("buscar: %v %d", err, len(rs))
	}
}
