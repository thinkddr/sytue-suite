// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La tarjeta de VERI*FACTU (`TarjetaVerifactu` de Verifactu.tsx)
//
// Encima de las facturas de cliente de una organización española: qué es, activarlo, y
// cómo van los envíos a la Agencia Tributaria. Medido en el arnés (1280×800 y 360×740,
// 2026-09-23): `mx-6 mt-4` (`mx-4` en el móvil), `px-4 py-3`; la fila del título
// (17,5 con insignias, `gap-2`, los enlaces empujados a la derecha); activado, los
// recuentos (`mt-1`, `gap-x-3`): 58; sin activar, la frase (`leading-relaxed`,
// `max-w-prose` 437,47) y «Activar VERI*FACTU» (`xs`) con `space-y-2`: 126,19.

// Medidas de la tarjeta.
const (
	TarjetaVFPX float32 = 4 * 0.25 * tema.Rem
	TarjetaVFPY float32 = 3 * 0.25 * tema.Rem
	// TarjetaVFMT es el `mt-4`; TarjetaVFMX el `md:mx-6` y TarjetaVFMXMovil el `mx-4`.
	TarjetaVFMT      float32 = 4 * 0.25 * tema.Rem
	TarjetaVFMX      float32 = 6 * 0.25 * tema.Rem
	TarjetaVFMXMovil float32 = 4 * 0.25 * tema.Rem
	// ProsaVF es el `max-w-prose` de la frase, medido.
	ProsaVF float32 = 499.96875
)

// relajadoXS es `text-xs leading-relaxed`: 1,625 del tamaño.
var relajadoXS = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}

var motivosVF = map[string]string{
	"sin_perfil": "vf.reason.profile", "pais": "vf.reason.country", "foral": "vf.reason.foral",
	"igic_ipsi": "vf.reason.igic", "nif": "vf.reason.taxId", "sin_certificado": "vf.reason.certificate",
	"declaracion": "vf.reason.declaration",
}

// TarjetaVF es el estado de la tarjeta.
type TarjetaVF struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Estado *api.ERPEstadoVerifactu
	// Administrador es quien lo activa; los demás leen «Lo activa quien administra…».
	Administrador, Activando bool

	AlActivar, AlDeclaracion, AlExportar func()
}

// MargenTarjetaVF es el `mx-4 md:mx-6` a ese ancho de ventana.
func MargenTarjetaVF(ventana float32) float32 {
	if ventana > 0 && ventana < ui.CorteMD {
		return TarjetaVFMXMovil
	}
	return TarjetaVFMX
}

// Objeto arma la tarjeta a ese ancho (el de la caja, sin los márgenes de fuera).
func (t *TarjetaVF) Objeto(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	e := t.Estado
	tr := func(clave string, vars ...i18n.Vars) string {
		if len(vars) > 0 {
			return t.tr.T(clave, vars[0])
		}
		return t.tr.T(clave, nil)
	}
	dentro := ancho - 2*tema.Borde - 2*TarjetaVFPX
	x0, y0 := tema.Borde+TarjetaVFPX, tema.Borde+TarjetaVFPY
	l := &lienzo{}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radioTarjeta())
	l.caja(caja, 0, 0) // se redimensiona al final, cuando se sabe el alto
	// La fila del título: escudo, nombre, insignias, el hueco que empuja y los enlaces.
	escudo := iconos.Trazo(iconos.Escudo, 16, "text", v)
	ps := []pieza{
		piezaDe(escudo.Objeto(), fyne.NewSize(16, 16)),
		textoPieza("VERI*FACTU", tema.TextoSM, ui.SemiNegrita, "text", v),
	}
	insignia := func(s string, tono ui.Tono) {
		ps = append(ps, piezaDe(ui.NuevaInsignia(s, tono), ui.MedidaInsignia(s)))
	}
	if e.Activo && e.Desde != nil {
		insignia(tr("vf.activeSince", i18n.Vars{"date": i18n.FormatDate(t.loc, *e.Desde)}), ui.InsigniaExito)
	}
	if e.Entorno == "pruebas" && (e.Activo || e.Motivo == "") {
		insignia(tr("vf.testEnvironment"), ui.InsigniaAviso)
	}
	hueco := len(ps)
	// El hueco mide lo que un enlace para que, en la misma línea, queden a la misma altura.
	ps = append(ps, pieza{ancho: 0, alto: tema.TextoXS.Linea})
	enlace := func(s string, al func()) {
		en := NuevoPulsador(s, tema.TextoXS, ui.Normal, al)
		en.Subrayado, en.Radio = true, tema.RadioSM
		ps = append(ps, piezaDe(en, en.MinSize()))
	}
	enlace(tr("vf.declaration"), t.AlDeclaracion)
	if e.Activo {
		enlace(tr("vf.export"), t.AlExportar)
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoDos, HuecoDos, dentro)
	// `flex-1`: si los enlaces caben en la línea del hueco, se van a la derecha.
	if pos[hueco+1].Y == pos[hueco].Y {
		ultimo := len(ps) - 1
		desplaza := dentro - (pos[ultimo].X + an[ultimo])
		for i := hueco + 1; i < len(ps); i++ {
			pos[i].X += desplaza
		}
	}
	for i, p := range ps {
		if i == hueco {
			continue
		}
		l.pieza(p, x0+pos[i].X, y0+pos[i].Y)
	}
	y := y0 + ocupa.Height
	if e.Activo {
		// Los recuentos, en su tono.
		var cs []pieza
		cuenta := func(s, token string) { cs = append(cs, textoPieza(s, tema.TextoXS, ui.Normal, token, v)) }
		if e.Pendientes+e.ConErrores+e.Rechazadas == 0 {
			cuenta(tr("vf.allSent"), "text-muted")
		}
		if e.Pendientes > 0 {
			cuenta(tr("vf.pending", i18n.Vars{"count": e.Pendientes}), "text-muted")
		}
		if e.ConErrores > 0 {
			cuenta(tr("vf.withErrors", i18n.Vars{"count": e.ConErrores}), "warning-text")
		}
		if e.Rechazadas > 0 {
			cuenta(tr("vf.rejected", i18n.Vars{"count": e.Rechazadas}), "danger-text")
		}
		if e.ErrorEnvio != "" {
			cuenta(tr("vf.sendError", i18n.Vars{"error": e.ErrorEnvio}), "danger-text")
		}
		an, al := medidasDe(cs)
		pos, ocupa := Envolver(an, al, HuecoFaltasX, 0, dentro)
		y += HuecoUno
		l.piezas(cs, pos, x0, y)
		y += ocupa.Height
	} else {
		y += HuecoUno
		intro := ui.NuevoParrafo(tr("vf.intro"), relajadoXS, ui.Normal, "text-muted", v)
		w := min(dentro, ProsaVF)
		h := ui.AltoParrafo(intro, w)
		l.poner(intro, x0, y, w, h)
		y += h + HuecoDos
		switch {
		case e.Motivo != "":
			m := ui.NuevoParrafo(tr(motivosVF[e.Motivo]), tema.TextoXS, ui.Normal, "text-subtle", v)
			h := ui.AltoParrafo(m, dentro)
			l.poner(m, x0, y, dentro, h)
			y += h
		case t.Administrador:
			b := ui.NuevoBoton(tr("vf.activate"), t.AlActivar).Con(ui.Primario, ui.TallaXS)
			b.Cargando = t.Activando
			m := b.MinSize()
			l.poner(b, x0, y, m.Width, m.Height)
			y += m.Height
		default:
			m := ui.NuevoParrafo(tr("vf.adminOnly"), tema.TextoXS, ui.Normal, "text-subtle", v)
			h := ui.AltoParrafo(m, dentro)
			l.poner(m, x0, y, dentro, h)
			y += h
		}
	}
	alto := y + TarjetaVFPY + tema.Borde
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}
