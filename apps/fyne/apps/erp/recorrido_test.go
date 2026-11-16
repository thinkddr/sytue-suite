// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	tamano := fyne.NewSize(1280, 800)
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		tamano = fyne.NewSize(360, 640)
	}
	p, w := pantallaArnes(t, tamano)
	t.Cleanup(w.Close)
	if p.lista.Fase != Exito || len(p.lista.Pagina.Registros) == 0 {
		t.Fatal("las facturas deben estar cargadas antes de girar")
	}
	// El punto de venta, con su catálogo y una caja: el recorrido llega a él por el carril
	// y pulsa productos, cantidades, categorías y el cobro (sin servicio, no envía nada).
	p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
	p.tpv.Productos, p.tpv.Sesiones, p.tpv.Sesion = productosTPV(), sesionesTPV(), sesionTPVArnes
	p.Repintar()
	montaje.ComprobarRotacion(t, w, "erp", iconos.ERP, true)
	recorrido.Comprobar(t, w, 400)
	// Y otra pasada que EMPIEZA en el punto de venta: la de arriba se queda en las
	// primeras pantallas del carril. En el teléfono no hay carril, así que el recorrido
	// —que pulsa lo último del árbol— se queda en la vista.
	telefono := fyne.NewSize(400, 1600)
	w.Resize(telefono)
	p.Ventana = telefono
	p.IrA(VistaTPV)
	inf := recorrido.Comprobar(t, w, 400)
	if !slices.ContainsFunc(inf.Lista, func(s string) bool { return strings.HasPrefix(s, "*erp.TeselaTPV") }) {
		t.Errorf("el recorrido del TPV no llegó a ningún producto: %v", inf.Lista)
	}
}
