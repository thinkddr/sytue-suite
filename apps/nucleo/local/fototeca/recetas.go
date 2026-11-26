// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Derivadas y recetas
//
// Cada foto tiene las derivadas de su receta vacía desde que entra, y las de cada receta
// que se guarda (`derivadas`, por la huella de la receta): editar no toca el original.
// La receta actual va en `ediciones`. Guardar una receta genera sus derivadas ANTES de
// la transacción que la confirma; mientras, la receta y su base esperan en
// `ediciones_a_medias`, que es lo que devuelve EdicionPendiente si el guardado se corta
// o falla (el editor lo ofrece para reintentar) y que se va al guardar.

// huellaActual es la huella de la receta guardada de la foto id (la vacía, sin editar).
func huellaActual(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string,
) (string, error) {
	var h string
	err := q.QueryRowContext(ctx, `SELECT huella FROM ediciones WHERE foto = ?`, id).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return recetaVacia(), nil
	}
	return h, err
}

// recetaActual es la receta guardada de la foto id (la vacía, sin editar).
func recetaActual(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string,
) (model.Receta, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT receta FROM ediciones WHERE foto = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.RecetaVacia(), nil
	}
	if err != nil {
		return model.Receta{}, err
	}
	var r model.Receta
	if json.Unmarshal([]byte(raw), &r) != nil || r.Valida() != nil {
		return model.Receta{}, ErrSource
	}
	return r, nil
}

// VariantDe son los bytes de la derivada variant de la foto id para la receta de huella
// fingerprint (vacía: la guardada). Sin derivada para la receta vacía, la previa se
// calcula en el acto del original (la que pide el editor para comparar).
func (f *Fototeca) VariantDe(ctx context.Context, id, variant, fingerprint string) (io.ReadCloser, error) {
	if !slices.Contains(model.Variantes(), model.Variante(variant)) {
		return nil, model.ErrInvalidInput
	}
	x, err := f.unaFoto(ctx, f.db, id)
	if err != nil {
		return nil, err
	}
	if x.fila.PurgedAt != nil {
		return nil, ErrNotFound
	}
	if fingerprint == "" {
		if fingerprint, err = huellaActual(ctx, f.db, id); err != nil {
			return nil, err
		}
	}
	var sha string
	err = f.db.QueryRowContext(ctx, `SELECT sha FROM derivadas WHERE foto = ? AND receta = ? AND variante = ?`, id, fingerprint, variant).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		if variant == string(model.VariantePrevia) && fingerprint == recetaVacia() {
			return f.previaSinEditar(ctx, x)
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := f.copiarObjeto(ctx, sha, &b); err != nil {
		return nil, err
	}
	return io.NopCloser(&b), nil
}

// previaSinEditar es la previa de x sin receta, calculada en el acto del original.
func (f *Fototeca) previaSinEditar(ctx context.Context, x foto) (io.ReadCloser, error) {
	if x.fila.Bytes > photorender.MaxOriginal {
		return nil, model.ErrInvalidInput
	}
	png := salidaPNG(x.fila)
	return storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
		return photos.PreviaSinEditar(ctx, out, func(ctx context.Context) ([]byte, error) { return f.leerOriginal(ctx, x, photorender.MaxOriginal) }, model.OrientacionDe(x.fila.Orientation), png)
	}), nil
}

// salidaPNG dice si lo que se genera de una foto editada va en PNG: si lo era.
func salidaPNG(a model.AssetRow) bool {
	return strings.EqualFold(a.MediaType, "image/png") || strings.HasSuffix(strings.ToLower(a.Name), ".png")
}

// fotoViva es la foto id fuera de la papelera.
func (f *Fototeca) fotoViva(ctx context.Context, id string) (foto, error) {
	x, err := f.unaFoto(ctx, f.db, id)
	if err == nil && x.fila.DeletedAt != nil {
		err = ErrNotFound
	}
	return x, err
}

// Export es la foto con su receta aplicada, en «jpeg» (o «jpg») o «png»: la de
// «Descargar». No guarda nada.
func (f *Fototeca) Export(ctx context.Context, id, format string) (io.ReadCloser, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "jpg" {
		format = "jpeg"
	}
	if format != "jpeg" && format != "png" {
		return nil, model.ErrInvalidInput
	}
	x, err := f.fotoViva(ctx, id)
	if err == nil && x.fila.Bytes > photorender.MaxOriginal {
		err = model.ErrInvalidInput
	}
	if err != nil {
		return nil, err
	}
	receta, err := recetaActual(ctx, f.db, id)
	if err != nil {
		return nil, err
	}
	return storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
		return photorender.Exportar(ctx, out, func(ctx context.Context) ([]byte, error) { return f.leerOriginal(ctx, x, photorender.MaxOriginal) }, model.OrientacionDe(x.fila.Orientation), receta, format)
	}), nil
}

// LeerReceta es la receta guardada de la foto (la vacía, sin editar).
func (f *Fototeca) LeerReceta(ctx context.Context, id string) (api.Receta, error) {
	if _, err := f.fotoViva(ctx, id); err != nil {
		return photos.VistaDeReceta(model.Receta{}), err
	}
	r, err := recetaActual(ctx, f.db, id)
	return photos.VistaDeReceta(r), err
}

// GuardarRecetaDesde guarda la receta de la foto con sus derivadas y devuelve su huella.
// base, si no está vacía, es la huella de la receta sobre la que se editó: si la guardada
// ya es otra, ErrConflict. Un vídeo no se edita.
func (f *Fototeca) GuardarRecetaDesde(ctx context.Context, id string, receta api.Receta, base string) (string, error) {
	r := photos.RecetaDeVista(receta)
	if err := r.Valida(); err != nil {
		return "", err
	}
	huella, err := r.Huella()
	if err != nil {
		return "", err
	}
	x, err := f.fotoViva(ctx, id)
	if err != nil {
		return "", err
	}
	actual, err := huellaActual(ctx, f.db, id)
	if err != nil {
		return "", err
	}
	if base != "" && base != actual {
		return "", ErrConflict
	}
	if photos.EsVideo(x.fila.MediaType) || x.fila.Bytes > photorender.MaxOriginal {
		return "", model.ErrInvalidInput
	}
	codificada, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	var hay int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM derivadas WHERE foto = ? AND receta = ?`, id, huella).Scan(&hay); err != nil {
		return "", err
	}
	var d *derivadasNuevas
	if hay < len(model.Variantes()) {
		if err := f.aMedias(ctx, id, string(codificada), base); err != nil {
			return "", err
		}
		if d, err = f.derivar(ctx, func(ctx context.Context) ([]byte, error) { return f.leerOriginal(ctx, x, photorender.MaxOriginal) }, x.fila, r, salidaPNG(x.fila)); err != nil {
			return "", err
		}
		defer d.tirar()
	}
	if err := f.confirmarReceta(ctx, id, base, string(codificada), huella, d); err != nil {
		return "", err
	}
	return huella, nil
}

// confirmarReceta guarda en una transacción la receta (con su huella) de la foto id y sus
// derivadas nuevas d (nil si ya las tenía), si la guardada sigue siendo base (o base está
// vacía), y suelta lo que esperaba a medias.
func (f *Fototeca) confirmarReceta(ctx context.Context, id, base, receta, huella string, d *derivadasNuevas) error {
	return f.tx(ctx, func(tx *sql.Tx) error {
		x, err := f.unaFoto(ctx, tx, id)
		if err != nil || x.fila.DeletedAt != nil {
			return ErrNotFound
		}
		ahora, err := huellaActual(ctx, tx, id)
		if err != nil {
			return err
		}
		if base != "" && base != ahora {
			return ErrConflict
		}
		if d != nil {
			if err := f.colocarDerivadas(ctx, tx, id, d); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ediciones (foto, receta, huella, editada) VALUES (?, ?, ?, ?)
			ON CONFLICT (foto) DO UPDATE SET receta = excluded.receta, huella = excluded.huella, editada = excluded.editada`,
			id, receta, huella, enMarca(f.ahora())); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM ediciones_a_medias WHERE foto = ?`, id)
		return err
	})
}

// aMedias deja la receta que se está guardando (y su base) hasta que se confirme
// (EdicionPendiente).
func (f *Fototeca) aMedias(ctx context.Context, id, receta, base string) error {
	return f.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO ediciones_a_medias (foto, receta, base, creada) VALUES (?, ?, ?, ?)
			ON CONFLICT (foto) DO UPDATE SET receta = excluded.receta, base = excluded.base, creada = excluded.creada`,
			id, receta, base, enMarca(f.ahora()))
		return err
	})
}

// EdicionPendiente es la receta (y su base) que se estaba guardando para la foto y no
// llegó a confirmarse; nil si no hay ninguna.
func (f *Fototeca) EdicionPendiente(ctx context.Context, id string) (*api.EdicionPhotos, error) {
	if _, err := f.fotoViva(ctx, id); err != nil {
		return nil, err
	}
	var raw, base string
	err := f.db.QueryRowContext(ctx, `SELECT receta, base FROM ediciones_a_medias WHERE foto = ?`, id).Scan(&raw, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // sin edición a medias no es un error
	}
	if err != nil {
		return nil, err
	}
	var r model.Receta
	if json.Unmarshal([]byte(raw), &r) != nil || r.Valida() != nil {
		return nil, ErrSource
	}
	return &api.EdicionPhotos{Receta: photos.VistaDeReceta(r), Base: base}, nil
}
