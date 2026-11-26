// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// # La carpeta de Drive en Android (DR-12)
//
// En Android, la raíz de Drive es de serie la carpeta de la app, que solo ve Drive. En
// Ajustes → «Carpeta de Drive» la persona puede elegir otra con el selector de carpetas
// del sistema (Descargas, una tarjeta…): `ficheros.ElegirArbol`, que deja guardado el
// permiso de leer y escribir. Drive se cambia a ella en el acto (`Archivos.CambiarRaiz`,
// sobre un `archivos.Arbol`), la recuerda en `<datos>/drive-carpeta.json` y vuelve a ella
// al abrir; si el permiso se perdió (la persona lo quitó en el sistema, la tarjeta no está),
// Drive abre la de la app y lo dice. Un documento de ahí se abre en Docus, Calcus, Diapos
// o CertiPDF de Sytue Office si están instaladas y, si no (u otro fichero), con la app del
// sistema; las dos reciben permiso para guardarlo en su sitio (arbol.go). «Copiar a…» y
// «Subir carpeta» usan el mismo selector para elegir el destino o el origen.

// ficheroCarpeta es donde se recuerda la carpeta elegida, en la carpeta de datos.
const ficheroCarpeta = "drive-carpeta.json"

// carpetaElegida es la carpeta de Drive elegida con el selector; vacía, la de la app.
type carpetaElegida struct {
	URI    string `json:"uri"`
	Nombre string `json:"nombre"`
}

func leerCarpeta(datos string) carpetaElegida {
	var c carpetaElegida
	if raw, err := os.ReadFile(filepath.Join(datos, ficheroCarpeta)); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	if !strings.HasPrefix(c.URI, "content://") {
		return carpetaElegida{}
	}
	return c
}

func guardarCarpeta(datos string, c carpetaElegida) error {
	r := filepath.Join(datos, ficheroCarpeta)
	if c.URI == "" {
		if err := os.Remove(r); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return biblioteca.EscribirAtomico(r, append(raw, '\n'))
}

// raizDeArbol es la ruta (virtual) donde Drive monta la carpeta elegida: la que enseña
// «Detalles» y la que guarda el índice de `.sytue`. Va bajo `/content`, que no existe en
// Android, para no tapar nunca una ruta de verdad (un fichero suelto que se abre por la
// suya).
func raizDeArbol(nombre string) string {
	nombre = strings.TrimSpace(strings.ReplaceAll(nombre, "/", "_"))
	if nombre == "" || nombre == "." || nombre == ".." {
		nombre = "Drive"
	}
	return "/content/" + nombre
}

// proveedorArbol es lo que `ficheros.Arbol` sabe hacer con los documentos del árbol.
type proveedorArbol interface {
	Raiz() (ficheros.DocumentoArbol, error)
	Datos(id string) (ficheros.DocumentoArbol, error)
	Hijos(id string) ([]ficheros.DocumentoArbol, error)
	Crear(padre, nombre string, carpeta bool) (ficheros.DocumentoArbol, error)
	Renombrar(id, nombre string) (ficheros.DocumentoArbol, error)
	Mover(id, origen, destino string) (ficheros.DocumentoArbol, error)
	Borrar(id string) error
	Leer(id string) (io.ReadCloser, error)
	Escribir(id string) (io.WriteCloser, error)
	Espacio() (int64, int64, error)
	Ver(id string, abrir bool) error
	VerEn(id, paquete, tipo string) (bool, error)
}

var _ proveedorArbol = ficheros.Arbol{}

// documentos presenta un proveedorArbol como los `archivos.Documentos` de un Arbol.
type documentos struct{ p proveedorArbol }

var _ archivos.Documentos = documentos{}

func deArbol(d ficheros.DocumentoArbol) archivos.Documento {
	return archivos.Documento{ID: d.ID, Nombre: d.Nombre, Carpeta: d.Carpeta, Tamano: d.Tamano, Modificado: d.Modificado}
}

func uno(d ficheros.DocumentoArbol, err error) (archivos.Documento, error) {
	if errors.Is(err, ficheros.ErrNoSoportado) {
		return archivos.Documento{}, fmt.Errorf("%w: %w", archivos.ErrNoSoportado, err)
	}
	if err != nil {
		return archivos.Documento{}, err
	}
	return deArbol(d), nil
}

func (d documentos) Raiz() (archivos.Documento, error)           { return uno(d.p.Raiz()) }
func (d documentos) Datos(id string) (archivos.Documento, error) { return uno(d.p.Datos(id)) }
func (d documentos) Crear(padre, nombre string, carpeta bool) (archivos.Documento, error) {
	return uno(d.p.Crear(padre, nombre, carpeta))
}

func (d documentos) Renombrar(id, nombre string) (archivos.Documento, error) {
	return uno(d.p.Renombrar(id, nombre))
}

func (d documentos) Mover(id, origen, destino string) (archivos.Documento, error) {
	return uno(d.p.Mover(id, origen, destino))
}
func (d documentos) Borrar(id string) error                     { return d.p.Borrar(id) }
func (d documentos) Leer(id string) (io.ReadCloser, error)      { return d.p.Leer(id) }
func (d documentos) Escribir(id string) (io.WriteCloser, error) { return d.p.Escribir(id) }
func (d documentos) Espacio() (total, libre int64, err error)   { return d.p.Espacio() }
func (d documentos) Hijos(id string) ([]archivos.Documento, error) {
	hs, err := d.p.Hijos(id)
	if err != nil {
		return nil, err
	}
	out := make([]archivos.Documento, len(hs))
	for i, h := range hs {
		out[i] = deArbol(h)
	}
	return out, nil
}

// carpetaDrive es la carpeta en uso y lo que hace falta para cambiarla.
type carpetaDrive struct {
	datos string // la carpeta de datos, donde se recuerda la elegida
	app   string // la raíz de la app
	disco *archivos.Archivos

	// Lo del sistema (ficheros); las pruebas ponen otro.
	elegir  func() (ficheros.ArbolElegido, error)
	vigente func(uri string) bool
	soltar  func(uri string)
	abrir   func(uri string) proveedorArbol

	mu     sync.Mutex
	actual carpetaElegida // vacía: la de la app
	prov   proveedorArbol
	arbol  *archivos.Arbol
}

func nuevaCarpetaDrive(datos, app string) *carpetaDrive {
	return &carpetaDrive{
		datos: datos, app: app, elegir: ficheros.ElegirArbol, vigente: ficheros.ArbolVigente,
		soltar: ficheros.SoltarArbol, abrir: func(uri string) proveedorArbol { return ficheros.Arbol{URI: uri} },
	}
}

// sistema monta la carpeta elegida: su raíz y su sistema de ficheros.
func (c *carpetaDrive) sistema(e carpetaElegida) (string, *archivos.Arbol, proveedorArbol) {
	p := c.abrir(e.URI)
	raiz := raizDeArbol(e.Nombre)
	return raiz, archivos.NuevoArbol(raiz, documentos{p}), p
}

// abrirDisco abre Drive en la carpeta recordada si sigue teniendo permiso y se deja
// abrir; si no, en la de la app, y devuelve el nombre de la que no se pudo abrir para
// decirlo. Sin permiso se olvida; con permiso pero sin abrirse (la tarjeta no está) se
// sigue recordando para la próxima vez.
func (c *carpetaDrive) abrirDisco(o archivos.Opciones) (*archivos.Archivos, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := leerCarpeta(c.datos)
	perdida := ""
	if e.URI != "" {
		if c.vigente(e.URI) {
			raiz, arbol, p := c.sistema(e)
			o.Sistema = arbol
			if d, err := archivos.Abrir(raiz, o); err == nil {
				c.disco, c.actual, c.prov, c.arbol = d, e, p, arbol
				return d, "", nil
			}
		} else {
			_ = guardarCarpeta(c.datos, carpetaElegida{})
		}
		perdida = e.Nombre
	}
	o.Sistema = nil
	d, err := archivos.Abrir(c.app, o)
	c.disco = d
	return d, perdida, err
}

// usar pasa Drive a la carpeta que dio el selector; recordarla depende de que el permiso
// quede guardado. Devuelve si se recuerda.
func (c *carpetaDrive) usar(e ficheros.ArbolElegido) (bool, error) {
	elegida := carpetaElegida{URI: e.URI, Nombre: e.Nombre}
	raiz, arbol, p := c.sistema(elegida)
	if err := c.disco.CambiarRaiz(raiz, arbol); err != nil {
		if e.Persistente {
			c.soltar(e.URI)
		}
		return false, err
	}
	c.mu.Lock()
	antes := c.anterior()
	c.actual, c.prov, c.arbol = elegida, p, arbol
	c.mu.Unlock()
	if antes != "" && antes != e.URI {
		c.soltar(antes)
	}
	if !e.Persistente {
		return false, guardarCarpeta(c.datos, carpetaElegida{})
	}
	return true, guardarCarpeta(c.datos, elegida)
}

// volver pasa Drive a la carpeta de la app y suelta el permiso de la elegida.
func (c *carpetaDrive) volver() error {
	if err := c.disco.CambiarRaiz(c.app, nil); err != nil {
		return err
	}
	c.mu.Lock()
	antes := c.anterior()
	c.actual, c.prov, c.arbol = carpetaElegida{}, nil, nil
	c.mu.Unlock()
	if antes != "" {
		c.soltar(antes)
	}
	return guardarCarpeta(c.datos, carpetaElegida{})
}

// anterior es la carpeta que hay que soltar al cambiar: la que está en uso o, si al abrir
// no se pudo (la tarjeta no estaba), la que se sigue recordando. Con c.mu tomado.
func (c *carpetaDrive) anterior() string {
	if c.actual.URI != "" {
		return c.actual.URI
	}
	return leerCarpeta(c.datos).URI
}

// enArbol dice si ruta es de la carpeta elegida y, si lo es, cómo enseñarla: abrir, en su
// app de Sytue Office o con la del sistema; si no, la hoja de compartir. Otra app no puede
// leer una ruta de ahí, así que se le da su documento. Sin carpeta elegida (o fuera de
// Android, donde c es nil), nada lo es.
func (c *carpetaDrive) enArbol(ruta string) (func(abrir bool) error, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	arbol, p := c.arbol, c.prov
	c.mu.Unlock()
	if arbol == nil || ruta == "" {
		return nil, false
	}
	if r := filepath.Clean(ruta); r != arbol.Raiz() && !strings.HasPrefix(r, arbol.Raiz()+string(filepath.Separator)) {
		return nil, false
	}
	return func(abrir bool) error {
		id, err := arbol.Documento(ruta)
		if err != nil {
			return err
		}
		if abrir && verEnSuApp(p, id, filepath.Base(ruta)) {
			return nil
		}
		return p.Ver(id, abrir)
	}, true
}

// nombre es el de la carpeta elegida; "" si es la de la app.
func (c *carpetaDrive) nombre() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.actual.Nombre
}

// seccion es la pestaña «Carpeta de Drive» de Ajustes.
func (c *carpetaDrive) seccion(p *drive.Pantalla, t func(string) string) func(w fyne.Window) fyne.CanvasObject {
	return func(w fyne.Window) fyne.CanvasObject {
		ayuda := widget.NewLabel(t("carpetaAyuda"))
		ayuda.Wrapping = fyne.TextWrapWord
		ahora := widget.NewLabel("")
		ahora.Wrapping = fyne.TextWrapWord
		volver := widget.NewButton(t("carpetaVolver"), nil)
		poner := func() {
			if n := c.nombre(); n != "" {
				ahora.SetText(strings.ReplaceAll(t("carpetaActual"), "{name}", n))
				volver.Show()
				return
			}
			ahora.SetText(t("carpetaApp"))
			volver.Hide()
		}
		listo := func(err error, aviso string) {
			fyne.Do(func() {
				poner()
				if err != nil {
					local.Info(w, t("carpetaFallo"), err.Error())
					return
				}
				p.IrAEnlace("/", "", nil)
				if aviso != "" {
					local.Info(w, t("carpetaTitulo"), aviso)
				}
			})
		}
		elegir := widget.NewButton(t("carpetaElegir"), func() {
			go func() {
				e, err := c.elegir()
				switch {
				case errors.Is(err, ficheros.ErrSoloLectura):
					fyne.Do(func() { local.Info(w, t("carpetaTitulo"), t("carpetaSoloLectura")) })
					return
				case err != nil:
					listo(err, "")
					return
				case e.URI == "":
					return // canceló
				}
				recordada, err := c.usar(e)
				aviso := ""
				if err == nil && !recordada {
					aviso = t("carpetaTemporal")
				}
				listo(err, aviso)
			}()
		})
		elegir.Importance = widget.HighImportance
		volver.OnTapped = func() { go func() { listo(c.volver(), "") }() }
		poner()
		return container.NewVBox(ayuda, ahora, container.NewHBox(elegir, volver))
	}
}
