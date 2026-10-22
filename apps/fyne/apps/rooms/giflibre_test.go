// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Volver a elegir el mismo resultado no duplica un archivo en Drive ni su
// entrada de catálogo; el mensaje reutiliza el atajo de la organización.
func TestElGifLibreSeImportaUnaSolaVez(t *testing.T) {
	ep := &conEmoji{servicioFalso: servicioConDos()}
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	g := api.RoomsFreeGif{ID: 123, Title: "Dance", License: "CC0 1.0"}
	for range 2 {
		e, err := importarGifLibre(context.Background(), ep, dr, g, []byte("already validated by Rooms"))
		if err != nil || e.Name != "commons-123" || !strings.HasPrefix(e.FileID, "f-commons-123-") {
			t.Fatalf("importación: %+v %v", e, err)
		}
	}
	if len(ep.catalogo) != 1 || len(dr.llamadas) != 1 || !strings.HasPrefix(dr.llamadas[0], "subir raiz commons-123-") {
		t.Fatalf("duplicó el GIF: catálogo=%v Drive=%v", ep.catalogo, dr.llamadas)
	}
}

// El fichero previo de Drive no se sustituye; solo el atajo sigue siendo estable.
func TestImportGifLibreNoReemplazaNombrePrevio(t *testing.T) {
	ep := &conEmoji{servicioFalso: servicioConDos()}
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	e, err := importarGifLibre(context.Background(), ep, dr, api.RoomsFreeGif{ID: 123}, []byte("gif"))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(e.FileID, "f-commons-123-"), ".gif")
	if _, err := uuid.Parse(id); err != nil || e.FileID == "f-commons-123.gif" {
		t.Fatalf("nombre sobrescribe el previo: %q", e.FileID)
	}
}

type emojiConcurrente struct{ *conEmoji }

func (e *emojiConcurrente) CreateCustomEmoji(_ context.Context, name, _ string) (api.RoomsCustomEmoji, error) {
	e.catalogo = append(e.catalogo, api.RoomsCustomEmoji{Name: name, FileID: "ganador", ContentType: "image/gif"})
	return api.RoomsCustomEmoji{}, &api.Error{Status: 409}
}

func TestImportGifLibreLimpiaSoloSuArchivoPerdedor(t *testing.T) {
	ep := &emojiConcurrente{&conEmoji{servicioFalso: servicioConDos()}}
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	e, err := importarGifLibre(context.Background(), ep, dr, api.RoomsFreeGif{ID: 123}, []byte("gif"))
	if err != nil || e.FileID != "ganador" {
		t.Fatalf("no reutilizó ganador: %+v %v", e, err)
	}
	if len(dr.llamadas) != 2 || !strings.HasPrefix(dr.llamadas[1], "borrar files f-commons-123-") || strings.Contains(dr.llamadas[1], "ganador") {
		t.Fatalf("borrado ajeno o ausente: %v", dr.llamadas)
	}
}

type gifCancelado struct {
	*conEmoji
	llamada context.Context
}

func (s *gifCancelado) FreeGifs(context.Context, string, string) (api.RoomsFreeGifPage, error) {
	return api.RoomsFreeGifPage{}, nil
}

func (s *gifCancelado) FreeGifContent(ctx context.Context, _ int64, _ bool) (io.ReadCloser, error) {
	s.llamada = ctx
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("preview sin cancelar")
}

func TestCerrarSelectorGifCancelaPreviewSinInsertar(t *testing.T) {
	base := servicioConDos()
	sa, _, _ := salaEn(t, base, 1280)
	ep := &gifCancelado{conEmoji: &conEmoji{servicioFalso: base}}
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	sa.S, sa.Ficheros = ep, dr
	sa.Arrancar(context.Background())
	sa.AbrirGif()
	picker := sa.gif
	if picker == nil || picker.libre == nil {
		t.Fatal("selector libre ausente")
	}
	var pending []func()
	sa.Lanzar = func(f func()) { pending = append(pending, f) }
	picker.libre.Choose(api.RoomsFreeGif{ID: 123})
	picker.alCerrarLibre()
	sa.gif = nil
	for _, f := range pending {
		f()
	}
	if ep.llamada == nil || !errors.Is(ep.llamada.Err(), context.Canceled) {
		t.Fatal("la red no recibió cancelación del selector")
	}
	if len(ep.catalogo) != 0 || len(dr.llamadas) != 0 || strings.Contains(sa.P.Canal.Compositor.Texto(), ":commons-123:") {
		t.Fatal("cerrar importó o insertó el GIF")
	}
}
