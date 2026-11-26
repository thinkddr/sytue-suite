// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Equipo es el único equipo que ve la pantalla: todas las cuentas cuelgan de él, y cada
// una es una categoría del carril cuando hay más de una.
const Equipo = "office"

// sep separa la cuenta del identificador propio del proveedor. Ni los de Notas ni los de
// Matrix (`!sala:servidor`, `$evento`) lo llevan.
const sep = "|"

// PrefijoCategoria es el de las categorías que son una cuenta.
const PrefijoCategoria = "cuenta:"

// ErrCuentaDesconocida: el identificador no es de ninguna cuenta abierta.
var ErrCuentaDesconocida = errors.New("chat: cuenta desconocida")

// Agregador junta los proveedores tras la interfaz de la pantalla de Rooms. Lo que es de
// quien mira y no de la cuenta (favoritos, categorías, borradores, guardados, fijados,
// avisos) lo guarda él, en claro, en `preferencias.json`.
type Agregador struct {
	mu     sync.Mutex
	orden  []string
	provs  map[string]Proveedor
	ruta   string
	prefs  preferencias
	Ahora  func() time.Time
	Equipo string // el nombre del equipo; vacío, «Sytue Office»

	// CarpetaSubidas es donde esperan los adjuntos elegidos hasta que se manda el
	// mensaje; vacía, una temporal del sistema.
	CarpetaSubidas string
	// GuardarEn es la carpeta de «Guardar en Drive» (la de Drive de la edición); vacía,
	// no se guarda.
	GuardarEn string
	// RotuloCuenta es el rótulo del selector de cuenta de «Crear canal».
	RotuloCuenta string
	// AlRecuperar avisa de que una cuenta cuya escucha se cayó por la red vuelve a
	// contestar: la pantalla vuelve a pedir los canales (si cayó al abrir, los suyos no
	// están). nil: nadie lo escucha.
	AlRecuperar func(cuenta string)
	// AlCambiarCanales avisa de que una cuenta tiene conversaciones que no estaban (un
	// chat de Teams o un espacio de Google Chat abierto en otro sitio): la pantalla
	// vuelve a pedir la lista sin recargar. nil: nadie lo escucha.
	AlCambiarCanales func(cuenta string)

	subidas map[string]subida
	nombres map[string]string // adjunto → nombre, de los mensajes vistos
}

type preferencias struct {
	Favoritos  map[string]bool               `json:"favoritos,omitempty"`
	Niveles    map[string]string             `json:"niveles,omitempty"`
	Categoria  map[string]string             `json:"categoria,omitempty"`
	Categorias []api.RoomsCategory           `json:"categorias,omitempty"`
	Plegadas   map[string]bool               `json:"plegadas,omitempty"`
	Ocultos    map[string]bool               `json:"ocultos,omitempty"`
	Borradores []api.RoomsDraft              `json:"borradores,omitempty"`
	Guardados  []api.RoomsMessage            `json:"guardados,omitempty"`
	Fijados    map[string][]api.RoomsMessage `json:"fijados,omitempty"`
	Avisos     api.RoomsNotifyPrefs          `json:"avisos"`
	// Seguidos son los hilos que se siguen (true) o se dejaron de seguir (false) a mano.
	Seguidos map[string]bool `json:"seguidos,omitempty"`
}

// NuevoAgregador abre las preferencias de ruta (un fichero JSON; vacío, solo en memoria).
func NuevoAgregador(ruta string) *Agregador {
	a := &Agregador{provs: map[string]Proveedor{}, ruta: ruta}
	if ruta != "" {
		if err := espacio.LeerJSON(ruta, &a.prefs); err != nil {
			slog.Warn("chat: preferencias ilegibles; se empieza de cero", "error", err)
		}
	}
	return a
}

// Poner añade (o sustituye) la cuenta id con su proveedor.
func (a *Agregador) Poner(id string, p Proveedor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.provs[id]; !ok {
		a.orden = append(a.orden, id)
	}
	a.provs[id] = p
}

// Quitar cierra la cuenta id (sus datos, si los tiene, los borra quien la dio de alta).
func (a *Agregador) Quitar(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.provs, id)
	a.orden = slices.DeleteFunc(a.orden, func(s string) bool { return s == id })
}

// Cuentas son las abiertas, en el orden en que se pusieron.
func (a *Agregador) Cuentas() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.orden)
}

func (a *Agregador) ahora() time.Time {
	if a.Ahora != nil {
		return a.Ahora()
	}
	return time.Now()
}

// ----- Identificadores -----

// IDCanal es el identificador que ve la pantalla de un canal (canal, el del proveedor)
// de la cuenta.
func IDCanal(cuenta, canal string) string { return cuenta + sep + canal }

// IDMensaje es el del mensaje.
func IDMensaje(cuenta, canal, mensaje string) string { return cuenta + sep + canal + sep + mensaje }

func partirCanal(id string) (cuenta, canal string, ok bool) {
	return strings.Cut(id, sep)
}

func partirMensaje(id string) (cuenta, canal, mensaje string, ok bool) {
	p := strings.SplitN(id, sep, 3)
	if len(p) != 3 {
		return "", "", "", false
	}
	return p[0], p[1], p[2], true
}

func (a *Agregador) prov(cuenta string) (Proveedor, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p, ok := a.provs[cuenta]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrCuentaDesconocida, cuenta)
}

func (a *Agregador) deCanal(id string) (Proveedor, string, string, error) {
	cuenta, canal, ok := partirCanal(id)
	if !ok {
		return nil, "", "", fmt.Errorf("%w: %q", ErrCuentaDesconocida, id)
	}
	p, err := a.prov(cuenta)
	return p, cuenta, canal, err
}

func (a *Agregador) deMensaje(id string) (Proveedor, string, string, string, error) {
	cuenta, canal, msg, ok := partirMensaje(id)
	if !ok {
		return nil, "", "", "", fmt.Errorf("%w: %q", ErrCuentaDesconocida, id)
	}
	p, err := a.prov(cuenta)
	return p, cuenta, canal, msg, err
}

func prefijarCanal(cuenta string, c api.RoomsChannel) api.RoomsChannel {
	c.ID = IDCanal(cuenta, c.ID)
	c.TeamID = Equipo
	return c
}

func prefijarMensaje(cuenta string, m api.RoomsMessage) api.RoomsMessage {
	canal := m.ChannelID
	m.ID = IDMensaje(cuenta, canal, m.ID)
	m.ChannelID = IDCanal(cuenta, canal)
	if m.ParentID != nil {
		p := IDMensaje(cuenta, canal, *m.ParentID)
		m.ParentID = &p
	}
	return m
}

func prefijarMensajes(cuenta string, ms []api.RoomsMessage) []api.RoomsMessage {
	out := make([]api.RoomsMessage, len(ms))
	for i, m := range ms {
		out[i] = prefijarMensaje(cuenta, m)
	}
	return out
}

func (a *Agregador) guardar() {
	if a.ruta == "" {
		return
	}
	if err := espacio.EscribirJSON(a.ruta, a.prefs); err != nil {
		slog.Warn("chat: no se pudieron guardar las preferencias", "error", err)
	}
}

func nuevoID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ----- Equipos y canales -----

// Teams es el equipo único.
func (a *Agregador) Teams(context.Context) ([]api.RoomsTeam, error) {
	return []api.RoomsTeam{{ID: Equipo, Slug: Equipo, Name: cmp.Or(a.Equipo, espacio.Nombre), Role: "member"}}, nil
}

// todos recorre las cuentas en orden; una que falla no tumba a las demás.
func (a *Agregador) todos(f func(cuenta string, p Proveedor) error) error {
	var errs []error
	for _, cuenta := range a.Cuentas() {
		p, err := a.prov(cuenta)
		if err != nil {
			continue
		}
		if err := f(cuenta, p); err != nil {
			slog.Warn("chat: una cuenta no contesta", "cuenta", cuenta, "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (a *Agregador) canalesYPendientes(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	var cs []api.RoomsChannel
	var us []api.RoomsUnread
	varias := len(a.Cuentas()) > 1
	err := a.todos(func(cuenta string, p Proveedor) error {
		c, u, err := p.Canales(ctx)
		if err != nil {
			return err
		}
		for _, x := range c {
			x = prefijarCanal(cuenta, x)
			if varias {
				x.CategoryID = PrefijoCategoria + cuenta
			}
			cs = append(cs, x)
		}
		for _, x := range u {
			x.ChannelID = IDCanal(cuenta, x.ChannelID)
			us = append(us, x)
		}
		return nil
	})
	if len(cs) == 0 && err != nil {
		return nil, nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cs = slices.DeleteFunc(cs, func(c api.RoomsChannel) bool { return a.prefs.Ocultos[c.ID] })
	for i := range cs {
		c := &cs[i]
		c.Favorite = a.prefs.Favoritos[c.ID]
		if n := a.prefs.Niveles[c.ID]; n != "" {
			c.NotifyLevel = n
			c.Muted = n == "none"
		}
		if k, ok := a.prefs.Categoria[c.ID]; ok {
			c.CategoryID = k
		}
	}
	return cs, us, nil
}

// Channels son los de todas las cuentas.
func (a *Agregador) Channels(ctx context.Context, _ string) ([]api.RoomsChannel, error) {
	cs, _, err := a.canalesYPendientes(ctx)
	return cs, err
}

// Unread es lo pendiente de todas las cuentas.
func (a *Agregador) Unread(ctx context.Context, _ string) ([]api.RoomsUnread, error) {
	_, us, err := a.canalesYPendientes(ctx)
	return us, err
}

// ExternalChannels: en local no hay conversaciones de otra organización.
func (a *Agregador) ExternalChannels(context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	return nil, nil, nil
}

// Categories son una por cuenta (con más de una) y las que creó la persona.
func (a *Agregador) Categories(context.Context) ([]api.RoomsCategory, error) {
	cuentas := a.Cuentas()
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []api.RoomsCategory
	if len(cuentas) > 1 {
		for i, cuenta := range cuentas {
			p := a.provs[cuenta]
			if p == nil {
				continue
			}
			id := PrefijoCategoria + cuenta
			out = append(out, api.RoomsCategory{ID: id, Name: p.Rotulo(), SortOrder: i, Collapsed: a.prefs.Plegadas[id]})
		}
	}
	for _, c := range a.prefs.Categorias {
		c.SortOrder += len(out)
		out = append(out, c)
	}
	return out, nil
}

// ----- Mensajes -----

// Messages es una página del canal.
func (a *Agregador) Messages(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	p, cuenta, c, err := a.deCanal(canal)
	if err != nil {
		return nil, err
	}
	ms, err := p.Mensajes(ctx, c, antes, limite)
	ms = prefijarMensajes(cuenta, ms)
	a.recordarAdjuntos(ms)
	return ms, err
}

// Post publica texto.
func (a *Agregador) Post(ctx context.Context, canal, texto string) (api.RoomsMessage, error) {
	return a.PostWith(ctx, canal, texto, api.RoomsSend{})
}

// Schedule: sin servidor que lo mande a su hora, no hay envío programado.
func (a *Agregador) Schedule(context.Context, string, string, []string, time.Time) (api.RoomsScheduled, error) {
	return api.RoomsScheduled{}, ErrNoDisponible
}

// Reschedule no existe.
func (a *Agregador) Reschedule(context.Context, string, time.Time) (api.RoomsScheduled, error) {
	return api.RoomsScheduled{}, ErrNoDisponible
}

// Scheduled no hay.
func (a *Agregador) Scheduled(context.Context) ([]api.RoomsScheduled, error) { return nil, nil }

// CancelScheduled no hay nada que cancelar.
func (a *Agregador) CancelScheduled(context.Context, string) error { return ErrNoDisponible }

// Ack: el acuse de lectura es de Sytue Rooms.
func (a *Agregador) Ack(context.Context, string) (int, error) { return 0, ErrNoDisponible }

// Readers no se saben.
func (a *Agregador) Readers(context.Context, string) ([]api.RoomsReader, int, error) {
	return nil, 0, nil
}

// VoteCard: no hay tarjetas.
func (a *Agregador) VoteCard(context.Context, string, string, bool) ([]api.RoomsCardVote, error) {
	return nil, ErrNoDisponible
}

// Edit cambia el texto de un mensaje.
func (a *Agregador) Edit(ctx context.Context, mensaje, texto string) (api.RoomsMessage, error) {
	p, cuenta, c, m, err := a.deMensaje(mensaje)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	e, ok := p.(Editor)
	if !ok {
		return api.RoomsMessage{}, ErrNoDisponible
	}
	out, err := e.Editar(ctx, c, m, texto)
	return prefijarMensaje(cuenta, out), err
}

// Delete borra un mensaje.
func (a *Agregador) Delete(ctx context.Context, mensaje string) error {
	p, _, c, m, err := a.deMensaje(mensaje)
	if err != nil {
		return err
	}
	e, ok := p.(Editor)
	if !ok {
		return ErrNoDisponible
	}
	return e.Borrar(ctx, c, m)
}

// React pone o quita una reacción.
func (a *Agregador) React(ctx context.Context, mensaje, emoji string, poner bool) error {
	p, _, c, m, err := a.deMensaje(mensaje)
	if err != nil {
		return err
	}
	r, ok := p.(Reaccionador)
	if !ok {
		return ErrNoDisponible
	}
	return r.Reaccionar(ctx, c, m, emoji, poner)
}

// MarkRead da el canal por leído.
func (a *Agregador) MarkRead(ctx context.Context, canal string) error {
	p, _, c, err := a.deCanal(canal)
	if err != nil {
		return err
	}
	return p.MarcarLeido(ctx, c)
}

// MarkUnread: se acepta sin más (el contador lo enseña la pantalla).
func (a *Agregador) MarkUnread(context.Context, string) error { return nil }

// Thread es el hilo de raiz.
func (a *Agregador) Thread(ctx context.Context, raiz string) (api.RoomsThread, error) {
	p, cuenta, c, r, err := a.deMensaje(raiz)
	if err != nil {
		return api.RoomsThread{}, err
	}
	h, ok := p.(Hilos)
	if !ok {
		return api.RoomsThread{}, ErrNoDisponible
	}
	t, err := h.Hilo(ctx, c, r)
	if err != nil {
		return api.RoomsThread{}, err
	}
	t.Root = prefijarMensaje(cuenta, t.Root)
	t.Replies = prefijarMensajes(cuenta, t.Replies)
	a.recordarAdjuntos(append([]api.RoomsMessage{t.Root}, t.Replies...))
	a.mu.Lock()
	if v, ok := a.prefs.Seguidos[t.Root.ID]; ok {
		t.Following = v
	}
	a.mu.Unlock()
	return t, nil
}

// Around es la última página del canal (los proveedores no saben centrar en un mensaje).
func (a *Agregador) Around(ctx context.Context, canal, mensaje string, limite int) ([]api.RoomsMessage, string, error) {
	ms, err := a.Messages(ctx, canal, time.Time{}, limite)
	raiz := ""
	for _, m := range ms {
		if m.ID == mensaje && m.ParentID != nil {
			raiz = *m.ParentID
		}
	}
	return ms, raiz, err
}

// Search busca en las cuentas que saben buscar.
func (a *Agregador) Search(ctx context.Context, q string, limite int) ([]api.RoomsMessage, error) {
	var out []api.RoomsMessage
	err := a.todos(func(cuenta string, p Proveedor) error {
		b, ok := p.(Buscador)
		if !ok {
			return nil
		}
		ms, err := b.Buscar(ctx, q, limite)
		out = append(out, prefijarMensajes(cuenta, ms)...)
		return err
	})
	slices.SortFunc(out, func(x, y api.RoomsMessage) int { return y.CreatedAt.Compare(x.CreatedAt) })
	if limite > 0 && len(out) > limite {
		out = out[:limite]
	}
	return out, err
}

// ----- Fijados y guardados: de quien mira, aquí -----

// Pin fija o desfija un mensaje del canal.
func (a *Agregador) Pin(ctx context.Context, mensaje string, poner bool) error {
	canal, ok := canalDeMensaje(mensaje)
	if !ok {
		return ErrCuentaDesconocida
	}
	var m api.RoomsMessage
	if poner {
		var err error
		if m, err = a.buscarMensaje(ctx, mensaje); err != nil {
			return err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Fijados == nil {
		a.prefs.Fijados = map[string][]api.RoomsMessage{}
	}
	l := slices.DeleteFunc(a.prefs.Fijados[canal], func(x api.RoomsMessage) bool { return x.ID == mensaje })
	if poner {
		l = append(l, m)
	}
	a.prefs.Fijados[canal] = l
	a.guardar()
	return nil
}

// canalDeMensaje es el id de pantalla del canal de un mensaje.
func canalDeMensaje(id string) (canal string, ok bool) {
	cuenta, canalLocal, _, ok := partirMensaje(id)
	return IDCanal(cuenta, canalLocal), ok
}

// buscarMensaje lee un mensaje de su canal para guardarlo o fijarlo.
func (a *Agregador) buscarMensaje(ctx context.Context, id string) (api.RoomsMessage, error) {
	canal, ok := canalDeMensaje(id)
	if !ok {
		return api.RoomsMessage{}, ErrCuentaDesconocida
	}
	ms, err := a.Messages(ctx, canal, time.Time{}, 200)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	for _, m := range ms {
		if m.ID == id {
			return m, nil
		}
	}
	return api.RoomsMessage{}, fmt.Errorf("chat: mensaje %q no encontrado entre los recientes", id)
}

// Pins son los fijados del canal.
func (a *Agregador) Pins(_ context.Context, canal string) ([]api.RoomsMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.prefs.Fijados[canal]), nil
}

// Save guarda o quita de guardados.
func (a *Agregador) Save(ctx context.Context, mensaje string, poner bool) error {
	var m api.RoomsMessage
	if poner {
		var err error
		if m, err = a.buscarMensaje(ctx, mensaje); err != nil {
			return err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.Guardados = slices.DeleteFunc(a.prefs.Guardados, func(x api.RoomsMessage) bool { return x.ID == mensaje })
	if poner {
		a.prefs.Guardados = append(a.prefs.Guardados, m)
	}
	a.guardar()
	return nil
}

// Saved son los guardados.
func (a *Agregador) Saved(context.Context) ([]api.RoomsMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.prefs.Guardados), nil
}

// ----- Canales: lo de quien mira, aquí; lo del canal, en su proveedor -----

// SetFavorite marca un canal como favorito.
func (a *Agregador) SetFavorite(_ context.Context, canal string, favorito bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Favoritos == nil {
		a.prefs.Favoritos = map[string]bool{}
	}
	if favorito {
		a.prefs.Favoritos[canal] = true
	} else {
		delete(a.prefs.Favoritos, canal)
	}
	a.guardar()
	return nil
}

// SetNotifyLevel fija los avisos del canal ("all", "mentions" o "none").
func (a *Agregador) SetNotifyLevel(_ context.Context, canal, nivel string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Niveles == nil {
		a.prefs.Niveles = map[string]string{}
	}
	a.prefs.Niveles[canal] = nivel
	a.guardar()
	return nil
}

// SetChannelCategory mueve el canal a una categoría ("" lo devuelve a la de su cuenta).
func (a *Agregador) SetChannelCategory(_ context.Context, canal, categoria string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Categoria == nil {
		a.prefs.Categoria = map[string]string{}
	}
	if categoria == "" {
		delete(a.prefs.Categoria, canal)
	} else {
		a.prefs.Categoria[canal] = categoria
	}
	a.guardar()
	return nil
}

// HideDirect quita un directo del carril.
func (a *Agregador) HideDirect(_ context.Context, canal string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Ocultos == nil {
		a.prefs.Ocultos = map[string]bool{}
	}
	a.prefs.Ocultos[canal] = true
	a.guardar()
	return nil
}

// UpdateChannel cambia nombre, asunto o archivo del canal.
func (a *Agregador) UpdateChannel(ctx context.Context, canal string, cambio api.RoomsChannelPatch) (api.RoomsChannel, error) {
	p, cuenta, c, err := a.deCanal(canal)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	g, ok := p.(Gestor)
	if !ok {
		return api.RoomsChannel{}, ErrNoDisponible
	}
	out, err := g.CambiarCanal(ctx, c, cambio)
	return prefijarCanal(cuenta, out), err
}

// ArchiveChannel archiva el canal.
func (a *Agregador) ArchiveChannel(ctx context.Context, canal string) error {
	si := true
	_, err := a.UpdateChannel(ctx, canal, api.RoomsChannelPatch{Archived: &si})
	return err
}

// DeleteChannel borra el canal.
func (a *Agregador) DeleteChannel(ctx context.Context, canal string) error {
	p, _, c, err := a.deCanal(canal)
	if err != nil {
		return err
	}
	g, ok := p.(Gestor)
	if !ok {
		return ErrNoDisponible
	}
	return g.BorrarCanal(ctx, c)
}

// LeaveChannel sale del canal.
func (a *Agregador) LeaveChannel(ctx context.Context, canal string) error {
	p, _, c, err := a.deCanal(canal)
	if err != nil {
		return err
	}
	s, ok := p.(Abandonador)
	if !ok {
		return ErrNoDisponible
	}
	return s.Salir(ctx, c)
}

// ArchivedChannels son los archivados de las cuentas que archivan.
func (a *Agregador) ArchivedChannels(ctx context.Context, _ string) ([]api.RoomsChannel, error) {
	var out []api.RoomsChannel
	err := a.todos(func(cuenta string, p Proveedor) error {
		g, ok := p.(Gestor)
		if !ok {
			return nil
		}
		cs, err := g.Archivados(ctx)
		for _, c := range cs {
			out = append(out, prefijarCanal(cuenta, c))
		}
		return err
	})
	return out, err
}

// ----- Personas -----

// AddTeamMember no existe.
func (a *Agregador) AddTeamMember(context.Context, string, string, string) error {
	return ErrNoDisponible
}

// RemoveTeamMember no existe.
func (a *Agregador) RemoveTeamMember(context.Context, string, string) error { return ErrNoDisponible }

// ChannelMembers son los del canal, si la cuenta los sabe; si no, quien mira.
func (a *Agregador) ChannelMembers(ctx context.Context, canal string) ([]api.RoomsMember, error) {
	p, _, c, err := a.deCanal(canal)
	if err != nil {
		return nil, err
	}
	if m, ok := p.(Miembros); ok {
		return m.Miembros(ctx, c)
	}
	return []api.RoomsMember{{Subject: Yo, Role: "admin"}}, nil
}

// RemoveChannelMember no está en esta entrega.
func (a *Agregador) RemoveChannelMember(context.Context, string, string) error {
	return ErrNoDisponible
}

// SetChannelMemberRole no está en esta entrega.
func (a *Agregador) SetChannelMemberRole(context.Context, string, string, string) (api.RoomsMember, error) {
	return api.RoomsMember{}, ErrNoDisponible
}

// ----- Comandos -----

// Commands: no hay comandos del servidor.
func (a *Agregador) Commands(context.Context) ([]api.RoomsCommand, error) { return nil, nil }

// ExecuteCommand no existe.
func (a *Agregador) ExecuteCommand(context.Context, string, string) (api.RoomsCommandResult, error) {
	return api.RoomsCommandResult{}, ErrNoDisponible
}

// ----- Avisos: en claro, aquí -----

// NotifyPrefs son las de quien mira.
func (a *Agregador) NotifyPrefs(context.Context) (api.RoomsNotifyPrefs, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prefs.Avisos, nil
}

// SetNotifyPause pausa los avisos hasta el momento dado (cero: los reanuda).
func (a *Agregador) SetNotifyPause(_ context.Context, hasta time.Time) (api.RoomsNotifyPrefs, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.Avisos.PausedUntil = nil
	if !hasta.IsZero() {
		a.prefs.Avisos.PausedUntil = &hasta
	}
	a.guardar()
	return a.prefs.Avisos, nil
}

// PutNotifyPrefs guarda palabras clave y horario de no molestar.
func (a *Agregador) PutNotifyPrefs(_ context.Context, p api.RoomsNotifyPrefs) (api.RoomsNotifyPrefs, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p.PausedUntil = a.prefs.Avisos.PausedUntil
	a.prefs.Avisos = p
	a.guardar()
	return p, nil
}

// ----- Categorías -----

// CreateCategory crea una categoría propia.
func (a *Agregador) CreateCategory(_ context.Context, nombre string) (api.RoomsCategory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := api.RoomsCategory{ID: "cat-" + nuevoID(), Name: nombre, SortOrder: len(a.prefs.Categorias)}
	a.prefs.Categorias = append(a.prefs.Categorias, c)
	a.guardar()
	return c, nil
}

// UpdateCategory renombra, ordena o pliega una categoría (la de una cuenta solo se pliega).
func (a *Agregador) UpdateCategory(_ context.Context, id string, cambio api.RoomsCategoryPatch) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.guardar()
	if strings.HasPrefix(id, PrefijoCategoria) {
		if cambio.Collapsed != nil {
			if a.prefs.Plegadas == nil {
				a.prefs.Plegadas = map[string]bool{}
			}
			a.prefs.Plegadas[id] = *cambio.Collapsed
		}
		return nil
	}
	i := slices.IndexFunc(a.prefs.Categorias, func(c api.RoomsCategory) bool { return c.ID == id })
	if i < 0 {
		return fmt.Errorf("chat: categoría %q desconocida", id)
	}
	c := &a.prefs.Categorias[i]
	if cambio.Name != nil {
		c.Name = *cambio.Name
	}
	if cambio.SortOrder != nil {
		c.SortOrder = *cambio.SortOrder
	}
	if cambio.Collapsed != nil {
		c.Collapsed = *cambio.Collapsed
	}
	return nil
}

// DeleteCategory borra una categoría propia; sus canales vuelven a la de su cuenta.
func (a *Agregador) DeleteCategory(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.Categorias = slices.DeleteFunc(a.prefs.Categorias, func(c api.RoomsCategory) bool { return c.ID == id })
	for canal, k := range a.prefs.Categoria {
		if k == id {
			delete(a.prefs.Categoria, canal)
		}
	}
	a.guardar()
	return nil
}

// ----- Borradores: en claro, aquí -----

// Drafts son los borradores.
func (a *Agregador) Drafts(context.Context) ([]api.RoomsDraft, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.prefs.Borradores), nil
}

// PutDraft guarda (o, en blanco, quita) el borrador del canal o del hilo.
func (a *Agregador) PutDraft(_ context.Context, canal, raiz, texto string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	mismo := func(d api.RoomsDraft) bool {
		r := ""
		if d.ParentID != nil {
			r = *d.ParentID
		}
		return d.ChannelID == canal && r == raiz
	}
	a.prefs.Borradores = slices.DeleteFunc(a.prefs.Borradores, mismo)
	if strings.TrimSpace(texto) != "" {
		d := api.RoomsDraft{ChannelID: canal, Body: texto, UpdatedAt: a.ahora()}
		if raiz != "" {
			d.ParentID = &raiz
		}
		a.prefs.Borradores = append(a.prefs.Borradores, d)
	}
	a.guardar()
	return nil
}

// ----- Tiempo real -----

// Typing: no se avisa a nadie de que se escribe.
func (a *Agregador) Typing(context.Context, string) error { return nil }

// Listen escucha a todas las cuentas a la vez hasta que el contexto muera. Una cuenta
// que se cae se reintenta sola, con espera creciente, sin cortar a las demás.
func (a *Agregador) Listen(ctx context.Context, _ []string, alLlegar func(api.RoomsEvent)) error {
	alLlegar(api.RoomsEvent{Type: api.RoomsReady})
	var wg sync.WaitGroup
	for _, cuenta := range a.Cuentas() {
		p, err := a.prov(cuenta)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.escucharCuenta(ctx, cuenta, p, alLlegar)
		}()
	}
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (a *Agregador) escucharCuenta(ctx context.Context, cuenta string, p Proveedor, alLlegar func(api.RoomsEvent)) {
	espera := time.Second
	caida := false
	for ctx.Err() == nil {
		if caida && a.AlRecuperar != nil {
			if _, _, err := p.Canales(ctx); err == nil {
				caida = false
				a.AlRecuperar(cuenta)
			}
		}
		inicio := time.Now()
		err := p.Escuchar(ctx, func(ev api.RoomsEvent) {
			if ev.Type == EventoCanales {
				if a.AlCambiarCanales != nil {
					a.AlCambiarCanales(cuenta)
				}
				return
			}
			alLlegar(prefijarEvento(cuenta, ev))
		})
		if ctx.Err() != nil {
			return
		}
		// Solo lo que es de la red (y no un «vuelve a conectar» del proveedor) cuenta
		// como caída de la que avisar al volver.
		var ue *url.Error
		caida = caida || errors.As(err, &ue)
		if time.Since(inicio) > time.Minute {
			espera = time.Second
		}
		slog.Info("chat: la escucha de una cuenta se cortó; se reintenta", "cuenta", cuenta, "error", err, "espera", espera)
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		espera = min(2*espera, 30*time.Second)
	}
}

func prefijarEvento(cuenta string, ev api.RoomsEvent) api.RoomsEvent {
	canal := ev.ChannelID
	if canal != "" {
		ev.ChannelID = IDCanal(cuenta, canal)
	}
	if ev.Subject != "" && ev.Subject != Yo {
		ev.Subject = cuenta + sep + ev.Subject
	}
	switch ev.Type {
	case "message", "message_edited", "message_deleted", "reaction":
		var m api.RoomsMessage
		if json.Unmarshal(ev.Payload, &m) == nil && m.ID != "" {
			if m.ChannelID == "" {
				m.ChannelID = canal
			}
			if raw, err := json.Marshal(prefijarMensaje(cuenta, m)); err == nil {
				ev.Payload = raw
			}
		}
	}
	return ev
}
