// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// Las pestañas GENÉRICAS de la cinta: las que los tres productos comparten.
//
// Lo que NO está aquí está en el informe: la web tiene trece pestañas y esta ola cablea
// las tres comunes y la de Docus. Bibliografía, correspondencia, notas al final, campos
// de formulario y consultas a datos externos no entran — el criterio fue «lo que una
// persona usa de verdad al escribir un documento» y, sobre todo, que el cromo y el
// lienzo salgan clavados antes que trece pestañas a medias. Las pestañas que hay y dónde
// se ven las dice la taxonomía común con la web (`office.Pestanas`).

// Grupos arma la banda de una pestaña. Es pública porque la usan las pestañas PROPIAS
// de cada producto, que viven en su paquete.
func Grupos(objs ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(FilaGrupos{}, objs...)
}

// cuerpoInicio es la pestaña «Inicio» de Docus y Diapos (`Inicio.tsx`): deshacer,
// fuente y párrafo. Calcus la sustituye por la suya con [Registro.Sustituir], como la
// web tiene `InicioSheets.tsx` aparte.
func cuerpoInicio(c *Contexto) fyne.CanvasObject {
	f := c.formato()
	return Grupos(
		NuevoGrupo(c.Traducir("group.clipboard"),
			c.boton(iconos.EdDeshacer, "action.undo", false, !f.PuedeDeshacer, Deshacer),
			c.boton(iconos.EdRehacer, "action.redo", false, !f.PuedeRehacer, Rehacer),
		),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.font"),
			c.boton(iconos.EdNegrita, "format.bold", f.Negrita, false, Negrita),
			c.boton(iconos.EdCursiva, "format.italic", f.Cursiva, false, Cursiva),
			c.boton(iconos.EdSubrayado, "format.underline", f.Subrayado, false, Subrayado),
			c.boton(iconos.EdTachado, "format.strike", f.Tachado, false, Tachado),
			c.boton(iconos.EdSinFormato, "format.clear", false, false, SinFormato),
		),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.paragraph"),
			c.boton(iconos.EdAlinearIzq, "align.left", f.Alineacion == Izquierda, false, AlinearIzq),
			c.boton(iconos.EdAlinearCentro, "align.center", f.Alineacion == Centro, false, AlinearCen),
			c.boton(iconos.EdAlinearDer, "align.right", f.Alineacion == Derecha, false, AlinearDer),
			c.boton(iconos.EdJustificar, "align.justify", f.Alineacion == Justificado, false, Justificar),
			c.boton(iconos.EdListaPuntos, "list.bullet", f.Puntos, false, ListaPuntos),
			c.boton(iconos.EdListaNumeros, "list.number", f.Numeros, false, ListaNumero),
			c.boton(iconos.EdMenosSangria, "indent.decrease", false, false, MenosSangria),
			c.boton(iconos.EdMasSangria, "indent.increase", false, false, MasSangria),
		),
	)
}

// ZoomPresets son los del selector de «Vista» (`ZOOM_PRESETS`).
var ZoomPresets = []int{50, 75, 100, 125, 150, 200}

// AnchoSelectorZoom y AnchoSelectorLienzo son `width={72}` y `width={92}`.
const (
	AnchoSelectorZoom   float32 = 72
	AnchoSelectorLienzo float32 = 92
)

// presetCercano es `nearestPreset`: el selector enseña el peldaño más cercano al zoom.
func presetCercano(z int) int {
	mejor := ZoomPresets[0]
	for _, p := range ZoomPresets {
		if abs(p-z) < abs(mejor-z) {
			mejor = p
		}
	}
	return mejor
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// cuerpoVista es la pestaña «Vista» (`Vista.tsx`, §6.3): zoom, qué se enseña, pantalla
// completa y el tema del lienzo.
//
// Y el grupo «Macros» (`macros.go`), con el que acaba la pestaña como en la web.
func cuerpoVista(c *Contexto) fyne.CanvasObject {
	menos := NuevoBotonCinta(iconos.EdMenos, c.Traducir("view.zoomOut"), func() { c.AjustarZoom(-ZoomPaso) })
	menos.Inerte = c.Zoom <= ZoomMin
	mas := NuevoBotonCinta(iconos.EdMas, c.Traducir("view.zoomIn"), func() { c.AjustarZoom(+ZoomPaso) })
	mas.Inerte = c.Zoom >= ZoomMax
	opciones := make([]Opcion, len(ZoomPresets))
	for i, p := range ZoomPresets {
		opciones[i] = Opcion{Valor: strconv.Itoa(p), Rotulo: strconv.Itoa(p) + " %"}
	}
	zoom := NuevoSelectorCinta(c.Traducir("status.zoom"), AnchoSelectorZoom, opciones, strconv.Itoa(presetCercano(c.Zoom)),
		func(v string) {
			n, _ := strconv.Atoi(v)
			c.PonerZoom(n)
		})
	// «Ajustar al ancho» solo en el folio: la hoja es una rejilla fluida y la
	// presentación ya se encaja sola.
	ajustar := NuevoBotonCinta(iconos.EdTamanoPagina, c.Traducir("view.fitWidth"), c.AjustarAlAncho)
	ajustar.Inerte = c.Tipo != Documento || c.AjustarAlAncho == nil

	mostrar := []fyne.CanvasObject{}
	if c.Tipo == Documento { // la regla es del folio: la web ni la ofrece en los otros dos
		regla := NuevoBotonCinta(iconos.EdRegla, c.Traducir("view.ruler"), func() {
			c.Regla = !c.Regla
			c.refrescar()
		})
		regla.Activo = c.Regla
		mostrar = append(mostrar, regla)
	}
	ajustes := NuevoBotonCinta(iconos.EdPanelDerecho, c.Traducir("view.settingsPanel"), func() {
		c.Ajustes = !c.Ajustes
		if c.Ajustes && ui.EsTactil() {
			c.Panel = SinPanel
		}
		c.refrescar()
	})
	ajustes.Activo = c.Ajustes
	mostrar = append(mostrar, ajustes)

	completa := NuevoBotonCinta(iconos.EdCompleta, c.Traducir("view.fullscreen"), nil)
	if c.PantallaCompleta != nil {
		completa.AlPulsar = func() { c.PantallaCompleta(); c.refrescar() }
		completa.Activo = c.EnPantallaCompleta != nil && c.EnPantallaCompleta()
	} else {
		completa.Inerte = true
	}

	lienzo := "claro"
	if c.Sepia {
		lienzo = "sepia"
	}
	temaLienzo := NuevoSelectorCinta(c.Traducir("group.canvasTheme"), AnchoSelectorLienzo, []Opcion{
		{Valor: "claro", Rotulo: c.Traducir("canvasTheme.light")},
		{Valor: "sepia", Rotulo: c.Traducir("canvasTheme.sepia")},
	}, lienzo, func(v string) {
		c.Sepia = v == "sepia"
		if c.Lienzo != nil {
			if o := c.Lienzo.Objeto(); o != nil {
				o.Refresh()
			}
		}
		c.refrescar()
	})

	return Grupos(
		NuevoGrupo(c.Traducir("group.zoom"), menos, zoom, mas, ajustar),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.show"), mostrar...),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.screen"), completa),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.canvasTheme"), temaLienzo),
		SeparadorGrupos(),
		c.grupoMacros(),
	)
}

// ---- azúcar del contexto que solo usan las pestañas ----

// formato es `ctx.Lienzo.Formato()` con el hueco tapado: sin lienzo, el cero.
func (c *Contexto) formato() Formato {
	if c.Lienzo == nil {
		return Formato{}
	}
	return c.Lienzo.Formato()
}

func (c *Contexto) refrescar() {
	if c.Refrescar != nil {
		c.Refrescar()
	}
}

// boton es un botón de comando: manda al lienzo y se apaga en solo lectura.
//
// El `Inerte` en solo lectura va aquí, además del cortafuegos de [Contexto.Mandar]:
// uno impide que el comando llegue y el otro que el botón parezca disponible. Los dos
// hacen falta —un botón que se pulsa y no hace nada es como se concluye que la
// aplicación está rota, que es el motivo de la franja de aviso del marco.
func (c *Contexto) Boton(i iconos.Ed, rotulo string, activo bool, cmd Comando) *BotonCinta {
	return c.boton(i, rotulo, activo, false, cmd)
}

// boton es Boton con el `inerte` propio que algunos controles necesitan (deshacer y
// rehacer, que se apagan cuando no hay historial).
func (c *Contexto) boton(i iconos.Ed, rotulo string, activo, inerte bool, cmd Comando) *BotonCinta {
	b := NuevoBotonCinta(i, c.Traducir(rotulo), func() { c.Mandar(cmd, "") })
	b.Activo = activo
	b.Inerte = inerte || c.bloqueado(cmd)
	return b
}

// bloqueado dice si el registro común (`office.Comandos`) da ese comando por apagado ahora:
// edita y el documento es de solo lectura. Un comando que el registro no conoce se apaga
// con el permiso, por si acaso. (El cortafuegos de [Contexto.Mandar] no mira el registro:
// en solo lectura no pasa ninguno.)
func (c *Contexto) bloqueado(cmd Comando) bool {
	if def, ok := office.BuscarComando(string(cmd)); ok {
		return def.Bloqueado(c.SoloLectura)
	}
	return c.SoloLectura
}

// AbrirPanel alterna el panel del carril, como hace el clic en la web.
func (c *Contexto) AbrirPanel(p Panel) {
	if c.Panel == p && (!ui.EsTactil() || !c.Ajustes) {
		c.Panel = SinPanel
	} else {
		c.Panel = p
	}
	if ui.EsTactil() {
		c.Ajustes = false
	}
	c.refrescar()
}

// AjustarZoom mueve el zoom en pasos, acotado a [ZoomMin]..[ZoomMax].
func (c *Contexto) AjustarZoom(delta int) {
	c.PonerZoom(c.Zoom + delta)
}

// PonerZoom fija el zoom acotado.
func (c *Contexto) PonerZoom(z int) {
	c.Zoom = min(max(z, ZoomMin), ZoomMax)
	c.refrescar()
}
