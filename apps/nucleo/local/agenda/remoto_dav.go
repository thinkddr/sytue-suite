// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"errors"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
)

// RemotoDAV es [Remoto] sobre CalDAV o CardDAV (`cuentas/dav`).
type RemotoDAV struct {
	Cliente *dav.Cliente
	Clase   dav.Clase
	// Casa es la de calendarios o la de libretas; vacía, se busca desde el principal.
	Casa string

	mu sync.Mutex
	// solo3: el servidor rechazó una vCard 4.0; lo que se sube va ya en 3.0.
	solo3 bool
	// plan es la planificación del principal, leída una vez (ocupacion.go).
	plan *dav.Planificacion
}

func errorDAV(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, dav.ErrConflicto):
		return errors.Join(ErrConflicto, err)
	case errors.Is(err, dav.ErrNoEsta):
		return errors.Join(ErrNoEsta, err)
	case errors.Is(err, dav.ErrFichaCaducada):
		return errors.Join(ErrFichaCaducada, err)
	case errors.Is(err, dav.ErrAcceso):
		return errors.Join(ErrAcceso, err)
	}
	return err
}

func (r *RemotoDAV) casa(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Casa != "" {
		return r.Casa, nil
	}
	_, cal, lib, err := r.Cliente.Principal(ctx)
	if err != nil {
		return "", errorDAV(err)
	}
	r.Casa = cal
	if r.Clase == dav.Libreta {
		r.Casa = lib
	}
	if r.Casa == "" {
		return "", errors.Join(ErrNoEsta, errors.New("agenda: el servidor no tiene casa de "+string(r.Clase)))
	}
	return r.Casa, nil
}

// Colecciones lista los calendarios o las libretas de la cuenta.
func (r *RemotoDAV) Colecciones(ctx context.Context) ([]Coleccion, error) {
	casa, err := r.casa(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := r.Cliente.Colecciones(ctx, casa, r.Clase)
	if err != nil {
		return nil, errorDAV(err)
	}
	out := make([]Coleccion, 0, len(cs))
	for _, c := range cs {
		col := Coleccion{
			ID: c.URL, Nombre: c.Nombre, Color: c.Color, SoloLectura: c.SoloLectura,
			Extra: map[string]string{"ctag": c.CTag, "sync": c.SyncToken},
		}
		if r.Clase == dav.Calendario && !c.SoloLectura {
			// Si el servidor no responde a OPTIONS, se da por que no manda él las
			// invitaciones: se mandan por correo.
			col.Planifica, _ = r.Cliente.Planifica(ctx, c.URL)
		}
		out = append(out, col)
	}
	return out, nil
}

func (r *RemotoDAV) coleccion(col Coleccion) dav.Coleccion {
	return dav.Coleccion{URL: col.ID, Clase: r.Clase, Nombre: col.Nombre, SoloLectura: col.SoloLectura, CTag: col.Extra["ctag"], SyncToken: col.Extra["sync"]}
}

// Cambios baja lo que cambió.
func (r *RemotoDAV) Cambios(ctx context.Context, col Coleccion, ficha string, etagDe func(string) (string, bool), desde time.Time) (Cambios, error) {
	ca, err := r.Cliente.Cambios(ctx, r.coleccion(col), ficha, etagDe, desde)
	if err != nil {
		return Cambios{}, errorDAV(err)
	}
	out := Cambios{Borrados: ca.Borrados, Completo: ca.Completo, Vistos: ca.Vistos, Ficha: ca.Ficha}
	for _, o := range ca.Puestos {
		out.Puestos = append(out.Puestos, Objeto{ID: o.URL, ETag: o.ETag, Datos: o.Datos})
	}
	for _, o := range ca.Fuera {
		out.Fuera = append(out.Fuera, Objeto{ID: o.URL, ETag: o.ETag})
	}
	return out, nil
}

// Leer trae un objeto.
func (r *RemotoDAV) Leer(ctx context.Context, _ Coleccion, id string) (Objeto, error) {
	o, err := r.Cliente.Leer(ctx, id)
	return Objeto{ID: o.URL, ETag: o.ETag, Datos: o.Datos}, errorDAV(err)
}

// IDNuevo es la URL de un objeto nuevo con ese UID.
func (r *RemotoDAV) IDNuevo(col Coleccion, uid string) string {
	return dav.URLNueva(col.ID, uid, r.Clase)
}

// Crear sube un objeto nuevo.
func (r *RemotoDAV) Crear(ctx context.Context, col Coleccion, uid string, datos []byte) (Objeto, error) {
	u := r.IDNuevo(col, uid)
	etag, err := r.poner(ctx, u, "", datos)
	if err != nil {
		return Objeto{}, errorDAV(err)
	}
	return r.conETag(ctx, u, etag)
}

// poner sube con dav.Poner. Lo de aquí es vCard 4.0; si la libreta no la quiere
// (ErrFormato), va en 3.0, y desde entonces todo lo de esta cuenta.
func (r *RemotoDAV) poner(ctx context.Context, u, etag string, datos []byte) (string, error) {
	if r.Clase != dav.Libreta {
		return r.Cliente.Poner(ctx, u, etag, datos, r.Clase)
	}
	r.mu.Lock()
	solo3 := r.solo3
	r.mu.Unlock()
	if !solo3 {
		e, err := r.Cliente.Poner(ctx, u, etag, datos, r.Clase)
		if !errors.Is(err, dav.ErrFormato) {
			return e, err
		}
	}
	v3, err := aVCard3(datos)
	if err != nil {
		return "", err
	}
	e, err := r.Cliente.Poner(ctx, u, etag, v3, r.Clase)
	if err == nil && !solo3 {
		r.mu.Lock()
		r.solo3 = true
		r.mu.Unlock()
	}
	return e, err
}

// Cambiar reemplaza un objeto que sigue en obj.ETag.
func (r *RemotoDAV) Cambiar(ctx context.Context, _ Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	if obj.ETag == "" {
		// No se sabe en qué versión está: que lo resuelva el conflicto.
		return Objeto{}, ErrConflicto
	}
	etag, err := r.poner(ctx, obj.ID, obj.ETag, datos)
	if err != nil {
		return Objeto{}, errorDAV(err)
	}
	return r.conETag(ctx, obj.ID, etag)
}

// conETag: si el servidor retocó lo subido y no dio ETag, se lee la que tiene ahora
// (lo que guardó es su versión de lo nuestro; no hay nada que no se haya visto).
func (r *RemotoDAV) conETag(ctx context.Context, u, etag string) (Objeto, error) {
	if etag != "" {
		return Objeto{ID: u, ETag: etag}, nil
	}
	o, err := r.Cliente.Leer(ctx, u)
	if err != nil {
		return Objeto{ID: u}, nil //nolint:nilerr // lo subido ya está; sin ETag, la vuelta siguiente lo lee.
	}
	return Objeto{ID: u, ETag: o.ETag}, nil
}

// Borrar quita un objeto.
func (r *RemotoDAV) Borrar(ctx context.Context, _ Coleccion, id, etag string) error {
	return errorDAV(r.Cliente.Borrar(ctx, id, etag))
}

// CreaColecciones, si el remoto lo implementa, sabe crear un calendario o una libreta en
// la cuenta.
type CreaColecciones interface {
	CrearColeccion(ctx context.Context, nombre, color string) (Coleccion, error)
}

// CrearColeccion crea en la cuenta un calendario con MKCALENDAR o una libreta con el
// MKCOL extendido (las libretas no llevan color).
func (r *RemotoDAV) CrearColeccion(ctx context.Context, nombre, color string) (Coleccion, error) {
	casa, err := r.casa(ctx)
	if err != nil {
		return Coleccion{}, err
	}
	var u string
	if r.Clase == dav.Libreta {
		u, err = r.Cliente.CrearLibreta(ctx, casa, nombre)
		color = ""
	} else {
		u, err = r.Cliente.CrearCalendario(ctx, casa, nombre, color)
	}
	if err != nil {
		return Coleccion{}, errorDAV(err)
	}
	return Coleccion{ID: u, Nombre: nombre, Color: color}, nil
}
