// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
)

// fototecaLocal es la fototeca de SQLite (`nucleo/local/fototeca`, el plan B de §5.6) con
// lo que Photos hace al vaciar la papelera: la fototeca ya suelta en el acto del disco el
// original y las derivadas de lo purgado, y aquí se quita además lo que se veía de los
// vídeos borrados. Lo demás es la fototeca tal cual (los métodos suben por la
// incrustación, también las interfaces opcionales que mira la fachada).
type fototecaLocal struct {
	*fototeca.Fototeca
	// copias son los ficheros con que se ven los vídeos copiados (copiar.go); nil, nada.
	copias *copiasParaVer
}

// Purgar borra para siempre y, si se fue algo, quita en segundo plano lo que se veía de
// los vídeos borrados (su enlace o su copia en la caché).
func (f *fototecaLocal) Purgar(ctx context.Context, id string) (int, error) {
	n, err := f.Fototeca.Purgar(ctx, id)
	if n > 0 && f.copias != nil {
		// Sin la cancelación de quien borró: lo borrado ya no está.
		seguir := context.WithoutCancel(ctx)
		go func() {
			c, fin := context.WithTimeout(seguir, time.Minute)
			defer fin()
			f.copias.podar(c)
		}()
	}
	return n, err
}

// textosDeVista son los del catálogo que en esta edición dicen otra cosa (no hay Sytue
// Drive): la galería vacía, lo que pasa al tirar a la papelera y al borrar para siempre,
// y que «Añadir fotos» y soltar admiten también vídeos (FO-09).
func textosDeVista(l *local.App) map[string]string {
	return map[string]string{
		"gallery.onlyImages":        texto(l, "vista.soloFotosYVideos"),
		"gallery.empty.description": texto(l, "vista.vacia"),
		"trash.emptyConfirm":        texto(l, "vista.vaciarPapelera"),
		"trash.purgeConfirm":        texto(l, "vista.purgar"),
		"gallery.trashedStays":      texto(l, "vista.enPapelera"),
	}
}
