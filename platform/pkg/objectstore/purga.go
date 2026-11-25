// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// BORRADO DE TODO LO DE UN TENANT (ADR 0054, enmienda del 2026-11-25).
//
// Lo usa el trabajador de purga de cada servicio (platform/pkg/purgatenant)
// cuando Sytue ID ya ha borrado las filas de una organización que desaparece.
// Recorre los DOS almacenes del cliente —el del servicio y el remoto— porque un
// tenant puede tener bytes en los dos: lo anterior a la decisión del remoto se
// quedó en local (ADR 0038), y el espacio compartido vive siempre en local.
//
// Con un bucket versionado, borrar deja una marca de borrado y las versiones
// anteriores caducan por el ciclo de vida del bucket (docs/security.md §8.1),
// igual que las copias de seguridad.

// ErrPrefijoDePurga es un prefijo que no se puede barrer entero: vacío, fuera
// del espacio de un tenant o del compartido, o con saltos de directorio.
var ErrPrefijoDePurga = errors.New("objectstore: prefijo de purga no válido")

// BorrarTenant borra todo lo que cuelga del prefijo del tenant
// (`tenants/<slug>/`) en todos los almacenes del cliente.
func (c *Client) BorrarTenant(ctx context.Context, t tenancy.Tenant) (int, error) {
	if !tenancy.ValidSlug(t.Slug) {
		return 0, ErrPrefijoDePurga
	}
	return c.BorrarPrefijo(ctx, t.StoragePrefix())
}

// BorrarPrefijo borra todo lo que cuelga de `prefijo` —una clave COMPLETA, sin
// tenant que anteponer— en todos los almacenes del cliente. Solo admite
// prefijos acabados en `/` bajo `tenants/<slug>/` o bajo el espacio compartido
// con un segmento propio debajo (`shared/mensajes/<slug>/`): un prefijo corto
// por error barrería el bucket de todas las organizaciones.
func (c *Client) BorrarPrefijo(ctx context.Context, prefijo string) (int, error) {
	if c == nil || !prefijoDePurgaValido(prefijo) {
		return 0, ErrPrefijoDePurga
	}
	total := 0
	var errs []error
	for _, b := range c.almacenes() {
		n, err := borrarEn(ctx, b.mc, b.bucket, prefijo)
		total += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return total, errors.Join(errs...)
}

// BorrarClave borra UN objeto del espacio compartido por su clave completa (un
// blob deduplicado cuyo contador llegó a cero con la purga). Que no exista no es
// un error: la orden es idempotente.
func (c *Client) BorrarClave(ctx context.Context, clave string) error {
	if c == nil || !strings.HasPrefix(clave, PrefijoCompartido) || !claveLimpia(clave) || strings.HasSuffix(clave, "/") {
		return ErrPrefijoDePurga
	}
	// El espacio compartido vive siempre en el almacén del servicio (backend).
	if err := c.mc.RemoveObject(ctx, c.bucket, clave, minio.RemoveObjectOptions{}); err != nil && !isNotFound(err) {
		return fmt.Errorf("objectstore: borrar %q: %w", clave, err)
	}
	return nil
}

type almacen struct {
	mc     *minio.Client
	bucket string
}

// almacenes devuelve los almacenes distintos del cliente: el del servicio y, si
// es otro, el remoto.
func (c *Client) almacenes() []almacen {
	out := []almacen{{c.mc, c.bucket}}
	if c.remote != nil && (c.remote != c.mc || c.rbucket != c.bucket) {
		out = append(out, almacen{c.remote, c.rbucket})
	}
	return out
}

func borrarEn(ctx context.Context, mc *minio.Client, bucket, prefijo string) (int, error) {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	objetos := make(chan minio.ObjectInfo)
	var errListar error
	go func() {
		defer close(objetos)
		for o := range mc.ListObjects(work, bucket, minio.ListObjectsOptions{Prefix: prefijo, Recursive: true}) {
			if o.Err != nil {
				errListar = fmt.Errorf("objectstore: listar %q: %w", prefijo, o.Err)
				return
			}
			// Un proveedor que devolviera algo fuera del prefijo no se toca.
			if !strings.HasPrefix(o.Key, prefijo) {
				errListar = fmt.Errorf("objectstore: el listado de %q trajo %q", prefijo, o.Key)
				return
			}
			select {
			case objetos <- o:
			case <-work.Done():
				return
			}
		}
	}()
	borrados := 0
	var errs []error
	for r := range mc.RemoveObjectsWithResult(work, bucket, objetos, minio.RemoveObjectsOptions{}) {
		if r.Err != nil && !isNotFound(r.Err) {
			errs = append(errs, fmt.Errorf("objectstore: borrar %q: %w", r.ObjectName, r.Err))
			continue
		}
		borrados++
	}
	// El canal de borrados se cierra después de que el de objetos se cierre, así
	// que leer errListar aquí no compite con la goroutine.
	if errListar != nil {
		errs = append(errs, errListar)
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return borrados, errors.Join(errs...)
}

func prefijoDePurgaValido(prefijo string) bool {
	if !claveLimpia(prefijo) || !strings.HasSuffix(prefijo, "/") {
		return false
	}
	partes := strings.Split(strings.TrimSuffix(prefijo, "/"), "/")
	switch {
	case partes[0] == "tenants":
		return len(partes) >= 2 && tenancy.ValidSlug(partes[1])
	case partes[0]+"/" == PrefijoCompartido:
		// `shared/<espacio>/<slug>/` como mínimo: `shared/` o `shared/mensajes/`
		// son de todas las organizaciones.
		return len(partes) >= 3 && tenancy.ValidSlug(partes[2])
	default:
		return false
	}
}

func claveLimpia(k string) bool {
	if k == "" || len(k) > 1024 || !utf8.ValidString(k) || strings.ContainsAny(k, "\x00\r\n\\") || strings.HasPrefix(k, "/") {
		return false
	}
	limpio := path.Clean(k)
	return limpio == strings.TrimSuffix(k, "/") && !strings.Contains(k, "..")
}
