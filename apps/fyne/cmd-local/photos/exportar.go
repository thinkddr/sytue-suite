// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// Textos propios de Photos Office, que no tienen clave en el catálogo de la web
// (PLAN §5.14): español e inglés aquí, los otros veinticuatro idiomas en
// textos_idiomas.go. Los «vista.*» sustituyen a los del catálogo (`Vista.Textos`) y
// llevan sus `{variables}`.
var textos = map[string][2]string{ // clave: {es, en}
	"exportar":        {"Exportar la fototeca", "Export library"},
	"exportar.elegir": {"Elige dónde dejar la fototeca", "Choose where to export the library"},
	"exportar.en":     {"Copiando los originales a %s…", "Copying originals to %s…"},
	"exportar.avance": {"%d de %d fotos", "%d of %d photos"},
	"exportar.hecho":  {"%d fotos copiadas a %s, en carpetas por año y mes.", "%d photos copied to %s, in folders by year and month."},
	"exportar.ya":     {"%d ya estaban y no se han vuelto a copiar.", "%d were already there and were not copied again."},
	"exportar.fallo":  {"%d no se han podido copiar: %s", "%d could not be copied: %s"},

	"migrar": {"Poniendo al día la fototeca", "Updating the library"},
	"migrar.texto": {
		"Sytue Photos está pasando tus fotos a su fototeca nueva, más rápida. Solo se hace una vez, y la de antes se guarda.",
		"Sytue Photos is moving your photos to its new, faster library. This happens only once, and your old library is kept.",
	},
	"migrar.enUso": {
		"Otra app de Sytue Office tiene la fototeca abierta (otra ventana de Sytue Photos o el Hub). Ciérrala y vuelve a abrir Sytue Photos.",
		"Another Sytue Office app has the library open (another Sytue Photos window, or the Hub). Close it and open Sytue Photos again.",
	},

	"importar":         {"Importar", "Import"},
	"importar.carrete": {"Importar del carrete", "Import from camera roll"},
	"importar.carreteTexto": {
		"Trae a la fototeca las fotos y los vídeos de la galería de este dispositivo, cada uno una sola vez. Se copian a la carpeta de Sytue Office; los de la galería no se tocan.",
		"Brings the photos and videos in this device's gallery into the library, each one only once. They are copied to the Sytue Office folder; the gallery is left as it is.",
	},
	"importar.automatico": {"Importar automáticamente las fotos y los vídeos nuevos", "Automatically import new photos and videos"},
	"importar.ahora":      {"Importar ahora", "Import now"},
	"importar.carpetaTexto": {
		"Enlaza en la fototeca las fotos y los vídeos de una carpeta y de sus subcarpetas, cada uno una sola vez, sin copiarlos: se quedan donde están y la fototeca guarda sus miniaturas.",
		"Links the photos and videos in a folder and its subfolders into the library, each one only once, without copying them: they stay where they are and the library keeps their thumbnails.",
	},
	"importar.carpeta":   {"Importar una carpeta…", "Import a folder…"},
	"importar.vigiladas": {"Carpetas vigiladas", "Watched folders"},
	"importar.vigiladasTexto": {
		"Lo que aparezca en ellas se enlaza solo, en cuanto llega y cada vez que se abre Sytue Photos. Si mueves o renombras un fichero dentro de ellas, la foto lo vuelve a encontrar.",
		"Whatever appears in them is linked on its own, as soon as it arrives and every time Sytue Photos opens. If you move or rename a file inside them, the photo finds it again.",
	},
	"importar.copiar": {"Copiar las fotos a la fototeca en vez de enlazarlas", "Copy photos into the library instead of linking them"},
	"importar.copiarTexto": {
		"Copiadas, siguen en la fototeca aunque borres o muevas el fichero; enlazadas no ocupan sitio otra vez. Los vídeos se enlazan siempre.",
		"Copied, they stay in the library even if you delete or move the file; linked, they don't take up space twice. Videos are always linked.",
	},

	"vista.vacia": {
		"Las fotos y los vídeos que importes o enlaces desde tus carpetas aparecerán aquí, ordenados por su fecha.",
		"The photos and videos you import or link from your folders will appear here, ordered by date.",
	},
	"vista.vaciarPapelera": {
		"¿Borrar para siempre las {count} fotos de la papelera? Se quitan de la fototeca; los ficheros enlazados se quedan en su carpeta.",
		"Delete the {count} photos in the trash for ever? They are removed from the library; linked files stay in their folder.",
	},
	"vista.enPapelera": {"Se puede restaurar desde la papelera.", "It can be restored from the trash."},
	"vista.purgar": {
		"¿Borrar esta foto para siempre? Se quita de la fototeca; si estaba enlazada, el fichero se queda en su carpeta.",
		"Delete this photo for ever? It is removed from the library; if it was linked, the file stays in its folder.",
	},
	"vista.soloFotosYVideos": {"Solo se pueden añadir fotos y vídeos.", "Only photos and videos can be added."},

	"original.falta":    {"El original ya no está en su carpeta", "The original is no longer in its folder"},
	"original.cambiado": {"El original ha cambiado fuera de Sytue Photos", "The original has changed outside Sytue Photos"},
	"video.reproducir":  {"Reproducir", "Play"},
	"video.pausa":       {"Pausa", "Pause"},
	"video.cast":        {"Abrir en Cast", "Open in Cast"},
	"video.falta":       {"El vídeo ya no está en %s.", "The video is no longer at %s."},
	"video.noSeVe": {
		"Este vídeo no se puede reproducir aquí. Ábrelo en Cast, que lo puede convertir, o con el reproductor del sistema.",
		"This video can't be played here. Open it in Cast, which can convert it, or with the system player.",
	},
	"importar.sinVigiladas": {"Ninguna todavía.", "None yet."},
	"importar.vigilar":      {"Vigilar una carpeta…", "Watch a folder…"},
	"importar.dejar":        {"Dejar de vigilar", "Stop watching"},
	"importar.sinHEIC": {
		"Las fotos HEIC no se pueden importar en este equipo: instala ffmpeg o libheif para que Sytue Photos las convierta.",
		"HEIC photos can't be imported on this device: install ffmpeg or libheif so Sytue Photos can convert them.",
	},
	"importar.avance": {"Importando %d de %d…", "Importing %d of %d…"},
	"importar.hecho":  {"Fotos nuevas importadas: %d", "New photos imported: %d"},
	"importar.nada":   {"No había fotos nuevas.", "There were no new photos."},
	"importar.fallo":  {"La importación se ha detenido", "The import stopped"},
	"importar.sinPermiso": {
		"Sytue Photos no tiene permiso para ver las fotos de este dispositivo. Puedes dárselo en los ajustes del sistema.",
		"Sytue Photos doesn't have permission to see this device's photos. You can grant it in the system settings.",
	},
}

// texto es el texto propio de clave en el idioma de la app; lo que falte cae al inglés.
func texto(l *local.App, clave string) string {
	return local.Propios{Base: textos, Traducciones: traducciones}.En(l.Idioma(), clave)
}

// exportarFototeca pide una carpeta y saca a ella los originales, por año y mes (FO-05).
// La copia corre fuera del hilo de la interfaz con un diálogo de espera que dice por
// dónde va, y al acabar se dice qué se hizo.
func exportarFototeca(l *local.App, w fyne.Window, repo *fototeca.Fototeca) {
	local.ElegirCarpeta(texto(l, "exportar.elegir"), func(dir string) {
		rotulo := widget.NewLabel(fmt.Sprintf(texto(l, "exportar.en"), dir))
		rotulo.Wrapping = fyne.TextWrapWord
		barra := widget.NewProgressBar()
		espera := ui.NuevoDialogo(texto(l, "exportar"), container.NewVBox(rotulo, barra)).Con(ui.DialogoSM, nil)
		espera.Ocupado(true)
		espera.Mostrar(w.Canvas())
		go func() {
			resumen, err := repo.ExportarFototeca(context.Background(), dir, func(hechas, total int) {
				fyne.Do(func() {
					barra.SetValue(float64(hechas) / float64(max(total, 1)))
					barra.TextFormatter = func() string { return fmt.Sprintf(texto(l, "exportar.avance"), hechas, total) }
					barra.Refresh()
				})
			})
			fyne.Do(func() {
				espera.Cerrar()
				if err != nil {
					l.Fallo(w, err)
					return
				}
				local.Info(w, texto(l, "exportar"), resumenDeExportacion(l, dir, resumen))
			})
		}()
	})
}

func resumenDeExportacion(l *local.App, dir string, r privatePhotos.ResumenExportacion) string {
	partes := []string{fmt.Sprintf(texto(l, "exportar.hecho"), r.Copiadas, dir)}
	if r.YaEstaban > 0 {
		partes = append(partes, fmt.Sprintf(texto(l, "exportar.ya"), r.YaEstaban))
	}
	if len(r.Fallidas) > 0 {
		partes = append(partes, fmt.Sprintf(texto(l, "exportar.fallo"), len(r.Fallidas), motivosDeFallo(r.Fallidas, texto(l, "original.falta"))))
	}
	return strings.Join(partes, "\n\n")
}

// motivosDeFallo son las fotos que no se copiaron, una por línea («nombre: motivo»). El
// motivo más corriente, que el original enlazado ya no está, se dice en el idioma de la
// app (falta); los demás son los del sistema.
func motivosDeFallo(fallidas []error, falta string) string {
	lineas := make([]string, 0, len(fallidas))
	for _, err := range fallidas {
		s := err.Error()
		if errors.Is(err, privatePhotos.ErrOriginalExterno) {
			s = strings.Replace(s, privatePhotos.ErrOriginalExterno.Error(), falta, 1)
		}
		lineas = append(lineas, s)
	}
	return strings.Join(lineas, "\n")
}
