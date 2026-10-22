// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mail es Sytue Mail en Fyne: el menú de carpetas, la lista de
// conversaciones y el lector, con paridad a píxel con `apps/frontend/apps/mail`.
//
// # De dónde salen los números
//
// De medir el DOM de producción (`corporate.sytue.eu/mail`, 1280×800, rem 14) con
// `test/e2e/medidas-web.mjs mail` el 2026-09-21. Cada constante de este paquete lleva
// escrito si es **medida** (salió de `getBoundingClientRect`) o de **hoja** (se deriva
// de las clases de Tailwind porque ninguna pantalla de producción la montaba).
//
// El barrido de ese día dio **522 elementos**. Es el número que dice que la medida
// vale: con la cookie caducada el cromo se pinta entero igual y la bandeja sale con
// 124, que es el estado de error midiéndose a sí mismo.
//
// # Lo que este paquete NO construye
//
// Ni un widget genérico. El armazón, el menú lateral, el botón, el campo, el avatar,
// el medidor y el estado vacío son de `apps/fyne/ui` y se importan. Lo único propio
// es lo que en la web también es propio de Mail: la FILA DE CONVERSACIÓN —que no es
// el `ListItem` de la suite, mide 75,75 contra 54 y lleva tres líneas— y la
// composición de las tres columnas.
package mail

// paridad-web: apps/frontend/apps/mail/src/pages/index.astro

import (
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Traductor es el de la suite con el catálogo común y el de Mail encima, que es el
// orden de `createAppTranslator` en la web.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.Mail())
}

// Estado son los SEIS estados en los que puede estar una vista. No es una
// simplificación: la web los tiene todos y cada uno pinta algo distinto —el esqueleto
// no es el vacío, y el vacío no es el error—. Confundir «no hay correos» con «no he
// podido preguntar» es lo que hace que alguien deje de buscar un correo que sí existe.
type Estado uint8

// Los seis.
const (
	// Cargando es el esqueleto con la geometría real de la fila, para que no salte el
	// layout al llegar los datos.
	Cargando Estado = iota
	// Listo hay datos y se pintan.
	Listo
	// Vacio la consulta fue bien y no hay nada.
	Vacio
	// Error la consulta falló; se ofrece reintentar.
	Error
	// SinConexion no hay red: lo que haya en memoria se enseña en SOLO LECTURA.
	SinConexion
	// SinPermiso el servidor contestó 403 (un buzón compartido sin ACL).
	SinPermiso
)

// Direccion es un `EmailAddress` del SDK.
type Direccion struct{ Nombre, Correo string }

// Mensaje es un `EmailSummary`: el sobre de un correo, sin el cuerpo.
type Mensaje struct {
	ID string
	// MessageID es la cabecera `Message-ID`: con ella se contesta dentro del hilo.
	MessageID string
	HiloID    string
	CarpetaID string
	Asunto    string
	// Extracto es el `preview` del servidor (~280 caracteres de texto plano).
	Extracto     string
	De, Para, CC []Direccion
	Recibido     time.Time
	Leido        bool
	Destacado    bool
	ConAdjunto   bool
	// Etiquetas son los `keywords` que son id de etiqueta.
	Etiquetas map[string]bool
	// EnHilo es el `threadCount` del listado: cuántos mensajes tiene el hilo en TODA
	// la cuenta, no en esta carpeta.
	EnHilo int
}

// Conversacion es un hilo plegado, igual que `groupByThread` en la web.
type Conversacion struct {
	HiloID string
	// Mensajes, más recientes primero (el orden de la lista).
	Mensajes []Mensaje
	// Ultimo define remitente, asunto y fecha de la fila.
	Ultimo Mensaje
	// Total es el tamaño del hilo ENTERO, que es lo que abre el lector.
	Total                          int
	SinLeer, Destacada, ConAdjunto bool
}

// Carpeta es un `Mailbox` de JMAP.
type Carpeta struct {
	ID, Nombre, Rol string
	SinLeer, Total  int
}

// Etiqueta es una etiqueta de la cuenta. `Color` es un TOKEN de la paleta de Mail, no
// un hex: lo traduce ColorEtiqueta.
type Etiqueta struct {
	ID, Nombre, Color string
	SinLeer           int
}

// AgruparPorHilo pliega la lista plana en conversaciones, conservando el orden de
// aparición: el hilo se sitúa por su mensaje más reciente, como Gmail.
//
// Un mensaje sin hilo es un hilo de uno (clave `msg:<id>`), y no todos bajo la clave
// vacía. Es `groupByThread` de `components/utils.ts`, línea por línea.
func AgruparPorHilo(mensajes []Mensaje) []Conversacion {
	porHilo := map[string]*Conversacion{}
	orden := make([]string, 0, len(mensajes))
	for _, m := range mensajes {
		clave := m.HiloID
		if clave == "" {
			clave = "msg:" + m.ID
		}
		c, ya := porHilo[clave]
		if !ya {
			c = &Conversacion{HiloID: clave, Ultimo: m}
			porHilo[clave] = c
			orden = append(orden, clave)
		}
		c.Mensajes = append(c.Mensajes, m)
		c.Total = max(c.Total, len(c.Mensajes), m.EnHilo)
		if !m.Leido {
			c.SinLeer = true
		}
		if m.Destacado {
			c.Destacada = true
		}
		if m.ConAdjunto {
			c.ConAdjunto = true
		}
	}
	out := make([]Conversacion, 0, len(orden))
	for _, k := range orden {
		out = append(out, *porHilo[k])
	}
	return out
}

// rolesTraducidos es `ROLE_KEYS` de utils.ts: las carpetas del SISTEMA se traducen y
// una creada por la persona conserva el nombre que le puso.
var rolesTraducidos = map[string]string{
	"inbox":   "folder.inbox",
	"sent":    "folder.sent",
	"drafts":  "folder.drafts",
	"trash":   "folder.trash",
	"junk":    "folder.junk",
	"archive": "folder.archive",
}

// RotuloCarpeta es `mailboxLabel`.
func RotuloCarpeta(t *i18n.Translator, nombre, rol string) string {
	if clave, hay := rolesTraducidos[rol]; hay {
		return t.T(clave, nil)
	}
	return nombre
}

// glifosDeRol es `ROLE_GLYPHS` de utils.ts. Son los glifos PROPIOS de Mail y no los
// del catálogo compartido a propósito: con los compartidos, Recibidos llevaba una
// CAMPANA y la Papelera un ASPA, que se lee «cerrar».
var glifosDeRol = map[string]string{
	"inbox":   SinLeerD,
	"sent":    ReenviarD,
	"trash":   PapeleraD,
	"junk":    SpamD,
	"archive": ArchivarD,
}

// GlifoCarpeta es `mailboxGlyph`. Borradores se queda en la carpeta neutra: no hay
// glifo de borrador y una carpeta miente menos que las tres rayas del menú.
func GlifoCarpeta(rol string) string {
	if g, hay := glifosDeRol[rol]; hay {
		return g
	}
	return CarpetaD
}

// NombreVisible es `displayName`: el nombre del primer remitente, o su dirección.
func NombreVisible(t *i18n.Translator, dirs []Direccion) string {
	if len(dirs) == 0 {
		return t.T("reader.unknownSender", nil)
	}
	if n := espaciosFuera(dirs[0].Nombre); n != "" {
		return n
	}
	return dirs[0].Correo
}

func espaciosFuera(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n') {
		j--
	}
	return s[i:j]
}

// FechaCorta es `shortDate`: hoy la hora, este año el día y el mes, y si no, con año.
// `ahora` se pasa para que el guardián no dependa del reloj.
func FechaCorta(l i18n.Locale, cuando, ahora time.Time) string {
	if cuando.IsZero() {
		return ""
	}
	ca, ma, da := cuando.Date()
	cb, mb, db := ahora.Date()
	switch {
	case ca == cb && ma == mb && da == db:
		return i18n.FormatTime(l, cuando)
	case ca == cb:
		return i18n.FormatDayMonth(l, cuando)
	}
	return i18n.FormatDate(l, cuando)
}

// FechaLarga es `fullDate`: el `title` de la fecha de una fila y la cabecera de la
// cita al responder.
func FechaLarga(l i18n.Locale, cuando time.Time) string {
	if cuando.IsZero() {
		return ""
	}
	return i18n.FormatDateTime(l, cuando)
}
