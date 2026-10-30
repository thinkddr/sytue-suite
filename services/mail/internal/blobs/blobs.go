// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package blobs guarda el .eml crudo de un mensaje UNA sola vez por
// organización: la clave es la organización y el SHA-256 del contenido, en el
// espacio compartido del almacén (objectstore.PrefijoCompartido), y el objeto va
// comprimido.
//
// Antes, un correo a cien buzones eran cien objetos idénticos. Con esto es uno
// por organización, y cada fila de `messages` que lo usa cuenta una referencia
// en `blob_refs` (la suma la hace el store en la misma transacción que el
// insert; la resta, la purga física).
//
// La organización va en la clave por decisión del dueño (2026-10-16), como en
// Drive: con la clave común a todas, si un mensaje ya existía no se subía y la
// importación o el borrador acababan antes, y eso dejaba averiguar si OTRA
// organización tenía un mensaje concreto. Los de antes (`shared/mensajes/<hash>.eml`,
// fila de `blob_refs` con el hash a secas) se quedan donde están, con su
// contador, que sigue bajando desde cualquier organización que los use: solo se
// borran al llegar a cero.
//
// El orden importa y está pensado para las carreras: se RESERVA la fila, se
// sube el objeto, y solo entonces se marca listo. Quien encuentra una reserva
// sin marcar sube los mismos bytes otra vez —es idempotente: misma clave, mismo
// contenido— en vez de apuntar a algo que otro proceso todavía está escribiendo.
package blobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Registro es la contabilidad de blobs compartidos (store.Store la implementa).
type Registro interface {
	ReservarBlobCompartido(ctx context.Context, hash, key string, size int64) (existe, listo bool, err error)
	MarcarBlobListo(ctx context.Context, hash string) error
	LiberarBlobCompartido(ctx context.Context, hash string) error
}

// Almacen es lo único que hace falta del objectstore: subir.
type Almacen interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
}

// Clave es la clave de un mensaje con ese hash en esa organización.
func Clave(t tenancy.Tenant, hash string) string {
	return objectstore.PrefijoCompartido + "mensajes/" + t.Slug + "/" + hash + ".eml"
}

// GuardarMensaje deja el mensaje en el almacén de la organización (si no estaba
// ya) y devuelve la clave que debe llevar la fila de `messages`.
func GuardarMensaje(ctx context.Context, reg Registro, alm Almacen, t tenancy.Tenant, raw []byte) (string, error) {
	return GuardarMensajeDesde(ctx, reg, alm, t, bytes.NewReader(raw), int64(len(raw)))
}

// GuardarMensajeDesde calcula el SHA-256 en flujo desde rs, reserva el blob compartido y
// sube el mensaje rebobinando rs sin cargar el mensaje entero en un []byte adicional.
func GuardarMensajeDesde(ctx context.Context, reg Registro, alm Almacen, t tenancy.Tenant, rs io.ReadSeeker, size int64) (string, error) {
	if t.Slug == "" {
		return "", fmt.Errorf("blobs: mensaje sin organización")
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("blobs: rebobinar para hash: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, rs); err != nil {
		return "", fmt.Errorf("blobs: leer para hash: %w", err)
	}
	hexHash := hex.EncodeToString(h.Sum(nil))
	// En `blob_refs` el «hash» identifica el blob: el de la organización.
	hash := t.Slug + "/" + hexHash
	key := Clave(t, hexHash)
	existe, listo, err := reg.ReservarBlobCompartido(ctx, hash, key, size)
	if err != nil {
		return "", fmt.Errorf("blobs: reservar: %w", err)
	}
	if existe && listo {
		return key, nil
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		if !existe {
			_ = reg.LiberarBlobCompartido(ctx, hash)
		}
		return "", fmt.Errorf("blobs: rebobinar para subida: %w", err)
	}
	// El objectstore no antepone el prefijo de la organización a `shared/`: la
	// organización ya va en la clave.
	err = alm.Put(ctx, t, key, rs, size,
		objectstore.PutOptions{ContentType: "message/rfc822", Comprimir: true})
	if err != nil {
		if !existe {
			_ = reg.LiberarBlobCompartido(ctx, hash)
		}
		return "", fmt.Errorf("blobs: subir: %w", err)
	}
	if !existe {
		// Si esto falla, la reserva queda sin marcar y el siguiente buzón con el
		// mismo mensaje vuelve a subirlo: se pierde trabajo, no datos.
		_ = reg.MarcarBlobListo(ctx, hash)
	}
	return key, nil
}
