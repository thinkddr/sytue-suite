// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package almacen es el bucket de Cast visto desde el servicio: envuelve
// platform/pkg/objectstore con la organización como parámetro (las claves son
// relativas a ella: `<video_id>/original`, `<video_id>/hls/…`) y traduce sus
// errores al dominio. El almacén elige el bucket por tenant (ADR 0038) y pone el
// prefijo del tenant a la clave.
package almacen

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Almacen implementa service.Objetos sobre el cliente S3 de la suite.
type Almacen struct {
	c *objectstore.Client
}

// New construye el almacén. Con cliente nil devuelve nil: el servicio lo trata
// como «sin almacén» y falla con claridad en lo que lo necesita.
func New(c *objectstore.Client) *Almacen {
	if c == nil {
		return nil
	}
	return &Almacen{c: c}
}

func tenantDe(slug string) tenancy.Tenant { return tenancy.Tenant{Slug: slug} }

// Put guarda un objeto de la organización.
func (a *Almacen) Put(ctx context.Context, tenant, key string, r io.Reader, size int64, contentType string) error {
	return a.c.Put(ctx, tenantDe(tenant), key, r, size, objectstore.PutOptions{ContentType: contentType})
}

// Get abre un objeto, entero o el rango pedido.
func (a *Almacen) Get(ctx context.Context, tenant, key, rango string) (service.Fichero, error) {
	o, err := a.c.GetRange(ctx, tenantDe(tenant), key, rango)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return service.Fichero{}, domain.ErrNoExiste
		}
		var rango *objectstore.RangoNoSatisfacible
		if errors.As(err, &rango) {
			return service.Fichero{}, &service.RangoNoSatisfacible{Total: rango.Total}
		}
		return service.Fichero{}, err
	}
	return service.Fichero{Cuerpo: o.Body, Tamano: o.Size, ContentType: o.ContentType, ContentRange: o.ContentRange, Modificado: o.LastModified}, nil
}

// DeletePrefix borra todo lo de un vídeo.
func (a *Almacen) DeletePrefix(ctx context.Context, tenant, prefix string) error {
	return a.c.DeletePrefix(ctx, tenantDe(tenant), prefix)
}

// Cliente expone el cliente de debajo para el transcodificador, que sube muchos
// ficheros y prefiere hablar con el almacén directamente.
func (a *Almacen) Cliente() *objectstore.Client { return a.c }
