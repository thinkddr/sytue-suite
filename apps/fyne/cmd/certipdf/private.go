// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// pdfVista es un presentador y su control montados sobre el repositorio privado de
// Drive del dispositivo verificado.
type pdfVista struct {
	pantalla *certipdf.Pantalla
	control  *control
}

// pdfStorage activa CertiPDF con un dispositivo: es la app CONSUMIDORA del
// repositorio privado de Drive, al que pide abrir, guardar, copiar, renombrar y
// papelera. El motor de PDF corre en LOCAL: nada del documento sale al servicio
// de transformación. Sin repositorio verificado no hay portada legible.
type pdfStorage struct {
	suite      *sesion.Suite
	aplicacion fyne.App
	ventana    fyne.Window
	traductor  *i18n.Translator

	drive   *cifrado.AplicacionDrive
	errores error
	vista   atomic.Pointer[pdfVista]
}

func newPDFStorage(suite *sesion.Suite, application fyne.App, window fyne.Window, translate *i18n.Translator) (*pdfStorage, error) {
	host := &pdfStorage{suite: suite, aplicacion: application, ventana: window, traductor: translate}
	texto := func(key string) string { return translate.T(key, nil) }
	// El repositorio de Drive va sobre la bóveda directa cuando la web ya llevó
	// ahí el Drive de esa persona, y si no, sobre Matrix como hasta ahora (ADR 0086).
	host.drive, host.errores = cifrado.NuevaAplicacionDrive(cifrado.AplicacionDriveConfig{
		Matrix: cifrado.PrivateApplicationConfig[*privateDrive.Repository]{
			Suite: suite, Application: application, Window: window, Product: "drive", Translate: texto,
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateDrive.Repository], error) {
				return cifrado.NewDriveApplicationPlan(ctx, resources, suite.URL("drive"), suite.URL("rooms"))
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
				return host.montar(resources, repository) //nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
			},
		},
		MontarBoveda: func(_ context.Context, cuenta cifrado.Cuenta, repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
			return host.montar(cuenta, repository) //nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
		},
	})
	if host.drive == nil && host.errores == nil {
		host.errores = cifrado.ErrPrivateResources
	}
	return host, host.errores
}

// montar es el arranque real de la app: la pantalla de CertiPDF sobre la fachada
// privada de Drive. Lo que sube va al repositorio cifrado y lo que transforma corre
// en el dispositivo; el servicio de IA sólo recibe lo que la persona le escriba,
// nunca el documento.
func (h *pdfStorage) montar(resources cifrado.Cuenta, repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
	pdf, err := api.NewPDF(h.suite.URL("docs"), h.suite.URL("drive"), resources.HTTP(), nil).WithPrivateDrive(repository)
	if err != nil {
		return cifrado.ApplicationView{}, err
	}
	p := certipdf.NuevaPantalla(h.traductor.T)
	p.IA = api.NewIA(h.suite.URL("ai"), resources.HTTP(), nil)
	p.Idioma = string(h.traductor.Locale())
	// Las tarjetas y los almacenes del sistema, para firmar en dos pasos (ADR 0051).
	p.Almacenes = certificados.DelSistema
	c := &control{a: h.aplicacion, s: h.suite, w: h.ventana, t: h.traductor, pdf: pdf, p: p}
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
			if err := h.suite.Entrar(context.Background()); err != nil {
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
	p.Atajos(h.ventana.Canvas())
	// Sin documento es la portada de la web (`PortadaPdf`): lo que hay en la carpeta
	// «PDF» del repositorio privado.
	p.MostrarPortada(pdf, h.traductor.Locale())
	// Lo que se anota va firmado con el nombre de quien ha entrado, como en la web.
	c.conocerAutor(h.suite.Estado())
	vista := &pdfVista{pantalla: p, control: c}
	h.vista.Store(vista)
	return cifrado.ApplicationView{
		Object: p.Objeto(),
		Detach: func() {
			p.Callar()
			if h.vista.Load() == vista {
				h.vista.Store(nil)
			}
		},
	}, nil
}

// abrirFichero atiende un PDF pulsado en Drive (`sytue-pdf:/pdf?path=…&volver=…`) o
// uno de la máquina: sin repositorio montado no hay nada legible que abrir, y el
// fallo es explícito, nunca una portada vacía.
func (h *pdfStorage) abrirFichero(ctx context.Context, ap api.Apertura) error {
	if vista := h.vista.Load(); vista != nil {
		if ap.Local != "" {
			return vista.control.abrirLocal(ctx, ap.Local)
		}
		return vista.control.abrirRuta(ctx, ap.Ref.Ruta, ap.Volver)
	}
	h.drive.Show()
	return api.ErrPrivatePDFOperation
}

func (h *pdfStorage) Objeto() fyne.CanvasObject {
	if h.drive != nil {
		return h.drive.Objeto()
	}
	return container.NewStack()
}

func (h *pdfStorage) Start() {
	if h.drive != nil {
		h.drive.Start()
	}
}

func (h *pdfStorage) Close() {
	if vista := h.vista.Load(); vista != nil {
		vista.pantalla.Callar()
	}
	if h.drive != nil {
		h.drive.Close()
	}
}
