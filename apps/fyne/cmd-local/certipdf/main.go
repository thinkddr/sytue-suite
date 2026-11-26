// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command certipdf es Sytue CertiPDF de Sytue Office: el mismo editor de PDF de la suite
// sobre los ficheros del disco, en claro, sin cuenta y sin servidor
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.3).
//
// Comparado con `cmd/certipdf`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión) ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado). La fachada
// `api.PDF` va enchufada con `WithPrivateDrive` a `local/archivos`, así que el motor de
// PDF corre entero en el dispositivo (`localMotor`) y lo que no tiene camino local falla
// cerrado, sin caer a ningún servicio.
//
//   - Un PDF de cualquier carpeta se abre por su ruta y NO se copia (D-24, P-01); guardar
//     lo escribe en su sitio con la versión anterior en `.sytue/versiones` (P-02).
//   - Lo nuevo (en blanco, escaneos) va a `~/Sytue Office/Drive/PDF`.
//   - No se monta «Pedir firma» (necesita a otra persona y un servidor).
//   - La firma con el certificado del sistema (tarjetas por PKCS#11, Llavero, almacén de
//     Windows, KeyChain) prepara y completa en el motor local (P-03, P-04); el sello de
//     tiempo es de la autoridad que elija la persona, o ninguno (P-05), y las anclas de
//     confianza, los certificados de una carpeta suya (P-06). Ver firma.go.
//   - «Reconocer texto» usa tesseract si está instalado; si no, en Windows, el OCR del
//     sistema (`Windows.Media.Ocr`) y, si no hay o falla, el modelo de visión de la
//     persona (P-07, ocr.go).
//   - La IA es la que la persona configura en Ajustes; la primera vez avisa a quién se
//     envía (P-08). Las anotaciones las firma el nombre del perfil (P-09).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// Medidas de la ventana al abrir, las de la edición conectada.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "pdf")
	if err != nil {
		slog.Error("certipdf", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := l.Traductor(catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	persona := l.Espacio.Persona()
	disco, err := archivos.Abrir(filepath.Join(l.Espacio.Rutas.Datos, "Drive"), archivos.Opciones{Instalacion: l.Espacio.ID, Autor: persona.Nombre})
	if err != nil {
		slog.Error("no se pudo abrir la carpeta de datos", "error", err)
		os.Exit(1)
	}
	// Las raíces no se usan: WithPrivateDrive cierra todo lo que iría por HTTP.
	pdf, err := api.NewPDF("https://sin-servidor.invalid", "https://sin-servidor.invalid", nil, nil).WithPrivateDrive(disco)
	if err != nil {
		slog.Error("certipdf", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	p := certipdf.NuevaPantalla(tr.T)
	p.Idioma = string(tr.Locale())
	p.Autor = persona.Nombre
	p.Barra().SinPedirFirma = true
	// La ruta de un reciente es la del disco: FileByPath la resuelve sin copiar nada.
	p.RutaDeReciente = func(f api.File) string { return f.Path }
	aviso := &avisoIA{l: l, w: w}
	ia := &iaActual{}
	ia.poner(aviso.envolver(l.Asistir()))
	p.Asistir = ia.leer()
	l.AlCambiarIA(func() {
		f := aviso.envolver(l.Asistir())
		ia.poner(f)
		fyne.Do(func() { p.Asistir = f })
	})

	// Firma con el certificado del sistema, sello, anclas y OCR, en el dispositivo.
	idioma := tr.Locale()
	p.Almacenes = certificados.DelSistema
	firma := nuevosAjustesFirma(l)
	firma.pantalla(p, propio(idioma, "anclasTitulo"))
	// La autoridad de sellado y las anclas, también en Ajustes (la pestaña de CertiPDF).
	l.AjustesDeLaApp("CertiPDF", func(fyne.Window) fyne.CanvasObject { return p.AjustesLocales() })
	ocr := &reconocedor{asistir: ia.leer, buscar: buscarTesseract, sistema: ocrWindows{}}
	p.NotaOCR = func() string { return ocr.descripcion(idioma) }
	pdf.ConLocal(api.PDFLocal{Sello: firma.opcionesSello, Anclas: firma.pool, Reconocedor: ocr})

	c := &control{a: a, l: l, w: w, t: tr, pdf: pdf, disco: disco, p: p}
	p.AlVolver = c.portada
	p.AlAbrirRuta = func(ruta string) { go func() { _ = c.abrirRuta(context.Background(), ruta) }() }
	p.AlReintentar = func() {
		if r := c.ultima(); r != "" {
			go func() { _ = c.abrirRuta(context.Background(), r) }()
		}
	}
	p.AlCrear = func() { go c.crear() }
	p.AlElegir = c.elegir
	p.AlAbrirLocal = func(ruta string) { go func() { _ = c.abrirRuta(context.Background(), ruta) }() }
	p.AlEscanear = c.escanear
	p.Atajos(w.Canvas())
	p.MostrarPortada(pdf, tr.Locale())

	l.AlAbrirFichero(c.abrirApertura, func(err error) { l.Fallo(w, err) })
	l.AceptarFicheros(w)
	soltar.AlArrastrar(w, p.ArrastrarEncima)
	// «Guardar en la nube»: el PDF abierto, tal cual está en el disco.
	l.AlExportar(func() (string, []byte, error) {
		r := c.ultima()
		if r == "" {
			return "", nil, fmt.Errorf("abre un PDF primero")
		}
		f, err := pdf.PorRuta(context.Background(), r)
		if err != nil {
			return "", nil, err
		}
		datos, err := pdf.Descargar(context.Background(), f.ID)
		return f.Name, datos, err
	})
	l.AlCerrar(p.Callar)

	w.SetContent(l.Montar(w, iconos.PDF, p.Objeto()))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))
	// Un PDF por su ruta (`?clase=fichero&id=…`, la clase de los ficheros del Hub): como
	// `?local=`, sin copiarlo.
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "fichero" && id != "" {
			go func() { _ = c.abrirRuta(context.Background(), id) }()
		}
	})
	w.ShowAndRun()
}

// iaActual guarda la IA de la persona para quien la lee fuera del hilo de la interfaz
// (el OCR): cambia cuando la persona la cambia en Ajustes.
type iaActual struct {
	mu sync.Mutex
	f  func(context.Context, api.PeticionIA) (string, error)
}

func (i *iaActual) poner(f func(context.Context, api.PeticionIA) (string, error)) {
	i.mu.Lock()
	i.f = f
	i.mu.Unlock()
}

func (i *iaActual) leer() func(context.Context, api.PeticionIA) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.f
}

// avisoIA es el aviso de primera vez (P-08): la barrera `CheckRemoteProcessing` de la
// edición conectada impide mandar a un servicio el texto de un documento privado; aquí la
// IA la eligió la persona, así que en vez de bloquear se le pregunta una vez por destino
// si quiere enviarle el texto de sus PDF. La respuesta se recuerda en sus ajustes.
type avisoIA struct {
	l *local.App
	w fyne.Window
}

const claveAvisoIA = "certipdf_ia_aceptado" // con guion bajo: con forma de clave de i18n, `catalogos` la buscaría

func (v *avisoIA) envolver(f func(context.Context, api.PeticionIA) (string, error)) func(context.Context, api.PeticionIA) (string, error) {
	if f == nil {
		return nil
	}
	return func(ctx context.Context, q api.PeticionIA) (string, error) {
		// Lo aceptado se recuerda con el destino del núcleo (no cambia con el idioma); el
		// aviso lo dice en el de la app.
		destino := v.l.IA.Destino()
		personal := v.l.Personal()
		if personal.Leer(claveAvisoIA) != destino && !v.preguntar(ctx, v.l.DestinoIA()) {
			return "", fmt.Errorf("certipdf: la persona no aceptó enviar el documento a %s", destino)
		}
		personal.Poner(claveAvisoIA, destino)
		return f(ctx, q)
	}
}

// preguntar enseña el aviso y espera la respuesta. Se llama fuera del hilo de la interfaz.
func (v *avisoIA) preguntar(ctx context.Context, destino string) bool {
	idioma := v.l.Idioma()
	titulo, cancelar, enviar := propio(idioma, "iaTitulo"), propio(idioma, "iaCancelar"), propio(idioma, "iaEnviar")
	r := make(chan bool, 1)
	fyne.Do(func() {
		etiqueta := widget.NewLabel(fmt.Sprintf(propio(idioma, "iaTexto"), destino))
		etiqueta.Wrapping = fyne.TextWrapWord
		var d *ui.Dialogo
		// Cerrar con Escape o «atrás» cuenta como no: el canal tiene hueco para uno.
		d = ui.Confirmacion(titulo, cancelar, enviar, false, etiqueta,
			func() { d.Cerrar(); r <- false },
			func() { d.Cerrar(); r <- true })
		d.Mostrar(v.w.Canvas())
	})
	select {
	case si := <-r:
		return si
	case <-ctx.Done():
		return false
	}
}
