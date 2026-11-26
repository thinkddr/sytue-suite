// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// La factura de cliente fuera de la app: su PDF y su envío por correo con el PDF
// adjunto (`erp.SalidaFactura`). En Sytue Office:
//
//   - «PDF» guarda donde elija la persona con el selector de guardado del sistema (sin
//     él, en Descargas) y lo abre en Sytue CertiPDF de este equipo o, si no está, con
//     la app del sistema;
//   - «Enviar por correo» sale por una cuenta de Sytue Mail de este equipo, por el
//     portero de red (`correoDeMail`); sin ninguna, «Abrir en el correo» abre el
//     redactor de Sytue Mail de este equipo con el mensaje escrito y el PDF ya adjunto
//     (`redactarEnMail`) y, sin Mail, el programa de correo del sistema (`mailto:`) con
//     el PDF guardado a la vista.

// salidaFactura son los gestos de la ficha de una factura en la edición local.
func salidaFactura(l *local.App) erp.SalidaFactura {
	return erp.SalidaFactura{
		PDF: true, Enviar: true,
		Guardar: func(titulo, nombre string, datos []byte, listo func(string, error)) {
			go func() { // el selector bloquea
				ruta, err := guardarPDF(titulo, nombre, datos)
				fyne.Do(func() { listo(ruta, err) })
			}()
		},
		Abrir: func(ruta string) {
			go func() { _ = l.AbrirEn("pdf", ruta) }() // lanza otro programa
		},
		Redactar: func(para, asunto, mensaje, ruta string) error {
			if err := api.AbrirCorreo(l.A, para, asunto, mensaje); err != nil {
				return err
			}
			go func() { _ = ficheros.Revelar(ruta) }() // para arrastrarlo al mensaje
			return nil
		},
		RedactarConPDF: func(para, asunto, mensaje, nombre string, pdf []byte) error {
			return redactarEnMail(l.A, l.HayApp("mail"), correo.ParaEnviar{Dir: l.Espacio.Rutas.ParaEnviar},
				para, asunto, mensaje, nombre, pdf)
		},
	}
}

// errSinMail: Sytue Mail no está en este equipo; se abre el programa de correo del sistema.
var errSinMail = errors.New("erp: Sytue Mail no está instalada")

// redactarEnMail deja el PDF para Sytue Mail de este equipo («para-enviar», por un
// identificador de un solo uso) y le abre el redactor con el mensaje escrito y ese
// adjunto. Sin Mail, sin carpeta compartida entre apps (Android) o si no se abre, falla
// sin dejar nada, y la ficha hace lo de siempre (`mailto:` y el PDF en Descargas).
func redactarEnMail(a api.Abridor, hayMail bool, salida correo.ParaEnviar, para, asunto, mensaje, nombre string, pdf []byte) error {
	if !hayMail {
		return errSinMail
	}
	id, err := salida.Dejar(nombre, pdf)
	if err != nil {
		return err
	}
	if err := api.AbrirEnOffice(a, correo.EnlaceRedactar(para, asunto, mensaje, id)); err != nil {
		salida.Quitar(id)
		return err
	}
	return nil
}

// guardarPDF escribe la factura donde diga el selector de guardado; cancelar es "".
// Sin selector en este sistema, va a la carpeta de descargas sin pisar nada.
func guardarPDF(titulo, nombre string, datos []byte) (string, error) {
	ruta, err := ficheros.GuardarComo(titulo, nombre, "pdf")
	if errors.Is(err, ficheros.ErrNoDisponible) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
	}
	if err != nil || ruta == "" {
		return "", err
	}
	return ruta, os.WriteFile(ruta, datos, 0o600) //nolint:gosec // G703: la ruta la eligió la persona en el selector del sistema
}

// correoDeMail envía con las cuentas de Sytue Mail de este equipo (`<Datos>/Correo`),
// sin abrir su buzón, y solo a sus servidores, por el portero de red.
func correoDeMail(l *local.App) *correo.Remitente {
	return correo.NuevoRemitente(filepath.Join(l.Espacio.Rutas.Datos, "Correo"), l.Secretos,
		correo.Red{Marcar: l.Red.Marcador(red.Correo), HTTP: l.Red.Cliente(red.OAuth)})
}
