// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"path/filepath"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// carpetaDescargas es la de descargas del sistema; las pruebas la cambian por una propia.
var carpetaDescargas = ficheros.Carpeta

// guardarEnDescargas deja `datos` en descargas con `nombre` —sin pisar uno que ya se
// llame igual— y devuelve la ruta. Es lo que hace el navegador con un enlace de descarga:
// sin preguntar dónde.
func guardarEnDescargas(nombre string, datos []byte) (string, error) {
	carpeta, err := carpetaDescargas()
	if err != nil {
		return "", err
	}
	return ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
}

// descargar es «Descargar» de la cinta: el PDF abierto, con sus cambios ya aplicados
// (los mismos bytes que imprime y que firma), en descargas. La web descarga con el
// navegador y su barra de descargas avisa; aquí no hay nada que avise, así que lo dice
// la franja verde con el nombre que ha quedado (puede llevar « (2)» si ya había otro).
func (p *Pantalla) descargar() {
	if p.doc == nil || p.Contenido == nil {
		return
	}
	f, _ := p.doc.paraElMotor()
	nombre := f.Nombre
	if nombre == "" {
		nombre = p.doc.Ruta()
	}
	nombre = nombreDeRuta(nombre)
	go func() {
		ruta, err := guardarEnDescargas(nombre, f.Datos)
		fyne.Do(func() {
			defer p.avisarRed()
			if err != nil {
				p.Aviso, p.Error = "", p.t("pdf.download.failed")
			} else {
				p.Error, p.Aviso = "", p.tv("pdf.download.done", i18n.Vars{"name": filepath.Base(ruta)})
			}
			p.Recomponer()
		})
	}()
}
