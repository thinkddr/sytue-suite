// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// CuentaNotas es el identificador de la cuenta local, que está siempre.
const CuentaNotas = "notas"

// Notas es el proveedor local: canales de notas para uno mismo, sin red. Cada canal es
// un fichero JSON legible en `<carpeta>/mensajes/<canal>.json`, y la lista de canales va
// en `<carpeta>/canales.json` (R-CLARO); todo se escribe a temporal y se renombra.
type Notas struct {
	carpeta string
	rotulo  string
	Ahora   func() time.Time
	// Autor es el nombre con el que se firman las notas al enseñarlas (el del perfil);
	// no se guarda en cada mensaje, así que cambiarlo cambia también las viejas.
	Autor string

	mu       sync.Mutex
	canales  []api.RoomsChannel
	mensajes map[string][]api.RoomsMessage // por canal, en orden de lectura
	oyentes  map[int]func(api.RoomsEvent)
	sigOyen  int
}

var (
	_ Proveedor    = (*Notas)(nil)
	_ Editor       = (*Notas)(nil)
	_ Reaccionador = (*Notas)(nil)
	_ Hilos        = (*Notas)(nil)
	_ Creador      = (*Notas)(nil)
	_ Gestor       = (*Notas)(nil)
	_ Buscador     = (*Notas)(nil)
)

// AbrirNotas abre (o crea) las notas de carpeta. Si no hay ningún canal, crea uno con
// el nombre primero y el asunto asunto, para que la app no se abra vacía.
func AbrirNotas(carpeta, rotulo, primero, asunto string) (*Notas, error) {
	if err := os.MkdirAll(filepath.Join(carpeta, "mensajes"), 0o700); err != nil {
		return nil, fmt.Errorf("chat: no se pudo crear la carpeta de Rooms: %w", err)
	}
	n := &Notas{carpeta: carpeta, rotulo: rotulo, mensajes: map[string][]api.RoomsMessage{}, oyentes: map[int]func(api.RoomsEvent){}}
	if err := espacio.LeerJSON(n.rutaCanales(), &n.canales); err != nil {
		return nil, fmt.Errorf("chat: canales.json ilegible: %w", err)
	}
	if len(n.canales) == 0 && primero != "" {
		if _, err := n.CrearCanal(context.Background(), primero, asunto, true); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func (n *Notas) rutaCanales() string { return filepath.Join(n.carpeta, "canales.json") }

func (n *Notas) rutaMensajes(canal string) string {
	return filepath.Join(n.carpeta, "mensajes", canal+".json")
}

func (n *Notas) ahora() time.Time {
	if n.Ahora != nil {
		return n.Ahora()
	}
	return time.Now()
}

// Rotulo es el nombre de la cuenta.
func (n *Notas) Rotulo() string { return n.rotulo }

// cargar trae los mensajes del canal a memoria la primera vez. Con n.mu cogido.
func (n *Notas) cargar(canal string) ([]api.RoomsMessage, error) {
	if ms, ok := n.mensajes[canal]; ok {
		return ms, nil
	}
	if !slices.ContainsFunc(n.canales, func(c api.RoomsChannel) bool { return c.ID == canal }) {
		return nil, fmt.Errorf("chat: canal %q desconocido", canal)
	}
	var ms []api.RoomsMessage
	if err := espacio.LeerJSON(n.rutaMensajes(canal), &ms); err != nil {
		return nil, fmt.Errorf("chat: mensajes de %q ilegibles: %w", canal, err)
	}
	n.mensajes[canal] = ms
	return ms, nil
}

func (n *Notas) escribir(canal string, ms []api.RoomsMessage) error {
	n.mensajes[canal] = ms
	if ms == nil {
		ms = []api.RoomsMessage{}
	}
	return espacio.EscribirJSON(n.rutaMensajes(canal), ms)
}

func (n *Notas) avisar(tipo, canal string, m api.RoomsMessage) {
	raw, _ := json.Marshal(m)
	ev := api.RoomsEvent{Type: tipo, ChannelID: canal, Subject: Yo, Payload: raw}
	n.mu.Lock()
	fs := make([]func(api.RoomsEvent), 0, len(n.oyentes))
	for _, f := range n.oyentes {
		fs = append(fs, f)
	}
	n.mu.Unlock()
	for _, f := range fs {
		f(ev)
	}
}

// Canales son los no archivados. En las notas no hay nada pendiente: todo es propio.
func (n *Notas) Canales(context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []api.RoomsChannel
	for _, c := range n.canales {
		if !c.Archived {
			out = append(out, c)
		}
	}
	return out, nil, nil
}

// Archivados son los archivados.
func (n *Notas) Archivados(context.Context) ([]api.RoomsChannel, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []api.RoomsChannel
	for _, c := range n.canales {
		if c.Archived {
			out = append(out, c)
		}
	}
	return out, nil
}

// Mensajes es una página del canal, sin las respuestas de hilo.
func (n *Notas) Mensajes(_ context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	ms, err := n.cargar(canal)
	if err != nil {
		return nil, err
	}
	var out []api.RoomsMessage
	for _, m := range ms {
		if m.ParentID == nil && (antes.IsZero() || m.CreatedAt.Before(antes)) {
			out = append(out, m)
		}
	}
	if limite > 0 && len(out) > limite {
		out = out[len(out)-limite:]
	}
	out = slices.Clone(out)
	for i := range out {
		out[i].AuthorName = cmp.Or(out[i].AuthorName, n.Autor)
	}
	return out, nil
}

// Enviar apunta una nota; con raiz, en su hilo.
func (n *Notas) Enviar(_ context.Context, canal, raiz, texto string) (api.RoomsMessage, error) {
	n.mu.Lock()
	ms, err := n.cargar(canal)
	if err != nil {
		n.mu.Unlock()
		return api.RoomsMessage{}, err
	}
	ahora := n.ahora()
	m := api.RoomsMessage{ID: nuevoID(), ChannelID: canal, AuthorSub: Yo, Body: texto, CreatedAt: ahora}
	ms = slices.Clone(ms)
	if raiz != "" {
		i := slices.IndexFunc(ms, func(x api.RoomsMessage) bool { return x.ID == raiz })
		if i < 0 {
			n.mu.Unlock()
			return api.RoomsMessage{}, fmt.Errorf("chat: mensaje %q desconocido", raiz)
		}
		m.ParentID = &raiz
		ms[i].ReplyCount++
		ms[i].LastReplyAt = &ahora
		ms[i].ThreadParticipants = []api.RoomsThreadParticipant{{Sub: Yo}}
	}
	ms = append(ms, m)
	err = n.escribir(canal, ms)
	n.mu.Unlock()
	if err != nil {
		return api.RoomsMessage{}, err
	}
	n.avisar("message", canal, m)
	return m, nil
}

// cambiar aplica f al mensaje id del canal y lo guarda.
func (n *Notas) cambiar(canal, id string, f func(*api.RoomsMessage)) (api.RoomsMessage, error) {
	n.mu.Lock()
	ms, err := n.cargar(canal)
	if err != nil {
		n.mu.Unlock()
		return api.RoomsMessage{}, err
	}
	i := slices.IndexFunc(ms, func(x api.RoomsMessage) bool { return x.ID == id })
	if i < 0 {
		n.mu.Unlock()
		return api.RoomsMessage{}, fmt.Errorf("chat: mensaje %q desconocido", id)
	}
	ms = slices.Clone(ms)
	f(&ms[i])
	m := ms[i]
	err = n.escribir(canal, ms)
	n.mu.Unlock()
	return m, err
}

// Editar cambia el texto de una nota.
func (n *Notas) Editar(_ context.Context, canal, mensaje, texto string) (api.RoomsMessage, error) {
	ahora := n.ahora()
	m, err := n.cambiar(canal, mensaje, func(m *api.RoomsMessage) { m.Body, m.EditedAt = texto, &ahora })
	if err == nil {
		n.avisar("message_edited", canal, m)
	}
	return m, err
}

// Borrar quita una nota y, si es raíz, su hilo.
func (n *Notas) Borrar(_ context.Context, canal, mensaje string) error {
	n.mu.Lock()
	ms, err := n.cargar(canal)
	if err != nil {
		n.mu.Unlock()
		return err
	}
	var borrada api.RoomsMessage
	ms = slices.DeleteFunc(slices.Clone(ms), func(x api.RoomsMessage) bool {
		if x.ID == mensaje {
			borrada = x
			return true
		}
		return x.ParentID != nil && *x.ParentID == mensaje
	})
	if borrada.ParentID != nil {
		for i := range ms {
			if ms[i].ID == *borrada.ParentID {
				ms[i].ReplyCount = max(0, ms[i].ReplyCount-1)
			}
		}
	}
	err = n.escribir(canal, ms)
	n.mu.Unlock()
	if err == nil {
		n.avisar("message_deleted", canal, api.RoomsMessage{ID: mensaje, ChannelID: canal})
	}
	return err
}

// Reaccionar pone o quita una reacción propia.
func (n *Notas) Reaccionar(_ context.Context, canal, mensaje, emoji string, poner bool) error {
	m, err := n.cambiar(canal, mensaje, func(m *api.RoomsMessage) {
		i := slices.IndexFunc(m.Reactions, func(r api.RoomsReaction) bool { return r.Emoji == emoji })
		switch {
		case poner && i < 0:
			m.Reactions = append(m.Reactions, api.RoomsReaction{Emoji: emoji, Count: 1, Subs: []string{Yo}})
		case !poner && i >= 0:
			m.Reactions = slices.Delete(m.Reactions, i, i+1)
		}
	})
	if err == nil {
		n.avisar("reaction", canal, m)
	}
	return err
}

// Hilo es la raíz con sus respuestas.
func (n *Notas) Hilo(_ context.Context, canal, raiz string) (api.RoomsThread, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	ms, err := n.cargar(canal)
	if err != nil {
		return api.RoomsThread{}, err
	}
	var t api.RoomsThread
	encontrada := false
	for _, m := range ms {
		switch {
		case m.ID == raiz:
			t.Root, encontrada = m, true
		case m.ParentID != nil && *m.ParentID == raiz:
			t.Replies = append(t.Replies, m)
		}
	}
	if !encontrada {
		return api.RoomsThread{}, fmt.Errorf("chat: mensaje %q desconocido", raiz)
	}
	t.Following = true
	return t, nil
}

// MarcarLeido: todo es propio.
func (n *Notas) MarcarLeido(context.Context, string) error { return nil }

// Escuchar entrega lo que se escribe en las notas (desde esta misma app).
func (n *Notas) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	n.mu.Lock()
	k := n.sigOyen
	n.sigOyen++
	n.oyentes[k] = alLlegar
	n.mu.Unlock()
	<-ctx.Done()
	n.mu.Lock()
	delete(n.oyentes, k)
	n.mu.Unlock()
	return ctx.Err()
}

// CrearCanal crea un canal de notas.
func (n *Notas) CrearCanal(_ context.Context, nombre, asunto string, privado bool) (api.RoomsChannel, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return api.RoomsChannel{}, fmt.Errorf("chat: el canal necesita un nombre")
	}
	kind := "public"
	if privado {
		kind = "private"
	}
	c := api.RoomsChannel{
		ID: nuevoID(), Slug: slug(nombre), Name: nombre, Topic: asunto, Kind: kind,
		CreatedBy: Yo, CreatedAt: n.ahora(), NotifyLevel: "all",
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	cs := append(slices.Clone(n.canales), c)
	if err := espacio.EscribirJSON(n.rutaCanales(), cs); err != nil {
		return api.RoomsChannel{}, err
	}
	n.canales = cs
	n.mensajes[c.ID] = nil
	return c, nil
}

// CambiarCanal renombra, cambia el asunto o archiva.
func (n *Notas) CambiarCanal(_ context.Context, canal string, p api.RoomsChannelPatch) (api.RoomsChannel, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	i := slices.IndexFunc(n.canales, func(c api.RoomsChannel) bool { return c.ID == canal })
	if i < 0 {
		return api.RoomsChannel{}, fmt.Errorf("chat: canal %q desconocido", canal)
	}
	cs := slices.Clone(n.canales)
	c := &cs[i]
	if p.Name != nil {
		c.Name = cmp.Or(strings.TrimSpace(*p.Name), c.Name)
		c.Slug = slug(c.Name)
	}
	if p.Topic != nil {
		c.Topic = *p.Topic
	}
	if p.Purpose != nil {
		c.Purpose = *p.Purpose
	}
	if p.Archived != nil {
		c.Archived = *p.Archived
	}
	if err := espacio.EscribirJSON(n.rutaCanales(), cs); err != nil {
		return api.RoomsChannel{}, err
	}
	n.canales = cs
	return cs[i], nil
}

// BorrarCanal quita el canal y sus notas.
func (n *Notas) BorrarCanal(_ context.Context, canal string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	cs := slices.DeleteFunc(slices.Clone(n.canales), func(c api.RoomsChannel) bool { return c.ID == canal })
	if err := espacio.EscribirJSON(n.rutaCanales(), cs); err != nil {
		return err
	}
	n.canales = cs
	delete(n.mensajes, canal)
	if err := os.Remove(n.rutaMensajes(canal)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Buscar busca el texto, sin distinguir mayúsculas, en todas las notas.
func (n *Notas) Buscar(_ context.Context, q string, limite int) ([]api.RoomsMessage, error) {
	q = buscar.Plegar(strings.TrimSpace(q))
	if q == "" {
		return nil, nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []api.RoomsMessage
	for _, c := range n.canales {
		ms, err := n.cargar(c.ID)
		if err != nil {
			continue
		}
		for _, m := range ms {
			if strings.Contains(buscar.Plegar(m.Body), q) {
				out = append(out, m)
			}
		}
	}
	slices.SortFunc(out, func(x, y api.RoomsMessage) int { return y.CreatedAt.Compare(x.CreatedAt) })
	if limite > 0 && len(out) > limite {
		out = out[:limite]
	}
	return out, nil
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '_':
			b.WriteRune('-')
		case '|':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
