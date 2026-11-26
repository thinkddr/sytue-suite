// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/url"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// «Copiar enlace» y «Copiar el texto» del menú de una nota, como NoteCard.tsx: sin
// abrir nada, al portapapeles y con un aviso que lo dice.

// enlaceANota es el `urlDeNota` de la web (client/enlaces.ts): la raíz de Notes con
// `?notebook=&note=`, el mismo enlace que abre la búsqueda de la suite y que el editor
// guarda al enlazar una nota. Solo lleva ids: abre a quien ya puede leerla. Vacío sin
// dirección de la web que componer.
func (p *Pantalla) enlaceANota(cuaderno, nota string) string {
	if p.AppURL == nil || cuaderno == "" || nota == "" {
		return ""
	}
	return p.AppURL("/?notebook=" + url.QueryEscape(cuaderno) + "&note=" + url.QueryEscape(nota))
}

// TextoParaCopiar es el `textoParaCopiar` de la web: el título, una línea en blanco y el
// cuerpo en texto plano (los bloques como líneas, las viñetas con su «•»). Sale del
// contenido ya descifrado en el dispositivo: de una nota cifrada no se copia nada que
// no estuviera en pantalla.
func TextoParaCopiar(n Nota) string {
	titulo := strings.TrimSpace(n.Titulo)
	cuerpo := strings.TrimSpace(htmltext.Text(n.Contenido))
	switch {
	case titulo == "":
		return cuerpo
	case cuerpo == "":
		return titulo
	}
	return titulo + "\n\n" + cuerpo
}

// copiarNota lleva el texto al portapapeles y lo confirma; si el portapapeles no lo
// toma (`ui.Copiar` lo lee de vuelta), lo dice.
func (p *Pantalla) copiarNota(texto, hecho string) {
	t := p.tr.T
	var portapapeles fyne.Clipboard
	if a := fyne.CurrentApp(); a != nil {
		portapapeles = a.Clipboard()
	}
	if !ui.Copiar(portapapeles, texto) {
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, t("note.copyFailed", nil), ""))
		return
	}
	p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, t(hecho, nil), ""))
}
