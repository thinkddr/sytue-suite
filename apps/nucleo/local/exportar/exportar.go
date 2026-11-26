// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package exportar es «Exportar en formatos abiertos» de Sytue Office
// (apps/fyne/PLAN-EDICION-LOCAL.md §5.13): lo de cada app en formatos que abren otros
// programas, para llevárselo sin depender de la suite.
//
//	Docus, Calcus, Diapos  cada documento en OOXML (.docx, .xlsx, .pptx)  [Biblioteca]
//	Notepads               cada cuaderno en Markdown                       [Cuadernos]
//	Calendar               cada calendario propio en su .ics               [Calendarios]
//	Contacts               todas las tarjetas en vCard y en CSV            [Contactos]
//	Bases                  cada tabla en CSV, en la carpeta de su base     [Bases]
//	Photos                 los originales tal cual, en AAAA/MM             (fototeca.Fototeca.ExportarFototeca)
//
// Cada app lo ofrece en Ajustes → Copia de seguridad con su repositorio ya abierto
// (`local.App.AlExportarTodo`), y el Hub lo hace con TODAS a la vez ([Todo]), abriendo
// cada almacén con el mismo repositorio que su app mientras están cerradas: son las
// MISMAS funciones, no una copia.
//
// Nada de aquí abre la red ni escribe en los almacenes: solo lee y deja ficheros en la
// carpeta de destino, sin salirse de ella.
package exportar

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// ----- Nombres -----

// NombreCarpeta es un nombre de carpeta para la app: su nombre sin lo que el sistema
// de ficheros no admite, o el producto.
func NombreCarpeta(nombre, producto string) string {
	n := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '_'
		}
		return r
	}, strings.TrimSpace(nombre))
	if n == "" {
		return producto
	}
	return n
}

// NombreFichero es NombreCarpeta para un fichero: sin separadores ni caracteres que
// Windows no admite, con la extensión, y sin pisar otro de la misma carpeta (vistos).
func NombreFichero(titulo, respaldo, ext string, vistos map[string]int) string {
	base := NombreCarpeta(titulo, respaldo)
	if len([]rune(base)) > 120 {
		base = string([]rune(base)[:120])
	}
	clave := strings.ToLower(base + ext)
	vistos[clave]++
	if n := vistos[clave]; n > 1 {
		return fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
	return base + ext
}

// Escribir deja datos en carpeta/nombre, sin salirse de carpeta.
func Escribir(carpeta, nombre string, datos []byte) error {
	return os.WriteFile(filepath.Join(carpeta, filepath.Base(nombre)), datos, 0o600)
}

// Copiar vuelca r en carpeta/nombre y lo cierra.
func Copiar(carpeta, nombre string, r io.ReadCloser) error {
	defer func() { _ = r.Close() }()
	f, err := os.OpenFile(filepath.Join(carpeta, filepath.Base(nombre)), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ----- Por app -----

// Biblioteca es lo de Docus, Calcus y Diapos: cada documento de la biblioteca con la
// extensión ext (docx, xlsx, pptx en OOXML; odt, ods, odp en ODF), con su título por
// nombre. La papelera no se exporta.
func Biblioteca(b *biblioteca.Biblioteca, ext string) func(context.Context, string) error {
	formato := "ooxml"
	switch ext {
	case "odt", "ods", "odp":
		formato = "odf"
	}
	return func(ctx context.Context, carpeta string) error {
		vistos := map[string]int{}
		cursor := ""
		for {
			docs, siguiente, err := b.ListDocuments(ctx, api.DocsFilter{Cursor: cursor, Limit: 200, ByTitle: true})
			if err != nil {
				return err
			}
			for _, d := range docs {
				if err := ctx.Err(); err != nil {
					return err
				}
				datos, err := b.ExportDocument(ctx, d.ID, formato, false, nil)
				if err != nil {
					return err
				}
				if err := Escribir(carpeta, NombreFichero(d.Title, d.ID, "."+ext, vistos), datos); err != nil {
					return err
				}
			}
			if siguiente == "" {
				return nil
			}
			cursor = siguiente
		}
	}
}

// Cuadernos es lo de Notepads: cada cuaderno en Markdown (`model.FormatoMarkdown`). Los
// cifrados (de una copia de la conectada) no se pueden leer y se saltan.
func Cuadernos(ctx context.Context, repo *privateNotes.Repository, carpeta string) error {
	cuadernos, err := repo.ListNotebooks(ctx)
	if err != nil {
		return err
	}
	vistos := map[string]int{}
	for _, c := range cuadernos {
		r, nombre, err := repo.ExportNotebook(ctx, c.ID, "", "md")
		if errors.Is(err, model.ErrEncryptedMigrationRequired) {
			continue
		}
		if err != nil {
			return err
		}
		ext := path.Ext(nombre)
		if err := Copiar(carpeta, NombreFichero(strings.TrimSuffix(nombre, ext), c.ID, ext, vistos), r); err != nil {
			return err
		}
	}
	return nil
}

// Calendarios es lo de Calendar: cada calendario propio en su `.ics`. El de los
// cumpleaños (de solo lectura) sale de Contacts y no se exporta.
func Calendarios(ctx context.Context, repo *privateCalendar.Repository, carpeta string) error {
	cals, err := repo.List(ctx)
	if err != nil {
		return err
	}
	vistos := map[string]int{}
	for _, cal := range cals {
		if cal.ReadOnly {
			continue
		}
		datos, err := repo.ExportICS(ctx, cal.ID)
		if err != nil {
			return err
		}
		nombre := cmp.Or(strings.TrimSpace(cal.DisplayName), "calendario")
		if err := Escribir(carpeta, NombreFichero(nombre, cal.ID, ".ics", vistos), datos); err != nil {
			return err
		}
	}
	return nil
}

// LibretaExportable es lo que hace falta de Contacts para exportarlo: lo cumplen la
// libreta SQLite de Sytue Office (`local/libreta`) y el repositorio de Contacts.
type LibretaExportable interface {
	All(ctx context.Context, limit, offset int) ([]api.Contact, int, error)
	BatchVCard(ctx context.Context, ids []string) ([]byte, error)
	ExportCSV(ctx context.Context) (string, []byte, error)
}

var _ LibretaExportable = (*privateContacts.Repository)(nil)

// Contactos es lo de Contacts: `Contactos.vcf` (todas las tarjetas, en lotes) y el CSV.
func Contactos(ctx context.Context, repo LibretaExportable, carpeta string) error {
	var ids []string
	// All da cada página y el desplazamiento de la siguiente (0 al acabar).
	for desde := 0; ; {
		cs, siguiente, err := repo.All(ctx, 500, desde)
		if err != nil {
			return err
		}
		for _, c := range cs {
			ids = append(ids, c.ID)
		}
		if len(cs) == 0 || siguiente <= desde {
			break
		}
		desde = siguiente
	}
	var vcf []byte
	for i := 0; i < len(ids); i += api.MaxLoteContactos {
		lote, err := repo.BatchVCard(ctx, ids[i:min(i+api.MaxLoteContactos, len(ids))])
		if err != nil {
			return err
		}
		vcf = append(vcf, lote...)
	}
	if err := Escribir(carpeta, "Contactos.vcf", vcf); err != nil {
		return err
	}
	nombre, csv, err := repo.ExportCSV(ctx)
	if err != nil {
		return err
	}
	return Escribir(carpeta, nombre, csv)
}

// Bases es lo de Bases: cada tabla en `<base>/<tabla>.csv`, por su fachada (la que usa
// la pantalla, enchufada al repositorio local con `WithPrivateData`).
func Bases(ctx context.Context, b *api.Bases, carpeta string) error {
	bs, err := b.ListBases(ctx)
	if err != nil {
		return err
	}
	carpetas := map[string]int{}
	for _, base := range bs {
		dir := filepath.Join(carpeta, NombreFichero(base.Name, base.ID, "", carpetas))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		ts, err := b.ListTables(ctx, base.ID)
		if err != nil {
			return err
		}
		vistos := map[string]int{}
		for _, t := range ts {
			r, err := b.Export(ctx, t.ID, "csv")
			if err != nil {
				return err
			}
			if err := Copiar(dir, NombreFichero(t.Name, t.ID, ".csv", vistos), r); err != nil {
				return err
			}
		}
	}
	return nil
}
