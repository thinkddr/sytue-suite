// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

// # El contrato de Bases (PLAN-EDICION-LOCAL §8.2)
//
// El MISMO juego de pruebas de comportamiento del repositorio de Bases —lo que la
// pantalla usa por `api.Bases`— sobre los dos puertos del motor de registros: el de las
// pruebas en memoria (`storagetest.Memoria`, el publicador de la conectada sin red) y el
// almacén en claro de Sytue Office en disco (`local/almacen`, como lo abre
// `cmd-local/bases`: con la actividad local junto al almacén y SoltarAdjuntos). Cada
// prueba corre en los dos (porCadaPuerto); TestContratoDeBasesVenLoMismo hace lo mismo
// en los dos y compara lo que enseña la pantalla.
//
// Diferencias aceptadas entre los dos puertos (puertoDeBases):
//
//   - liberar: solo el almacén en disco quita del disco el fichero de un adjunto
//     soltado (al compactarse); en una sala de la conectada un fichero publicado no se
//     borra nunca. Lo que se ve por la API es lo mismo: el adjunto ya no está.
//   - lineaCortada: solo el disco puede dejar a medias la última línea de su diario.
//
// VISTO EN ROJO (2026-11-25): esperando el valor de antes de la fórmula tras cambiarla,
// TestContratoDeBasesRegistros falló en los dos puertos con el mismo mensaje; abriendo
// sin SoltarAdjuntos, TestContratoDeBasesAdjuntos falló donde se libera el disco; y,
// sin los dos arreglos del almacén (ver el contrato de Calendar),
// TestContratoDeBasesDosProcesos y TestContratoDeBasesCorteAMitad fallaron solo en
// disco.
//
// VISTO EN ROJO (2026-11-26), en los dos puertos y con el código que comparte la
// conectada: repetir tras un corte un alta (CreateTable, CreateRecord, CreateComment…) o
// un UploadAttachment creaba otra fila, porque el id se elegía antes y entraba en la
// operación del motor; repetir la celda cortada (TestContratoDeBasesCorteAMitad)
// devolvía un registro vacío; renombrar otra vez lo que un corte dejó a medias retomaba
// lo cortado y perdía el nombre nuevo; y lo repetido no se anotaba en la actividad
// (TestContratoDeBasesRepetirTrasUnCorte).

const sujetoDeBases = "instalacion-1"

// almacenDeBases es donde viven las bases en uno de los dos puertos.
type almacenDeBases struct {
	// dir es la carpeta del almacén en disco; "" en memoria.
	dir string
	// actividad es el registro de actividad de este almacén (junto a él en disco).
	actividad string
	// abrir es un proceso sobre el almacén: el repositorio de Bases con su diario
	// transitorio, y la fachada de las pantallas delante. Abrir otra vez el mismo
	// proceso es ese proceso al volver a arrancar.
	abrir func(t *testing.T, proceso string, o aperturaDeBases) (*api.Bases, func())
}

// aperturaDeBases es lo que cambia al abrir un repositorio en una prueba.
type aperturaDeBases struct {
	// comoLaConectada: sin actividad local ni SoltarAdjuntos, como lo usa la conectada.
	comoLaConectada bool
	// corte, si no es nil, pierde la respuesta de la próxima publicación de un índice
	// cuando se active: la escritura queda hecha pero quien la pidió ve un fallo.
	corte *atomic.Bool
}

type puertoDeBases struct {
	nombre string
	nuevo  func(t *testing.T) *almacenDeBases
	// liberar, con el almacén cerrado, hace que se vaya del disco lo que ya nadie cita
	// y dice cuántos ficheros de adjuntos quedan antes y después; nil si el puerto no
	// borra nunca un fichero publicado (ver arriba).
	liberar func(t *testing.T, a *almacenDeBases) (antes, despues int)
	// lineaCortada deja a medias la última escritura del almacén; nil si el puerto no lo
	// puede simular.
	lineaCortada func(t *testing.T, a *almacenDeBases)
}

var errCorteDeBases = errors.New("prueba: la escritura se hizo pero la respuesta se perdió")

// discoDeBasesConCorte es el almacén en disco con el corte de aperturaDeBases.
type discoDeBasesConCorte struct {
	*almacen.Puerto
	corte *atomic.Bool
}

func (d *discoDeBasesConCorte) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	v, err := d.Puerto.PutStorageObjectAt(ctx, t, txn, objeto, revision, commit)
	if err == nil && d.corte.CompareAndSwap(true, false) {
		return storage.ObjectVersion{}, errCorteDeBases
	}
	return v, err
}

// memoriaDeBasesConCorte es la memoria con el corte de aperturaDeBases.
type memoriaDeBasesConCorte struct {
	*storagetest.Memoria
	corte *atomic.Bool
}

func (m *memoriaDeBasesConCorte) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	v, err := m.Memoria.PutStorageObjectAt(ctx, t, txn, objeto, revision, commit)
	if err == nil && m.corte.CompareAndSwap(true, false) {
		return storage.ObjectVersion{}, errCorteDeBases
	}
	return v, err
}

// fachadaDeBases es el repositorio sobre cfg, como lo abre cmd-local/bases (o como la
// conectada), preparado, con la fachada de las pantallas delante.
func fachadaDeBases(t *testing.T, cfg records.Config, actividad string, o aperturaDeBases) (*api.Bases, func()) {
	t.Helper()
	repo, err := bases.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !o.comoLaConectada {
		repo.Actividad = bases.NuevaActividad(actividad)
		repo.SoltarAdjuntos = true
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		repo.Close()
		t.Fatal("preparar:", err)
	}
	b, err := api.NewBases("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return b, repo.Close
}

// objetosDelAlmacen cuenta los ficheros de objetos/ del almacén en disco.
func objetosDelAlmacen(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(filepath.Join(dir, "objetos"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

var puertosDeBases = []puertoDeBases{
	{nombre: "en memoria", nuevo: func(t *testing.T) *almacenDeBases {
		scope := storage.Scope{Tenant: "local", Subject: sujetoDeBases, Product: "bases"}
		sala := storagetest.NuevaMemoria(scope)
		origen := storagetest.NuevoOrigenVacio(scope)
		type diario struct {
			dir      string
			secretos *authtest.Secrets
		}
		diarios := map[string]diario{}
		a := &almacenDeBases{actividad: filepath.Join(t.TempDir(), bases.FicheroActividad)}
		a.abrir = func(t *testing.T, proceso string, o aperturaDeBases) (*api.Bases, func()) {
			t.Helper()
			d, ok := diarios[proceso]
			if !ok {
				d = diario{dir: t.TempDir(), secretos: &authtest.Secrets{}}
				diarios[proceso] = d
			}
			journal, err := offline.OpenWithSecrets(d.dir, d.secretos)
			if err != nil {
				t.Fatal(err)
			}
			var crypto records.Crypto = sala
			if o.corte != nil {
				crypto = &memoriaDeBasesConCorte{Memoria: sala, corte: o.corte}
			}
			return fachadaDeBases(t, records.Config{
				Context: context.Background(), Scope: scope, Crypto: crypto,
				Gateway: api.NewDrive("https://sin-red.invalid", nil, nil), Journal: journal, Source: origen,
			}, a.actividad, o)
		}
		return a
	}},
	{nombre: "en disco", nuevo: func(t *testing.T) *almacenDeBases {
		dir := t.TempDir()
		a := &almacenDeBases{dir: dir, actividad: filepath.Join(dir, bases.FicheroActividad)}
		a.abrir = func(t *testing.T, _ string, o aperturaDeBases) (*api.Bases, func()) {
			t.Helper()
			// Cada proceso abre la carpeta entera, con su diario transitorio dentro.
			rec, err := almacen.Abrir(context.Background(), dir, "bases", sujetoDeBases)
			if err != nil {
				t.Fatal(err)
			}
			cfg := rec.Config
			if o.corte != nil {
				cfg.Crypto = &discoDeBasesConCorte{Puerto: rec.Puerto, corte: o.corte}
			}
			b, cerrar := fachadaDeBases(t, cfg, a.actividad, o)
			return b, func() { cerrar(); rec.Cerrar() }
		}
		return a
	}, liberar: func(t *testing.T, a *almacenDeBases) (int, int) {
		t.Helper()
		rec, err := almacen.Abrir(context.Background(), a.dir, "bases", sujetoDeBases)
		if err != nil {
			t.Fatal(err)
		}
		defer rec.Cerrar()
		antes := objetosDelAlmacen(t, a.dir)
		if _, err := rec.Compactar(t.Context()); err != nil {
			t.Fatal(err)
		}
		return antes, objetosDelAlmacen(t, a.dir)
	}, lineaCortada: func(t *testing.T, a *almacenDeBases) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(a.dir, "registros.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(`{"t":"registro","ref":{"room_id":"!local:bases","event_id":"$cortad`)
		if err = errors.Join(err, f.Close()); err != nil {
			t.Fatal(err)
		}
	}},
}

// porCadaPuerto corre prueba en cada puerto, con un almacén nuevo.
func porCadaPuerto(t *testing.T, prueba func(t *testing.T, p puertoDeBases, a *almacenDeBases)) {
	t.Helper()
	for _, p := range puertosDeBases {
		t.Run(p.nombre, func(t *testing.T) { prueba(t, p, p.nuevo(t)) })
	}
}

func nombresDeTablas(ts []api.BasesTable) []string {
	out := make([]string, len(ts))
	for i, x := range ts {
		out[i] = x.Name
	}
	return out
}

func idsDeCampos(fs []api.BasesField) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}

// Bases, tablas, campos y vistas: validar con las reglas del servicio, el orden, cambiar
// y la papelera; lo que pide un servidor falla cerrado.
func TestContratoDeBasesEsquema(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		b, cerrar := a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		if _, err := b.CreateBase(ctx, "   "); err == nil {
			t.Error("una base sin nombre entra")
		}
		base, err := b.CreateBase(ctx, "  Tienda  ")
		if err != nil || base.Name != "Tienda" || base.Role != api.BasesOwner || base.OwnerSub != sujetoDeBases {
			t.Fatalf("crear base: %+v %v", base, err)
		}
		otra, _ := b.CreateBase(ctx, "Otra")
		if lista, err := b.ListBases(ctx); err != nil || len(lista) != 2 {
			t.Fatalf("listar bases: %+v %v", lista, err)
		}
		if err := b.UpdateBase(ctx, base.ID, "Tienda online", "  Lo que vendemos  "); err != nil {
			t.Fatal(err)
		}
		if got, err := b.GetBase(ctx, base.ID); err != nil || got.Name != "Tienda online" || got.Description != "Lo que vendemos" {
			t.Fatalf("leer la base cambiada: %+v %v", got, err)
		}
		if err := b.DeleteBase(ctx, otra.ID); err != nil {
			t.Fatal(err)
		}
		if lista, _ := b.ListBases(ctx); len(lista) != 1 || lista[0].ID != base.ID {
			t.Fatalf("la base borrada sigue en la lista: %+v", lista)
		}

		pedidos, err := b.CreateTable(ctx, base.ID, "Pedidos")
		if err != nil || pedidos.Position != 0 {
			t.Fatalf("crear tabla: %+v %v", pedidos, err)
		}
		clientes, _ := b.CreateTable(ctx, base.ID, "Clientes")
		borrador, _ := b.CreateTable(ctx, base.ID, "Borrador")
		if clientes.Position != 1 || borrador.Position != 2 {
			t.Fatalf("posiciones de las tablas: %d %d", clientes.Position, borrador.Position)
		}
		if _, err := b.CreateTable(ctx, base.ID, ""); codigo(err) != "invalid" {
			t.Errorf("una tabla sin nombre: %v", err)
		}
		if _, err := b.CreateTable(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", "x"); codigo(err) != "not_found" {
			t.Errorf("una tabla en una base que no existe: %v", err)
		}
		if err := b.RenameTable(ctx, clientes.ID, "  Clientela "); err != nil {
			t.Fatal(err)
		}
		if err := b.DeleteTable(ctx, borrador.ID); err != nil {
			t.Fatal(err)
		}
		if ts, err := b.ListTables(ctx, base.ID); err != nil || !slices.Equal(nombresDeTablas(ts), []string{"Pedidos", "Clientela"}) {
			t.Fatalf("tablas: %q %v", nombresDeTablas(ts), err)
		}

		nombre, _ := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Cliente", Kind: "text"})
		importe, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Importe", Kind: "currency"})
		if err != nil || importe.Position != 1 || importe.Currency != "EUR" {
			t.Fatalf("un campo de moneda sin moneda va en euros: %+v %v", importe, err)
		}
		estado, _ := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Estado", Kind: "select", Choices: []string{"nuevo", "hecho"}})
		cliente, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Quién", Kind: "link", LinkTableID: clientes.ID})
		if err != nil {
			t.Fatal("un enlace a otra tabla de la base:", err)
		}
		doble, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Doble", Kind: "formula", Formula: "{" + importe.ID + "}*2"})
		if err != nil || doble.Result == "" {
			t.Fatalf("fórmula: %+v %v", doble, err)
		}
		for que, f := range map[string]api.BasesNewField{
			"un tipo que no existe":        {Name: "x", Kind: "hologram"},
			"una selección sin opciones":   {Name: "x", Kind: "select"},
			"un enlace a otra base":        {Name: "x", Kind: "link", LinkTableID: otra.ID},
			"una fórmula a un campo ajeno": {Name: "x", Kind: "formula", Formula: "{00000000-0000-4000-8000-000000000000}"},
			"una fórmula que no se lee":    {Name: "x", Kind: "formula", Formula: "((("},
		} {
			if _, err := b.CreateField(ctx, pedidos.ID, f); codigo(err) != "invalid" {
				t.Errorf("%s: %v", que, err)
			}
		}
		if err := b.RenameField(ctx, nombre.ID, "Nombre del cliente"); err != nil {
			t.Fatal(err)
		}
		if err := b.SetFieldChoices(ctx, nombre.ID, []string{"a"}); codigo(err) != "invalid" {
			t.Errorf("opciones en un campo de texto: %v", err)
		}
		if err := b.SetFieldChoices(ctx, estado.ID, []string{"hecho", "cancelado"}); err != nil {
			t.Fatal(err)
		}
		if err := b.ReorderFields(ctx, pedidos.ID, []string{importe.ID, nombre.ID}); codigo(err) != "rev_conflict" {
			t.Errorf("reordenar sin todos: %v", err)
		}
		orden := []string{doble.ID, cliente.ID, estado.ID, importe.ID, nombre.ID}
		if err := b.ReorderFields(ctx, pedidos.ID, orden); err != nil {
			t.Fatal(err)
		}
		ts, _ := b.ListTables(ctx, base.ID)
		if !slices.Equal(idsDeCampos(ts[0].Fields), orden) || ts[0].Fields[4].Name != "Nombre del cliente" || !slices.Equal(ts[0].Fields[2].Choices, []string{"hecho", "cancelado"}) {
			t.Fatalf("los campos, en su orden y cambiados: %+v", ts[0].Fields)
		}
		if err := b.DeleteField(ctx, cliente.ID); err != nil {
			t.Fatal(err)
		}
		if ts, _ := b.ListTables(ctx, base.ID); len(ts[0].Fields) != 4 || slices.Contains(idsDeCampos(ts[0].Fields), cliente.ID) {
			t.Fatalf("el campo borrado sigue: %+v", ts[0].Fields)
		}

		vista, err := b.CreateView(ctx, pedidos.ID, "Grandes", "grid", api.BasesViewConfig{
			Filters: []api.BasesFilter{{Field: importe.ID, Op: "gt", Value: 10}}, Sort: []api.BasesSort{{Field: importe.ID, Dir: "desc"}},
		})
		if err != nil || vista.CreatedBy != sujetoDeBases {
			t.Fatalf("crear vista: %+v %v", vista, err)
		}
		if _, err := b.CreateView(ctx, pedidos.ID, "Mal", "grid", api.BasesViewConfig{Filters: []api.BasesFilter{{Field: importe.ID, Op: "contains", Value: 1}}}); codigo(err) != "invalid" {
			t.Errorf("un operador que no es del tipo: %v", err)
		}
		if _, err := b.CreateView(ctx, pedidos.ID, "Mal", "hologram", api.BasesViewConfig{}); codigo(err) != "invalid" {
			t.Errorf("un tipo de vista que no existe: %v", err)
		}
		tablero, _ := b.CreateView(ctx, pedidos.ID, "Por estado", "kanban", api.BasesViewConfig{GroupBy: estado.ID})
		if v, err := b.UpdateView(ctx, vista.ID, "Pequeños", api.BasesViewConfig{Filters: []api.BasesFilter{{Field: importe.ID, Op: "lt", Value: 10}}}); err != nil || v.Name != "Pequeños" || v.Kind != "grid" {
			t.Fatalf("cambiar vista: %+v %v", v, err)
		}
		if err := b.DeleteView(ctx, tablero.ID); err != nil {
			t.Fatal(err)
		}
		if vs, err := b.ListViews(ctx, pedidos.ID); err != nil || len(vs) != 1 || vs[0].Name != "Pequeños" || vs[0].Config.Filters[0].Op != "lt" {
			t.Fatalf("vistas: %+v %v", vs, err)
		}

		if _, err := b.ListPermissions(ctx, base.ID); err == nil {
			t.Error("los permisos sin servidor no fallan")
		}
		if _, err := b.CreateFormLink(ctx, vista.ID, 10); err == nil {
			t.Error("los formularios públicos sin servidor no fallan")
		}
		if err := b.TransferBase(ctx, base.ID, "otra@ejemplo.org"); err == nil {
			t.Error("traspasar sin servidor no falla")
		}
		if g, err := b.MyGroups(ctx); err != nil || len(g) != 0 {
			t.Errorf("sin servidor no hay grupos: %v %v", g, err)
		}
	})
}

// tiendaDelContrato es una base de pedidos enlazados con sus clientes.
type tiendaDelContrato struct {
	base                                  api.BasesBase
	pedidos, clientes                     api.BasesTable
	nombre, importe, estado, quien, doble api.BasesField
	cliente, ciudad                       api.BasesField
	ana, bea, cleo                        api.BasesRecord
	luz, mar                              api.BasesRecord
}

// tienda monta una base con dos tablas enlazadas, una fórmula y unos registros.
func tienda(t *testing.T, b *api.Bases) tiendaDelContrato {
	t.Helper()
	ctx := t.Context()
	var x tiendaDelContrato
	var err error
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	x.base, err = b.CreateBase(ctx, "Tienda")
	must(err)
	x.pedidos, err = b.CreateTable(ctx, x.base.ID, "Pedidos")
	must(err)
	x.clientes, err = b.CreateTable(ctx, x.base.ID, "Clientes")
	must(err)
	x.cliente, err = b.CreateField(ctx, x.clientes.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
	must(err)
	x.ciudad, err = b.CreateField(ctx, x.clientes.ID, api.BasesNewField{Name: "Ciudad", Kind: "text"})
	must(err)
	x.nombre, err = b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Pedido", Kind: "text"})
	must(err)
	x.importe, err = b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Importe", Kind: "number"})
	must(err)
	x.estado, err = b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Estado", Kind: "select", Choices: []string{"nuevo", "hecho"}})
	must(err)
	x.quien, err = b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Cliente", Kind: "link", LinkTableID: x.clientes.ID})
	must(err)
	x.doble, err = b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Doble", Kind: "formula", Formula: "{" + x.importe.ID + "}*2"})
	must(err)
	x.luz, err = b.CreateRecord(ctx, x.clientes.ID, map[string]any{x.cliente.ID: "Luz", x.ciudad.ID: "Soria"}, nil)
	must(err)
	x.mar, err = b.CreateRecord(ctx, x.clientes.ID, map[string]any{x.cliente.ID: "Mar", x.ciudad.ID: "Lugo"}, nil)
	must(err)
	x.ana, err = b.CreateRecord(ctx, x.pedidos.ID, map[string]any{x.nombre.ID: "Ana", x.importe.ID: 5, x.estado.ID: "nuevo"}, map[string][]string{x.quien.ID: {x.luz.ID}})
	must(err)
	x.bea, err = b.CreateRecord(ctx, x.pedidos.ID, map[string]any{x.nombre.ID: "Bea", x.importe.ID: 20, x.estado.ID: "hecho"}, map[string][]string{x.quien.ID: {x.luz.ID, x.mar.ID}})
	must(err)
	x.cleo, err = b.CreateRecord(ctx, x.pedidos.ID, map[string]any{x.nombre.ID: "Cleo", x.importe.ID: 15}, nil)
	must(err)
	return x
}

// Registros: validar al escribir, la celda con su conflicto, las fórmulas (también las
// que agregan por un enlace), las vistas, la búsqueda y el orden de un momento por
// páginas, la integridad de los enlaces al borrar, exportar e importar.
func TestContratoDeBasesRegistros(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		b, cerrar := a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		x := tienda(t, b)
		if _, err := b.CreateRecord(ctx, x.pedidos.ID, map[string]any{x.importe.ID: "mucho"}, nil); codigo(err) != "invalid" {
			t.Errorf("un número que no lo es: %v", err)
		}
		if _, err := b.CreateRecord(ctx, x.pedidos.ID, map[string]any{x.estado.ID: "otro"}, nil); codigo(err) != "invalid" {
			t.Errorf("una opción que no existe: %v", err)
		}
		if _, err := b.CreateRecord(ctx, x.pedidos.ID, nil, map[string][]string{x.quien.ID: {x.ana.ID}}); codigo(err) != "invalid" {
			t.Errorf("un enlace a un registro de otra tabla: %v", err)
		}
		if x.bea.Data[x.doble.ID] != 40.0 || x.bea.Rev != 1 || x.bea.CreatedBy != sujetoDeBases || len(x.bea.Links[x.quien.ID]) != 2 {
			t.Fatalf("crear: %+v", x.bea)
		}
		if _, err := b.WriteCell(ctx, x.ana.ID, x.doble.ID, api.BasesCell{Value: 1}, "op-formula"); codigo(err) != "invalid" {
			t.Errorf("escribir una fórmula a mano: %v", err)
		}
		// La celda: sin conflicto si se veía lo que había; con él, gana el último.
		res, err := b.WriteCell(ctx, x.ana.ID, x.importe.ID, api.BasesCell{Value: 7, Expected: 5}, "op-1")
		if err != nil || res.Conflict || res.Record.Data[x.doble.ID] != 14.0 || res.Record.Rev != 2 {
			t.Fatalf("escribir una celda: %+v %v", res, err)
		}
		res, err = b.WriteCell(ctx, x.ana.ID, x.importe.ID, api.BasesCell{Value: 9, Expected: 5}, "op-2")
		if err != nil || !res.Conflict || res.Record.Data[x.importe.ID] != 9.0 {
			t.Fatalf("una celda que otro cambió: %+v %v", res, err)
		}
		if res, err := b.WriteCell(ctx, x.ana.ID, x.importe.ID, api.BasesCell{}, "op-3"); err != nil || !res.Conflict || res.Record.Data[x.importe.ID] != nil || res.Record.Data[x.doble.ID] != 0.0 {
			t.Fatalf("vaciar una celda sin decir lo que había (su fórmula cuenta 0): %+v %v", res, err)
		}
		if _, err := b.WriteCell(ctx, x.ana.ID, x.importe.ID, api.BasesCell{Value: 5}, "op-4"); err != nil {
			t.Fatal(err)
		}
		// Una opción retirada no rompe lo escrito, ni impide reescribirlo.
		if err := b.SetFieldChoices(ctx, x.estado.ID, []string{"hecho", "cancelado"}); err != nil {
			t.Fatal(err)
		}
		if _, err := b.WriteCell(ctx, x.ana.ID, x.estado.ID, api.BasesCell{Value: "nuevo", Expected: "nuevo"}, "op-5"); err != nil {
			t.Fatalf("reescribir una opción retirada: %v", err)
		}
		// La fórmula se recalcula al cambiarla, y agrega por un enlace.
		if err := b.SetFieldFormula(ctx, x.doble.ID, "={"+x.importe.ID+"}*3"); err != nil {
			t.Fatal(err)
		}
		if r, _ := b.GetRecord(ctx, x.bea.ID); r.Data[x.doble.ID] != 60.0 {
			t.Fatalf("la fórmula no se recalcula: %v", r.Data[x.doble.ID])
		}
		total, err := b.CreateField(ctx, x.clientes.ID, api.BasesNewField{Name: "Pedidos", Kind: "link", LinkTableID: x.pedidos.ID})
		if err != nil {
			t.Fatal(err)
		}
		suma, err := b.CreateField(ctx, x.clientes.ID, api.BasesNewField{Name: "Gastado", Kind: "formula", Formula: "SUM({" + total.ID + "." + x.importe.ID + "})"})
		if err != nil {
			t.Fatal("agregado:", err)
		}
		if _, err := b.WriteCell(ctx, x.luz.ID, total.ID, api.BasesCell{Links: []string{x.ana.ID, x.bea.ID}}, "op-6"); err != nil {
			t.Fatal(err)
		}
		if r, _ := b.GetRecord(ctx, x.luz.ID); r.Data[suma.ID] != 25.0 {
			t.Fatalf("el agregado: %v", r.Data[suma.ID])
		}
		if _, err := b.WriteCell(ctx, x.bea.ID, x.importe.ID, api.BasesCell{Value: 30, Expected: 20}, "op-7"); err != nil {
			t.Fatal(err)
		}
		if r, _ := b.GetRecord(ctx, x.luz.ID); r.Data[suma.ID] != 35.0 {
			t.Fatalf("el agregado no se recalcula por el enlace: %v", r.Data[suma.ID])
		}

		// Vistas, búsqueda y orden de un momento, de uno en uno.
		vista, err := b.CreateView(ctx, x.pedidos.ID, "Grandes", "grid", api.BasesViewConfig{
			Filters: []api.BasesFilter{{Field: x.importe.ID, Op: "gt", Value: 10}}, Sort: []api.BasesSort{{Field: x.importe.ID, Dir: "desc"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if p, err := b.ListRecords(ctx, x.pedidos.ID, vista.ID, "", 0); err != nil || !slices.Equal(nombres(t, p, x.nombre.ID), []string{"Bea", "Cleo"}) || len(p.Fields) != 5 {
			t.Fatalf("la vista: %q %v", nombres(t, p, x.nombre.ID), err)
		}
		var todos []string
		cursor := ""
		for {
			p, err := b.ListRecordsCon(ctx, x.pedidos.ID, "", cursor, 1, api.BasesLectura{Sort: x.importe.ID + ":asc"})
			if err != nil {
				t.Fatal(err)
			}
			todos = append(todos, nombres(t, p, x.nombre.ID)...)
			if cursor = p.NextCursor; cursor == "" {
				break
			}
		}
		if !slices.Equal(todos, []string{"Ana", "Cleo", "Bea"}) {
			t.Fatalf("de uno en uno con orden: %q", todos)
		}
		if _, err := b.ListRecords(ctx, x.pedidos.ID, vista.ID, "no-es-un-cursor", 1); codigo(err) != "invalid" {
			t.Errorf("un cursor que no es de esta lectura: %v", err)
		}
		if p, _ := b.ListRecordsCon(ctx, x.pedidos.ID, "", "", 0, api.BasesLectura{Q: "LEO"}); !slices.Equal(nombres(t, p, x.nombre.ID), []string{"Cleo"}) {
			t.Fatalf("buscar: %q", nombres(t, p, x.nombre.ID))
		}

		// Borrar algo enlazado: sin forzar es un conflicto; forzando, se quitan los enlaces.
		if err := b.DeleteRecord(ctx, x.mar.ID, false); codigo(err) != "referenced" {
			t.Fatalf("borrar algo enlazado: %v", err)
		}
		if err := b.DeleteRecord(ctx, x.mar.ID, true); err != nil {
			t.Fatal(err)
		}
		if r, _ := b.GetRecord(ctx, x.bea.ID); !slices.Equal(r.Links[x.quien.ID], []string{x.luz.ID}) {
			t.Fatalf("el enlace al borrado sigue: %v", r.Links)
		}
		if _, err := b.GetRecord(ctx, x.mar.ID); codigo(err) != "not_found" {
			t.Fatalf("leer uno borrado: %v", err)
		}
		if p, _ := b.ListRecords(ctx, x.clientes.ID, "", "", 0); len(p.Records) != 1 {
			t.Fatalf("el borrado sigue en la tabla: %+v", p.Records)
		}

		// Exportar: CSV con los enlaces por su texto y las celdas protegidas; JSON.
		if _, err := b.WriteCell(ctx, x.cleo.ID, x.nombre.ID, api.BasesCell{Value: "=1+1"}, "op-8"); err != nil {
			t.Fatal(err)
		}
		rc, err := b.Export(ctx, x.pedidos.ID, "csv")
		if err != nil {
			t.Fatal(err)
		}
		crudo, _ := io.ReadAll(rc)
		filas, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(crudo, []byte("\xef\xbb\xbf")))).ReadAll()
		if err != nil || len(filas) != 4 || !slices.ContainsFunc(filas[1:], func(f []string) bool { return slices.Contains(f, "'=1+1") }) {
			t.Fatalf("csv: %q %v", filas, err)
		}
		if _, err := b.Export(ctx, x.pedidos.ID, "xlsx-de-mentira"); err == nil {
			t.Error("un formato que no existe")
		}
		// Importar un CSV: los tipos inferidos y las filas en su orden.
		gente, err := b.ImportCSV(ctx, x.base.ID, "Gente", []byte("Nombre;Edad;Alta\nAna;30;2024-01-02\nBea;;2025-05-06\n"))
		if err != nil {
			t.Fatal(err)
		}
		tipos := make([]string, 0, len(gente.Fields))
		for _, f := range gente.Fields {
			tipos = append(tipos, f.Kind)
		}
		p, _ := b.ListRecords(ctx, gente.ID, "", "", 0)
		if !slices.Equal(tipos, []string{"text", "number", "date"}) || !slices.Equal(nombres(t, p, gente.Fields[0].ID), []string{"Ana", "Bea"}) {
			t.Fatalf("importar: %q %q", tipos, nombres(t, p, gente.Fields[0].ID))
		}
	})
}

// imagenDePrueba es un PNG de 900×600.
func imagenDePrueba(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 900, 600))
	for y := range 600 {
		for x := range 900 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var foto bytes.Buffer
	if err := png.Encode(&foto, img); err != nil {
		t.Fatal(err)
	}
	return foto.Bytes()
}

// Adjuntos retenidos: los bytes pasan por el puerto y vuelven iguales (también al
// reabrir), la miniatura se hace al pedirla, y borrar uno lo quita; con SoltarAdjuntos
// (Sytue Office) su fichero se suelta y, donde el puerto lo libera, se va del disco, y
// sin él (la conectada) se queda con la ficha en la papelera.
func TestContratoDeBasesAdjuntos(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, p puertoDeBases, a *almacenDeBases) {
		for _, conectada := range []bool{false, true} {
			ctx := t.Context()
			proceso := fmt.Sprint("conectada-", conectada)
			b, cerrar := a.abrir(t, proceso, aperturaDeBases{comoLaConectada: conectada})
			base, _ := b.CreateBase(ctx, fmt.Sprint("Fichas ", conectada))
			tabla, _ := b.CreateTable(ctx, base.ID, "Cosas")
			campo, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Ficheros", Kind: "attachment"})
			texto, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
			fila, _ := b.CreateRecord(ctx, tabla.ID, map[string]any{texto.ID: "Lámpara"}, nil)
			foto := imagenDePrueba(t)
			if _, err := b.UploadAttachment(ctx, fila.ID, texto.ID, "a.png", foto); codigo(err) != "invalid" {
				t.Errorf("adjuntar a un campo que no es de adjuntos: %v", err)
			}
			if _, err := b.UploadAttachment(ctx, fila.ID, campo.ID, "virus.exe", []byte("MZ...")); codigo(err) != "invalid" {
				t.Errorf("un ejecutable: %v", err)
			}
			imagen, err := b.UploadAttachment(ctx, fila.ID, campo.ID, "lampara.png", foto)
			if err != nil || imagen.Mime != "image/png" || !imagen.HasThumb || imagen.Size != int64(len(foto)) {
				t.Fatalf("subir: %+v %v", imagen, err)
			}
			nota, err := b.UploadAttachment(ctx, fila.ID, campo.ID, "nota.txt", []byte("un adjunto de prueba"))
			if err != nil || nota.HasThumb {
				t.Fatalf("subir texto: %+v %v", nota, err)
			}
			cerrar()

			b, cerrar = a.abrir(t, proceso, aperturaDeBases{comoLaConectada: conectada})
			r, err := b.GetRecord(ctx, fila.ID)
			if err != nil || len(r.Attachments[campo.ID]) != 2 || r.Attachments[campo.ID][0].Name != "lampara.png" || r.Attachments[campo.ID][1].Name != "nota.txt" {
				t.Fatalf("las fichas tras reabrir, en su orden: %+v %v", r.Attachments, err)
			}
			rc, err := b.DownloadAttachment(ctx, fila.ID, imagen.ID)
			if err != nil {
				t.Fatal(err)
			}
			if datos, _ := io.ReadAll(rc); !bytes.Equal(datos, foto) {
				t.Fatal("los bytes no vuelven iguales")
			}
			rc, err = b.DownloadThumb(ctx, fila.ID, imagen.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mini, _, err := image.Decode(rc); err != nil || mini.Bounds().Dx() > 512 {
				t.Fatalf("miniatura: %v", err)
			}
			if _, err := b.DownloadThumb(ctx, fila.ID, nota.ID); codigo(err) != "not_found" {
				t.Errorf("la miniatura de un texto: %v", err)
			}
			if _, err := b.DownloadAttachment(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", imagen.ID); codigo(err) != "not_found" {
				t.Errorf("un adjunto pedido desde otro registro: %v", err)
			}
			if err := b.DeleteAttachment(ctx, fila.ID, imagen.ID); err != nil {
				t.Fatal(err)
			}
			if r, _ := b.GetRecord(ctx, fila.ID); len(r.Attachments[campo.ID]) != 1 || r.Attachments[campo.ID][0].ID != nota.ID {
				t.Fatalf("el adjunto borrado sigue: %+v", r.Attachments)
			}
			if _, err := b.DownloadAttachment(ctx, fila.ID, imagen.ID); codigo(err) != "not_found" {
				t.Fatalf("bajar uno borrado: %v", err)
			}
			cerrar()
			if p.liberar == nil {
				continue
			}
			antes, despues := p.liberar(t, a)
			if !conectada && despues >= antes {
				t.Fatalf("soltando, el fichero del borrado sigue en el disco: %d → %d", antes, despues)
			}
			if conectada && despues != antes {
				t.Fatalf("sin soltar, el adjunto en la papelera perdió su fichero: %d → %d", antes, despues)
			}
			// Lo que no se borró sigue entero.
			b, cerrar = a.abrir(t, proceso, aperturaDeBases{comoLaConectada: conectada})
			rc, err = b.DownloadAttachment(ctx, fila.ID, nota.ID)
			if err != nil {
				t.Fatal("el que queda, tras liberar:", err)
			}
			if datos, _ := io.ReadAll(rc); string(datos) != "un adjunto de prueba" {
				t.Fatalf("el que queda, tras liberar: %q", datos)
			}
			cerrar()
		}
	})
}

// Comentarios: limpios, en hilos de un nivel, y una raíz borrada con respuestas se queda
// como lápida hasta que se queda sin ellas.
func TestContratoDeBasesComentarios(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		b, cerrar := a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		x := tienda(t, b)
		raiz, err := b.CreateComment(ctx, x.ana.ID, "  ¿Pagado?  ", "")
		if err != nil || raiz.Body != "¿Pagado?" || !raiz.CanDelete || raiz.AuthorSub != sujetoDeBases {
			t.Fatalf("comentar: %+v %v", raiz, err)
		}
		resp, err := b.CreateComment(ctx, x.ana.ID, "Sí", raiz.ID)
		if err != nil || resp.ParentID != raiz.ID {
			t.Fatalf("responder: %+v %v", resp, err)
		}
		if _, err := b.CreateComment(ctx, x.ana.ID, "x", resp.ID); codigo(err) != "invalid" {
			t.Errorf("responder a una respuesta: %v", err)
		}
		if _, err := b.CreateComment(ctx, x.ana.ID, "   ", ""); codigo(err) != "invalid" {
			t.Errorf("un comentario vacío: %v", err)
		}
		if _, err := b.CreateComment(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", "x", ""); codigo(err) != "not_found" {
			t.Errorf("comentar un registro que no existe: %v", err)
		}
		otro, _ := b.CreateComment(ctx, x.bea.ID, "En otro registro", "")
		if err := b.DeleteComment(ctx, raiz.ID); err != nil {
			t.Fatal(err)
		}
		cs, err := b.ListComments(ctx, x.ana.ID)
		if err != nil || len(cs) != 2 || !cs[0].Deleted || cs[0].Body != "" || cs[0].CanDelete || cs[1].Body != "Sí" {
			t.Fatalf("la lápida: %+v %v", cs, err)
		}
		if err := b.DeleteComment(ctx, resp.ID); err != nil {
			t.Fatal(err)
		}
		if cs, _ := b.ListComments(ctx, x.ana.ID); len(cs) != 0 {
			t.Fatalf("la lápida sin respuestas sigue: %+v", cs)
		}
		if cs, _ := b.ListComments(ctx, x.bea.ID); len(cs) != 1 || cs[0].ID != otro.ID {
			t.Fatalf("los del otro registro: %+v", cs)
		}
	})
}

func accionesDe(es []api.BasesAuditEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Action
	}
	return out
}

// La actividad local (B-06): cada cambio, de lo más reciente a lo más antiguo, por base
// y por páginas, y sobrevive a reabrir; sin ella (la conectada) falla cerrada.
func TestContratoDeBasesActividad(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		b, cerrar := a.abrir(t, "app", aperturaDeBases{})
		base, _ := b.CreateBase(ctx, "Clientes")
		otra, _ := b.CreateBase(ctx, "Otra")
		tabla, _ := b.CreateTable(ctx, base.ID, "Personas")
		nombre, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
		ciudad, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Ciudad", Kind: "text"})
		reg, _ := b.CreateRecord(ctx, tabla.ID, map[string]any{nombre.ID: "Ana"}, nil)
		if _, err := b.WriteCell(ctx, reg.ID, nombre.ID, api.BasesCell{Value: "Ana Ruiz"}, "op-1"); err != nil {
			t.Fatal(err)
		}
		if err := b.ReorderFields(ctx, tabla.ID, []string{ciudad.ID, nombre.ID}); err != nil {
			t.Fatal(err)
		}
		if err := b.DeleteRecord(ctx, reg.ID, false); err != nil {
			t.Fatal(err)
		}
		cerrar()
		b, cerrar = a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		es, err := b.ListAudit(ctx, base.ID, 0, 50)
		quiero := []string{"record.delete", "field.reorder", "record.update", "record.create", "field.create", "field.create", "table.create", "base.create"}
		if err != nil || !slices.Equal(accionesDe(es), quiero) {
			t.Fatalf("actividad = %q, %v", accionesDe(es), err)
		}
		if es[2].RecordID != reg.ID || es[2].FieldID != nombre.ID || es[2].Actor != sujetoDeBases {
			t.Errorf("la celda escrita no dice qué, dónde y quién: %+v", es[2])
		}
		if mas, err := b.ListAudit(ctx, base.ID, es[1].ID, 2); err != nil || !slices.Equal(accionesDe(mas), quiero[2:4]) {
			t.Fatalf("la página siguiente = %q, %v", accionesDe(mas), err)
		}
		if es, err := b.ListAudit(ctx, otra.ID, 0, 50); err != nil || !slices.Equal(accionesDe(es), []string{"base.create"}) {
			t.Fatalf("la otra base = %q, %v", accionesDe(es), err)
		}
		c, cerrarC := a.abrir(t, "conectada", aperturaDeBases{comoLaConectada: true})
		defer cerrarC()
		if _, err := c.ListAudit(ctx, base.ID, 0, 50); !errors.Is(err, api.ErrPrivateBasesOperation) {
			t.Fatalf("sin actividad = %v", err)
		}
	})
}

// vistaDeBases es lo que enseña la pantalla de un almacén: sus bases, tablas, campos,
// vistas, registros (con sus fórmulas, enlaces y adjuntos), comentarios, sin los
// identificadores (que son al azar) más que como nombres.
func vistaDeBases(t *testing.T, b *api.Bases) []string {
	t.Helper()
	ctx := t.Context()
	lista, err := b.ListBases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nombreDe := map[string]string{}
	var out []string
	var tablas []api.BasesTable
	for _, base := range lista {
		ts, err := b.ListTables(ctx, base.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range ts {
			nombreDe[x.ID] = x.Name
			for _, f := range x.Fields {
				nombreDe[f.ID] = f.Name
			}
		}
		tablas = append(tablas, ts...)
	}
	// Los registros se nombran por su primer campo.
	for _, x := range tablas {
		p, err := b.ListRecords(ctx, x.ID, "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Records {
			if len(x.Fields) > 0 {
				nombreDe[r.ID] = fmt.Sprint(r.Data[x.Fields[0].ID])
			}
		}
	}
	nombrar := func(s string) string {
		for id, n := range nombreDe {
			s = strings.ReplaceAll(s, id, "«"+n+"»")
		}
		return s
	}
	for _, base := range lista {
		out = append(out, fmt.Sprintf("base %q %q %s", base.Name, base.Description, base.Role))
	}
	for _, x := range tablas {
		out = append(out, fmt.Sprintf("tabla %q %d", x.Name, x.Position))
		for _, f := range x.Fields {
			out = append(out, nombrar(fmt.Sprintf("  campo %q %s %v %s %q %s %d", f.Name, f.Kind, f.Choices, f.LinkTableID, f.Formula, f.Currency, f.Position)))
		}
		vs, err := b.ListViews(ctx, x.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range vs {
			out = append(out, nombrar(fmt.Sprintf("  vista %q %s %+v", v.Name, v.Kind, v.Config)))
		}
		p, err := b.ListRecords(ctx, x.ID, "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		var filas []string
		for _, r := range p.Records {
			var celdas []string
			for k, v := range r.Data {
				celdas = append(celdas, nombrar(fmt.Sprintf("%s=%v", k, v)))
			}
			for k, v := range r.Links {
				celdas = append(celdas, nombrar(fmt.Sprintf("%s→%v", k, v)))
			}
			for k, as := range r.Attachments {
				for _, x := range as {
					celdas = append(celdas, nombrar(fmt.Sprintf("%s📎%s %s %d", k, x.Name, x.Mime, x.Size)))
				}
			}
			sort.Strings(celdas)
			cs, err := b.ListComments(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cs {
				celdas = append(celdas, fmt.Sprintf("💬%q borrado=%v respuesta=%v", c.Body, c.Deleted, c.ParentID != ""))
			}
			filas = append(filas, fmt.Sprintf("  registro rev=%d %v", r.Rev, celdas))
		}
		sort.Strings(filas)
		out = append(out, filas...)
	}
	return out
}

// juegoDeBasesParaComparar es un poco de todo lo anterior, sobre un almacén nuevo.
func juegoDeBasesParaComparar(t *testing.T, a *almacenDeBases) []string {
	t.Helper()
	ctx := t.Context()
	b, cerrar := a.abrir(t, "app", aperturaDeBases{})
	x := tienda(t, b)
	if err := b.UpdateBase(ctx, x.base.ID, "Tienda online", "La de siempre"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteCell(ctx, x.ana.ID, x.importe.ID, api.BasesCell{Value: 12, Expected: 5}, "op-1"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFieldFormula(ctx, x.doble.ID, "{"+x.importe.ID+"}*10"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateView(ctx, x.pedidos.ID, "Grandes", "grid", api.BasesViewConfig{Filters: []api.BasesFilter{{Field: x.importe.ID, Op: "gt", Value: 10}}}); err != nil {
		t.Fatal(err)
	}
	campo, _ := b.CreateField(ctx, x.pedidos.ID, api.BasesNewField{Name: "Factura", Kind: "attachment"})
	if _, err := b.UploadAttachment(ctx, x.bea.ID, campo.ID, "factura.txt", []byte("Factura 1: 20 €")); err != nil {
		t.Fatal(err)
	}
	raiz, _ := b.CreateComment(ctx, x.bea.ID, "¿Enviado?", "")
	if _, err := b.CreateComment(ctx, x.bea.ID, "Ayer", raiz.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteComment(ctx, raiz.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteRecord(ctx, x.mar.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ImportCSV(ctx, x.base.ID, "Proveedores", []byte("Nombre,Teléfono\nAceites Paco,600111222\n")); err != nil {
		t.Fatal(err)
	}
	antes := vistaDeBases(t, b)
	cerrar()
	b, cerrar = a.abrir(t, "app", aperturaDeBases{})
	defer cerrar()
	if despues := vistaDeBases(t, b); !slices.Equal(antes, despues) {
		t.Fatalf("al reabrir cambia lo que se ve:\nantes:   %q\ndespués: %q", antes, despues)
	}
	return antes
}

// Lo que enseña la pantalla tras el mismo juego es lo mismo en los dos puertos, y lo
// escrito sobrevive a reabrir.
func TestContratoDeBasesVenLoMismo(t *testing.T) {
	var vistas [][]string
	for _, p := range puertosDeBases {
		t.Run(p.nombre, func(t *testing.T) { vistas = append(vistas, juegoDeBasesParaComparar(t, p.nuevo(t))) })
	}
	if len(vistas) == 2 && !slices.Equal(vistas[0], vistas[1]) {
		t.Fatalf("los dos puertos no enseñan lo mismo:\n%s: %q\n%s: %q", puertosDeBases[0].nombre, vistas[0], puertosDeBases[1].nombre, vistas[1])
	}
	if len(vistas) == 2 && len(vistas[0]) < 15 {
		t.Fatalf("el juego enseña demasiado poco para comparar: %q", vistas[0])
	}
}

// Dos procesos sobre el mismo almacén (Bases y el «Traer» del Hub): cada uno ve lo que
// escribe el otro al volver a leer (que en Bases es al escribir: leer no vuelve a leer
// el almacén), el que escribe una celda después del otro lo hace sobre lo de ese otro (y
// lo dice: hay conflicto si no lo había visto) y al reabrir está todo y abre.
func TestContratoDeBasesDosProcesos(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		bA, cerrarA := a.abrir(t, "bases", aperturaDeBases{})
		base, _ := bA.CreateBase(ctx, "Compartida")
		tabla, _ := bA.CreateTable(ctx, base.ID, "Cosas")
		campo, _ := bA.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
		fila, err := bA.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "de A"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		bB, cerrarB := a.abrir(t, "hub", aperturaDeBases{comoLaConectada: true})
		if _, err := bB.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "de B"}, nil); err != nil {
			t.Fatal(err)
		}
		if p, err := bA.ListRecords(ctx, tabla.ID, "", "", 0); err != nil || len(p.Records) != 1 {
			t.Fatalf("A no vuelve a leer al leer: %+v %v", p.Records, err)
		}
		if _, err := bA.WriteCell(ctx, fila.ID, campo.ID, api.BasesCell{Value: "A lo cambia", Expected: "de A"}, "op-a"); err != nil {
			t.Fatal(err)
		}
		if p, err := bA.ListRecords(ctx, tabla.ID, "", "", 0); err != nil || len(p.Records) != 2 {
			t.Fatalf("A ve lo de B tras escribir: %+v %v", p.Records, err)
		}
		res, err := bB.WriteCell(ctx, fila.ID, campo.ID, api.BasesCell{Value: "B lo cambia después", Expected: "de A"}, "op-b")
		if err != nil || !res.Conflict || res.Record.Rev != 3 {
			t.Fatalf("B escribe sobre lo de A, y lo dice: %+v %v", res, err)
		}
		cerrarB()
		cerrarA()
		b, cerrar := a.abrir(t, "bases", aperturaDeBases{})
		defer cerrar()
		p, err := b.ListRecordsCon(ctx, tabla.ID, "", "", 0, api.BasesLectura{Sort: campo.ID + ":asc"})
		if got := nombres(t, p, campo.ID); err != nil || !slices.Equal(got, []string{"B lo cambia después", "de B"}) {
			t.Fatalf("al reabrir, lo de los dos: %q %v", got, err)
		}
	})
}

// Un corte a mitad de escritura: la publicación se hizo pero su respuesta se perdió.
// Repetir la misma celda (la misma operación) no la aplica dos veces y devuelve el
// registro escrito; cerrar ahí y volver a abrir deja lo cortado hecho una vez; y, donde
// se puede simular, una línea a medias al final del almacén no lo tumba, ni lo que se
// escribe después. Lo demás que se repite tras un corte, en
// TestContratoDeBasesRepetirTrasUnCorte.
func TestContratoDeBasesCorteAMitad(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, p puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		corte := &atomic.Bool{}
		b, cerrar := a.abrir(t, "app", aperturaDeBases{corte: corte})
		base, _ := b.CreateBase(ctx, "Cortes")
		tabla, _ := b.CreateTable(ctx, base.ID, "Cosas")
		campo, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
		fila, _ := b.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "antes"}, nil)
		corte.Store(true)
		celda := api.BasesCell{Value: "después", Expected: "antes"}
		if _, err := b.WriteCell(ctx, fila.ID, campo.ID, celda, "op-cortada"); err == nil {
			t.Fatal("el corte no llegó a quien escribía")
		}
		res, err := b.WriteCell(ctx, fila.ID, campo.ID, celda, "op-cortada")
		if err != nil || !res.Replayed || res.Record.ID != fila.ID || res.Record.Rev != 2 || res.Record.Data[campo.ID] != "después" {
			t.Fatalf("repetir la celda cortada devuelve el registro escrito: %+v %v", res, err)
		}
		if r, err := b.GetRecord(ctx, fila.ID); err != nil || r.Rev != 2 || r.Data[campo.ID] != "después" {
			t.Fatalf("la celda cortada y repetida, escrita una vez: %+v %v", r, err)
		}
		// Ahora el corte y el cierre, sin repetir: al volver a abrir, está una vez.
		corte.Store(true)
		if _, err := b.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "cortado al crear"}, nil); err == nil {
			t.Fatal("el corte no llegó a quien escribía")
		}
		cerrar()
		b, cerrar = a.abrir(t, "app", aperturaDeBases{})
		pg, err := b.ListRecordsCon(ctx, tabla.ID, "", "", 0, api.BasesLectura{Sort: campo.ID + ":asc"})
		if got := nombres(t, pg, campo.ID); err != nil || !slices.Equal(got, []string{"cortado al crear", "después"}) {
			t.Fatalf("lo cortado al cerrar, al volver a abrir: %q %v", got, err)
		}
		cerrar()
		if p.lineaCortada == nil {
			return
		}
		p.lineaCortada(t, a)
		b, cerrar = a.abrir(t, "app", aperturaDeBases{})
		if pg, err := b.ListRecords(ctx, tabla.ID, "", "", 0); err != nil || len(pg.Records) != 2 {
			t.Fatalf("una línea a medias al final del almacén: %+v %v", pg.Records, err)
		}
		if _, err := b.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "tras el corte de luz"}, nil); err != nil {
			t.Fatal(err)
		}
		cerrar()
		b, cerrar = a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		if pg, err := b.ListRecords(ctx, tabla.ID, "", "", 0); err != nil || len(pg.Records) != 3 {
			t.Fatalf("lo escrito tras una línea a medias, al volver a abrir: %+v %v", pg.Records, err)
		}
	})
}

// Repetir tras un corte la MISMA operación da lo que habría dado sin el corte, hecho
// una vez (y anotado una vez), también al volver a abrir: un alta es el MISMO registro
// (o tabla, o comentario), un adjunto la MISMA ficha con su fichero y una celda devuelve
// el registro escrito. Fuera de esa repetición, la misma alta otra vez es otra fila, y
// un cambio distinto del mismo objeto (renombrar a otra cosa) no retoma el cortado.
func TestContratoDeBasesRepetirTrasUnCorte(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeBases, a *almacenDeBases) {
		ctx := t.Context()
		corte := &atomic.Bool{}
		b, cerrar := a.abrir(t, "app", aperturaDeBases{corte: corte})
		base, _ := b.CreateBase(ctx, "Reintentos")
		cosas, _ := b.CreateTable(ctx, base.ID, "Cosas")
		campo, _ := b.CreateField(ctx, cosas.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
		ficheros, _ := b.CreateField(ctx, cosas.ID, api.BasesNewField{Name: "Ficheros", Kind: "attachment"})
		cortado := func(que string, err error) {
			t.Helper()
			if err == nil {
				t.Fatalf("%s: el corte no llegó a quien escribía", que)
			}
		}

		corte.Store(true)
		_, err := b.CreateTable(ctx, base.ID, "Cortada")
		cortado("la tabla", err)
		tabla, err := b.CreateTable(ctx, base.ID, "Cortada")
		if ts, _ := b.ListTables(ctx, base.ID); err != nil || len(ts) != 2 || ts[1].ID != tabla.ID || ts[1].Position != 1 {
			t.Fatalf("la tabla cortada y repetida, una vez y la misma: %+v %+v %v", tabla, ts, err)
		}

		datos := map[string]any{campo.ID: "alta cortada"}
		corte.Store(true)
		_, err = b.CreateRecord(ctx, cosas.ID, datos, nil)
		cortado("el alta", err)
		fila, err := b.CreateRecord(ctx, cosas.ID, datos, nil)
		if err != nil || fila.ID == "" || fila.Rev != 1 || fila.Data[campo.ID] != "alta cortada" {
			t.Fatalf("repetir el alta cortada: %+v %v", fila, err)
		}
		if pg, err := b.ListRecords(ctx, cosas.ID, "", "", 0); err != nil || len(pg.Records) != 1 || pg.Records[0].ID != fila.ID {
			t.Fatalf("el alta cortada y repetida, una vez y la misma: %+v %v", pg.Records, err)
		}
		otra, err := b.CreateRecord(ctx, cosas.ID, datos, nil)
		if err != nil || otra.ID == fila.ID {
			t.Fatalf("la misma alta sin corte es otro registro: %+v %v", otra, err)
		}
		if err := b.DeleteRecord(ctx, otra.ID, false); err != nil {
			t.Fatal(err)
		}

		corte.Store(true)
		_, err = b.UploadAttachment(ctx, fila.ID, ficheros.ID, "nota.txt", []byte("adjunto cortado"))
		cortado("el adjunto", err)
		nota, err := b.UploadAttachment(ctx, fila.ID, ficheros.ID, "nota.txt", []byte("adjunto cortado"))
		if err != nil || nota.ID == "" || nota.Name != "nota.txt" || nota.Size != int64(len("adjunto cortado")) {
			t.Fatalf("repetir el adjunto cortado: %+v %v", nota, err)
		}
		if r, err := b.GetRecord(ctx, fila.ID); err != nil || len(r.Attachments[ficheros.ID]) != 1 || r.Attachments[ficheros.ID][0].ID != nota.ID {
			t.Fatalf("el adjunto cortado y repetido, una vez y el mismo: %+v %v", r.Attachments, err)
		}

		corte.Store(true)
		_, err = b.CreateComment(ctx, fila.ID, "comentario cortado", "")
		cortado("el comentario", err)
		comentario, err := b.CreateComment(ctx, fila.ID, "comentario cortado", "")
		if cs, _ := b.ListComments(ctx, fila.ID); err != nil || len(cs) != 1 || cs[0].ID != comentario.ID || !comentario.CanDelete {
			t.Fatalf("el comentario cortado y repetido, uno y el mismo: %+v %+v %v", comentario, cs, err)
		}

		corte.Store(true)
		celda := api.BasesCell{Value: "después", Expected: "alta cortada"}
		_, err = b.WriteCell(ctx, fila.ID, campo.ID, celda, "op-repetida")
		cortado("la celda", err)
		res, err := b.WriteCell(ctx, fila.ID, campo.ID, celda, "op-repetida")
		if err != nil || !res.Replayed || res.Conflict || res.Record.ID != fila.ID || res.Record.TableID != cosas.ID || res.Record.Rev != 2 || res.Record.Data[campo.ID] != "después" || len(res.Record.Attachments[ficheros.ID]) != 1 {
			t.Fatalf("repetir la celda cortada devuelve el registro escrito: %+v %v", res, err)
		}

		// Un cambio cortado no se lleva el siguiente, distinto, del mismo objeto.
		corte.Store(true)
		cortado("renombrar la tabla", b.RenameTable(ctx, tabla.ID, "Dos"))
		if err := b.RenameTable(ctx, tabla.ID, "Tres"); err != nil {
			t.Fatal(err)
		}
		corte.Store(true)
		cortado("renombrar el campo", b.RenameField(ctx, campo.ID, "Mal"))
		if err := b.RenameField(ctx, campo.ID, "Título"); err != nil {
			t.Fatal(err)
		}
		corte.Store(true)
		cortado("cambiar la base", b.UpdateBase(ctx, base.ID, "Mal", ""))
		if err := b.UpdateBase(ctx, base.ID, "Reintentada", "bien"); err != nil {
			t.Fatal(err)
		}
		cerrar()

		b, cerrar = a.abrir(t, "app", aperturaDeBases{})
		defer cerrar()
		pg, err := b.ListRecords(ctx, cosas.ID, "", "", 0)
		if err != nil || len(pg.Records) != 1 || pg.Records[0].ID != fila.ID || pg.Records[0].Rev != 2 || len(pg.Records[0].Attachments[ficheros.ID]) != 1 {
			t.Fatalf("al volver a abrir, lo repetido está una vez: %+v %v", pg.Records, err)
		}
		rc, err := b.DownloadAttachment(ctx, fila.ID, nota.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := io.ReadAll(rc); string(got) != "adjunto cortado" {
			t.Fatalf("los bytes del adjunto repetido: %q", got)
		}
		ts, _ := b.ListTables(ctx, base.ID)
		bs, _ := b.GetBase(ctx, base.ID)
		if got := nombresDeTablas(ts); !slices.Equal(got, []string{"Cosas", "Tres"}) || ts[0].Fields[0].Name != "Título" || bs.Name != "Reintentada" || bs.Description != "bien" {
			t.Fatalf("lo último que se pidió, no lo cortado: %q %+v %+v", got, ts[0].Fields, bs)
		}
		es, err := b.ListAudit(ctx, base.ID, 0, 50)
		quiero := []string{
			"base.update", "field.rename", "table.rename", "record.update", "comment.create", "attachment.add",
			"record.delete", "record.create", "record.create", "table.create", "field.create", "field.create", "table.create", "base.create",
		}
		if err != nil || !slices.Equal(accionesDe(es), quiero) {
			t.Fatalf("lo repetido, anotado una vez: %q %v", accionesDe(es), err)
		}
		if es[3].RecordID != fila.ID || es[3].FieldID != campo.ID || es[5].RecordID != fila.ID || es[5].FieldID != ficheros.ID || es[9].TableID != tabla.ID {
			t.Errorf("lo anotado al repetir no dice dónde: %+v %+v %+v", es[3], es[5], es[9])
		}
	})
}
