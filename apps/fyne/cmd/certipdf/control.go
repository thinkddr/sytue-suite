// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// control es lo que une la ventana con el motor: qué documento hay abierto y de dónde
// vino. Uno por montaje del repositorio privado: su `pdf` es la fachada de ese
// dispositivo verificado.
type control struct {
	a   fyne.App
	s   *sesion.Suite
	w   fyne.Window
	t   *i18n.Translator
	pdf *api.PDF
	p   *certipdf.Pantalla

	mu           sync.Mutex
	ruta, volver string
}

func (c *control) carpeta() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.volver
}

func (c *control) ultima() (ruta, volver string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ruta, c.volver
}

// conocerAutor trae de Sytue ID el nombre (o, sin él, el correo) de quien ha entrado.
func (c *control) conocerAutor(e sesion.Estado) {
	if !e.Dentro() || c.p.Autor != "" {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 15*time.Second)
		defer parar()
		yo, err := api.NewAccount(c.s.URL("id"), c.s.Sesion, nil).Me(ctx)
		if err != nil {
			return
		}
		nombre := yo.Name
		if nombre == "" {
			nombre = yo.Email
		}
		fyne.Do(func() { c.p.Autor = nombre })
	}()
}

// abrirRuta trae de Drive el PDF de esa ruta y lo enseña. Un fichero que no está se
// devuelve como error, para que la sesión enseñe «no encontrada»; cualquier otro fallo
// se enseña AQUÍ, con «Reintentar», que es lo que hace la web.
//
// `ctx` es el de la APERTURA: acaba cuando el fichero llega. Lo que la pantalla pide
// después —las páginas, los cambios— vive lo que viva el documento, así que va con el
// contexto de la pantalla y no con este.
//
//nolint:contextcheck // ver arriba: el documento vive más que la petición que lo abrió
func (c *control) abrirRuta(ctx context.Context, ruta, volver string) error {
	c.mu.Lock()
	c.ruta, c.volver = ruta, volver
	c.mu.Unlock()
	fyne.Do(func() {
		c.p.Barra().DesdeDrive = volver != ""
		c.p.Cargar()
	})
	doc, err := certipdf.AbrirDocumento(ctx, c.pdf, ruta)
	if err != nil {
		if certipdf.NoEsta(err) {
			return err
		}
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return nil
	}
	c.mostrar(doc)
	return nil
}

// abrirLocal sube a Drive un PDF de la máquina y lo enseña.
//
//nolint:contextcheck // como en abrirRuta: el documento vive más que su apertura
func (c *control) abrirLocal(ctx context.Context, uri string) error {
	nombre, datos, err := sesion.LeerLocal(uri)
	if err != nil {
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return err
	}
	fyne.Do(func() {
		c.p.Barra().DesdeDrive = false
		c.p.Cargar()
	})
	doc, err := certipdf.SubirYAbrir(ctx, c.pdf, nombre, datos)
	if err != nil {
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return err
	}
	c.mu.Lock()
	c.ruta, c.volver = doc.Ruta(), ""
	c.mu.Unlock()
	c.mostrar(doc)
	return nil
}

func (c *control) mostrar(doc *certipdf.Documento) {
	fyne.Do(func() {
		// `${nombre} — ${t('pdf.title')}`, el título de la página en la web.
		c.w.SetTitle(doc.Nombre() + " — " + c.t.T("pdf.title", nil))
		c.p.Abrir(doc)
	})
}

// salir es la salida de la cinta: a la carpeta de Drive de la que vino el PDF, o al Hub.
func (c *control) salir() {
	// La vuelta es de la sesión: canónica con la organización activa (ADR 0056).
	org := c.s.Sesion.Organizacion
	if v := c.carpeta(); v != "" {
		_ = api.AbrirEn(c.a, c.s.Apex(), "drive", org(), "?path="+url.QueryEscape(v))
		return
	}
	_ = api.AbrirEn(c.a, c.s.Apex(), "one", org(), "")
}

// crear es «PDF · En blanco»: una página A4 vacía (`api.PDFEnBlanco`, el `pdfEnBlanco` de
// la web) que se sube a la carpeta «PDF» con el nombre del catálogo y se abre.
func (c *control) crear() {
	fyne.Do(func() { c.p.EstadoCrear(true, false) })
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer parar()
	doc, err := certipdf.SubirYAbrir(ctx, c.pdf, c.t.T("pdf.new.filename", nil), api.PDFEnBlanco())
	if err != nil {
		slog.Info("certipdf: no se ha podido crear el PDF en blanco", "error", err)
		fyne.Do(func() { c.p.EstadoCrear(false, true) })
		return
	}
	fyne.Do(func() { c.p.EstadoCrear(false, false) })
	c.mu.Lock()
	c.ruta, c.volver = doc.Ruta(), ""
	c.mu.Unlock()
	c.mostrar(doc)
}

// elegir es «Elegir un fichero» / «Abrir un archivo»: el selector del SISTEMA
// (`ficheros.Elegir`). Un PDF se sube a «PDF» y se abre, que es lo que hace la web; un
// fichero de otra app de la suite se lo pasa al sistema, que lo abre en SU app (§13.6).
//
// En el móvil `ficheros.Elegir` es el selector del sistema que pone la sesión
// (`anfitrion/elegir_movil.go`): uno cada vez, ya copiado a una ruta.
func (c *control) elegir() {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil || len(rutas) == 0 {
			return
		}
		// Varias a la vez: los PDF, el primero en esta ventana; los demás, cada uno
		// en su app. Una ventana, un documento (como con «Abrir con»).
		abierto := false
		for _, r := range rutas {
			if suya, ok := api.AppQueAbre(r); ok && suya != "pdf" {
				_ = ficheros.Abrir(r)
				continue
			}
			if !abierto {
				abierto = true
				c.abrirElegido(r)
			}
		}
	}()
}

// abrirElegido sube el PDF de esa ruta a «PDF» y lo abre, con «Abriendo…» en la portada
// mientras tanto y la frase de la web si no es un PDF o no se puede.
func (c *control) abrirElegido(ruta string) {
	portada := c.estadoPortada
	portada(true, "")
	nombre, datos, err := sesion.LeerLocal(storage.NewFileURI(ruta).String())
	if err != nil {
		portada(false, c.t.T("pdf.open.failed", nil))
		return
	}
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer parar()
	doc, err := certipdf.SubirYAbrir(ctx, c.pdf, nombre, datos)
	switch {
	case errors.Is(err, api.ErrNoEsPDF):
		portada(false, c.t.T("pdf.open.notPdf", nil))
		return
	case err != nil:
		portada(false, c.t.T("pdf.open.failed", nil))
		return
	}
	portada(false, "")
	c.mu.Lock()
	c.ruta, c.volver = doc.Ruta(), ""
	c.mu.Unlock()
	c.mostrar(doc)
}

// estadoPortada es EstadoAbrir desde otra rutina.
func (c *control) estadoPortada(abriendo bool, fallo string) {
	fyne.Do(func() { c.p.EstadoAbrir(abriendo, fallo) })
}

// escanear es «Escanear con la cámara»: unas fotos se convierten en UN PDF con el motor
// (`pdf-escanear.ts`) y se abre. En el ordenador, como en la web, el mismo botón ofrece
// elegir fotos ya hechas. En Android abre la cámara del sistema y la foto queda en la
// caché privada de la app, no en la galería (`ficheros.Fotografiar`); donde no hay
// cámara que pedir, se eligen.
func (c *control) escanear() {
	go func() {
		foto, err := ficheros.Fotografiar(context.Background())
		if foto != "" {
			defer func() { _ = os.Remove(foto) }() // ya está en el PDF, o no sirve
		}
		rutas := []string{foto}
		if errors.Is(err, ficheros.ErrNoDisponible) {
			rutas, err = ficheros.Elegir()
		}
		if err != nil || len(rutas) == 0 || rutas[0] == "" {
			return
		}
		fotos := make([]api.PDFFichero, 0, len(rutas))
		for _, r := range rutas {
			nombre, datos, err := sesion.LeerLocal(storage.NewFileURI(r).String())
			if err != nil {
				c.estadoPortada(false, c.t.T("pdf.open.scanFailed", nil))
				return
			}
			fotos = append(fotos, api.PDFFichero{Nombre: nombre, Datos: datos})
		}
		c.estadoPortada(true, "")
		ctx, parar := context.WithTimeout(context.Background(), 2*time.Minute)
		defer parar()
		f, err := c.pdf.Escanear(ctx, fotos, c.t.T("pdf.open.scanName", nil))
		if err != nil {
			c.estadoPortada(false, c.t.T("pdf.open.scanFailed", nil))
			return
		}
		c.estadoPortada(false, "")
		_ = c.abrirRuta(ctx, "/"+api.PDFCarpeta+"/"+f.Name, "")
	}()
}
