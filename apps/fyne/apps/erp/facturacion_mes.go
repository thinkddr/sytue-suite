// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La facturación por mes de la portada (`FacturacionMes.tsx` y `facturacionMensual.ts`)
//
// Los doce últimos meses en barras, con el total encima, a lo ancho de la rejilla del
// resumen (`sm:col-span-3`, a `gap-3` de las cifras). Las cifras dicen cuánto; esto
// dice cuándo. Se pide al motor de informes (`/informes/datos`), el de los tableros de
// Sytue AI, que ya agrupa por mes, suma y respeta permisos y módulos apagados.
//
//	tarjeta   `Card p-4`: título `text-xs font-medium` text-muted y el total `text-xs`
//	          text-subtle a la derecha, `mb-3`
//	barras    `BarChart` de 120 con la fila de meses (`mt-1`, `text-xs` text-subtle)
//	recortada `mt-2 text-xs` text-subtle

// MesesFacturacion son los meses del gráfico: el actual y los once anteriores.
const MesesFacturacion = 12

// MesFacturado es un mes del gráfico: su clave («2026-11») y lo facturado, en UNIDADES
// (no en céntimos), que es como llegan los importes de un informe.
type MesFacturado struct {
	Mes   string
	Valor float64
}

// FacturacionMensual es lo que la portada sabe de la facturación por mes.
type FacturacionMensual struct {
	// Cargando: la petición está en camino y las barras se esbozan.
	Cargando bool
	// Serie son los doce meses; nil sin cargar o si no llegó (sin permiso, servicio
	// caído): entonces la tarjeta no se pinta, como en la web.
	Serie []MesFacturado
	// Recortada: calculada solo con las primeras filas.
	Recortada bool
}

// claveMes es la clave con la que el servicio agrupa un mes (`fecha:mes`).
func claveMes(t time.Time) string { return t.Format("2006-01") }

// MesesHasta son los n meses que acaban en el de hoy, del más antiguo al actual.
func MesesHasta(hoy time.Time, n int) []string {
	out := make([]string, n)
	for i := range n {
		// El día 1: con el 31, restar meses saltaría al siguiente (31 de febrero).
		out[i] = claveMes(time.Date(hoy.Year(), hoy.Month()-time.Month(n-1-i), 1, 0, 0, 0, 0, hoy.Location()))
	}
	return out
}

// InformeFacturacionMensual es `informeFacturacionMensual`: las facturas de venta no
// anuladas desde el primer día del mes más antiguo, sumando el total por mes. Las
// anuladas fuera, como en la cifra «Ventas» del resumen. La fecha de corte la pone el
// cliente: el servicio cuenta los meses en UTC, y a las 00:30 del día 1 en Madrid
// «inicio_mes-11» perdería el mes en curso.
func InformeFacturacionMensual(titulo string, hoy time.Time) api.ERPInforme {
	desde := MesesHasta(hoy, MesesFacturacion)[0] + "-01"
	consulta, _ := json.Marshal(map[string]any{
		"fuente": api.ERPFacturaVenta,
		"filtros": []map[string]string{
			{"campo": "estado", "op": "!=", "valor": "anulado"},
			{"campo": "fecha", "op": ">=", "valor": desde},
		},
		"agrupar": "fecha:mes",
		"medida":  "suma:total",
		"orden":   "grupo",
	})
	return api.ERPInforme{Titulo: titulo, Partes: []api.ERPParteInforme{
		{Titulo: titulo, Vista: api.ERPVistaBarras, Consulta: consulta},
	}}
}

// SerieMensual es `serieMensual`: los doce meses con su importe. Un mes sin facturas
// es una barra a cero, no un hueco; lo que cae fuera de la ventana no se pinta.
func SerieMensual(t api.ERPTablaInforme, hoy time.Time) []MesFacturado {
	porMes := map[string]float64{}
	for _, f := range t.Filas {
		n, ok := numero(celdaDeFila(f, 1))
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			continue
		}
		porMes[fmt.Sprint(celdaDeFila(f, 0))] += n
	}
	meses := MesesHasta(hoy, MesesFacturacion)
	out := make([]MesFacturado, len(meses))
	for i, m := range meses {
		out[i] = MesFacturado{Mes: m, Valor: porMes[m]}
	}
	return out
}

// TotalCentimos es la suma de la serie en céntimos, sin el error de los decimales.
func TotalCentimos(s []MesFacturado) int64 {
	var n int64
	for _, m := range s {
		n += int64(math.Round(m.Valor * 100))
	}
	return n
}

// cargarFacturacion pide la facturación por mes. Lo que falle deja la serie en nil y
// la tarjeta no se pinta: la portada sigue sirviendo con las cifras.
func (p *Pantalla) cargarFacturacion() {
	b := p.inicio
	if p.svc == nil || !conFacturasVenta(p.Catalogo.Modulos, p.Catalogo.Activos) {
		return
	}
	hoy := b.hoy()
	inf := InformeFacturacionMensual(p.tr.T("dashboard.monthlyTitle", nil), hoy)
	b.Facturacion = FacturacionMensual{Cargando: true}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, err := p.svc.DatosInforme(ctx, inf)
		enHilo(func() {
			b.Facturacion = FacturacionMensual{}
			if err == nil && len(datos) > 0 && datos[0].Error == "" {
				b.Facturacion = FacturacionMensual{Serie: SerieMensual(datos[0], hoy), Recortada: datos[0].Recortada}
			}
			p.Repintar()
		})
	})
}

// --- La tarjeta -------------------------------------------------------------------

// ConFacturasVenta dice si la portada pinta la facturación por mes: solo con las
// facturas de venta encendidas, que sin ellas el servicio diría «fuente desconocida».
func (b *Bienvenida) ConFacturasVenta() bool { return conFacturasVenta(b.Modulos, b.Activos) }

func conFacturasVenta(modulos []Modulo, activos []string) bool {
	for _, m := range modulos {
		if !slices.Contains(activos, m.Clave) {
			continue
		}
		if slices.ContainsFunc(m.Modelos, func(x Modelo) bool { return x.Clave == api.ERPFacturaVenta }) {
			return true
		}
	}
	return false
}

// hoy es la fecha de referencia de la portada.
func (b *Bienvenida) hoy() time.Time {
	if b.Hoy.IsZero() {
		return time.Now()
	}
	return b.Hoy
}

// tarjetaFacturacion es `FacturacionMes`: la tarjeta con su título, el total y las
// barras de los doce meses (o su esbozo mientras llegan). nil si no hay que pintarla.
func (b *Bienvenida) tarjetaFacturacion(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	f := b.Facturacion
	if !b.ConFacturasVenta() || (!f.Cargando && f.Serie == nil) {
		return nil, 0
	}
	dentro := max(ancho-2*tema.Borde-2*TarjetaPX, 0)
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	titulo := b.t("dashboard.monthlyTitle")

	// La cabecera: el título a la izquierda y el total, en su línea base, a la derecha.
	var der fyne.CanvasObject
	var wDer float32
	if !f.Cargando {
		total := TotalCentimos(f.Serie)
		s := b.t("dashboard.monthlyNone")
		if total > 0 {
			s = b.t("dashboard.monthlyTotal", i18n.Vars{"amount": i18n.FormatCurrency(b.loc, total)})
		}
		der, wDer = textoTabular(s, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	}
	tt := ui.Texto(titulo, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	ui.Truncar(tt, titulo, tema.TextoXS, ui.Medio, max(dentro-wDer-HuecoTarjetas, 0))
	y := x0
	l.texto(tt, x0, y)
	if der != nil {
		l.poner(der, x0+dentro-wDer, y, wDer, tema.TextoXS.Linea)
	}
	y += tema.TextoXS.Linea + TituloParteMB

	if f.Cargando {
		// El mismo alto que las barras y su fila de meses: nada salta al llegar.
		l.poner(ui.NuevoEsqueletoFraccion(1, AltoBarras), x0, y, dentro, AltoBarras)
		y += AltoBarras + AltoEjeBarras
	} else {
		puntos := make([]PuntoBarras, len(f.Serie))
		for i, m := range f.Serie {
			t, _ := time.Parse("2006-01", m.Mes)
			puntos[i] = PuntoBarras{Rotulo: i18n.FormatMonthYear(b.loc, t), Eje: i18n.FormatMonthShort(b.loc, t), Valor: m.Valor}
		}
		barras := NuevasBarras(titulo, puntos)
		barras.Formato = func(n float64) string { return i18n.FormatCurrency(b.loc, int64(math.Round(n*100))) }
		alto := barras.MinSize().Height
		l.poner(barras, x0, y, dentro, alto)
		y += alto
		if f.Recortada {
			y += RecortadaMT
			r := ui.Texto(b.t("ai.truncated"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
			ui.Truncar(r, r.Text, tema.TextoXS, ui.Normal, dentro)
			l.texto(r, x0, y)
			y += tema.TextoXS.Linea
		}
	}
	alto := y + TarjetaPX + tema.Borde
	fondo, _ := fondoConCaja(v, tema.Color("surface-raised", v), tema.Color("border", v), tema.SombraSM)
	// El fondo debajo de todo: el alto no se sabe hasta haber colocado lo de dentro.
	l.puestos = append([]puesto{{o: fondo, w: ancho, h: alto}}, l.puestos...)
	return l.objeto(alto)
}
