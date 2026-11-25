// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command certipdf es Sytue CertiPDF de escritorio y móvil.
//
// Es SU PROPIA aplicación, no una pantalla de un binario que las contenga a todas:
// `docs/CONVENTIONS.md` §13.6 excepción 2. Lo que en la web es ir a otro subdominio,
// aquí es invocar a la otra app por su esquema de URL, que está declarado en
// `FyneApp.toml` y del que la cadena de empaquetado hace las cinco copias que cada
// sistema necesita.
//
// Es el CUARTO binario de la ofimática, con Docus, Calcus y Diapos. Comparte con ellos
// el catálogo de idioma ([catalogos.Docs]) porque en la web montan el mismo paquete
// (`@sytue/office`); NO comparte el cromo del editor, y el porqué está en el
// comentario del paquete `certipdf`.
//
// Los documentos viven en el repositorio PRIVADO de Drive del dispositivo verificado:
// aquí ya no se construye el cliente legible del servicio de transformación. Abrir,
// guardar, copiar, renombrar, papelera, texto, render, formularios, anotaciones,
// protección, censura, compresión, recorte, imágenes y PDF/A se ejecutan en LOCAL; lo
// que no tiene implementación local sigue BLOQUEADO, sin caer al servicio. Sin
// dispositivo verificado no hay portada legible.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/certipdf/...`, que pinta lo mismo con
// el pintor de software y compara los píxeles.
// paridad-web: apps/frontend/apps/certipdf/src/pages/auth/matrix/callback.astro
package main

// La portada y el estudio son los de `apps/certipdf`; esta es su app, como en la web
// la de `apps/certipdf` monta la vista `Pdf` de `@sytue/office` en su portada y en `/pdf`.
//
// paridad-web: apps/frontend/apps/certipdf/src/pages/index.astro
// paridad-web: apps/frontend/apps/certipdf/src/pages/pdf.astro

import (
	"context"
	"log/slog"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Medidas de la ventana al abrir: el ancho canónico de la suite y un alto que deja ver
// una página A4 entera al tamaño al que se abre.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue CertiPDF")
	w.SetPadded(false)
	// El widget nativo lo lee su propia Fuente, sin depender de la pantalla, y se
	// vuelve a publicar tras cada escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(tr))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	storage, err := newPDFStorage(s, a, w, tr)
	if err != nil || storage == nil {
		w.SetContent(ui.NuevoVacioError(tr.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.PDF).ConSinSesionPropia().Montar(storage.Objeto()))
		// La voz del sistema no se calla sola al cerrar: sigue en su propio proceso.
		w.SetOnClosed(storage.Close)
		// Con interfaz: lanzada por iOS solo para el widget no se pide nada.
		s.CuandoHayaEscena(func() {
			storage.Start()
		})
		// Un PDF pulsado en Drive llega como `sytue-pdf:/pdf?path=…&volver=…`: CertiPDF
		// abre por RUTA y no por identificador, que es lo que acepta su página en la
		// web. Sin repositorio montado el fallo es explícito, como el de Photos.
		s.AlAbrirFichero("pdf", storage.abrirFichero)
		// Mientras se arrastra por encima, la zona de abrir de la portada se enciende.
		soltar.AlArrastrar(w, func(activa bool) {
			if vista := storage.vista.Load(); vista != nil {
				vista.pantalla.ArrastrarEncima(activa)
			}
		})
		// Un PDF de la máquina —soltado en la ventana, con «Abrir con» o en los
		// argumentos— se sube a la carpeta «PDF» del repositorio privado y se abre,
		// como «abrir un PDF del ordenador».
		s.AceptarFicheros(w, tr.T("pdf.error.open", nil), tr.T("pdf.error.upload", nil))
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
