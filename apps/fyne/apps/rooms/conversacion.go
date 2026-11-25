// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// «Descargar la conversación», como `lib/conversacionEnTexto.ts` de la web: lo cargado
// del canal abierto en un .txt, «[AAAA-MM-DD HH:MM] Nombre: texto» en hora local, con
// las líneas de un mensaje largo sangradas bajo su autor. Se arma en el dispositivo
// con los mensajes ya descifrados: no pide nada al servicio ni le manda nada.

// ConversacionEnTexto es el texto del fichero: la cabecera con el canal y cada mensaje
// en su línea.
func ConversacionEnTexto(canal string, mensajes []api.RoomsMessage) string {
	var b strings.Builder
	b.WriteString("# " + canal + "\n\n")
	for _, m := range mensajes {
		if !m.CreatedAt.IsZero() {
			b.WriteString("[" + m.CreatedAt.Local().Format("2006-01-02 15:04") + "] ")
		}
		cuerpo := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(m.Body), "\r\n", "\n"), "\n", "\n    ")
		// El mismo nombre que el resumen del canal: el visible, o el `sub` si no lo hay.
		b.WriteString(nombreDe(m.AuthorName, m.AuthorSub) + ": " + cuerpo + "\n")
	}
	return b.String()
}

// prohibidosEnNombre son los caracteres que un sistema de ficheros no acepta en un
// nombre, y los de control.
var prohibidosEnNombre = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// NombreDeConversacion es el `nombreDeConversacion` de la web: el canal saneado y la
// fecha.
func NombreDeConversacion(canal string, hoy time.Time) string {
	limpio := strings.Join(strings.Fields(prohibidosEnNombre.ReplaceAllString(canal, " ")), " ")
	if r := []rune(limpio); len(r) > 60 {
		limpio = strings.TrimSpace(string(r[:60]))
	}
	if limpio == "" {
		limpio = "rooms"
	}
	return limpio + " " + hoy.Local().Format("2006-01-02") + ".txt"
}

// DescargarConversacion deja la conversación abierta en la carpeta de descargas y dice
// dónde, como cualquier otra descarga de la app.
func (sa *Sala) DescargarConversacion(c Canal) {
	if c.ID != sa.abierto || len(sa.mensajes) == 0 {
		return
	}
	t := sa.P.T
	texto := ConversacionEnTexto(c.Nombre, sa.mensajes)
	carpeta, err := ficheros.Carpeta()
	var ruta string
	if err == nil {
		ruta, err = ficheros.Guardar(carpeta, NombreDeConversacion(c.Nombre, time.Now()), strings.NewReader(texto))
	}
	if err != nil {
		sa.P.Avisar(ui.AvisoPeligro, t.T("channel.downloadFailed", nil))
		return
	}
	sa.P.Avisar(ui.AvisoExito, t.T("channel.downloaded", nil)+" "+ruta)
}
