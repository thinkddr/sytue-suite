// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// lineaSinProyectar es la línea de tiempo como se calculaba antes de proyectarla
// (linea.go): la galería entera leída, filtrada, ordenada y cortada en cada página. Es
// la referencia con la que se compara.
func (r *Repository) lineaSinProyectar(ctx context.Context, cursor string, desdeFecha bool, limit int, favorites bool) ([]api.Photo, string, error) {
	after, err := r.decodeTimelineCursor(cursor, desdeFecha)
	if err != nil {
		return nil, "", err
	}
	assets, err := r.assets(ctx)
	if err != nil {
		return nil, "", err
	}
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	values := []api.Photo{}
	for _, asset := range assets {
		if asset.DeletedAt != nil || favorites && !asset.Favorite {
			continue
		}
		view := photoView(asset)
		if cursor != "" && (view.TakenAt.After(after.TakenAt) || view.TakenAt.Equal(after.TakenAt) && view.ID >= after.ID) {
			continue
		}
		values = append(values, view)
	}
	slices.SortFunc(values, func(a, b api.Photo) int {
		if order := b.TakenAt.Compare(a.TakenAt); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	next := ""
	if len(values) > limit {
		last := values[limit-1]
		raw, _ := json.Marshal(timelineCursor{Scope: r.scope, TakenAt: last.TakenAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
		values = values[:limit]
	}
	return conEstados(values), next, nil
}

func jpegDeLinea(t *testing.T, tono uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := range 12 {
		for x := range 16 {
			img.Set(x, y, color.RGBA{tono, uint8(x * 16), uint8(y * 20), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// La línea de tiempo proyectada (linea.go) da exactamente las mismas páginas que
// calcularla entera en cada página, con y sin favoritas, con cualquier tamaño de página,
// desde una fecha, y después de cambiar favoritas, papelera y purgas (la revisión del
// catálogo la vuelve a proyectar). Cambiar lo que devuelve una página no cambia la
// siguiente.
func TestLaLineaProyectadaDaLasMismasPaginas(t *testing.T) {
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), t.TempDir(), "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	repo, err := New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 9)
	for i := range 9 {
		foto := jpegDeLinea(t, uint8(i*25))
		p, err := repo.ImportarFoto(ctx, uuid.NewString(), "foto.jpg", "image/jpeg", bytes.NewReader(foto), int64(len(foto)))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	comprobar := func(paso string) {
		t.Helper()
		for _, favoritas := range []bool{false, true} {
			for _, limite := range []int{1, 2, 4, 100, 0} {
				cursor, otro := "", ""
				for pagina := 0; ; pagina++ {
					got, siguiente, err := repo.Timeline(ctx, cursor, limite, favoritas, "")
					want, siguienteRef, errRef := repo.lineaSinProyectar(ctx, otro, false, limite, favoritas)
					if err != nil || errRef != nil || !reflect.DeepEqual(got, want) || siguiente != siguienteRef {
						t.Fatalf("%s, favoritas %v, página %d de %d: %v %v\n%+v\nfrente a\n%+v", paso, favoritas, pagina, limite, err, errRef, got, want)
					}
					// Lo devuelto es de quien lo pide: cambiarlo no cambia la proyección.
					for i := range got {
						got[i].Tags = append(got[i].Tags, "tocada")
						got[i].Name = "cambiada"
					}
					if siguiente == "" {
						break
					}
					cursor, otro = siguiente, siguienteRef
				}
			}
		}
		desde, _, err := repo.TimelineDesde(ctx, "2999", 3, false)
		cursor, _ := json.Marshal(timelineCursor{Scope: repo.scope, TakenAt: mustLimite(t, "2999")})
		ref, _, errRef := repo.lineaSinProyectar(ctx, base64.RawURLEncoding.EncodeToString(cursor), true, 3, false)
		if err != nil || errRef != nil || !reflect.DeepEqual(desde, ref) {
			t.Fatalf("%s, desde una fecha: %v %v %+v %+v", paso, err, errRef, desde, ref)
		}
	}
	comprobar("recién importadas")
	revision := repo.linea.revision
	if _, _, err := repo.Timeline(ctx, "", 2, false, ""); err != nil || repo.linea.revision != revision {
		t.Fatal("sin cambios se vuelve a proyectar", err)
	}
	for _, id := range ids[:4] {
		if err := repo.SetFavorite(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}
	comprobar("con favoritas")
	if err := repo.Trash(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := repo.Trash(ctx, ids[5]); err != nil {
		t.Fatal(err)
	}
	comprobar("con dos en la papelera")
	if err := repo.Restaurar(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Purgar(ctx, ids[5]); err != nil {
		t.Fatal(err)
	}
	comprobar("restaurada una y purgada otra")
	if repo.linea.revision == revision {
		t.Fatal("los cambios no cambiaron la revisión proyectada")
	}
}

func mustLimite(t *testing.T, desde string) time.Time {
	t.Helper()
	l, err := limiteDeFecha(desde)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
