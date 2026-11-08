// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package logo

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// Cache recuerda el logo de cada dominio en la memoria de la réplica. El logo de un
// dominio es el mismo para todo el mundo y cambia muy de tarde en tarde. Los fallos
// también se recuerdan —una web sin icono no debe recibir una petición por cada fila
// de cada bandeja—, pero menos tiempo.
//
// Dos peticiones del mismo dominio que llegan a la vez comparten UNA búsqueda.
type Cache struct {
	// Buscar es la búsqueda de un dominio que no está; Ahora, el reloj. Los dos se
	// sustituyen en los tests.
	Buscar func(ctx context.Context, dominio string) ([]byte, error)
	Ahora  func() time.Time

	mu      sync.Mutex
	lru     *list.List // de *entrada; el frente es el usado más recientemente
	indice  map[string]*list.Element
	enCurso map[string]*espera
	bytes   int
}

const (
	ttlAcierto  = 7 * 24 * time.Hour
	ttlFallo    = 24 * time.Hour
	maxEntradas = 5_000
	maxBytes    = 32 << 20
)

type entrada struct {
	dominio string
	png     []byte // nil: no hay logo
	caduca  time.Time
}

// entradaDe es el valor de un elemento de la lista, que siempre es una *entrada.
func entradaDe(el *list.Element) *entrada {
	e, _ := el.Value.(*entrada)
	return e
}

type espera struct {
	hecho chan struct{}
	png   []byte
}

// NuevaCache crea la caché de producción, que busca con TraerSeguro.
func NuevaCache() *Cache {
	return &Cache{
		Buscar: func(ctx context.Context, d string) ([]byte, error) { return Buscar(ctx, d, TraerSeguro) },
		Ahora:  time.Now,
	}
}

// Logo devuelve el PNG del logo de `dominio` (ya validado con Valido), o nil si no
// tiene uno aprovechable.
func (c *Cache) Logo(ctx context.Context, dominio string) []byte {
	c.mu.Lock()
	if c.lru == nil {
		c.lru, c.indice, c.enCurso = list.New(), map[string]*list.Element{}, map[string]*espera{}
	}
	if el, ok := c.indice[dominio]; ok {
		e := entradaDe(el)
		if c.Ahora().Before(e.caduca) {
			c.lru.MoveToFront(el)
			c.mu.Unlock()
			return e.png
		}
	}
	if w, ok := c.enCurso[dominio]; ok {
		c.mu.Unlock()
		select {
		case <-w.hecho:
			return w.png
		case <-ctx.Done():
			return nil
		}
	}
	w := &espera{hecho: make(chan struct{})}
	c.enCurso[dominio] = w
	c.mu.Unlock()

	// La búsqueda no muere con la petición que la lanzó: otras pueden estar
	// esperando el mismo dominio, y su resultado se guarda para las siguientes.
	png, err := c.Buscar(context.WithoutCancel(ctx), dominio)
	if err != nil {
		png = nil
	}

	c.mu.Lock()
	c.guardar(dominio, png)
	delete(c.enCurso, dominio)
	w.png = png
	close(w.hecho)
	c.mu.Unlock()
	return png
}

// guardar mete la entrada al frente y saca por detrás lo que pase de los topes.
func (c *Cache) guardar(dominio string, png []byte) {
	if el, ok := c.indice[dominio]; ok {
		c.bytes -= len(entradaDe(el).png)
		c.lru.Remove(el)
	}
	ttl := ttlFallo
	if png != nil {
		ttl = ttlAcierto
	}
	c.indice[dominio] = c.lru.PushFront(&entrada{dominio: dominio, png: png, caduca: c.Ahora().Add(ttl)})
	c.bytes += len(png)
	for c.lru.Len() > maxEntradas || c.bytes > maxBytes {
		ultimo := c.lru.Back()
		e := entradaDe(ultimo)
		c.bytes -= len(e.png)
		delete(c.indice, e.dominio)
		c.lru.Remove(ultimo)
	}
}
