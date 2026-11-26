// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// control une la ventana con el motor local: qué documento hay abierto y por qué ruta.
// Es el de `cmd/certipdf/control.go` sin sesión: no hay Drive al que subir lo que se
// abre, ni Hub al que volver.
type control struct {
	a     fyne.App
	l     *local.App
	w     fyne.Window
	t     *i18n.Translator
	pdf   *api.PDF
	disco *archivos.Archivos
	p     *certipdf.Pantalla

	mu   sync.Mutex
	ruta string
}

func (c *control) ultima() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ruta
}

// abrirApertura atiende lo que llega por «Abrir con», los argumentos, soltar en la
// ventana o un `sytueoffice-pdf:` de otra app.
func (c *control) abrirApertura(ctx context.Context, ap api.Apertura) error {
	if ap.Local == "" {
		if ap.Ref.Ruta == "" {
			return local.ErrSinFichero
		}
		return c.abrirRuta(ctx, ap.Ref.Ruta)
	}
	if ruta, ok := rutaDeURI(ap.Local); ok {
		return c.abrirRuta(ctx, ruta)
	}
	// `content://` (Android) no tiene ruta: se lee y se guarda en «PDF» de la carpeta de
	// datos, que es la única forma de poder guardarlo después. Se lee con el tope del motor
	// (100 MiB, P-10), no con los 25 MiB del conversor.
	return c.abrirCopia(ctx, ap.Local)
}

// rutaDeURI es la ruta del disco de un `file://`.
func rutaDeURI(uri string) (string, bool) {
	u, err := storage.ParseURI(uri)
	if err != nil || u.Scheme() != "file" {
		return "", false
	}
	return filepath.Clean(u.Path()), true
}

// abrirRuta abre un PDF por su ruta: la absoluta del disco (un fichero suelto, que NO se
// copia) o la de la carpeta de datos («/PDF/x.pdf»). Los fallos se enseñan en la
// pantalla, con «Reintentar».
//
//nolint:contextcheck // el documento vive más que la petición que lo abrió
func (c *control) abrirRuta(ctx context.Context, ruta string) error {
	c.mu.Lock()
	c.ruta = ruta
	c.mu.Unlock()
	fyne.Do(func() {
		c.p.Barra().DesdeDrive = false
		c.p.Cargar()
	})
	doc, err := certipdf.AbrirDocumento(ctx, c.pdf, ruta)
	if err != nil {
		slog.Info("certipdf: no se pudo abrir", "error", err)
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return nil //nolint:nilerr // el fallo ya se enseña en la pantalla, con «Reintentar»: devolverlo lo repetiría en un diálogo
	}
	c.mostrar(doc)
	return nil
}

// abrirCopia lee un fichero sin ruta y lo guarda en «PDF» antes de abrirlo.
//
//nolint:contextcheck // como abrirRuta
func (c *control) abrirCopia(ctx context.Context, uri string) error {
	nombre, datos, err := local.LeerLocalHasta(uri, api.PDFMaxBytes)
	if err != nil {
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return err
	}
	fyne.Do(func() { c.p.Cargar() })
	doc, err := certipdf.SubirYAbrir(ctx, c.pdf, nombre, datos)
	if err != nil {
		fyne.Do(func() { c.p.FalloAlAbrir(err) })
		return err
	}
	c.mu.Lock()
	c.ruta = doc.Ruta()
	c.mu.Unlock()
	c.mostrar(doc)
	return nil
}

func (c *control) mostrar(doc *certipdf.Documento) {
	fyne.Do(func() {
		c.w.SetTitle(doc.Nombre() + " — " + c.a.Metadata().Name)
		c.p.Abrir(doc)
	})
}

// portada es la salida de la cinta: sin Hub ni Drive al que volver, se vuelve a la
// portada con los recientes.
func (c *control) portada() {
	c.mu.Lock()
	c.ruta = ""
	c.mu.Unlock()
	c.p.Callar()
	c.w.SetTitle(c.a.Metadata().Name)
	c.p.MostrarPortada(c.pdf, c.t.Locale())
}

// crear es «PDF · En blanco»: una página A4 en «PDF» de la carpeta de datos.
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
	c.ruta = doc.Ruta()
	c.mu.Unlock()
	c.mostrar(doc)
}

// elegir es «Abrir un archivo»: el selector del sistema. Un PDF se abre aquí por su
// ruta; un fichero de otra app se le pasa al sistema.
func (c *control) elegir() {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil || len(rutas) == 0 {
			return
		}
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

// abrirElegido comprueba que es un PDF antes de dejar la portada.
func (c *control) abrirElegido(ruta string) {
	c.estadoPortada(true, "")
	cabecera := make([]byte, 5)
	f, err := os.Open(ruta)
	if err == nil {
		_, err = f.Read(cabecera)
		_ = f.Close()
	}
	switch {
	case err != nil:
		c.estadoPortada(false, c.t.T("pdf.open.failed", nil))
		return
	case !bytes.Equal(cabecera, []byte("%PDF-")):
		c.estadoPortada(false, c.t.T("pdf.open.notPdf", nil))
		return
	}
	c.estadoPortada(false, "")
	_ = c.abrirRuta(context.Background(), ruta)
}

func (c *control) estadoPortada(abriendo bool, fallo string) {
	fyne.Do(func() { c.p.EstadoAbrir(abriendo, fallo) })
}

// escanear convierte fotos en UN PDF con el motor local y lo abre.
func (c *control) escanear() {
	go func() {
		foto, err := ficheros.Fotografiar(context.Background())
		if foto != "" {
			defer func() { _ = os.Remove(foto) }()
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
			nombre, datos, err := leerPorRuta(r)
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
		_ = c.abrirRuta(ctx, f.Path)
	}()
}

// leerPorRuta lee un fichero del disco con el tope del motor (100 MiB, api.PDFMaxBytes) y
// no con los 25 MiB de `local.LeerLocal` (P-10): una foto o un PDF que el motor acepta no
// se corta antes por el camino.
func leerPorRuta(ruta string) (string, []byte, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = f.Close() }()
	datos, err := io.ReadAll(io.LimitReader(f, api.PDFMaxBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(datos) > api.PDFMaxBytes {
		return "", nil, api.ErrPDFGrande
	}
	return filepath.Base(ruta), datos, nil
}
