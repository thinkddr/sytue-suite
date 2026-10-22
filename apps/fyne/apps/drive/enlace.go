// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # «Abrir en la aplicación» desde el explorador web
//
// La web manda la carpeta y el fichero por RUTA (`resourceHref` de `FileBrowser.tsx`:
// `?path=<carpeta>&preview=<fichero>`), que es como navega ella; esta app navega por id.
// Se baja de la raíz nombre a nombre, se entra en la carpeta con sus migas de verdad y
// el fichero, si lo hay, se abre como si se hubiera pulsado: en su app, en la vista o
// bajándolo.

// DestinoDeEnlace es la carpeta y el fichero de un enlace a Drive, con o sin el prefijo
// `/drive` de la web. ok es false si el enlace no lleva ninguno de los dos.
func DestinoDeEnlace(u *url.URL) (carpeta, fichero string, ok bool) {
	if u == nil || u.Scheme != api.Esquema("drive") {
		return "", "", false
	}
	if r := strings.Trim(strings.TrimPrefix(u.Path, "/drive"), "/"); r != "" {
		return "", "", false
	}
	q := u.Query()
	carpeta, fichero = q.Get("path"), q.Get("preview")
	if carpeta == "" && fichero != "" {
		carpeta = path.Dir(fichero) // la web no manda `path` cuando es la raíz
	}
	return carpeta, fichero, carpeta != "" || fichero != ""
}

// seccionesDeLaWeb son las claves de `Section` de DriveSidebar.tsx.
var seccionesDeLaWeb = map[string]Seccion{
	"my": MiUnidad, "documents": Documentos, "team": UnidadesDeEquipo, "shared": CompartidoConmigo,
	"recent": Recientes, "favorites": Favoritos, "trash": Papelera,
}

// SeccionDeEnlace es la sección de un enlace `?section=`, el del aviso de «compartido
// contigo» (`hubnotify.EnlaceCompartidoConmigo`): quien recibe no ve la ruta del dueño.
func SeccionDeEnlace(u *url.URL) (Seccion, bool) {
	clave, _ := api.LeerSeccion(u)
	s, ok := seccionesDeLaWeb[clave]
	return s, ok
}

// IrASeccion entra en una sección, como pulsarla en el carril.
func (p *Pantalla) IrASeccion(s Seccion) { p.irA(s) }

// AbrirCompartido entra en «Compartido conmigo» y abre lo compartido con ese id en
// cuanto llega la lista (el aviso de «compartido contigo»). Si ya no está —se dejó de
// compartir—, se queda en la lista.
func (p *Pantalla) AbrirCompartido(id string) {
	p.pedidoCompartido = id
	p.irA(CompartidoConmigo)
}

// AtenderPeticion enseña, a quien compartió `id`, quién le pide editarlo, con «Dar
// acceso» y «Rechazar»: es a lo que lleva el aviso de una petición de edición.
func (p *Pantalla) AtenderPeticion(id string) {
	if p.svc == nil {
		return
	}
	peticion.Enganchar(&p.peticionDueno, p.svc.Peticiones(id), peticion.Opciones{
		PuedeConceder: true, Recomponer: p.Repintar, T: p.tr.T,
	})
}

// errNoEsta es que alguna carpeta del camino, o el fichero, no está.
var errNoEsta = errors.New("drive: el enlace apunta a algo que no está")

// Resolver baja de la raíz hasta `carpeta` y busca en ella `fichero` (vacío: ninguno).
// Devuelve las migas, sin la raíz, y el fichero. `raiz` y `hijos` son los de
// `api.Drive`; se pasan para que el guardián no necesite servicio.
//
// ponytail: una petición por nivel y por página. Un camino de diez carpetas son diez
// viajes; si se nota, el servicio puede resolver una ruta de una vez.
func Resolver(ctx context.Context, raiz func(context.Context) (api.Folder, error),
	hijos func(context.Context, string, string) (api.Children, error), carpeta, fichero string,
) ([]Elemento, *Elemento, error) {
	r, err := raiz(ctx)
	if err != nil {
		return nil, nil, err
	}
	var migas []Elemento
	id := r.ID
	for _, nombre := range strings.Split(strings.Trim(carpeta, "/"), "/") {
		if nombre == "" {
			continue
		}
		el, err := buscarHijo(ctx, hijos, id, nombre, true)
		if err != nil {
			return nil, nil, err
		}
		migas, id = append(migas, el), el.ID
	}
	if fichero == "" {
		return migas, nil, nil
	}
	el, err := buscarHijo(ctx, hijos, id, path.Base(fichero), false)
	if err != nil {
		return nil, nil, err
	}
	el.Ruta = fichero
	return migas, &el, nil
}

// buscarHijo pasa las páginas de una carpeta hasta dar con `nombre`.
func buscarHijo(ctx context.Context, hijos func(context.Context, string, string) (api.Children, error),
	carpeta, nombre string, esCarpeta bool,
) (Elemento, error) {
	cursor := ""
	for {
		c, err := hijos(ctx, carpeta, cursor)
		if err != nil {
			return Elemento{}, err
		}
		for _, el := range DeHijos(c) {
			if el.Carpeta == esCarpeta && el.Nombre == nombre {
				return el, nil
			}
		}
		if cursor = c.NextCursor; cursor == "" {
			return Elemento{}, errNoEsta
		}
	}
}

// IrAEnlace entra en `carpeta` y abre `fichero`, los de [DestinoDeEnlace]. Si algo del
// camino ya no está, llama a `noEsta` —la página «no encontrada» de la sesión— en vez de
// quedarse callada. Se llama en la rutina de la interfaz; lo que pide va aparte.
func (p *Pantalla) IrAEnlace(carpeta, fichero string, noEsta func()) {
	if !p.dentro || p.svc == nil {
		return // sin sesión no hay a qué entrar; la pantalla ya enseña cómo entrar
	}
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		migas, el, err := Resolver(ctx, p.svc.Root, p.svc.Children, carpeta, fichero)
		if err != nil {
			if errors.Is(err, errNoEsta) && noEsta != nil {
				enHilo(noEsta)
			}
			return
		}
		enHilo(func() {
			p.salirDeLaBusqueda()
			p.explor.Ruta = migas
			p.irA(MiUnidad)
			if el != nil {
				p.abrir(*el)
			}
		})
	}()
}
