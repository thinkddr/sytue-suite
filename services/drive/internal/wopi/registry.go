// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrNoEditorURL indica que no se dio dirección que comprobar.
var ErrNoEditorURL = errors.New("wopi: falta la dirección del editor")

// UN EDITOR POR ORGANIZACIÓN.
//
// El catálogo (`Discovery`) sabe hablar con UN editor. Esto guarda uno por organización,
// porque quién edita los documentos de una empresa lo decide esa empresa: antes salía de
// una variable de entorno del servicio y en una instalación con varias organizaciones
// todas compartían editor lo quisieran o no.
//
// La caché es por URL y no por organización: dos empresas que apunten al mismo Collabora
// —lo normal en una instalación propia— comparten el catálogo descargado en vez de pedir
// el mismo XML dos veces.

// EditorSource resuelve qué editor tiene configurado una organización. Lo satisface el
// store; se declara aquí para que este paquete no dependa de él.
type EditorSource func(ctx context.Context, tenant string) (url string, ok bool)

// Registry entrega el catálogo del editor de cada organización.
type Registry struct {
	lookup EditorSource
	ttl    time.Duration
	// confiable es la dirección del despliegue (ver Confiar).
	confiable string

	mu    sync.Mutex
	byURL map[string]*Discovery
}

// NewRegistry construye el registro. `lookup` nil ⇒ ninguna organización tiene editor y
// todo lo que consulte recibe «no hay», que es lo correcto en un despliegue sin ninguno.
func NewRegistry(lookup EditorSource, ttl time.Duration) *Registry {
	return &Registry{lookup: lookup, ttl: ttl, byURL: map[string]*Discovery{}}
}

// For devuelve el catálogo del editor de una organización, o nil si no tiene.
//
// Devolver nil y no un error es deliberado: `Discovery` trata el nil como «no hay
// editor» en todos sus métodos, así que quien llama no necesita ramificar.
func (r *Registry) For(ctx context.Context, tenant string) *Discovery {
	if r == nil || r.lookup == nil || tenant == "" {
		return nil
	}
	url, ok := r.lookup(ctx, tenant)
	if !ok || url == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, hit := r.byURL[url]; hit {
		return d
	}
	nuevo := NewDiscovery
	if url == r.confiable {
		nuevo = NewDiscoveryDeConfianza
	}
	d := nuevo(url, r.ttl)
	if d == nil {
		return nil
	}
	// Cota: una instalación tiene un puñado de editores distintos, no miles. Si el mapa
	// creciera sin freno sería porque cada organización apunta a uno propio, y entonces
	// el límite razonable es el número de organizaciones.
	r.byURL[url] = d
	return d
}

// Confiar marca la dirección que configura quien DESPLIEGA (`WOPI_DISCOVERY_URL`):
// es la única que puede ser interna o ir sin TLS. Cualquier otra la ha escrito una
// organización y pasa por la barrera SSRF. Se llama una vez, al montar el router.
func (r *Registry) Confiar(url string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.confiable = strings.TrimSpace(url)
	r.mu.Unlock()
}

// Forget descarta el catálogo cacheado de una URL. Se llama al reconfigurar un editor:
// sin esto, cambiar la dirección dejaría sirviendo el catálogo viejo hasta que caducara.
func (r *Registry) Forget(url string) {
	if r == nil || url == "" {
		return
	}
	r.mu.Lock()
	delete(r.byURL, url)
	r.mu.Unlock()
}

// Probe descarga el catálogo de una URL SIN cachearlo y devuelve lo que declara.
//
// Es lo que hace el botón «comprobar» de la consola: quien conecta un editor tiene que
// ver en el acto si responde y qué tipos de fichero abre, en vez de guardar a ciegas y
// descubrir el fallo cuando alguien intente abrir un documento.
func Probe(ctx context.Context, url string, ttl time.Duration) ([]string, error) {
	d := NewDiscovery(url, ttl)
	if d == nil {
		return nil, ErrNoEditorURL
	}
	if err := d.Refresh(ctx); err != nil {
		return nil, err
	}
	return d.Extensions(ctx), nil
}
