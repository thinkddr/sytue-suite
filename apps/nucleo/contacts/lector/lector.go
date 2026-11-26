// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package lector lee, en SOLO LECTURA, la libreta de Contacts de Sytue Office desde otra
// app (PLAN-EDICION-LOCAL.md §6.6 K-04): Calendar saca los cumpleaños y Mail puede
// autocompletar y «Elegir de Contactos».
//
// Contacts puede estar abierto a la vez y es el único que escribe. Su libreta es la base
// SQLite de `local/libreta` (`Datos/contacts/libreta.sqlite`, en modo WAL): aquí se abre
// en solo lectura, una vez, y cada consulta va a la base por sus índices y ve lo último
// que Contacts confirmó, sin copiar nada en memoria ni releer la libreta cuando cambia.
//
// Mientras Contacts no se abra con esa base por primera vez (y pase a ella su almacén de
// registros), la libreta sigue en el almacén de antes y se lee como siempre: cada lectura
// abre una vista nueva del almacén y la suelta al acabar, con el puerto envuelto para que
// cualquier escritura falle, la fuente de migración sin congelar y un diario transitorio
// del motor temporal y propio, nunca el de Contacts.
package lector

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// ErrSoloLectura es lo que contesta cualquier intento de escribir desde el lector.
var ErrSoloLectura = errors.New("contactos: el lector entre apps es de solo lectura")

// Lector sabe dónde está la libreta de Contacts y de quién es.
type Lector struct {
	dir, sujeto string

	mu sync.Mutex
	// base es la libreta SQLite en solo lectura, abierta la primera vez que la hay; visto
	// es su fichero entonces (si lo sustituyen, al restaurar una copia, se vuelve a abrir).
	base  *libreta.Libreta
	visto os.FileInfo
}

// Nuevo prepara el lector de la libreta de Contacts en dir (la carpeta del producto
// «contacts»), con el sujeto de la instalación. No abre nada todavía.
func Nuevo(dir, sujeto string) *Lector { return &Lector{dir: dir, sujeto: sujeto} }

// libretaSQLite es la libreta SQLite de Contacts en solo lectura; hay es falso si aún no
// la hay (Contacts sigue en el almacén de registros, o nunca se ha abierto aquí).
func (l *Lector) libretaSQLite(ctx context.Context) (base *libreta.Libreta, hay bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fi, err := os.Stat(filepath.Join(l.dir, libreta.Fichero))
	if err != nil {
		l.soltar()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if l.base != nil && os.SameFile(fi, l.visto) {
		return l.base, true, nil
	}
	l.soltar()
	b, err := libreta.AbrirLectura(ctx, l.dir, l.sujeto)
	if errors.Is(err, libreta.ErrSinMigrar) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	l.base, l.visto = b, fi
	return b, true, nil
}

// soltar cierra la libreta SQLite abierta, si la hay. Con l.mu.
func (l *Lector) soltar() {
	if l.base != nil {
		_ = l.base.Cerrar()
		l.base, l.visto = nil, nil
	}
}

// Cerrar suelta la libreta SQLite, si se abrió.
func (l *Lector) Cerrar() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.soltar()
}

// Contactos son todos los contactos vivos, en el orden de `All`.
//
// Salen de UNA instantánea del almacén y de una sola proyección: por páginas de `All`,
// cada página rehacía las dos enteras, y con 20.000 contactos (40 páginas) releer la
// libreta pasaba de tres minutos (medido: libreta_bench_test.go).
func (l *Lector) Contactos(ctx context.Context) ([]api.Contact, error) {
	if base, hay, err := l.libretaSQLite(ctx); err != nil || hay {
		if err != nil {
			return nil, err
		}
		return base.Contactos(ctx)
	}
	var out []api.Contact
	err := l.leer(ctx, func(ctx context.Context, r *contacts.Repository) error {
		foto, err := r.Snapshot(ctx)
		if err != nil {
			return err
		}
		out, err = foto.ContactosVisibles()
		return err
	})
	return out, err
}

// Buscar es la búsqueda de Contacts (nombre, organización, correo…), hasta limite.
func (l *Lector) Buscar(ctx context.Context, texto string, limite int) ([]api.Contact, error) {
	if base, hay, err := l.libretaSQLite(ctx); err != nil || hay {
		if err != nil {
			return nil, err
		}
		return base.Search(ctx, texto, limite)
	}
	var out []api.Contact
	err := l.leer(ctx, func(ctx context.Context, r *contacts.Repository) error {
		var err error
		out, err = r.Search(ctx, texto, limite)
		return err
	})
	return out, err
}

// Cumple es el cumpleaños de un contacto: el mes y el día, y el año si se sabe.
type Cumple struct {
	ContactoID, Nombre string
	Mes                time.Month
	Dia, Ano           int // Ano 0: sin año
}

// Cumpleanos son los cumpleaños de los contactos que lo tienen, por fecha del año.
func (l *Lector) Cumpleanos(ctx context.Context) ([]Cumple, error) {
	var todos []api.Contact
	base, hay, err := l.libretaSQLite(ctx)
	switch {
	case err != nil:
		return nil, err
	case hay:
		// La base sabe quién tiene cumpleaños: no se leen todos.
		todos, err = base.ConCumpleanos(ctx)
	default:
		todos, err = l.Contactos(ctx)
	}
	if err != nil {
		return nil, err
	}
	var out []Cumple
	for _, c := range todos {
		nombre := strings.TrimSpace(c.FormattedName)
		if nombre == "" {
			nombre = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
		}
		switch {
		case c.Birthday != nil:
			b := c.Birthday.UTC()
			out = append(out, Cumple{ContactoID: c.ID, Nombre: nombre, Mes: b.Month(), Dia: b.Day(), Ano: b.Year()})
		case len(c.BirthdayNoYear) == len("--MM-DD"):
			t, err := time.Parse("--01-02", c.BirthdayNoYear)
			if err == nil {
				out = append(out, Cumple{ContactoID: c.ID, Nombre: nombre, Mes: t.Month(), Dia: t.Day()})
			}
		}
	}
	slices.SortFunc(out, func(a, b Cumple) int {
		if a.Mes != b.Mes {
			return int(a.Mes) - int(b.Mes)
		}
		if a.Dia != b.Dia {
			return a.Dia - b.Dia
		}
		return strings.Compare(a.Nombre, b.Nombre)
	})
	return out, nil
}

// leer abre una vista de solo lectura del almacén de registros, la usa y la suelta. Sin
// almacén (Contacts nunca se ha abierto en este equipo) no hay contactos: no es un error.
func (l *Lector) leer(ctx context.Context, f func(context.Context, *contacts.Repository) error) error {
	if _, err := os.Stat(filepath.Join(l.dir, "registros.jsonl")); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	scope := storage.Scope{Tenant: "local", Subject: l.sujeto, Product: "contacts"}
	puerto, err := almacen.AbrirPuerto(l.dir, scope) //nolint:contextcheck // abrir el almacén espera su cerrojo con el plazo propio de almacen (EsperaCerrojo)
	if err != nil {
		return err
	}
	defer func() { _ = puerto.Cerrar() }()
	temporal, err := os.MkdirTemp("", "sytue-office-lector-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporal) }()
	diario, err := offline.OpenWithSecretsContext(ctx, filepath.Join(temporal, "diario"), almacen.ClaveEnFichero{Dir: filepath.Join(temporal, "clave")})
	if err != nil {
		return err
	}
	repo, err := contacts.New(records.Config{
		Context: ctx,
		Scope:   scope,
		Crypto:  soloLectura{puerto},
		Gateway: api.NewDrive("https://sin-red.invalid", nil, nil),
		Journal: diario,
		Source:  origenSoloLectura{almacen.NuevoSinOrigen(filepath.Join(l.dir, "origen.json"))},
	})
	if err != nil {
		return err
	}
	defer repo.Close() //nolint:contextcheck // cerrar no se cancela: suelta lo abierto aunque ctx ya no valga
	if err := repo.Initialize(ctx); err != nil {
		if errors.Is(err, records.ErrNotReady) {
			return nil // Contacts aún no ha terminado su primera apertura
		}
		return err
	}
	return f(ctx, repo)
}

// soloLectura es el puerto de Contacts con las escrituras cerradas.
type soloLectura struct{ *almacen.Puerto }

func (soloLectura) PutStorageRecordAt(context.Context, storage.PublicationTarget, string, storage.Record) (storage.Reference, error) {
	return storage.Reference{}, ErrSoloLectura
}

func (soloLectura) PutStorageObjectAt(context.Context, storage.PublicationTarget, string, string, string, storage.ObjectCommit) (storage.ObjectVersion, error) {
	return storage.ObjectVersion{}, ErrSoloLectura
}

func (soloLectura) UploadStorageFileAt(context.Context, *api.Drive, *offline.Store, string, storage.PublicationTarget, storage.FileRequest, io.ReaderAt) (storage.Reference, error) {
	return storage.Reference{}, ErrSoloLectura
}

func (soloLectura) AcknowledgeStorageFile(context.Context, *offline.Store, string, storage.Reference) error {
	return ErrSoloLectura
}

// origenSoloLectura nunca congela la fuente: eso es cosa de Contacts.
type origenSoloLectura struct{ *almacen.SinOrigen }

func (origenSoloLectura) Begin(context.Context, storage.Scope, string) (records.SnapshotState, error) {
	return records.SnapshotState{}, ErrSoloLectura
}
