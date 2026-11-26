// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El inicio del canal es `InicioDelCanal.tsx`: el bloque «Este es el principio de
// #canal» que Slack y Teams pintan al llegar al principio del historial, y no solo con
// el canal vacío. Dice quién creó el canal y cuándo, de qué va y su tema, y ofrece
// «Añadir personas» y «Editar el tema» a quien puede usarlas. En un directo es
// «Conversación con Ana», con su presencia.

// LadoIlustracionInicio es el `size={72}` de la ilustración del bloque: pequeña,
// porque encabeza la conversación y no es un estado vacío a pantalla.
const LadoIlustracionInicio float32 = 72

// InicioCanal es lo que necesita el bloque. El historial lo pinta cuando no quedan
// mensajes anteriores (`hasOlder=false`); nil, no se pinta.
type InicioCanal struct {
	Canal Canal
	// Yo es quien mira: si creó él el canal, se dice «Creaste…».
	Yo string
	// NombreCreador sale del roster; vacío, se dice «alguien».
	NombreCreador string
	// AlAnadirPersonas abre el panel de personas en «Añadir»; nil, quien mira no puede.
	AlAnadirPersonas func()
	// AlEditarTema abre el campo del tema en la cabecera; nil, quien mira no puede.
	AlEditarTema func()
}

// LineaDeCreacion es `lineaDeCreacion`: «Ana creó este canal el 3 de mayo de 2026.».
// Sin fecha de creación no dice nada antes que inventarse una.
func LineaDeCreacion(t *i18n.Translator, l i18n.Locale, c Canal, yo, nombre string) string {
	if c.Creado.IsZero() {
		return ""
	}
	fecha := i18n.FormatDateLong(l, c.Creado.In(time.Local))
	if c.CreadoPor != "" && c.CreadoPor == yo {
		return t.T("message.introCreatedByYou", i18n.Vars{"date": fecha})
	}
	if strings.TrimSpace(nombre) == "" {
		nombre = t.T("message.introSomeone", nil)
	}
	return t.T("message.introCreatedBy", i18n.Vars{"name": nombre, "date": fecha})
}

// TituloInicio es el `<h2>` del bloque.
func TituloInicio(t *i18n.Translator, c Canal) string {
	if c.Clase == Directo {
		return t.T("message.introDm", i18n.Vars{"name": c.Nombre})
	}
	return t.T("message.channelStart", i18n.Vars{"channel": c.Nombre})
}

// NuevoBloqueInicio construye el bloque. Con `sinMensajes` añade la invitación a
// escribir el primero, porque ocupa el sitio del estado vacío de antes.
func NuevoBloqueInicio(t *i18n.Translator, l i18n.Locale, in InicioCanal, sinMensajes bool) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := in.Canal
	directo := c.Clase == Directo
	parrafo := func(s, token string) *ui.Parrafo {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, token, v)
	}

	var marca fyne.CanvasObject
	if directo {
		marca = ui.NuevoAvatar(c.Nombre).Con(ui.AvatarXL)
	} else {
		// El raster lleva su tamaño y no un mínimo: la rejilla se lo da, para que el
		// flujo le reserve el sitio.
		ilus := iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracionInicio).Objeto()
		marca = container.NewGridWrap(ilus.Size(), ilus)
	}
	titulo := ui.NuevoParrafo(TituloInicio(t, c), tema.TextoXL, ui.SemiNegrita, "text", v)
	titulo.PartirPalabras = true
	partes := []fyne.CanvasObject{container.New(ui.Flujo{}, marca), titulo}

	if directo {
		// La presencia solo en un 1:1: en un directo de grupo no hay «el otro».
		if c.Personas == 0 && (!c.SinPresencia || c.Presencia != ui.SinPresencia) {
			p := t.T(clavePresencia(ponerDesconectado(c.Presencia)), nil)
			if c.EstadoPar != "" {
				p += " · " + c.EstadoPar
			}
			partes = append(partes, parrafo(p, "text-muted"))
		}
		partes = append(partes, parrafo(t.T("message.introDmStart", i18n.Vars{"name": c.Nombre}), "text-muted"))
	} else {
		if s := LineaDeCreacion(t, l, c, in.Yo, in.NombreCreador); s != "" {
			partes = append(partes, parrafo(s, "text-muted"))
		}
		if c.Proposito != "" {
			partes = append(partes, parrafo(c.Proposito, "text"))
		}
		if c.Tema != "" {
			partes = append(partes, parrafo(t.T("channel.topic", nil)+": "+c.Tema, "text-muted"))
		}
	}
	if sinMensajes {
		partes = append(partes, parrafo(t.T("message.writeFirst", nil), "text-muted"))
	}
	if !directo && (in.AlAnadirPersonas != nil || in.AlEditarTema != nil) {
		var botones []fyne.CanvasObject
		if in.AlAnadirPersonas != nil {
			botones = append(botones, ui.NuevoBoton(t.T("members.addPeople", nil), in.AlAnadirPersonas).
				Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Mas, 14))
		}
		if in.AlEditarTema != nil {
			rotulo := t.T("channel.addTopic", nil)
			if c.Tema != "" {
				rotulo = t.T("channel.editTopic", nil)
			}
			botones = append(botones, ui.NuevoBoton(rotulo, in.AlEditarTema).
				Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Lapiz, 14))
		}
		partes = append(partes, container.New(ui.Flujo{Hueco: tema.Espacio(2)}, botones...))
	}
	columna := container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)
	// `px-2 pt-6 pb-4`
	return container.New(ui.Margen{
		Izq: tema.Espacio(2), Arr: tema.Espacio(6), Der: tema.Espacio(2), Abj: tema.Espacio(4),
	}, columna)
}
