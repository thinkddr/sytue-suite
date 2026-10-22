// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # La portada, contra el arnés ERP real (Figtree, raíz 16 px, 2026-10-04)

// portadaArnes son los módulos del arnés que importan para medir: uno encendido con
// dependencias, uno apagado con ellas y uno encendido sin ninguna.
func portadaArnes(t *testing.T, ventana float32) *Bienvenida {
	b := NuevaBienvenida(traductor(t))
	b.Modulos = []Modulo{
		{Clave: "ayuda", Etiqueta: "Soporte", Dependencias: []string{"contactos"}, Modelos: []Modelo{{Clave: "ticket", Modulo: "ayuda", Etiqueta: "Tickets"}}},
		{Clave: "crm", Etiqueta: "CRM", Dependencias: []string{"contactos"}, Modelos: []Modelo{{Clave: "oportunidad", Modulo: "crm", Etiqueta: "Oportunidades"}}},
		{Clave: "facturacion", Etiqueta: "Facturación", Modelos: []Modelo{{Clave: api.ERPFacturaVenta, Modulo: "facturacion", Etiqueta: "Facturas"}}},
		{Clave: "rrhh", Etiqueta: "Empleados", Modelos: []Modelo{{Clave: "empleado", Modulo: "rrhh", Etiqueta: "Empleados"}}},
		{Clave: "contactos", Etiqueta: "Contactos", Modelos: []Modelo{{Clave: "contacto", Modulo: "contactos", Etiqueta: "Contactos"}}},
	}
	b.Activos = []string{"crm", "facturacion", "rrhh", "contactos"}
	b.Nombre, b.Ventana, b.Fase = "Ada Lovelace", ventana, Exito
	b.Tareas, b.ConPerfil, b.SabePerfil, b.VerObligaciones = 3, true, true, true
	b.Hoy = time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	b.Proximas = obligacionesArnes().Obligaciones[:3]
	r := api.ERPResumen{}
	r.Facturacion.TotalCentimos, r.Facturacion.TotalRegistros = 440696, 4
	r.Facturacion.PendienteCobroCentimos, r.Facturacion.FacturasPendientes = 302500, 1
	r.Facturacion.ProximoVencimiento = "2026-10-05"
	r.Contabilidad.TotalAsientos, r.Contabilidad.TotalDebe, r.Contabilidad.Cuadra = 17, 1234500, true
	b.Resumen = &r
	b.AlCambiar = func(Modulo, bool) {}
	return b
}

// TestLaPortadaMideLoDeLaWeb: cabecera, avisos, próximas obligaciones, resumen y las
// tarjetas de las áreas. Las sondas body854 y body318 fijan el ancho interior
// expresamente; los casos de pantalla completa se miden por separado en el arnés.
//
// VISTO EN ROJO (frontal, 2026-09-23): sumando el `mb-1` de la fila al `mt-3` de los
// modelos en un área sin «Requiere» —en vez de fundirlos—, «Empleados» mide 139,75 y
// no 136,25. Y con la tecla ⌘K en Figtree en vez de la mono del navegador, la lupa
// mide 73,27 y no 68,66.
func TestLaPortadaMideLoDeLaWeb(t *testing.T) {
	b := portadaArnes(t, 1280)
	_, cab := b.cabecera(tema.Claro, 854)
	cerca(t, "la cabecera", cab, 83)
	_, aviso := b.aviso(tema.Claro, 854, "Tus tareas", "Tienes 3 tareas para hoy o vencidas.", "Ver tareas", tema.Color("warning", tema.Claro), nil)
	cerca(t, "el aviso de las tareas", aviso, 74)
	b.Proximas = append(b.Proximas, b.Proximas[1])
	_, prox := b.proximas(tema.Claro, 854)
	cerca(t, "las próximas obligaciones", prox, 253)
	_, res := b.resumen(tema.Claro, 854)
	cerca(t, "el resumen", res, 132)
	cerca(t, "una tarjeta del resumen", AltoKPI, 108)
	_, siempre := b.siempreActiva(tema.Claro, 854)
	cerca(t, "Obligaciones, siempre activa", siempre, 76)
	w := float32(276.65625)
	cerca(t, "Soporte (con «Requiere»)", b.tarjeta(tema.Claro, b.Modulos[0], w).alto, 177)
	cerca(t, "Empleados (sin «Requiere»)", b.tarjeta(tema.Claro, b.Modulos[3], w).alto, 155)
	antes := textoTecla
	textoTecla = "⌘K" // la medida de la web, que es la de un Mac
	cerca(t, "la lupa con ⌘K", NuevoBotonLupa("", nil).MinSize().Width, 76.40625)
	textoTecla = antes

	m := portadaArnes(t, 360)
	_, cab = m.cabecera(tema.Claro, 318)
	cerca(t, "la cabecera en el móvil", cab, 123)
	_, aviso = m.aviso(tema.Claro, 318, "Tus tareas", "Tienes 3 tareas para hoy o vencidas.", "Ver tareas", tema.Color("warning", tema.Claro), nil)
	cerca(t, "el aviso en el móvil", aviso, 118)
	_, res = m.resumen(tema.Claro, 318)
	cerca(t, "el resumen en el móvil", res, 372)
}

// TestLaPortadaSePinta guarda la portada a los dos anchos para mirarla, y comprueba que
// lleva lo de la semana y que «Encender» llama a quien cambia el área.
func TestLaPortadaSePinta(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.Abrir(Modelo{})
	b := p.inicio
	arnes := portadaArnes(t, 1280)
	p.Catalogo.Modulos, p.Catalogo.Activos = arnes.Modulos, arnes.Activos
	p.Organizacion, p.SabeOrg, p.TareasHoy = &api.ERPOrganizacion{Pais: "ES"}, true, 3
	b.Nombre, b.Resumen, b.Proximas, b.Hoy = arnes.Nombre, arnes.Resumen, arnes.Proximas, arnes.Hoy
	var cambio string
	b.AlCambiar = func(m Modulo, activo bool) {
		if activo {
			cambio = m.Clave
		}
	}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-portada.png")
	textos := textosDe(b.Objeto(tema.Claro))
	for _, s := range []string{"Hola, Ada Lovelace", "Tus tareas", "Ver el calendario completo", "4406,96", "Requiere: Contactos", "Siempre activo"} {
		if !contiene(textos, s) {
			t.Errorf("la portada no dice «%s»", s)
		}
	}
	recorrer(b.tarjeta(tema.Claro, arnes.Modulos[0], 277.66).o, func(x fyne.CanvasObject) {
		if bt, ok := x.(*ui.Boton); ok && bt.Texto == p.t("welcome.enable") {
			bt.Tapped(nil)
		}
	})
	if cambio != "ayuda" {
		t.Errorf("«Encender» en Soporte no pide encenderla: %q", cambio)
	}
	w.Resize(fyne.NewSize(360, 740))
	p.Ventana = fyne.NewSize(360, 740)
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-portada-movil.png")
}
