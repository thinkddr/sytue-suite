// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// trashGCBatch acota cuántos elementos de NIVEL SUPERIOR de la papelera purga cada
// transacción del worker (una carpeta arrastra su subárbol por cascada, así que el
// nº de blobs/filas afectadas puede ser mayor). Cota por ciclo: no purga millones
// de golpe; el resto se drena en transacciones sucesivas del mismo barrido.
// ponytail: lote fijo; subirlo o repartir entre más réplicas si el volumen de
// papelera caducada lo exige (el claim con SKIP LOCKED ya reparte entre réplicas).
const trashGCBatch = 200

// PurgeExpiredTrash borra DEFINITIVAMENTE los elementos de la papelera cuya
// retención (cfg.TrashRetention) venció, CROSS-TENANT y cross-réplica seguro (claim
// con SKIP LOCKED + borrado terminal en la misma tx; ver store.ClaimAndPurgeExpiredTrash).
// Reusa la purga del store (cuota + guarda de blob compartido) y borra de S3 SOLO
// los blobs que quedaron HUÉRFANOS, cada uno bajo su tenant. Drena en lotes hasta
// vaciar lo caducado (o hasta que el contexto se cancele). Devuelve cuántos
// elementos de nivel superior purgó. Un TrashRetention <= 0 la desactiva (defensa:
// nunca purga la papelera entera por una config a cero).
func (s *Service) PurgeExpiredTrash(ctx context.Context) (int, error) {
	// Va en la misma ronda que la papelera aunque no sea papelera: es el único
	// barrido periódico de Drive, y lo de las subidas no espera a una retención.
	s.barrerSubidasCaducadas(ctx)
	if s.cfg.TrashRetention <= 0 {
		return 0, nil
	}
	total := 0
	for {
		select {
		case <-ctx.Done():
			return total, nil // apagado ordenado: lo restante se drena al reanudar
		default:
		}
		cutoff := time.Now().Add(-s.cfg.TrashRetention)
		claimed, blobs, err := s.store.ClaimAndPurgeExpiredTrash(ctx, cutoff, trashGCBatch)
		if err != nil {
			return total, err
		}
		s.deleteOrphanedBlobs(ctx, blobs)
		total += claimed
		if claimed < trashGCBatch {
			break // papelera caducada drenada
		}
	}
	// Los blobs compartidos que las purgas dejaron como candidatos: pasado el
	// margen y sin ninguna organización que los use, fuera.
	if n, err := s.purgarBlobsCompartidos(ctx); err != nil {
		log.From(ctx).Warn("drive: barrido de blobs compartidos", "error", err)
	} else if n > 0 {
		log.From(ctx).Info("drive: blobs compartidos borrados", "n", n)
	}
	return total, nil
}

// deleteOrphanedBlobs borra de S3 (best-effort) los blobs que quedaron huérfanos,
// cada uno bajo SU tenant. Conserva los que una copia/versión viva aún referencia
// (Orphaned=false): borrarlos perdería datos de la copia superviviente. Dedup por
// (tenant, clave): un blob referenciado por varias filas del subárbol se borra una
// sola vez. Un fallo de borrado no aborta el barrido: el metadato ya no existe y el
// objeto queda como huérfano recuperable por un GC de blobs posterior.
func (s *Service) deleteOrphanedBlobs(ctx context.Context, blobs []store.PurgedBlob) {
	seen := make(map[string]struct{}, len(blobs))
	for _, b := range blobs {
		if !b.Orphaned {
			continue
		}
		k := b.Tenant + "\x00" + b.StorageKey
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		s.soltarBlob(ctx, tenancy.Tenant{Slug: b.Tenant}, b.StorageKey)
	}
}

// barrerSubidasCaducadas borra el objeto de cada URL de subida directa que ya no
// vale (FILE-08): si se finalizó, el fichero vive en otra clave y lo que haya en
// esta es lo que se escribió después; si no, es una subida abandonada que no
// gastaba cuota. Nada que un fichero use se borra.
func (s *Service) barrerSubidasCaducadas(ctx context.Context) {
	for {
		caducadas, err := s.store.ReclamarSubidasCaducadas(ctx, time.Now(), trashGCBatch)
		if err != nil {
			log.From(ctx).Warn("drive: barrido de subidas directas caducadas", "error", err)
			return
		}
		for _, c := range caducadas {
			t := tenancy.Tenant{Slug: c.Tenant}
			for _, clave := range []string{c.StorageKey, c.StorageKey + sufijoFirme} {
				if _, err := s.store.GetFileByStorageKey(tenancy.Into(ctx, t), clave); err == nil {
					continue
				}
				if err := s.blobs.Delete(ctx, t, clave); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
					log.From(ctx).Warn("drive: borrar una subida directa caducada", "key", clave, "error", err)
				}
			}
		}
		if len(caducadas) < trashGCBatch {
			return
		}
	}
}
