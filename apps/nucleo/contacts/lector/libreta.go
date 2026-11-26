// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// RefrescoLibreta es cada cuánto, como mucho, Libreta mira si Contacts cambió su diario.
const RefrescoLibreta = 30 * time.Second

// ficheroDiario es el diario del almacén de Contacts: si no cambia, la libreta tampoco.
// Partido para que el guardián de catálogos no lo tome por una clave.
const ficheroDiario = "registros" + ".jsonl"

// Libreta es la libreta de Contacts de este equipo para otra app: los invitados de un
// evento en Calendar (Buscar) y el autocompletado de Para/CC/CCO y «Elegir de Contactos»
// del redactor de Mail (Search y All, M-13), con la búsqueda y las páginas de Contacts.
//
// Con la libreta SQLite de Contacts (`local/libreta`), cada consulta va a la base, en
// solo lectura y por sus índices: autocompletar no espera a nada ni hay nada que releer
// cuando Contacts cambia (la base en modo WAL da siempre lo último confirmado), y no se
// guarda copia en memoria.
//
// Mientras Contacts siga en el almacén de registros de antes (no se ha abierto desde que
// hay base), leer el almacén entero a cada letra sería caro: la libreta se lee una vez y
// se guarda en memoria, y se vuelve a leer cuando el diario de Contacts cambia, mirándolo
// como mucho cada RefrescoLibreta, con lo que mira la búsqueda de cada contacto plegado
// una vez por lectura (`contacts.TextosBuscables`), no a cada letra. Releerla NO se
// espera: con 20.000 contactos son segundos. Cuando el diario cambia, se contesta con la
// lista que ya hay y se relee detrás, una sola vez aunque lleguen muchas letras; la nueva
// sustituye a la vieja al terminar. Solo la primera lectura, sin nada que enseñar, se
// espera (Precargar la adelanta).
type Libreta struct {
	l *Lector

	mu      sync.Mutex
	lista   []api.Contact
	textos  []string // lo que mira la búsqueda de cada uno, plegado una vez por lectura
	leida   bool
	visto   time.Time     // la última vez que se miró si el diario cambió
	version string        // tamaño y fecha del diario leído
	leyendo chan struct{} // la lectura en curso, cerrado al acabar; nil si no hay
	fallo   error         // el de la última lectura, para quien la esperaba
}

// leerTodo es cómo se lee la libreta entera; las pruebas la frenan.
var leerTodo = (*Lector).Contactos

// Libreta prepara la libreta en memoria. No lee nada todavía.
func (l *Lector) Libreta() *Libreta { return &Libreta{l: l} }

// Precargar empieza a leer la libreta detrás, si aún no está leída: así la primera letra
// que se escribe en un campo de direcciones no espera a leerla.
func (b *Libreta) Precargar() {
	if _, hay, _ := b.l.libretaSQLite(context.Background()); hay {
		return // con la base no hay nada que precargar
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.leida {
		_ = b.mirar()
	}
}

// Contactos son los contactos vivos. Si el diario de Contacts cambió, los que ya había
// mientras se releen detrás; la primera vez, los que se leen.
func (b *Libreta) Contactos(ctx context.Context) ([]api.Contact, error) {
	if base, hay, err := b.l.libretaSQLite(ctx); err != nil || hay {
		if err != nil {
			return nil, err
		}
		return base.Contactos(ctx)
	}
	lista, _, err := b.foto(ctx)
	return lista, err
}

// foto son los contactos de Contactos con sus textos de búsqueda.
func (b *Libreta) foto(ctx context.Context) ([]api.Contact, []string, error) {
	b.mu.Lock()
	if b.leida {
		lista, textos := b.lista, b.textos
		if time.Since(b.visto) >= RefrescoLibreta {
			_ = b.mirar() //nolint:contextcheck // la relectura va detrás con su propio contexto (ver releer)
		}
		b.mu.Unlock()
		return lista, textos, nil
	}
	if err := b.mirar(); err != nil { //nolint:contextcheck // la relectura va detrás con su propio contexto (ver releer); a ella se la espera con ctx
		b.mu.Unlock()
		return nil, nil, err
	}
	leyendo := b.leyendo
	b.mu.Unlock()
	if leyendo != nil {
		select {
		case <-leyendo:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.leida {
		return nil, nil, b.fallo
	}
	return b.lista, b.textos, nil
}

// mirar mira el diario de Contacts y, si cambió desde lo leído, empieza a releer detrás
// (si no se está releyendo ya). Con b.mu.
func (b *Libreta) mirar() error {
	b.visto = time.Now()
	fi, err := os.Stat(filepath.Join(b.l.dir, ficheroDiario))
	if errors.Is(err, fs.ErrNotExist) {
		// Contacts aún no tiene nada en este equipo: no es un error.
		b.lista, b.textos, b.leida, b.version = nil, nil, true, ""
		return nil
	}
	if err != nil {
		return err
	}
	version := fi.ModTime().String() + "/" + strconv.FormatInt(fi.Size(), 10)
	if (b.leida && version == b.version) || b.leyendo != nil {
		return nil
	}
	b.leyendo = make(chan struct{})
	go b.releer(version, b.leyendo)
	return nil
}

// releer lee la libreta entera fuera de quien pregunta, con su propio contexto (que se
// corte una búsqueda no corta la lectura que esperan las demás), y la pone. Si falla,
// se queda la que había y se vuelve a intentar en la próxima mirada.
func (b *Libreta) releer(version string, hecho chan struct{}) {
	lista, err := leerTodo(b.l, context.Background())
	var textos []string
	if err == nil {
		textos = contacts.TextosBuscables(lista)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fallo = err
	if err == nil {
		b.lista, b.textos, b.leida, b.version = lista, textos, true, version
	}
	b.leyendo = nil
	close(hecho)
}

// Direccion es una dirección de correo de la libreta con el nombre de su contacto.
type Direccion struct{ Nombre, Correo string }

// Buscar son las direcciones de los contactos que casan con q (la búsqueda de Contacts),
// la principal de cada uno primero, sin repetir (sin mayúsculas) y como mucho limite:
// lo que autocompleta un campo de direcciones.
func (b *Libreta) Buscar(ctx context.Context, q string, limite int) ([]Direccion, error) {
	if strings.TrimSpace(q) == "" {
		return nil, nil
	}
	cs, err := b.Search(ctx, q, max(limite, 1)*2)
	if err != nil {
		return nil, err
	}
	var out []Direccion
	visto := map[string]bool{}
	for _, c := range cs {
		correos := slices.Clone(c.Emails)
		slices.SortStableFunc(correos, func(a, b api.ContactEmail) int {
			switch {
			case a.IsPrimary == b.IsPrimary:
				return 0
			case a.IsPrimary:
				return -1
			}
			return 1
		})
		for _, e := range correos {
			correo := strings.TrimSpace(e.Email)
			if correo == "" || visto[strings.ToLower(correo)] {
				continue
			}
			visto[strings.ToLower(correo)] = true
			out = append(out, Direccion{Nombre: strings.TrimSpace(c.FormattedName), Correo: correo})
			if len(out) >= limite {
				return out, nil
			}
		}
	}
	return out, nil
}

// Search es lo mismo que `Search` del repositorio de Contacts sobre la libreta en memoria:
// el autocompletado del redactor de Mail (`mail.Buscador`). Sin Contacts en el equipo, nada.
func (b *Libreta) Search(ctx context.Context, q string, limite int) ([]api.Contact, error) {
	if base, hay, err := b.l.libretaSQLite(ctx); err != nil || hay {
		if err != nil {
			return nil, err
		}
		cs, err := base.Search(ctx, q, limite)
		if (err != nil || len(cs) == 0) && vacia(ctx, base) {
			return nil, nil // sin contactos no hay nada que buscar, ni error
		}
		return cs, err
	}
	lista, textos, err := b.foto(ctx)
	if err != nil || len(lista) == 0 {
		return nil, err
	}
	return contacts.BuscarEnTextos(lista, textos, q, limite)
}

// All es lo mismo que `All` del repositorio de Contacts sobre la libreta en memoria (una
// página y el desplazamiento de la siguiente, o 0): «Elegir de Contactos» del redactor de
// Mail (`mail.Libreta`). Sin Contacts en el equipo, nada.
func (b *Libreta) All(ctx context.Context, limite, offset int) ([]api.Contact, int, error) {
	if base, hay, err := b.l.libretaSQLite(ctx); err != nil || hay {
		if err != nil {
			return nil, 0, err
		}
		cs, siguiente, err := base.All(ctx, limite, offset)
		if err != nil || len(cs) == 0 {
			return nil, 0, err
		}
		return cs, siguiente, nil
	}
	lista, err := b.Contactos(ctx)
	if err != nil || len(lista) == 0 {
		return nil, 0, err
	}
	pagina, siguiente := contacts.PaginarEn(lista, limite, offset)
	return pagina, siguiente, nil
}

// vacia dice si la libreta SQLite no tiene ningún contacto que se vea.
func vacia(ctx context.Context, base *libreta.Libreta) bool {
	cs, _, err := base.All(ctx, 1, 0)
	return err == nil && len(cs) == 0
}
