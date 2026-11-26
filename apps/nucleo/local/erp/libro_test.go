// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// n43 es un extracto con un cobro de 121,00 € de Bolt y una comisión de 5,00 €.
func n43() []byte {
	var b strings.Builder
	linea := func(s string) { b.WriteString(fmt.Sprintf("%-80s", s) + "\r\n") }
	linea("11" + "2100" + "0001" + "0123456789" + "260901" + "260930" + "2" + "00000000100000" + "978" + "3" + "TALLER RUIZ SL")
	linea("22" + "    " + "0001" + "260910" + "260910" + "04" + "000" + "2" + "00000000012100" + "0000000000" + "000000000000" + "FV-2026-00001")
	linea("23" + "01" + fmt.Sprintf("%-38s", "TRANSF DE BOLT ESPA\xd1A SL") + fmt.Sprintf("%-38s", "PAGO FACTURA"))
	linea("22" + "    " + "0001" + "260915" + "260915" + "17" + "000" + "1" + "00000000000500" + "0000000000" + "000000000000" + "")
	linea("23" + "01" + fmt.Sprintf("%-38s", "COMISION MANTENIMIENTO") + fmt.Sprintf("%-38s", ""))
	linea("33" + "2100" + "0001" + "0123456789" + "00001" + "00000000000500" + "00001" + "00000000012100" + "2" + "00000000111600" + "978")
	linea("88" + "999999999999999999" + "000006")
	return []byte(b.String())
}

func codigo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code()
	}
	return ""
}

func TestLibroBancoYCierre(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()
	alm.ahora = func() time.Time { return time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC) }
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Taller Ruiz", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Bolt España SL", "tipo": "organizacion"})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-09-05", "base": 10000, "iva": 2100, "total": 12100})
	if err != nil {
		t.Fatal(err)
	}
	if ref, err := e.Asentar(ctx, "factura_venta", fac.ID); err != nil || ref != "VENTAS-2026-00001" {
		t.Fatalf("asentar: %v %q", err, ref)
	}
	if _, err := e.Asentar(ctx, "contacto", cli.ID); codigo(err) != "no_contable" {
		t.Fatalf("un contacto no va al libro: %v", err)
	}

	// El extracto: el cobro se casa solo con su factura y la comisión se explica sola.
	res, err := e.ImportarExtracto(ctx, n43())
	if err != nil || res.Nuevos != 2 || res.Casados != 1 || res.Explicados != 1 || res.Pendientes != 0 || len(res.Cuentas) != 1 {
		t.Fatalf("importar el Norma 43: %v %+v", err, res)
	}
	if r, _ := e.Obtener(ctx, "factura_venta", fac.ID); r.Estado != "pagado" {
		t.Fatalf("la factura cobrada queda pagada: %q", r.Estado)
	}
	if res, err := e.ImportarExtracto(ctx, n43()); err != nil || res.Repetidos != 2 || res.Nuevos != 0 {
		t.Fatalf("reimportar no duplica: %v %+v", err, res)
	}
	b, err := e.Banco(ctx)
	if err != nil || len(b.Pendientes) != 0 || len(b.Hechas) != 2 || len(b.Motivos) == 0 || b.Motivos[0].Rotulo == "" {
		t.Fatalf("banco: %v %+v", err, b)
	}
	var cobro api.ERPMovimientoBanco
	for _, h := range b.Hechas {
		if h.Importe > 0 {
			cobro = h
		}
	}
	if cobro.FacturaID != fac.ID || cobro.FacturaRef != "FV-2026-00001" {
		t.Fatalf("el cobro dice su factura: %+v", cobro)
	}

	// Deshacer y volver a casar a mano.
	if err := e.ReabrirMovimiento(ctx, cobro.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.ReabrirMovimiento(ctx, cobro.ID); codigo(err) != "no_justificado" {
		t.Fatalf("reabrir dos veces: %v", err)
	}
	b, _ = e.Banco(ctx)
	if len(b.Pendientes) != 1 || len(b.Pendientes[0].Candidatas) != 1 || !b.Pendientes[0].Candidatas[0].Segura {
		t.Fatalf("la tarea propone su factura: %+v", b.Pendientes)
	}
	if err := e.ConciliarMovimiento(ctx, cobro.ID, fac.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.ExplicarMovimiento(ctx, cobro.ID, "otro"); codigo(err) != "ya_justificado" {
		t.Fatalf("explicar uno ya casado: %v", err)
	}

	// Libro: la factura, el cobro, la comisión y el cobro anulado al reabrir.
	libro, err := e.Libro(ctx, 50)
	if err != nil || len(libro) != 4 {
		t.Fatalf("libro: %v %d", err, len(libro))
	}
	anulados := 0
	for _, a := range libro {
		if a.Estado == "anulado" {
			anulados++
		}
		if len(a.Apuntes) < 2 {
			t.Fatalf("el libro trae los apuntes: %+v", a)
		}
	}
	bal, err := e.Balance(ctx)
	if err != nil || bal.TotalDebe != bal.TotalHaber || bal.TotalDebe != 12100+12100+500 {
		t.Fatalf("balance: %v %+v", err, bal)
	}
	if r, err := e.Resumen(ctx); err != nil || r.Contabilidad.TotalAsientos != 3 || !r.Contabilidad.Cuadra {
		t.Fatalf("resumen: %v %+v", err, r.Contabilidad)
	}
	if anulados != 1 {
		t.Fatalf("anulados: %d", anulados)
	}
	as, err := e.AsientosDe(ctx, "factura_venta", fac.ID)
	if err != nil || len(as) != 1 || as[0].Ref != "VENTAS-2026-00001" || !strings.HasPrefix(as[0].Fecha, "2026-09-05") {
		t.Fatalf("asientos de la factura: %v %+v", err, as)
	}

	// Cierre: no se asienta ni se anula en lo cerrado; reabrir pide motivo.
	if err := e.Cerrar(ctx, "2026-12-31"); err == nil {
		t.Fatal("cerrar un período que no ha acabado")
	}
	if err := e.Cerrar(ctx, "2026-09-30"); err != nil {
		t.Fatal(err)
	}
	if err := e.Cerrar(ctx, "2026-08-31"); codigo(err) != "cierre_hacia_atras" {
		t.Fatalf("cerrar hacia atrás: %v", err)
	}
	f2, _ := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-09-20", "base": 100})
	if _, err := e.Asentar(ctx, "factura_venta", f2.ID); codigo(err) != "periodo_cerrado" {
		t.Fatalf("asentar en lo cerrado: %v", err)
	}
	if err := e.AnularAsiento(ctx, as[0].ID, "error"); codigo(err) != "periodo_cerrado" {
		t.Fatalf("anular en lo cerrado: %v", err)
	}
	if err := e.Reabrir(ctx, "", ""); estado(err) != http.StatusBadRequest {
		t.Fatalf("reabrir sin motivo: %v", err)
	}
	if err := e.Reabrir(ctx, "", "Corregir septiembre"); err != nil {
		t.Fatal(err)
	}
	c, err := e.Cierre(ctx)
	if err != nil || c.CerradoHasta != "" || len(c.Historial) != 2 || c.Historial[0].Accion != "reapertura" || c.Historial[1].Hasta == nil {
		t.Fatalf("cierre: %v %+v", err, c)
	}
	if err := e.AnularAsiento(ctx, as[0].ID, "error"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Asentar(ctx, "factura_venta", fac.ID); err != nil {
		t.Fatalf("anulado el asiento, el documento se puede volver a asentar: %v", err)
	}
}

func TestEmitirExigeElCuadre(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()
	cli, _ := e.Crear(ctx, "contacto", map[string]any{"nombre": "Ana", "tipo": "persona"})
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-03-01", "base": 1000, "iva": 300})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Crear(ctx, "linea_factura", map[string]any{"factura": fac.ID, "descripcion": "x", "cantidad": "1", "precio_con_iva": 1210, "tipo_iva": "21"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.CambiarEstado(ctx, "factura_venta", fac.ID, "confirmado")
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusUnprocessableEntity || !strings.Contains(ae.Message(), "tienen que ser los de sus líneas") {
		t.Fatalf("emitir una factura que no cuadra: %v", err)
	}
	if n, err := e.Lote(ctx, "factura_venta", "estado", []string{fac.ID}, "pagado"); estado(err) != http.StatusUnprocessableEntity || n != 0 {
		t.Fatalf("en lote tampoco: %v %d", err, n)
	}
	if _, err := e.Actualizar(ctx, "factura_venta", fac.ID, map[string]any{"cliente": cli.ID, "fecha": "2026-03-01", "base": 1000, "iva": 210}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarEstado(ctx, "factura_venta", fac.ID, "confirmado"); err != nil {
		t.Fatalf("con la cabecera de sus líneas, sí: %v", err)
	}
	// Anular no se comprueba; volver a emitir desde anulada, sí.
	if _, err := e.CambiarEstado(ctx, "factura_venta", fac.ID, "anulado"); err != nil {
		t.Fatal(err)
	}
}
