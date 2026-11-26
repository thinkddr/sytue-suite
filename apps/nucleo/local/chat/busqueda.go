// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// cargados son los mensajes que una cuenta ha traído (al leer, al sondear, al mandar),
// para buscar en ellos cuando el proveedor no da búsqueda con los permisos de la persona
// (Teams sin Microsoft Search, Google Chat). Solo en memoria: lo que no se ha visto en
// esta sesión no se encuentra.
type cargados struct {
	mu  sync.Mutex
	por map[string]map[string]api.RoomsMessage // canal → id → mensaje
}

// maxCargadosCanal es cuántos mensajes se guardan por canal como mucho (los más nuevos).
const maxCargadosCanal = 1000

// apuntar guarda (o cambia) los mensajes; los avisos de Sytue Office no.
func (c *cargados) apuntar(ms ...api.RoomsMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.por == nil {
		c.por = map[string]map[string]api.RoomsMessage{}
	}
	for _, m := range ms {
		if m.ID == "" || m.AuthorSub == AutorSistema {
			continue
		}
		canal := c.por[m.ChannelID]
		if canal == nil {
			canal = map[string]api.RoomsMessage{}
			c.por[m.ChannelID] = canal
		}
		canal[m.ID] = m
		if len(canal) > maxCargadosCanal {
			viejo := ""
			for id, x := range canal {
				if viejo == "" || x.CreatedAt.Before(canal[viejo].CreatedAt) {
					viejo = id
				}
			}
			delete(canal, viejo)
		}
	}
}

// quitar olvida un mensaje borrado.
func (c *cargados) quitar(canal, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.por[canal], id)
}

// buscar son los mensajes con todas las palabras de q en el texto, el autor o el nombre
// de un adjunto (sin mayúsculas ni acentos, como en toda la suite), del más nuevo al más
// viejo y como mucho limite (cero: 50).
func (c *cargados) buscar(q string, limite int) []api.RoomsMessage {
	terminos := buscar.Terminos(q)
	if len(terminos) == 0 {
		return nil
	}
	c.mu.Lock()
	var out []api.RoomsMessage
	for _, canal := range c.por {
		for _, m := range canal {
			texto := m.Body + "\n" + m.AuthorName
			for _, a := range m.Attachments {
				texto += "\n" + a.Name
			}
			todos := true
			for _, t := range terminos {
				if !buscar.Contiene(texto, t) {
					todos = false
					break
				}
			}
			if todos {
				out = append(out, m)
			}
		}
	}
	c.mu.Unlock()
	slices.SortFunc(out, func(a, b api.RoomsMessage) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if limite <= 0 {
		limite = 50
	}
	if len(out) > limite {
		out = out[:limite]
	}
	return out
}
