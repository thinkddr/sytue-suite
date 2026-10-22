// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Deduplicación de ficheros DENTRO de cada organización.
//
// Cada fichero se sube a su clave propia —como siempre— y, con el hash ya
// calculado, se MUEVE a la clave por contenido `shared/drive/<organización>/<sha256>`:
// si ya existía (otra persona de la misma organización subió el mismo fichero)
// no se copia nada y la subida propia se borra. Las filas de metadatos no saben
// nada de esto: apuntan a una storage_key, sea de quien sea.
//
// La organización va en la clave por decisión del dueño (2026-10-16): con la
// clave común a todas, si un fichero ya existía o no se notaba (no se copia y
// la subida acaba antes), y eso dejaba averiguar si OTRA organización tenía un
// fichero concreto. Lo guardado antes, en `shared/drive/<sha256>`, se queda
// donde está y puede seguir referenciado desde varias organizaciones.
//
// Lo que cambia es el BORRADO: un blob compartido no se puede quitar de S3 al
// purgar una fila, porque otra fila —y, en las claves de antes, otra
// organización— puede seguir usándolo, y bajo el aislamiento por tenant no hay
// forma de saberlo. Por eso las claves compartidas no se borran en el acto: se
// anotan como candidatas y el barrido de mantenimiento (que ve todas las
// organizaciones) las borra pasado un margen, si nadie las usa.
//
// La cuota que ve la organización sigue siendo la LÓGICA: un fichero de 10 MB
// ocupa 10 MB de su cuota aunque los bytes estén compartidos.

const prefijoDrive = objectstore.PrefijoCompartido + "drive/"

// gracia es lo que espera un candidato antes de que el barrido lo considere:
// más que lo que tarda cualquier subida en confirmar su metadato.
const gracia = 15 * time.Minute

func esCompartida(key string) bool { return strings.HasPrefix(key, objectstore.PrefijoCompartido) }

// compartirBlob lleva el objeto recién subido bajo `propia` a su clave por
// contenido y devuelve la clave que debe guardar la fila. Si algo falla por el
// camino se queda con la clave propia: sin deduplicar, pero sin perder nada.
//
// Los blobs del almacén de TERCEROS (store.EsExterno) NO se deduplican: viven
// en el bucket del cliente y moverlos a shared/drive/ los sacaría de allí y los
// borraría de su almacenamiento propio.
func (s *Service) compartirBlob(ctx context.Context, t tenancy.Tenant, propia, hash string) string {
	if hash == "" || t.Slug == "" || esCompartida(propia) || store.EsExterno(propia) {
		return propia
	}
	destino := prefijoDrive + t.Slug + "/" + hash
	// Si el barrido tenía la clave como candidata a borrar, retirarla ANTES de
	// mirar si existe: la llamada espera a que el barrido suelte la fila, así
	// que lo que se compruebe después es lo que hay de verdad.
	if s.team != nil {
		if err := s.team.OlvidarBlobCandidato(ctx, destino); err != nil {
			log.From(ctx).Warn("drive: retirar candidato", "key", destino, "error", err)
			return propia
		}
	}
	existe, err := s.blobs.Existe(ctx, t, destino)
	if err != nil {
		log.From(ctx).Warn("drive: comprobar blob compartido", "key", destino, "error", err)
		return propia
	}
	if !existe {
		if err := s.blobs.Copiar(ctx, t, propia, destino); err != nil {
			log.From(ctx).Warn("drive: compartir blob", "key", destino, "error", err)
			return propia
		}
	}
	if err := s.blobs.Delete(ctx, t, propia); err != nil {
		// El objeto propio sobra; si no se pudo borrar lo hará un barrido. No es
		// motivo para renunciar a la clave compartida.
		log.From(ctx).Warn("drive: borrar subida propia tras compartir", "key", propia, "error", err)
	}
	return destino
}

// hashDe calcula el SHA-256 de un objeto ya en S3: lo necesita la subida directa
// (prefirmada), cuyos bytes no pasaron por aquí. Si tope > 0, corta en tope+1
// con LimitReader y devuelve ErrUploadSizeMismatch si el flujo los supera
// (p. ej. ante un objeto con metadato de compresión o un backend cuyo flujo
// excede Stat).
func (s *Service) hashDe(ctx context.Context, t tenancy.Tenant, key string, tope int64) (string, error) {
	rc, err := s.blobs.Get(ctx, t, key)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	var r io.Reader = rc
	if tope > 0 {
		r = io.LimitReader(rc, tope+1)
	}
	n, err := io.Copy(h, r)
	if err != nil {
		return "", err
	}
	if tope > 0 && n > tope {
		return "", ErrUploadSizeMismatch
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// soltarBlob es lo que se hace con una clave que una fila dejó de referenciar:
// las propias se borran; las compartidas se anotan para el barrido.
func (s *Service) soltarBlob(ctx context.Context, t tenancy.Tenant, key string) {
	if !esCompartida(key) {
		if err := s.blobs.Delete(ctx, t, key); err != nil {
			log.From(ctx).Warn("drive: borrar blob", "key", key, "error", err)
		}
		return
	}
	if s.team == nil {
		return
	}
	if err := s.team.AnotarBlobCandidato(ctx, key); err != nil {
		log.From(ctx).Warn("drive: anotar candidato", "key", key, "error", err)
	}
}

// purgarBlobsCompartidos borra de S3 los candidatos que, pasado el margen, no
// usa ninguna organización. Lo llama el worker de papelera en cada ciclo.
func (s *Service) purgarBlobsCompartidos(ctx context.Context) (int, error) {
	if s.team == nil {
		return 0, nil
	}
	return s.team.PurgarBlobsCandidatos(ctx, time.Now().Add(-gracia), trashGCBatch,
		func(ctx context.Context, key string) error {
			return s.blobs.Delete(ctx, tenancy.Tenant{}, key)
		})
}
