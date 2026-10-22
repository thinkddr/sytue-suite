// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// storeCursor construye un FileCursor de prueba con un UUID determinista a partir
// del nombre (para tests reproducibles).
func storeCursor(name string) store.FileCursor {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
	return store.FileCursor{NameLower: strings.ToLower(name), ID: id}
}

// encodeGarbage devuelve un token base64 válido cuyo contenido NO es JSON.
func encodeGarbage() string {
	return base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xfe, 0x00, 0x01})
}

// TestSplitPathTraversal comprueba que la normalización de rutas WebDAV neutraliza
// intentos de path traversal ("..") y separadores redundantes antes de resolver.
func TestSplitPathTraversal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"raiz vacia", "", nil},
		{"raiz barra", "/", nil},
		{"raiz punto", ".", nil},
		{"simple", "/a/b/c", []string{"a", "b", "c"}},
		{"barras duplicadas", "//a///b//", []string{"a", "b"}},
		{"sin barra inicial", "a/b", []string{"a", "b"}},
		{"traversal sube por encima de raiz", "/../../etc/passwd", []string{"etc", "passwd"}},
		{"traversal interno se colapsa", "/a/../b", []string{"b"}},
		{"traversal encadenado", "/a/b/../../c", []string{"c"}},
		{"punto simple se elimina", "/a/./b", []string{"a", "b"}},
		{"espacios recortados", "  /a/b  ", []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitPath(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("splitPath(%q)=%v, quiero %v", c.in, got, c.want)
			}
			// Invariante de seguridad: ningún segmento resultante es "..".
			for _, seg := range got {
				if seg == ".." || strings.Contains(seg, "/") {
					t.Fatalf("segmento inseguro %q en %v", seg, got)
				}
			}
		})
	}
}

// TestFileCursorRoundTrip verifica que el cursor keyset se codifica/decodifica sin
// pérdida y que un token corrupto no revienta (se traduce a entrada inválida).
func TestFileCursorRoundTrip(t *testing.T) {
	t.Parallel()

	// Primera página: cursor vacío decodifica al cero-valor.
	got, err := decodeFileCursor("")
	if err != nil {
		t.Fatalf("cursor vacío: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("cursor vacío debe ser cero, got %+v", got)
	}

	orig := storeCursor("informe final.pdf")
	tok := encodeFileCursor(orig)
	if tok == "" {
		t.Fatal("token codificado vacío")
	}
	back, err := decodeFileCursor(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back != orig {
		t.Fatalf("roundtrip: got %+v, quiero %+v", back, orig)
	}

	// Token corrupto -> error de entrada, no de servidor.
	if _, err := decodeFileCursor("!!!no-es-base64!!!"); err == nil {
		t.Fatal("token corrupto debe fallar")
	}
	if _, err := decodeFileCursor(encodeGarbage()); err == nil {
		t.Fatal("payload no-JSON debe fallar")
	}
}

// TestBlobKeyOpaque verifica que la clave S3 no expone el nombre del usuario y es
// impredecible (dos llamadas con los mismos argumentos difieren).
func TestBlobKeyOpaque(t *testing.T) {
	t.Parallel()
	fid := storeCursor("secreto.txt").ID // reutilizamos un UUID cualquiera
	k1 := blobKey(fid, "nómina-confidencial.pdf")
	k2 := blobKey(fid, "nómina-confidencial.pdf")
	if k1 == k2 {
		t.Fatal("blobKey debe ser impredecible (aleatorio)")
	}
	if strings.Contains(k1, "nómina") || strings.Contains(k1, "confidencial") {
		t.Fatalf("blobKey no debe filtrar el nombre: %q", k1)
	}
	if !strings.HasPrefix(k1, "files/"+fid.String()+"/") {
		t.Fatalf("estructura inesperada: %q", k1)
	}
}

// TestSecureTokenDistinct verifica que los tokens de enlace son únicos y URL-safe.
func TestSecureTokenDistinct(t *testing.T) {
	t.Parallel()
	a, err := secureToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := secureToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("dos tokens no deben coincidir")
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("token no es URL-safe: %q", a)
	}
}

// ---------------------------------------------------------------------------
// ResolvePath: cuántas filas pide para recorrer una ruta
// ---------------------------------------------------------------------------

// arbolEspia es un dataStore con un árbol de carpetas de verdad que además CUENTA
// qué consulta usó cada tramo. Empotra fakeStore para heredar el resto de la
// interfaz y solo redefine lo que ResolvePath toca.
type arbolEspia struct {
	*fakeStore
	// hijos: carpeta → subcarpetas; ficheros: carpeta → ficheros vivos.
	hijos    map[uuid.UUID][]domain.Folder
	ficheros map[uuid.UUID][]domain.File
	// Llamadas por método: lo que se vigila es que los tramos INTERMEDIOS no pidan
	// ficheros (ListChildren los trae todos, sin LIMIT, para descartarlos).
	nChildren   int
	nSubfolders int
	nResolve    int
}

// ResolveUnder imita lo que hace el SQL: baja por CARPETAS y solo en el último
// tramo mira ficheros. Cuenta sus llamadas para poder exigir que ResolvePath
// haga UNA, sea cual sea la profundidad de la ruta.
func (a *arbolEspia) ResolveUnder(_ context.Context, root uuid.UUID, segs []string) (*domain.Folder, *domain.File, error) {
	a.nResolve++
	actual := root
	for i, seg := range segs {
		if i == len(segs)-1 {
			for _, f := range a.ficheros[actual] {
				if strings.EqualFold(f.Name, seg) {
					return nil, &f, nil
				}
			}
		}
		var siguiente *domain.Folder
		for j := range a.hijos[actual] {
			if strings.EqualFold(a.hijos[actual][j].Name, seg) {
				siguiente = &a.hijos[actual][j]
				break
			}
		}
		if siguiente == nil {
			return nil, nil, nil
		}
		if i == len(segs)-1 {
			return siguiente, nil, nil
		}
		actual = siguiente.ID
	}
	return nil, nil, nil
}

func (a *arbolEspia) ListChildren(_ context.Context, id uuid.UUID) ([]domain.Folder, []domain.File, error) {
	a.nChildren++
	return a.hijos[id], a.ficheros[id], nil
}

func (a *arbolEspia) ListSubfolders(_ context.Context, id uuid.UUID) ([]domain.Folder, error) {
	a.nSubfolders++
	return a.hijos[id], nil
}

// arbolDePrueba monta /a/b/c/d.pdf y deja un fichero suelto en cada carpeta
// intermedia (los que NO hay que leer para resolver la ruta).
func arbolDePrueba() (*arbolEspia, *Service) {
	raiz := domain.Folder{ID: uuid.New(), IsRoot: true}
	a := domain.Folder{ID: uuid.New(), Name: "a"}
	b := domain.Folder{ID: uuid.New(), Name: "b"}
	c := domain.Folder{ID: uuid.New(), Name: "c"}
	hoja := domain.File{ID: uuid.New(), Name: "d.pdf", FolderID: c.ID}

	espia := &arbolEspia{
		fakeStore: &fakeStore{root: raiz},
		hijos: map[uuid.UUID][]domain.Folder{
			raiz.ID: {a}, a.ID: {b}, b.ID: {c},
		},
		ficheros: map[uuid.UUID][]domain.File{
			raiz.ID: {{Name: "ruido-raiz.bin"}},
			a.ID:    {{Name: "ruido-a.bin"}},
			b.ID:    {{Name: "ruido-b.bin"}},
			c.ID:    {hoja},
		},
	}
	return espia, &Service{store: espia}
}

// TestResolvePathNoLeeFicherosIntermedios: los tramos intermedios se resuelven con
// ListSubfolders. Con ListChildren el resultado sería el MISMO, así que comprobar
// solo el nodo devuelto no distingue nada: hay que mirar qué se le pidió al store.
func TestResolvePathNoLeeFicherosIntermedios(t *testing.T) {
	t.Parallel()
	espia, s := arbolDePrueba()

	n, err := s.ResolvePath(context.Background(), "/a/b/c/d.pdf")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if n.IsDir() || n.File == nil || n.File.Name != "d.pdf" {
		t.Fatalf("esperaba el fichero d.pdf, obtuve %+v", n)
	}
	// UNA sola consulta para los cuatro tramos, y ninguna de las de listar. Antes
	// era una llamada por tramo —tres ListSubfolders y un ListChildren—, así que
	// el coste crecía con la profundidad de la ruta; ahora es fijo.
	if espia.nResolve != 1 {
		t.Fatalf("ResolveUnder llamado %d veces; la ruta entera se resuelve en una", espia.nResolve)
	}
	if espia.nChildren != 0 || espia.nSubfolders != 0 {
		t.Fatalf("listados por tramo: ListChildren=%d ListSubfolders=%d; esperaba 0 y 0",
			espia.nChildren, espia.nSubfolders)
	}
}

// TestResolvePathCarpetaYFicheroSiguenResolviendo protege el caso que el cambio
// podría haber roto: que el último tramo siga encontrando TANTO carpetas como
// ficheros, y que una ruta inexistente siga dando ErrNotFound.
func TestResolvePathCarpetaYFicheroSiguenResolviendo(t *testing.T) {
	t.Parallel()
	_, s := arbolDePrueba()
	ctx := context.Background()

	n, err := s.ResolvePath(ctx, "/a/b/c")
	if err != nil {
		t.Fatalf("ResolvePath carpeta: %v", err)
	}
	if !n.IsDir() || n.Folder.Name != "c" {
		t.Fatalf("esperaba la carpeta c, obtuve %+v", n)
	}

	// La raíz sin tocar el store de listados.
	r, err := s.ResolvePath(ctx, "/")
	if err != nil || !r.IsDir() || !r.Folder.IsRoot {
		t.Fatalf("esperaba la raíz, obtuve %+v (err=%v)", r, err)
	}

	// errors.Is y no `!=`: en cuanto alguien envuelva el error con %w por el camino
	// —anotar la ruta que falló es lo natural— la comparación directa dejaría de
	// verlo y el test pasaría a verde sin comprobar nada.
	if _, err := s.ResolvePath(ctx, "/a/b/no-existe/d.pdf"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, obtuve %v", err)
	}
	// Un fichero NO es un tramo intermedio válido: /a/b/c/d.pdf/x no existe.
	if _, err := s.ResolvePath(ctx, "/a/b/c/d.pdf/x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound al atravesar un fichero, obtuve %v", err)
	}
}
