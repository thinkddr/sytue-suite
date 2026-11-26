// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// diasVencida es la regla de `vencimiento.ts`: solo una factura abierta, contando días de
// calendario; el día del vencimiento aún no es «vencido».
func TestDiasVencida(t *testing.T) {
	hoy := time.Date(2026, 11, 25, 9, 30, 0, 0, time.Local)
	f := func(estado, venc string) Registro {
		r := Registro{Estado: estado, Data: map[string]any{}}
		if venc != "" {
			r.Data["vencimiento"] = venc
		}
		return r
	}
	casos := []struct {
		r    Registro
		dias int
	}{
		{f("confirmado", "2026-11-20"), 5},
		{f("confirmado", "2026-11-25"), 0},
		{f("pendiente", "2026-12-01"), 0},
		{f("pagado", "2026-01-01"), 0},
		{f("borrador", "2026-01-01"), 0},
		{f("confirmado", ""), 0},
		{f("confirmado", "01/01/2026"), 0},
	}
	for _, c := range casos {
		if got := diasVencida(c.r, hoy); got != c.dias {
			t.Errorf("%s %v: %d días, quería %d", c.r.Estado, c.r.Data["vencimiento"], got, c.dias)
		}
	}
}

// Las líneas de la portada, del estado de cuenta y del mayor llevan las cifras del
// servicio con su formato y sus rótulos traducidos.
func TestLineasDeLaCartera(t *testing.T) {
	tr := traductor(t)
	b := NuevaBienvenida(tr)
	if b.lineasAntiguedad() != nil || len(b.lineasBajoMinimo()) != 0 {
		t.Fatal("sin datos, la portada no tiene tarjetas de cartera")
	}
	b.Antiguedad = &api.ERPAntiguedad{Total: 70000, Vencido: 60000, Tramos: []api.ERPTramoAntiguedad{
		{Tramo: "al_dia", Centimos: 10000}, {Tramo: "mas_90", Centimos: 40000},
	}}
	ls := b.lineasAntiguedad()
	if len(ls) != 3 || !strings.Contains(ls[0], "600") || !strings.Contains(ls[2], tr.T("aging.over90", nil)) {
		t.Errorf("antigüedad: %q", ls)
	}
	b.BajoMinimo = []api.ERPProductoBajoMinimo{{Nombre: "Tornillo", Stock: "50.5", Minimo: "100", Unidad: "ud"}}
	if ls := b.lineasBajoMinimo(); len(ls) != 1 || !strings.Contains(ls[0], "Tornillo") || !strings.Contains(ls[0], "50.5 ud") {
		t.Errorf("bajo mínimo: %q", ls)
	}
	if c := b.tarjetasCartera(0, 600); len(c) != 2 || c[0].alto <= 0 {
		t.Errorf("tarjetas: %d", len(c))
	}

	f := NuevaFicha(tr, Modelo{Clave: "contacto"}, &Registro{ID: "c1"}, nil, nil)
	f.EstadoCuenta = &api.ERPEstadoCuenta{
		Ventas:     api.ERPResumenCuenta{Facturado: 40000, Pagado: 30000, Pendiente: 10000, Vencido: 10000},
		Documentos: []api.ERPDocumentoCuenta{{Ref: "FV-1", Fecha: "2026-10-01", Estado: "confirmado", Total: 10000, DiasVencida: 24}},
	}
	ls = f.lineasEstadoCuenta()
	if len(ls) != 2 || !strings.HasPrefix(ls[0], tr.T("account.sales", nil)) || !strings.Contains(ls[1], "FV-1") {
		t.Errorf("estado de cuenta: %q", ls)
	}
	if strings.Contains(strings.Join(ls, "\n"), tr.T("account.purchases", nil)) {
		t.Error("un lado sin nada facturado no se enseña")
	}

	m := lineasMayor(tr, api.ERPMayor{SaldoInicial: 500, SaldoFinal: 1710, Apuntes: []api.ERPApunteMayor{
		{Fecha: "2026-11-02", Concepto: "Factura FV-1", Debe: 1210, Saldo: 1710},
	}})
	if len(m) != 3 || !strings.Contains(m[1], "Factura FV-1") || !strings.Contains(m[2], "17,10") {
		t.Errorf("mayor: %q", m)
	}
}

// Ctrl+S (⌘S) guarda la ficha desde un campo; otro atajo sigue al editor, y quien solo
// lee no guarda nada.
func TestCtrlSGuardaLaFichaDesdeUnCampo(t *testing.T) {
	tr := traductor(t)
	f := NuevaFicha(tr, facturaArnes(), nil, nil, nil)
	guardadas := 0
	f.AlGuardar = func() { guardadas++ }
	ctrlS := &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault}
	var e *ui.Entrada
	for _, k := range f.controles {
		if k.entrada != nil {
			e = k.entrada
			break
		}
	}
	if e == nil || e.AlAtajo == nil {
		t.Fatal("los campos de la ficha no atienden Ctrl+S")
	}
	e.TypedShortcut(ctrlS)
	if guardadas != 1 {
		t.Fatalf("Ctrl+S en un campo: %d guardados", guardadas)
	}
	if e.AlAtajo(&fyne.ShortcutCopy{}) {
		t.Error("copiar se lo queda la ficha en vez del editor")
	}
	f.SoloLectura = true
	e.TypedShortcut(ctrlS)
	if guardadas != 1 {
		t.Error("una ficha de solo lectura se guarda con Ctrl+S")
	}
	if !slices.ContainsFunc(gruposDelERP[1].filas, func(x filaDeAtajo) bool { return x.descripcion == "shortcuts.save" }) {
		t.Error("la hoja de atajos no enseña Ctrl+S")
	}
}
