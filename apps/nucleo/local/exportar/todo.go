// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateBases "git.sytue.eu/sytue/one/apps/nucleo/bases"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # Todas las apps a la vez (el Hub)
//
// [Todo] abre el almacén o la biblioteca de cada app con el MISMO repositorio que usa
// esa app (`notes.NewLocal`, `calendar.New`… sobre `almacen.Abrir`, como «Traer desde
// Sytue») y llama a la misma función de exportar que su Ajustes → Copia. Los almacenes
// del motor no admiten dos escritores, así que quien llama se asegura antes de que esas
// apps están cerradas ([Productos]). Lo que nunca se abrió (sin su diario o sin la
// carpeta de su biblioteca) no se abre: no se crea un almacén vacío para exportarlo.

// Productos son las apps (por su clave de producto) que exportan algo, en el orden del
// lanzador: tienen que estar cerradas mientras [Todo] trabaja.
var Productos = []string{"calendar", "contacts", "docs", "sheets", "slides", "notes", "photos", "bases"}

// extensiones es el formato de cada biblioteca, el mismo que su app.
var extensiones = map[string]string{"docs": "docx", "sheets": "xlsx", "slides": "pptx"}

// sinRed es la raíz que se le da a la fachada de Bases: nunca se llama, porque
// `WithPrivateData` cambia el transporte por uno que falla cerrado sin salir del proceso.
const sinRed = "https://sin-red.invalid"

// Origen es la instalación de la que se exporta: su carpeta de datos
// (`espacio.Rutas.Datos`), quién es la persona en sus almacenes (`espacio.ID`), cómo
// firma los documentos (`espacio.Persona()`) y el idioma de la app.
type Origen struct {
	Datos   string
	Sujeto  string
	Persona biblioteca.Persona
	Idioma  string
}

// Parte es cómo ha ido una app.
type Parte struct {
	Producto string
	// Carpeta es donde ha quedado lo suyo; vacía si no tenía nada.
	Carpeta string
	// Ficheros son los que ha dejado; Fallidos, los que no ha podido (una foto
	// enlazada cuyo fichero ya no está).
	Ficheros, Fallidos int
	Err                error
}

// Todo exporta lo de cada app de [Productos] que tenga datos en
// `destino/<carpeta(producto)>/` (sin carpeta, la clave del producto). Si una falla
// sigue con las demás; el error junta los de todas.
func Todo(ctx context.Context, o Origen, destino string, carpeta func(producto string) string) ([]Parte, error) {
	rutas := espacio.Rutas{Datos: o.Datos}
	var partes []Parte
	var errs []error
	for _, p := range Productos {
		if err := ctx.Err(); err != nil {
			return partes, errors.Join(append(errs, err)...)
		}
		nombre := p
		if carpeta != nil {
			nombre = NombreCarpeta(carpeta(p), p)
		}
		parte := Parte{Producto: p, Carpeta: filepath.Join(destino, nombre)}
		hecho, err := exportarProducto(ctx, o, rutas, p, parte.Carpeta, &parte.Fallidos)
		if hecho {
			parte.Ficheros = contar(parte.Carpeta)
		}
		if err != nil {
			parte.Err = err
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
		}
		if parte.Ficheros == 0 {
			_ = os.RemoveAll(parte.Carpeta) // nada que enseñar: ni la carpeta vacía
			parte.Carpeta = ""
		}
		if hecho || err != nil {
			partes = append(partes, parte)
		}
	}
	return partes, errors.Join(errs...)
}

// exportarProducto abre lo de p y lo exporta en dir. hecho es falso si p no tiene datos.
func exportarProducto(ctx context.Context, o Origen, rutas espacio.Rutas, p, dir string, fallidos *int) (hecho bool, err error) {
	if ext, ok := extensiones[p]; ok {
		return exportarBiblioteca(ctx, o, rutas, p, ext, dir)
	}
	if p == "photos" {
		return exportarFototeca(ctx, o, rutas, dir, fallidos)
	}
	almacenDir := rutas.Producto(p)
	if p == "contacts" {
		// La libreta SQLite de Contacts (`local/libreta`), en solo lectura; mientras
		// Contacts no la haya estrenado, su almacén de registros de antes (abajo).
		lib, err := libreta.AbrirLectura(ctx, almacenDir, o.Sujeto)
		if err == nil {
			defer func() { _ = lib.Cerrar() }()
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return true, err
			}
			return true, Contactos(ctx, lib, dir)
		}
		if !errors.Is(err, libreta.ErrSinMigrar) {
			return true, err
		}
	}
	if !almacen.EsAlmacen(almacenDir) {
		return false, nil
	}
	rec, err := almacen.Abrir(ctx, almacenDir, p, o.Sujeto)
	if err != nil {
		return true, err
	}
	defer rec.Cerrar() //nolint:contextcheck // cerrar (y compactar) lleva su propio plazo: se cierra también si ctx ya se canceló
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return true, err
	}
	switch p {
	case "notes":
		// NewLocal: el almacén de Notepads de Sytue Office puede tener notas de imagen con
		// la imagen dentro y enlaces a ficheros (N-06, N-07), que New no lee.
		return true, conRepositorio(ctx, rec.Config, privateNotes.NewLocal, func(repo *privateNotes.Repository) error {
			return Cuadernos(ctx, repo, dir)
		})
	case "calendar":
		return true, conRepositorio(ctx, rec.Config, privateCalendar.New, func(repo *privateCalendar.Repository) error {
			return Calendarios(ctx, repo, dir)
		})
	case "contacts":
		return true, conRepositorio(ctx, rec.Config, privateContacts.New, func(repo *privateContacts.Repository) error {
			return Contactos(ctx, repo, dir)
		})
	case "bases":
		return true, conRepositorio(ctx, rec.Config, privateBases.New, func(repo *privateBases.Repository) error {
			fachada, err := api.NewBases(sinRed, nil, nil).WithPrivateData(repo)
			if err != nil {
				return err
			}
			return Bases(ctx, fachada, dir)
		})
	}
	return false, fmt.Errorf("exportar: %q no es una app que exporte", p)
}

// exportarFototeca exporta los originales de Photos en dir, tal cual, en AAAA/MM: lo
// mismo que Photos en su Ajustes → Copia (y en «Exportar la fototeca»). La fototeca vive
// en SQLite (`local/fototeca`); si aún está en el almacén de antes, abrirla la migra.
// hecho es falso si Photos no tiene datos.
func exportarFototeca(ctx context.Context, o Origen, rutas espacio.Rutas, dir string, fallidos *int) (hecho bool, err error) {
	carpeta := rutas.Producto("photos")
	if _, err := os.Stat(filepath.Join(carpeta, fototeca.Fichero)); err != nil && !almacen.EsAlmacen(carpeta) {
		return false, nil //nolint:nilerr // sin fototeca ni almacén no hay fotos que exportar: no es un fallo
	}
	f, err := fototeca.Abrir(ctx, carpeta, o.Sujeto, fototeca.Opciones{})
	if err != nil {
		return true, err
	}
	defer f.Close() //nolint:contextcheck // cerrar no depende de ctx: se cierra también si ya se canceló
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return true, err
	}
	r, err := f.ExportarFototeca(ctx, dir, func(int, int) {})
	*fallidos = len(r.Fallidas)
	return true, err
}

// exportarBiblioteca exporta los documentos de p (docs, sheets, slides) en dir, con la
// extensión ext. hecho es falso si p no tiene carpeta.
func exportarBiblioteca(ctx context.Context, o Origen, rutas espacio.Rutas, p, ext, dir string) (hecho bool, err error) {
	if fi, err := os.Stat(filepath.Join(rutas.Biblioteca(), p)); err != nil || !fi.IsDir() {
		return false, nil //nolint:nilerr // sin carpeta no hay documentos que exportar: no es un fallo
	}
	b, err := biblioteca.Abrir(rutas.Biblioteca(), p, o.Persona)
	if err != nil {
		return true, err
	}
	b.Idioma = o.Idioma
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return true, err
	}
	return true, Biblioteca(b, ext)(ctx, dir)
}

// repositorio es lo que hace falta de un repositorio del motor para exportarlo:
// prepararlo antes de leer y cerrarlo al terminar.
type repositorio interface {
	almacen.Inicializable
	Close()
}

// conRepositorio abre el repositorio del almacén con abrir, lo prepara, exporta con él y
// lo cierra.
func conRepositorio[R repositorio](ctx context.Context, cfg records.Config, abrir func(records.Config) (R, error), exportar func(R) error) error {
	repo, err := abrir(cfg)
	if err != nil {
		return err
	}
	defer repo.Close() //nolint:contextcheck // cerrar no depende de ctx: se cierra también si ya se canceló
	if err := almacen.Preparar(ctx, repo); err != nil {
		return err
	}
	return exportar(repo)
}

// contar son los ficheros que hay bajo dir.
func contar(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n
}
