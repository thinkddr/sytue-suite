// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # La presentación del servicio: abrir y guardar
//
// Es `useDocument.ts` recortado a lo que hace una presentación: leer la ficha y el
// registro `doc:slides`, y escribirlo entero con un `set` LWW 1,5 s después del último
// cambio (AUTOSAVE_MS). La llamada es de `apps/nucleo/api` (`Slides`), que ya comprueba
// antes de escribir que nadie haya escrito entretanto.

// AutoguardadoMS es AUTOSAVE_MS: cuánto se espera tras el último cambio para guardar.
const AutoguardadoMS = editor.AutoguardadoMS

// Documento es una presentación abierta del servicio.
type Documento struct {
	svc   *api.Slides
	id    string
	actor string

	mu       sync.Mutex
	base     api.SlidesContent
	contador uint64
}

// Apertura es lo que trae abrir una presentación: su ficha, si se puede escribir y el
// contenido tal cual.
type Apertura struct {
	Doc      *Documento
	Titulo   string
	Escribir bool
	// Rol es el papel de quien abre (viewer, commenter, editor u owner): la franja de
	// solo lectura ofrece pedir acceso o, al dueño, concederlo.
	Rol       string
	Contenido string
}

// Abrir lee la ficha y el contenido. El ROL decide si se puede escribir: «viewer» y
// «commenter» abren en solo lectura, como en la web.
func Abrir(ctx context.Context, svc *api.Slides, id string) (Apertura, error) {
	ficha, rol, err := svc.Get(ctx, id)
	if err != nil {
		return Apertura{}, err
	}
	c, err := svc.Content(ctx, id)
	if err != nil {
		return Apertura{}, err
	}
	d := &Documento{svc: svc, id: id, actor: uuid(), base: c}
	return Apertura{Doc: d, Titulo: ficha.Title, Escribir: api.SlidesPuedeEscribir(rol), Rol: rol, Contenido: c.Modelo}, nil
}

// Guardar escribe el modelo entero encima de lo último que se leyó o se escribió.
func (d *Documento) Guardar(ctx context.Context, modelo string) error {
	d.mu.Lock()
	base := d.base
	d.contador++
	n := d.contador
	d.mu.Unlock()
	nuevo, err := d.svc.Save(ctx, d.id, base, d.actor, n, modelo)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.base = nuevo
	d.mu.Unlock()
	return nil
}

// Renombrar cambia el título.
func (d *Documento) Renombrar(ctx context.Context, titulo string) error {
	return d.svc.Rename(ctx, d.id, titulo)
}

// EstadoDe es la pantalla que toca cuando abrir falla: sin red, «sin conexión» —si no,
// parecería que el servicio está caído—; un 403, «sin permiso» —no hay nada que
// reintentar—; lo demás, «no se pudo abrir».
func EstadoDe(err error) editor.Estado {
	var e *api.Error
	var red net.Error
	var ue *url.Error
	switch {
	case err == nil:
		return editor.Listo
	case errors.As(err, &e) && (e.Status == http.StatusForbidden || e.Status == http.StatusUnauthorized):
		return editor.SinPermiso
	case errors.As(err, &e):
		return editor.Error
	case errors.As(err, &red), errors.As(err, &ue):
		return editor.SinConexion
	}
	return editor.Error
}

// IDDeEnlace saca el identificador de un enlace de edición: el esquema propio
// (`sytue-slides:/edit?id=…`) y la ruta de la web con su prefijo
// (`sytue-slides:/diapos/edit?id=…`, que es lo que arma `api.AbrirURL` con la dirección
// de la web). Vacío si el enlace no es de editar.
func IDDeEnlace(u *url.URL) string { return api.IDDeEdicion("slides", u) }

// Autoguardado es el del cromo compartido (`editor.Autoguardado`), que nació aquí.
type Autoguardado = editor.Autoguardado
