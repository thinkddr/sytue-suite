// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los textos PROPIOS del Hub de Sytue Office: la portada local no tiene web de la que
// salgan sus claves (PLAN §5.14). Español e inglés aquí; los otros veinticuatro idiomas,
// en textos_idiomas.go (`TestLaTablaDelHubCuadra` exige las mismas claves en todos). Las
// descripciones de las apps sí son las del catálogo común (`app.<clave>.description`).
var textos = map[string][2]string{ // clave: {es, en}
	"nombre":            {"Sytue Office", "Sytue Office"},
	"titulo":            {"Tus aplicaciones", "Your apps"},
	"buscar":            {"Buscar en documentos, correo, notas, contactos, eventos, fotos, bases y archivos", "Search documents, mail, notes, contacts, events, photos, bases and files"},
	"cancelar":          {"Cancelar", "Cancel"},
	"hoy":               {"Hoy", "Today"},
	"hoy.todoElDia":     {"Todo el día", "All day"},
	"hoy.recordatorio":  {"Recordatorio", "Reminder"},
	"hoy.tarea":         {"Tarea por hacer", "To-do"},
	"hoy.masTareas":     {"Y {n} tareas más", "And {n} more to-dos"},
	"hoy.sinLeer":       {"{n} correos sin leer", "{n} unread emails"},
	"hoy.sinLeer.uno":   {"1 correo sin leer", "1 unread email"},
	"hoy.vacio":         {"Nada para hoy", "Nothing for today"},
	"hoy.vacio.texto":   {"Aquí salen los eventos de Calendar, los recordatorios y tareas de Notepads y el correo sin leer de Mail.", "Events from Calendar, reminders and to-dos from Notepads and unread mail from Mail show up here."},
	"bandeja.abrir":     {"Abrir {nombre}", "Open {nombre}"},
	"bandeja.apps":      {"Abrir una app", "Open an app"},
	"bandeja.alIniciar": {"Abrir al iniciar sesión", "Open at login"},
	"traer.boton":       {"Traer desde Sytue", "Bring from Sytue"},
	"traer.titulo":      {"Traer desde Sytue", "Bring from Sytue"},
	"traer.verbo":       {"Traer", "Bring"},
	"traer.confirmar.texto": {
		"Este fichero trae: {productos}.\n\nCada cosa entra en su app sin borrar nada, y lo que ya se trajo antes no se repite: si ha cambiado en Sytue y aquí no lo has tocado, se pone al día; si lo has cambiado aquí, se queda como lo dejaste y lo de Sytue entra aparte. Lo que no tiene app en Sytue Office se deja tal cual en {carpeta}. No se abre ninguna conexión.",
		"This file brings: {productos}.\n\nEach item is added to its app without deleting anything, and what was brought in before is not added again: if it changed in Sytue and you haven't touched it here, it is brought up to date; if you changed it here, it stays as you left it and the Sytue version is added separately. What has no app in Sytue Office is left as is in {carpeta}. No connection is opened.",
	},
	"traer.noEs":         {"No es una copia de Sytue", "Not a Sytue copy"},
	"traer.noEs.texto":   {"Elige el ZIP que descargaste en Sytue, en «Tus datos» → «Descargar una copia».", "Choose the ZIP you downloaded from Sytue, in “Your data” → “Download a copy”."},
	"traer.cerrar":       {"Cierra antes las apps", "Close the apps first"},
	"traer.cerrar.texto": {"Mientras se traen tus datos tienen que estar cerradas: {apps}.", "These must be closed while your data is brought in: {apps}."},
	"traer.trayendo":     {"Repartiendo tus datos en las apps de este equipo…", "Sorting your data into the apps on this device…"},
	"traer.hecho":        {"Tus datos ya están aquí", "Your data is here"},
	"traer.aMedias":      {"Se ha traído una parte", "Part of it was brought in"},
	"traer.parte":        {"{app}: {n}", "{app}: {n}"},
	"traer.nada":         {"No había nada que poner en las apps.", "There was nothing to add to the apps."},
	"traer.copiados":     {"{n} ficheros se han dejado tal cual en {carpeta}.", "{n} files were left as is in {carpeta}."},
	"traer.fallidos":     {"{n} no han entrado en su app; están en esa carpeta.", "{n} could not be added to their app; they are in that folder."},
	"traer.repetidos":    {"{n} ya estaban aquí de otra vez y no se han repetido ({lista}).", "{n} were already here from an earlier time and were not added again ({lista})."},
	"traer.actualizados": {"{n} habían cambiado en Sytue y, como aquí no se habían tocado, se han puesto al día ({lista}).", "{n} had changed in Sytue and, as they hadn't been touched here, were brought up to date ({lista})."},
	"traer.conservados":  {"{n} habían cambiado aquí y también en Sytue: lo de aquí se queda como estaba y lo de Sytue se ha dejado aparte ({lista}).", "{n} had changed both here and in Sytue: yours stays as it was and the Sytue version was kept separately ({lista})."},
	"traer.borrados":     {"{n} que quitaste aquí no se han vuelto a traer ({lista}).", "{n} that you removed here were not brought back ({lista})."},
	"traer.yaEnDrive":    {"{n} ficheros ya estaban en Drive de otra vez y no se han vuelto a copiar.", "{n} files were already in Drive from an earlier time and were not copied again."},
	"traer.sinAbrir":     {"{n} fotos o vídeos se han quedado en Drive: Photos no los sabe abrir en este equipo.", "{n} photos or videos were left in Drive: Photos can't open them on this device."},
	"traer.sinHEIC":      {"Para llevar las fotos HEIC a Photos hace falta ffmpeg o libheif: instala uno de los dos y vuelve a traer la copia.", "HEIC photos need ffmpeg or libheif to go into Photos: install one of them and bring the copy in again."},
	// «Exportar todo» (exportar.go): lo de todas las apps en formatos abiertos.
	"exportar.boton":        {"Exportar todo", "Export all"},
	"exportar.titulo":       {"Exportar en formatos abiertos", "Export in open formats"},
	"exportar.verbo":        {"Exportar", "Export"},
	"exportar.elegir":       {"Elige dónde dejar lo exportado", "Choose where to put the export"},
	"exportar.cerrar.texto": {"Mientras se exporta tienen que estar cerradas: {apps}.", "These must be closed while exporting: {apps}."},
	"exportar.exportando":   {"Exportando lo de cada app en formatos abiertos…", "Exporting each app's data in open formats…"},
	"exportar.hecho":        {"Exportado", "Export finished"},
	"exportar.hecho.texto": {
		"Lo de {apps} está en {carpeta}, en formatos que abren otros programas.",
		"The data from {apps} is in {carpeta}, in formats other programs can open.",
	},
	"exportar.aMedias":    {"Se ha exportado una parte", "Part of it was exported"},
	"exportar.nada":       {"No había nada que exportar en este equipo.", "There was nothing to export on this device."},
	"exportar.fallidos":   {"{app}: {n} no se han podido copiar porque su fichero ya no está.", "{app}: {n} could not be copied because their file is gone."},
	"recientes":           {"Reciente", "Recent"},
	"resultados":          {"Resultados", "Results"},
	"sinApps":             {"No hay otras apps de Sytue Office junto al Hub", "No other Sytue Office apps next to the Hub"},
	"sinApps.texto":       {"El Hub abre las apps instaladas en su misma carpeta. Instala el paquete completo de Sytue Office.", "The Hub opens the apps installed in its own folder. Install the full Sytue Office bundle."},
	"sinApps.movil":       {"En el teléfono cada app de Sytue Office se abre por separado.", "On a phone, each Sytue Office app opens on its own."},
	"sinRecientes":        {"Aún no hay nada reciente", "Nothing recent yet"},
	"sinRecientes.texto":  {"Lo que crees o abras en Docus, Calcus, Diapos, Notepads o Drive aparecerá aquí.", "What you create or open in Docus, Calcus, Diapos, Notepads or Drive will show up here."},
	"sinResultados":       {"Nada coincide con «{q}»", "Nothing matches “{q}”"},
	"sinResultados.texto": {"Se busca sin conexión, en lo guardado en este equipo.", "Search runs offline, over what is saved on this device."},
	"noInstalada":         {"{app} no está instalada", "{app} is not installed"},
	"noInstalada.texto":   {"Instala {app} de Sytue Office junto al Hub para abrirlo desde aquí.", "Install Sytue Office {app} next to the Hub to open it from here."},
	"noSeAbre":            {"No se ha podido abrir {app}", "Could not open {app}"},
	"fallos":              {"No se ha podido leer: {fuentes}", "Could not read: {fuentes}"},
}

var tabla = local.Propios{Base: textos, Traducciones: traducciones}

// texto es la traducción de clave con sus variables `{nombre}` sustituidas.
func texto(l i18n.Locale, clave string, vars ...string) string {
	s := tabla.En(l, clave)
	for i := 0; i+1 < len(vars); i += 2 {
		s = strings.ReplaceAll(s, "{"+vars[i]+"}", vars[i+1])
	}
	return s
}
