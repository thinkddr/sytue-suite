// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// # Lo demás de las carpetas del teléfono (DR-12)
//
// En Android no hay selector de carpetas que dé una RUTA: el del sistema da un árbol de
// documentos (`ficheros.ElegirArbol`, el de «Carpeta de Drive»). Con él:
//
//   - «Copiar a…» elige el destino y copia por `DocumentsContract`
//     (`archivos.Remotas.CopiarFueraEn` sobre un `archivos.Arbol`);
//   - «Subir carpeta» elige el origen y sube sus ficheros con su árbol, leyendo cada uno
//     del proveedor cuando le toca (no se abren todos a la vez);
//   - abrir un documento de la carpeta de Drive elegida lo da a Docus, Calcus, Diapos o
//     CertiPDF de Sytue Office con un `ACTION_VIEW` explícito y permiso de leerlo y
//     escribirlo (`ficheros.Arbol.VerEn`): un `content://` no viaja con permiso por
//     `sytueoffice-<producto>:`. Sin esa app instalada, con la del sistema, como antes.
//
// El permiso que el selector deja guardado sobre la carpeta elegida para copiar o subir se
// suelta al acabar, salvo que sea la carpeta de Drive.

// appsQueAbren son las apps de Sytue Office que abren un documento de la carpeta elegida,
// por la clave de producto que da `productoDe`: la carpeta de su `cmd-local`, que es el
// final de su paquete (`eu.sytue.office.<app>`).
var appsQueAbren = map[string]string{"docs": "docus", "sheets": "calcus", "slides": "diapos", "pdf": "certipdf"}

// tiposOffice es el tipo MIME que esas apps declaran para cada extensión en su filtro de
// «Abrir con» (`empaquetar.sh`, `tipo_mime` sobre las `extensiones` de su FyneApp.toml):
// el VIEW explícito tiene que llevar uno de esos para que Android lo entregue.
// TestLosTiposSonLosDelManifiesto lo compara con el script.
var tiposOffice = map[string]string{
	"docx":    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"dotx":    "application/vnd.openxmlformats-officedocument.wordprocessingml.template",
	"odt":     "application/vnd.oasis.opendocument.text",
	"ott":     "application/vnd.oasis.opendocument.text-template",
	"pages":   "application/vnd.apple.pages",
	"txt":     "text/plain",
	"md":      "text/markdown",
	"html":    "text/html",
	"docus":   "application/vnd.sytue.docus+zip",
	"xlsx":    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"xltx":    "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	"ods":     "application/vnd.oasis.opendocument.spreadsheet",
	"ots":     "application/vnd.oasis.opendocument.spreadsheet-template",
	"numbers": "application/vnd.apple.numbers",
	"csv":     "text/csv",
	"tsv":     "text/tab-separated-values",
	"calcus":  "application/vnd.sytue.calcus+zip",
	"pptx":    "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"potx":    "application/vnd.openxmlformats-officedocument.presentationml.template",
	"odp":     "application/vnd.oasis.opendocument.presentation",
	"otp":     "application/vnd.oasis.opendocument.presentation-template",
	"key":     "application/vnd.apple.keynote",
	"diapos":  "application/vnd.sytue.diapos+zip",
	"pdf":     "application/pdf",
}

// suApp es el paquete de la app de Sytue Office que abre ese fichero y el tipo con el que
// pedírselo; ok es falso si ninguna lo abre (va a la del sistema).
func suApp(nombre string) (paquete, tipo string, ok bool) {
	app, hay := appsQueAbren[productoDe(nombre)]
	tipo, conTipo := tiposOffice[drive.Extension(nombre)]
	if !hay || !conTipo {
		return "", "", false
	}
	return "eu.sytue.office." + app, tipo, true
}

// verEnSuApp abre el documento id del árbol en su app de Sytue Office. Falso si no hay
// app para él, no está instalada o no lo acepta, o si falla: quien llama lo abre con la
// del sistema, como siempre.
func verEnSuApp(p proveedorArbol, id, nombre string) bool {
	paquete, tipo, ok := suApp(nombre)
	if !ok {
		return false
	}
	abierto, err := p.VerEn(id, paquete, tipo)
	return err == nil && abierto
}

// montar da el sistema de ficheros de una carpeta elegida con el selector para copiar o
// subir: el de Drive si es la misma (así «una carpeta no se copia dentro de sí misma» la
// reconoce) y si no uno nuevo. soltar devuelve el permiso que el selector dejó guardado,
// salvo que sea el de la carpeta de Drive (la de ahora o la que se recuerda).
func (c *carpetaDrive) montar(e ficheros.ArbolElegido) (s archivos.Sistema, raiz string, soltar func()) {
	c.mu.Lock()
	actual, arbol, deDrive := c.actual, c.arbol, c.anterior()
	c.mu.Unlock()
	if arbol != nil && e.URI == actual.URI {
		r := arbol.Raiz()
		return arbol, r, func() {}
	}
	raiz, nuevo, _ := c.sistema(carpetaElegida{URI: e.URI, Nombre: e.Nombre})
	var una sync.Once
	return nuevo, raiz, func() {
		una.Do(func() {
			if e.Persistente && e.URI != deDrive {
				c.soltar(e.URI)
			}
		})
	}
}

// copiarA es «Copiar a…» en el teléfono: pide la carpeta de destino al selector y deja ahí
// la copia de id con copiar (`Remotas.CopiarFueraEn`). Devuelve cómo enseñar dónde ha
// quedado («Download/plan (2).pdf»); elegida es falso si la persona canceló.
func (c *carpetaDrive) copiarA(ctx context.Context, id string,
	copiar func(ctx context.Context, id string, s archivos.Sistema, carpeta string) (string, error),
) (destino string, elegida bool, err error) {
	e, err := c.elegir()
	if err != nil {
		return "", true, err // ErrSoloLectura incluido: no se puede dejar nada ahí
	}
	if e.URI == "" {
		return "", false, nil
	}
	s, raiz, soltar := c.montar(e)
	defer soltar()
	got, err := copiar(ctx, id, s, raiz)
	if err != nil {
		return "", true, err
	}
	return strings.TrimPrefix(got, "/content/"), true, nil
}

// entradasDe es «Subir carpeta» en el teléfono: pide la carpeta al selector (vale una que
// solo deja leer) y da sus ficheros como entradas de subida, cada uno en su subcarpeta
// empezando por el nombre de la elegida, como al soltar una carpeta en el escritorio
// (`drive.EntradasDe`). Lo oculto (`.sytue`, `.thumbnails`) no sube. Cada fichero se abre
// al subirlo, y el permiso se suelta cuando se ha cerrado el último. Sin nada elegido, nil.
func (c *carpetaDrive) entradasDe() ([]drive.Entrada, error) {
	e, err := c.elegir()
	if err != nil && (!errors.Is(err, ficheros.ErrSoloLectura) || !e.SoloLectura) {
		return nil, err
	}
	if e.URI == "" {
		return nil, nil
	}
	s, raiz, soltar := c.montar(e)
	nombre := strings.TrimSpace(e.Nombre)
	if nombre == "" || strings.ContainsAny(nombre, `/\`) || nombre == "." || nombre == ".." {
		nombre = path.Base(filepath.ToSlash(raiz))
	}
	var rec recogidos
	if err := s.WalkDir(raiz, func(r string, d fs.DirEntry, err error) error { return rec.visitar(raiz, nombre, r, d, err) }); err != nil {
		soltar()
		return nil, err
	}
	if len(rec.rutas) == 0 {
		soltar()
		return nil, nil
	}
	quedan := &cuenta{n: len(rec.rutas), fin: soltar}
	out := make([]drive.Entrada, len(rec.rutas))
	for i, r := range rec.rutas {
		en := drive.Fluido(path.Base(filepath.ToSlash(r)), rec.tamanos[i], &perezoso{abrir: func() (io.ReadCloser, error) { return s.Open(r) }, alCerrar: quedan.una})
		en.Dir = rec.dirs[i]
		out[i] = en
	}
	return out, nil
}

// recogidos son los ficheros de la carpeta elegida que «Subir carpeta» sube: la ruta de
// cada uno, su tamaño (-1 si no se sabe) y la subcarpeta donde va.
type recogidos struct {
	rutas   []string
	tamanos []int64
	dirs    []string
}

// visitar es el paso de WalkDir por r, dentro de raiz: apunta los ficheros, cada uno en
// su subcarpeta empezando por nombre, y salta lo oculto y lo que no se puede leer.
func (rec *recogidos) visitar(raiz, nombre, r string, d fs.DirEntry, err error) error {
	if err != nil {
		// Lo que no se puede leer se salta, como al soltar una carpeta.
		if d != nil && d.IsDir() && r != raiz {
			return filepath.SkipDir
		}
		return nil
	}
	if r != raiz && strings.HasPrefix(d.Name(), ".") {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if !d.Type().IsRegular() {
		return nil
	}
	rel, err := filepath.Rel(raiz, filepath.Dir(r))
	if err != nil {
		return nil //nolint:nilerr // lo que no cae dentro de la raíz no se sube, y el recorrido sigue
	}
	tamano := int64(-1) // sin decirlo, la subida va troceada
	if fi, err := d.Info(); err == nil && fi.Size() > 0 {
		tamano = fi.Size()
	}
	rec.rutas = append(rec.rutas, r)
	rec.tamanos = append(rec.tamanos, tamano)
	rec.dirs = append(rec.dirs, path.Join(nombre, filepath.ToSlash(rel)))
	return nil
}

// cuenta llama a fin cuando se han cerrado las n entradas.
type cuenta struct {
	mu  sync.Mutex
	n   int
	fin func()
}

func (c *cuenta) una() {
	c.mu.Lock()
	c.n--
	fin := c.n == 0
	c.mu.Unlock()
	if fin {
		c.fin()
	}
}

// perezoso es un fichero que se abre al leerlo por primera vez: una carpeta entera no
// abre todos sus ficheros antes de subir el primero.
type perezoso struct {
	abrir    func() (io.ReadCloser, error)
	alCerrar func()

	r       io.ReadCloser
	err     error
	cerrado bool
}

func (p *perezoso) Read(b []byte) (int, error) {
	if p.r == nil && p.err == nil {
		if p.cerrado {
			return 0, fs.ErrClosed
		}
		p.r, p.err = p.abrir()
	}
	if p.err != nil {
		return 0, p.err
	}
	return p.r.Read(b)
}

func (p *perezoso) Close() error {
	if p.cerrado {
		return nil
	}
	p.cerrado = true
	var err error
	if p.r != nil {
		err = p.r.Close()
	}
	if p.alCerrar != nil {
		p.alCerrar()
	}
	return err
}
