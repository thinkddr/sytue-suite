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
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/certipdf/...`, que pinta lo mismo con
// el pintor de software y compara los píxeles.
package main

// La portada y el estudio son los de `apps/certipdf`; esta es su app, como en la web
// la de `apps/certipdf` monta la vista `Pdf` de `@sytue/office` en su portada y en `/pdf`.
//
// paridad-web: apps/frontend/apps/certipdf/src/pages/index.astro
// paridad-web: apps/frontend/apps/certipdf/src/pages/pdf.astro

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
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

	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue CertiPDF")
	w.SetPadded(false)

	pdf := api.NewPDF(s.URL("docs"), s.URL("drive"), s.Sesion, nil)
	p := certipdf.NuevaPantalla(tr.T)
	// El Asistente va en directo al servicio de IA, por el ingress que el chart abre a
	// las apps nativas; la web pasa por su BFF, que solo reenvía.
	p.IA = api.NewIA(s.URL("ai"), s.Sesion, nil)
	p.Idioma = string(tr.Locale())
	// Las tarjetas y los almacenes del sistema, para firmar en dos pasos (ADR 0051).
	p.Almacenes = certificados.DelSistema
	// La voz del sistema no se calla sola al cerrar: sigue en su propio proceso.
	w.SetOnClosed(p.Callar)
	c := &control{a: a, s: s, w: w, t: tr, pdf: pdf, p: p}
	// Lo que se anota va firmado con el nombre de quien ha entrado, como en la web: se
	// pide a Sytue ID al entrar (el estado transversal solo lleva el `sub`).
	s.AlCambiar(c.conocerAutor)
	c.conocerAutor(s.Estado())
	p.AlVolver = c.salir
	// Lo que falle al abrir desde la propia ventana se enseña en ella: no hay enlace
	// entrante al que devolverle el error.
	p.AlAbrirRuta = func(ruta string) { go func() { _ = c.abrirRuta(context.Background(), ruta, c.carpeta()) }() }
	p.AlReintentar = func() {
		if r, v := c.ultima(); r != "" {
			go func() { _ = c.abrirRuta(context.Background(), r, v) }()
		}
	}
	// Sin sesión se entra con Sytue ID en el navegador del sistema y, al volver, se
	// reintenta lo que se estaba abriendo.
	p.AlEntrar = func() {
		go func() {
			if err := s.Entrar(context.Background()); err != nil {
				slog.Info("certipdf: no se ha entrado", "error", err)
				return
			}
			if r, v := c.ultima(); r != "" {
				_ = c.abrirRuta(context.Background(), r, v)
			}
		}()
	}
	// «Crear nuevo», «Abrir un archivo» y «Escanear»: en la portada y, abierto un
	// documento, en el grupo «Archivo» de la cinta.
	p.AlCrear = func() { go c.crear() }
	p.AlElegir = c.elegir
	p.AlAbrirLocal = func(ruta string) { go c.abrirElegido(ruta) }
	p.AlEscanear = c.escanear
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.PDF).ConSinSesionPropia().Montar(p.Objeto()))
	p.Atajos(w.Canvas())
	// Sin documento es la portada de la web (`PortadaPdf`): lo que hay en la carpeta
	// «PDF» de Drive.
	p.MostrarPortada(pdf, tr.Locale())
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	// Un PDF pulsado en Drive llega como `sytue-pdf:/pdf?path=…&volver=…`: CertiPDF abre
	// por RUTA y no por identificador, que es lo que acepta su página en la web. Si no
	// se puede traer, la sesión lo manda a esa misma página en la web —o a «no
	// encontrada», si el fichero ya no está—.
	s.AlAbrirFichero("pdf", func(ctx context.Context, ap api.Apertura) error {
		if ap.Local != "" {
			return c.abrirLocal(ctx, ap.Local)
		}
		return c.abrirRuta(ctx, ap.Ref.Ruta, ap.Volver)
	})
	// Un PDF de la máquina —soltado en la ventana, con «Abrir con» o en los argumentos—
	// se sube a la carpeta «PDF» de Drive y se abre, como «abrir un PDF del ordenador».
	s.AceptarFicheros(w, tr.T("pdf.error.open", nil), tr.T("pdf.error.upload", nil))
	// Y mientras se arrastra por encima, la zona de abrir de la portada se enciende.
	soltar.AlArrastrar(w, p.ArrastrarEncima)
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// control es lo que une la ventana con el motor: qué documento hay abierto y de dónde
// vino.
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
// (`sesion/elegir_movil.go`): uno cada vez, ya copiado a una ruta.
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
