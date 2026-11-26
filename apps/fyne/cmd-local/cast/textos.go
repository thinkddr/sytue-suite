// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los textos PROPIOS de Cast Office (convertir, ffmpeg, publicar en este equipo). Van aquí
// y no en `apps/fyne/catalogos`, que exige las claves de un `en.ts` de la web y esto no
// tiene web (PLAN §5.14). Español e inglés aquí; los otros veinticuatro idiomas, en
// textos_idiomas.go (`TestLasTablasDeCastCuadran` exige las mismas claves en todos).
var textos = map[string][2]string{ // clave: {es, en}
	"importar":            {"Importar vídeo", "Import video"},
	"importar.elegir":     {"Elige vídeos para la videoteca", "Choose videos for your library"},
	"convertir":           {"Convertir a…", "Convert to…"},
	"convertir.titulo":    {"Convertir «%s»", "Convert “%s”"},
	"convertir.formato":   {"Formato", "Format"},
	"convertir.destino":   {"Guardar como", "Save as"},
	"convertir.carpeta":   {"Elegir carpeta…", "Choose folder…"},
	"convertir.empezar":   {"Convertir", "Convert"},
	"convertir.en":        {"Convirtiendo «%s»…", "Converting “%s”…"},
	"convertir.listo":     {"Convertido. Está en %s", "Done. It is at %s"},
	"convertir.anadir":    {"Añadir a la videoteca", "Add to library"},
	"convertir.existe":    {"Ya hay un fichero con ese nombre; elige otro.", "A file with that name already exists; choose another."},
	"convertir.ninguno":   {"Este ffmpeg no trae ninguno de los codificadores necesarios.", "This ffmpeg has none of the required encoders."},
	"sistema":             {"Abrir con el reproductor del sistema", "Open in system player"},
	"carpeta":             {"Mostrar en la carpeta", "Show in folder"},
	"publicar":            {"Publicar en Explorar", "Publish to Explore"},
	"retirar":             {"Retirar de Explorar", "Remove from Explore"},
	"editar":              {"Editar", "Edit"},
	"editar.titulo":       {"Título", "Title"},
	"editar.descripcion":  {"Descripción", "Description"},
	"poster":              {"Póster…", "Poster…"},
	"poster.elegir":       {"Elige una imagen para el póster", "Choose a poster image"},
	"poster.fotograma":    {"Usar el fotograma de ahora", "Use current frame"},
	"poster.imagen":       {"Elegir una imagen…", "Choose an image…"},
	"quitar":              {"Quitar", "Remove"},
	"quitar.pregunta":     {"¿Quitar «%s» de la videoteca?", "Remove “%s” from your library?"},
	"quitar.fichero":      {"Borrar también el fichero (es una copia en la carpeta de vídeos)", "Also delete the file (it is a copy in the videos folder)"},
	"quitar.apuntado":     {"El fichero original no se borra: está fuera de la videoteca.", "The original file is not deleted: it lives outside your library."},
	"preparando":          {"Preparando para verlo aquí: %d %%", "Getting it ready to play here: %d%%"},
	"preparando.sin":      {"Preparando para verlo aquí…", "Getting it ready to play here…"},
	"sinffmpeg":           {"Este formato (%s) no se reproduce dentro de la app sin ffmpeg. Ábrelo con el reproductor del sistema, o instala ffmpeg para verlo aquí y convertirlo.", "This format (%s) cannot play inside the app without ffmpeg. Open it in your system player, or install ffmpeg to play and convert it here."},
	"sinffmpeg.corto":     {"%s: para verlo aquí hace falta ffmpeg", "%s: ffmpeg is needed to play it here"},
	"sinffmpeg.barra":     {"ffmpeg no está instalado: aquí solo se ven los WebM (VP9 + Opus); lo demás se abre con el reproductor del sistema.", "ffmpeg is not installed: only WebM (VP9 + Opus) plays here; everything else opens in your system player."},
	"sinffmpeg.gst":       {"ffmpeg no está instalado: lo que no es WebM se ve con GStreamer, el del sistema, sin convertir (buscar en un vídeo largo tarda). Para convertir, instala ffmpeg.", "ffmpeg is not installed: anything other than WebM plays through the system’s GStreamer, without converting (seeking in a long video is slow). To convert, install ffmpeg."},
	"sinffmpeg.mf":        {"ffmpeg no está instalado: lo que no es WebM se ve con los descodificadores de Windows (Media Foundation), sin convertir. Para convertir, instala ffmpeg.", "ffmpeg is not installed: anything other than WebM plays through Windows’ own decoders (Media Foundation), without converting. To convert, install ffmpeg."},
	"sinffmpeg.movil":     {"En el teléfono, lo que no sea WebM se abre con el reproductor del sistema.", "On phones, anything other than WebM opens in the system player."},
	"movil.sistema":       {"En el teléfono, lo que no es WebM se ve con los descodificadores del sistema; lo que el teléfono no sepa leer se abre con su reproductor.", "On phones, anything other than WebM plays through the system decoders; whatever the phone cannot read opens in its player."},
	"movil.noLee":         {"Este teléfono no sabe descodificar este formato (%s). Ábrelo con el reproductor del sistema.", "This phone cannot decode this format (%s). Open it in the system player."},
	"movil.noLeeCorto":    {"%s: este teléfono no lo sabe descodificar", "%s: this phone cannot decode it"},
	"fallo":               {"No se pudo preparar: %s", "Could not get it ready: %s"},
	"reintentar":          {"Reintentar", "Retry"},
	"ffmpeg":              {"Conversión (ffmpeg)…", "Conversion (ffmpeg)…"},
	"ffmpeg.titulo":       {"Conversión de vídeo", "Video conversion"},
	"ffmpeg.ayuda":        {"Sytue Cast usa el ffmpeg que tengas instalado en el equipo para ver y convertir MP4, MOV, MKV, AVI, HEVC y otros formatos. No lo trae dentro ni lo descarga. Sin él, solo se ven los WebM (VP9 + Opus).", "Sytue Cast uses the ffmpeg installed on your computer to play and convert MP4, MOV, MKV, AVI, HEVC and other formats. It neither bundles nor downloads it. Without it, only WebM (VP9 + Opus) plays."},
	"ffmpeg.ruta":         {"Ruta de ffmpeg (vacía: la del sistema)", "ffmpeg path (empty: from system PATH)"},
	"ffmpeg.encontrado":   {"Encontrado: %s", "Found: %s"},
	"ffmpeg.noencontrado": {"No se ha encontrado ffmpeg.", "ffmpeg was not found."},
	"ffmpeg.sinprobe":     {"Sin ffprobe: no se pueden leer los códecs ni la duración antes de convertir.", "No ffprobe: codecs and duration cannot be read before converting."},
	"ffmpeg.buscar":       {"Buscar…", "Browse…"},
	"ffmpeg.instalar":     {"Instálalo con el gestor de paquetes de tu sistema (apt install ffmpeg, brew install ffmpeg, winget install ffmpeg).", "Install it with your system’s package manager (apt install ffmpeg, brew install ffmpeg, winget install ffmpeg)."},
	"yo":                  {"Yo", "Me"},
	"canal":               {"En este equipo", "On this device"},
	"cancelar":            {"Cancelar", "Cancel"},
	"guardar":             {"Guardar", "Save"},
	"aceptar":             {"Aceptar", "OK"},
	"cerrar":              {"Cerrar", "Close"},
	"abrir.error":         {"No se pudo abrir el vídeo", "Could not open the video"},
	"coleccion":           {"Colección", "Collection"},
	"coleccion.ninguna":   {"Ninguna (escribe una nueva)", "None (type a new one)"},
	"colecciones":         {"Colecciones", "Collections"},
	"colecciones.vacio":   {"Aún no hay colecciones. Ponle una a un vídeo desde «Editar».", "No collections yet. Give a video one from “Edit”."},
	"colecciones.cuenta":  {"%d vídeos · %d publicados", "%d videos · %d published"},
	"colecciones.abrir":   {"Abrir", "Open"},
	"colecciones.ayuda":   {"Una colección agrupa vídeos de la videoteca. Se pone en «Editar» de cada vídeo.", "A collection groups videos in your library. Set it from each video’s “Edit”."},
}

// propios son los textos del catálogo de Cast que en Sytue Office dirían algo que no es
// verdad: aquí no se sube a ningún servicio, «No listado» es un borrador y nadie cuenta
// las visualizaciones (`quitados`: la pantalla quita la pieza). Las `{variables}` son las
// del catálogo. Los otros veinticuatro idiomas, en textos_idiomas.go.
var propios = map[string][2]string{ // clave del catálogo: {es, en}
	"nav.upload":                  {"Importar", "Import"},
	"upload.title":                {"Importar vídeos", "Import videos"},
	"visibility.unlisted":         {"Borrador", "Draft"},
	"visibility.public":           {"Publicado", "Published"},
	"home.empty.description":      {"Importa un vídeo del equipo o suéltalo en la ventana.", "Import a video from your device or drop it on the window."},
	"explore.empty.description":   {"Aquí aparecen los vídeos que publiques en este equipo.", "Videos you publish on this device show up here."},
	"channel.empty.description":   {"Esta colección aún no tiene vídeos, o no hay nada publicado.", "This collection has no videos yet, or nothing is published."},
	"watch.failed.description":    {"Prueba a importar de nuevo el fichero original.", "Try importing the original file again."},
	"watch.uploading.title":       {"Este vídeo aún se está preparando", "This video is still being prepared"},
	"watch.uploading.description": {"Se podrá ver en cuanto termine.", "It will play as soon as it is ready."},
}

// quitados son las claves del catálogo cuya pieza no sale en Sytue Office: nadie cuenta
// las visualizaciones. Vista.Propios devuelve para ellas un texto vacío.
var quitados = map[string]bool{"watch.views": true}

var (
	tablaTextos  = local.Propios{Base: textos, Traducciones: traducciones}
	tablaPropios = local.Propios{Base: propios, Traducciones: traduccionesPropias}
)

// propiosDe es Vista.Propios en el idioma dado (los que no tienen traducción, en inglés).
func propiosDe(loc i18n.Locale) func(string) (string, bool) {
	return func(clave string) (string, bool) {
		if quitados[clave] {
			return "", true
		}
		if _, ok := propios[clave]; !ok {
			return "", false
		}
		return tablaPropios.En(loc, clave), true
	}
}

// textosDe devuelve la función de traducción de la edición en el idioma dado.
func textosDe(loc i18n.Locale) func(string) string {
	return func(clave string) string { return tablaTextos.En(loc, clave) }
}
