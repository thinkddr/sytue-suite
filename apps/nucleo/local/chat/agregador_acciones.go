// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// Lo del Agregador que reparte a la cuenta que toca: directos, personas, crear y
// explorar canales, adjuntos e hilos.

// ErrOtraCuenta: se mezclan personas o canales de cuentas distintas.
var ErrOtraCuenta = errors.New("chat: las personas y el canal tienen que ser de la misma cuenta")

// IDPersona es como ve la pantalla a una persona de una cuenta.
func IDPersona(cuenta, persona string) string { return cuenta + sep + persona }

// varias dice si hay más de una cuenta (y por tanto categorías por cuenta).
func (a *Agregador) varias() bool { return len(a.Cuentas()) > 1 }

// prefijar es prefijarCanal más la categoría de su cuenta si hay varias.
func (a *Agregador) prefijar(cuenta string, c api.RoomsChannel) api.RoomsChannel {
	c = prefijarCanal(cuenta, c)
	if a.varias() {
		a.mu.Lock()
		k, ok := a.prefs.Categoria[c.ID]
		a.mu.Unlock()
		c.CategoryID = PrefijoCategoria + cuenta
		if ok {
			c.CategoryID = k
		}
	}
	return c
}

// ----- Crear y explorar -----

// CuentasParaCrear son las cuentas donde se puede crear un canal, con el rótulo del
// selector: «Crear canal» deja elegir entre ellas y lo elegido llega como equipo a
// CreateChannel.
func (a *Agregador) CuentasParaCrear() (string, []api.RoomsTeam) {
	var out []api.RoomsTeam
	for _, cuenta := range a.Cuentas() {
		if p, err := a.prov(cuenta); err == nil {
			if _, ok := p.(Creador); ok {
				out = append(out, api.RoomsTeam{ID: cuenta, Name: p.Rotulo()})
			}
		}
	}
	return a.RotuloCuenta, out
}

// CreateChannel crea el canal en la cuenta equipo si es una de las abiertas que crean;
// si no (el equipo único de la pantalla), en la primera que sabe: la de Notas.
func (a *Agregador) CreateChannel(ctx context.Context, equipo, nombre, asunto string, privado bool) (api.RoomsChannel, error) {
	cuentas := a.Cuentas()
	if slices.Contains(cuentas, equipo) {
		cuentas = []string{equipo}
	}
	for _, cuenta := range cuentas {
		p, err := a.prov(cuenta)
		if err != nil {
			continue
		}
		if c, ok := p.(Creador); ok {
			out, err := c.CrearCanal(ctx, nombre, asunto, privado)
			if err != nil {
				return api.RoomsChannel{}, err
			}
			return a.prefijar(cuenta, out), nil
		}
	}
	return api.RoomsChannel{}, ErrNoDisponible
}

// Browse busca canales a los que unirse en las cuentas que exploran (o solo en la
// cuenta equipo). Con varias cuentas, el propósito empieza por la cuenta.
func (a *Agregador) Browse(ctx context.Context, equipo, q string) ([]api.RoomsChannel, error) {
	var out []api.RoomsChannel
	varias := a.varias()
	solo := ""
	if slices.Contains(a.Cuentas(), equipo) {
		solo = equipo
	}
	err := a.todos(func(cuenta string, p Proveedor) error {
		e, ok := p.(Explorador)
		if !ok || (solo != "" && cuenta != solo) {
			return nil
		}
		cs, err := e.Explorar(ctx, q)
		for _, c := range cs {
			c = a.prefijar(cuenta, c)
			if varias {
				c.Purpose = strings.TrimSuffix(p.Rotulo()+" · "+c.Purpose, " · ")
			}
			out = append(out, c)
		}
		return err
	})
	if len(out) > 0 {
		return out, nil
	}
	return out, err
}

// Join entra en el canal: si ya se está, lo devuelve; si no, se une por su cuenta.
func (a *Agregador) Join(ctx context.Context, canal string) (api.RoomsChannel, error) {
	p, cuenta, c, err := a.deCanal(canal)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	if e, ok := p.(Explorador); ok {
		out, err := e.Unirse(ctx, c)
		if err != nil {
			return api.RoomsChannel{}, err
		}
		return a.prefijar(cuenta, out), nil
	}
	cs, err := a.Channels(ctx, Equipo)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	for _, x := range cs {
		if x.ID == canal {
			return x, nil
		}
	}
	return api.RoomsChannel{}, ErrNoDisponible
}

// ----- Personas y directos -----

// partirPersona separa la cuenta de una persona prefijada. Sin prefijo, vuelve con ok
// falso y la persona tal cual.
func (a *Agregador) partirPersona(id string) (cuenta, persona string, ok bool) {
	cuenta, persona, ok = strings.Cut(id, sep)
	if !ok || !slices.Contains(a.Cuentas(), cuenta) {
		return "", id, false
	}
	return cuenta, persona, true
}

// conPersonas son las cuentas que conocen gente (las de Notas, no).
func (a *Agregador) conPersonas() []string {
	var con []string
	for _, cuenta := range a.Cuentas() {
		if p, err := a.prov(cuenta); err == nil {
			if _, ok := p.(Personas); ok {
				con = append(con, cuenta)
			}
		}
	}
	return con
}

// TeamMembers es quien mira más la gente de cada cuenta que la conoce, prefijada por su
// cuenta (con varias que la conocen, el nombre lleva la cuenta detrás y PlainName es el
// de la persona, del que salen las iniciales de su avatar).
func (a *Agregador) TeamMembers(ctx context.Context, _ string) ([]api.RoomsMember, error) {
	out := []api.RoomsMember{{Subject: Yo, Role: "admin"}}
	con := a.conPersonas()
	for _, cuenta := range con {
		// La cuenta pudo cerrarse desde conPersonas: entonces p es nil y no conoce a nadie.
		p, _ := a.prov(cuenta)
		pe, ok := p.(Personas)
		if !ok {
			continue
		}
		gs, err := pe.Gente(ctx)
		if err != nil {
			continue
		}
		for _, g := range gs {
			g.Subject = IDPersona(cuenta, g.Subject)
			if len(con) > 1 {
				g.PlainName = cmp.Or(g.Name, g.Subject)
				g.Name = g.PlainName + " · " + p.Rotulo()
			}
			g.Role = "member"
			out = append(out, g)
		}
	}
	return out, nil
}

// ResolvePerson busca a alguien en las cuentas que buscan, por orden; se queda con el
// primero que aparezca. Un `@nombre:servidor` solo se busca en Matrix y un correo
// primero en Slack, porque es lo que son. Como en TeamMembers, el nombre lleva la cuenta
// detrás solo si hay más de una que conoce gente, y entonces PlainName es el de la
// persona: la pantalla lo usa para su avatar y de nombre del directo que abre (el que
// le dará la cuenta en cuanto vuelva a pedir la lista).
func (a *Agregador) ResolvePerson(ctx context.Context, q string) (api.RoomsPerson, error) {
	q = strings.TrimSpace(q)
	var errs []error
	for _, cuenta := range a.Cuentas() {
		p, err := a.prov(cuenta)
		if err != nil {
			continue
		}
		pe, ok := p.(Personas)
		if !ok {
			continue
		}
		_, esMatrix := p.(*Matrix)
		if strings.HasPrefix(q, "@") && strings.Contains(q, ":") && !esMatrix {
			continue
		}
		per, err := pe.Resolver(ctx, q)
		if err != nil {
			if !errors.Is(err, ErrNoEncontrada) {
				errs = append(errs, err)
			}
			continue
		}
		per.UserID = IDPersona(cuenta, per.UserID)
		if len(a.conPersonas()) > 1 {
			per.PlainName = cmp.Or(per.Name, per.Username)
			per.Name = per.PlainName + " · " + p.Rotulo()
		}
		return per, nil
	}
	if len(errs) > 0 {
		return api.RoomsPerson{}, errors.Join(errs...)
	}
	return api.RoomsPerson{}, &api.Error{Status: 404, Body: ErrNoEncontrada.Error()}
}

// OpenDirect abre el directo con esas personas, que tienen que ser de la misma cuenta
// (la de su prefijo).
func (a *Agregador) OpenDirect(ctx context.Context, _ string, personas []string) (api.RoomsChannel, error) {
	cuenta := ""
	var propias []string
	for _, id := range personas {
		c, per, ok := a.partirPersona(id)
		if !ok {
			return api.RoomsChannel{}, fmt.Errorf("%w: %q", ErrCuentaDesconocida, id)
		}
		if cuenta != "" && c != cuenta {
			return api.RoomsChannel{}, ErrOtraCuenta
		}
		cuenta = c
		propias = append(propias, per)
	}
	if cuenta == "" {
		return api.RoomsChannel{}, errors.New("chat: falta con quién")
	}
	p, err := a.prov(cuenta)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	d, ok := p.(Directos)
	if !ok {
		return api.RoomsChannel{}, ErrNoDisponible
	}
	c, err := d.AbrirDirecto(ctx, propias)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	return a.prefijar(cuenta, c), nil
}

// AddChannelMember mete a alguien de la cuenta del canal.
func (a *Agregador) AddChannelMember(ctx context.Context, canal, persona, _ string) (api.RoomsMember, error) {
	p, cuenta, c, err := a.deCanal(canal)
	if err != nil {
		return api.RoomsMember{}, err
	}
	if k, per, ok := a.partirPersona(persona); ok {
		if k != cuenta {
			return api.RoomsMember{}, ErrOtraCuenta
		}
		persona = per
	}
	in, ok := p.(Invitador)
	if !ok {
		return api.RoomsMember{}, ErrNoDisponible
	}
	return in.Anadir(ctx, c, persona)
}

// ----- Adjuntos -----

// subida es un adjunto elegido que espera al mensaje.
type subida struct {
	canal, ruta, nombre, tipo string
	tamano                    int64
}

func (a *Agregador) carpetaSubidas() (string, error) {
	if a.CarpetaSubidas == "" {
		d, err := os.MkdirTemp("", "sytue-rooms-")
		if err != nil {
			return "", err
		}
		a.CarpetaSubidas = d
	}
	return a.CarpetaSubidas, os.MkdirAll(a.CarpetaSubidas, 0o700)
}

// UploadToChannel deja el fichero en espera, copiado aparte, hasta que se mande el
// mensaje (Slack lo sube y lo comparte de una vez; Matrix lo sube al mandar). Solo en
// las cuentas que mandan ficheros.
func (a *Agregador) UploadToChannel(ctx context.Context, canal, nombre, tipo string, cuerpo io.Reader, _ int64) (api.File, error) {
	p, _, _, err := a.deCanal(canal)
	if err != nil {
		return api.File{}, err
	}
	if _, ok := p.(Adjuntos); !ok {
		return api.File{}, ErrNoDisponible
	}
	a.mu.Lock()
	dir, err := a.carpetaSubidas()
	a.mu.Unlock()
	if err != nil {
		return api.File{}, err
	}
	id := "subida-" + nuevoID()
	ruta := filepath.Join(dir, id)
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return api.File{}, err
	}
	n, err := io.Copy(f, readerCtx{ctx, cuerpo})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(ruta)
		return api.File{}, err
	}
	tipo = cmp.Or(tipo, "application/octet-stream")
	a.mu.Lock()
	if a.subidas == nil {
		a.subidas = map[string]subida{}
	}
	a.subidas[id] = subida{canal: canal, ruta: ruta, nombre: nombre, tipo: tipo, tamano: n}
	a.mu.Unlock()
	return api.File{ID: id, Name: nombre, SizeBytes: n, ContentType: tipo}, nil
}

// readerCtx corta la copia si el contexto muere (quitar la ficha a medio subir).
type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r readerCtx) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

// ForgetUpload suelta un adjunto en espera.
func (a *Agregador) ForgetUpload(_, id string) {
	a.mu.Lock()
	s, ok := a.subidas[id]
	delete(a.subidas, id)
	a.mu.Unlock()
	if ok {
		_ = os.Remove(s.ruta)
	}
}

// ForgetUploads suelta los en espera del canal.
func (a *Agregador) ForgetUploads(canal string) {
	a.mu.Lock()
	var rutas []string
	for id, s := range a.subidas {
		if s.canal == canal {
			rutas = append(rutas, s.ruta)
			delete(a.subidas, id)
		}
	}
	a.mu.Unlock()
	for _, r := range rutas {
		_ = os.Remove(r)
	}
}

// ficherosDe toma los adjuntos en espera de ids (del canal) para mandarlos.
func (a *Agregador) ficherosDe(canal string, ids []string) ([]Fichero, []string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var fs []Fichero
	var rutas []string
	for _, id := range ids {
		s, ok := a.subidas[id]
		if !ok || s.canal != canal {
			return nil, nil, fmt.Errorf("chat: el adjunto %q ya no está", id)
		}
		ruta := s.ruta
		fs = append(fs, Fichero{Nombre: s.nombre, Tipo: s.tipo, Tamano: s.tamano, Abrir: func() (io.ReadCloser, error) { return os.Open(ruta) }})
		rutas = append(rutas, ruta)
	}
	return fs, rutas, nil
}

// enviar manda texto (y los adjuntos en espera) al canal o al hilo de raiz.
func (a *Agregador) enviar(ctx context.Context, p Proveedor, cuenta, canalPantalla, c, raiz, texto string, o api.RoomsSend) (api.RoomsMessage, error) {
	if len(o.FileIDs) == 0 {
		m, err := p.Enviar(ctx, c, raiz, texto)
		return prefijarMensaje(cuenta, m), err
	}
	ad, ok := p.(Adjuntos)
	if !ok {
		return api.RoomsMessage{}, ErrNoDisponible
	}
	fs, rutas, err := a.ficherosDe(canalPantalla, o.FileIDs)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	m, err := ad.EnviarFicheros(ctx, c, raiz, texto, fs)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	a.mu.Lock()
	for _, id := range o.FileIDs {
		delete(a.subidas, id)
	}
	a.mu.Unlock()
	for _, r := range rutas {
		_ = os.Remove(r)
	}
	m = prefijarMensaje(cuenta, m)
	a.recordarAdjuntos([]api.RoomsMessage{m})
	return m, nil
}

// PostWith publica texto y, si los hay, los adjuntos en espera. Prioridad, acuse y
// tarjetas no existen en estos proveedores y se ignoran.
func (a *Agregador) PostWith(ctx context.Context, canal, texto string, o api.RoomsSend) (api.RoomsMessage, error) {
	p, cuenta, c, err := a.deCanal(canal)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	return a.enviar(ctx, p, cuenta, canal, c, "", texto, o)
}

// ReplyWith responde en el hilo de raiz.
func (a *Agregador) ReplyWith(ctx context.Context, raiz, texto string, o api.RoomsSend) (api.RoomsMessage, error) {
	p, cuenta, c, r, err := a.deMensaje(raiz)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	return a.enviar(ctx, p, cuenta, IDCanal(cuenta, c), c, r, texto, o)
}

// ChannelContent trae un adjunto de un mensaje del canal (o uno en espera).
func (a *Agregador) ChannelContent(ctx context.Context, canal, fichero string, miniatura bool) (io.ReadCloser, error) {
	a.mu.Lock()
	s, enEspera := a.subidas[fichero]
	a.mu.Unlock()
	if enEspera && s.canal == canal {
		return os.Open(s.ruta)
	}
	p, _, c, err := a.deCanal(canal)
	if err != nil {
		return nil, err
	}
	ad, ok := p.(Adjuntos)
	if !ok {
		return nil, ErrNoDisponible
	}
	return ad.Bajar(ctx, c, fichero, miniatura)
}

// SaveChannelFileToMyDrive guarda el adjunto en la carpeta de Drive de la edición, sin
// pisar lo que haya.
func (a *Agregador) SaveChannelFileToMyDrive(ctx context.Context, canal, fichero string) (api.File, error) {
	if a.GuardarEn == "" {
		return api.File{}, ErrNoDisponible
	}
	rc, err := a.ChannelContent(ctx, canal, fichero, false)
	if err != nil {
		return api.File{}, err
	}
	defer rc.Close()
	a.mu.Lock()
	nombre := a.nombres[fichero]
	a.mu.Unlock()
	nombre = cmp.Or(filepath.Base(nombre), "adjunto")
	if err := os.MkdirAll(a.GuardarEn, 0o700); err != nil {
		return api.File{}, err
	}
	ruta, err := ficheros.Guardar(a.GuardarEn, nombre, rc)
	if err != nil {
		return api.File{}, err
	}
	fi, _ := os.Stat(ruta)
	out := api.File{ID: ruta, Name: filepath.Base(ruta)}
	if fi != nil {
		out.SizeBytes = fi.Size()
	}
	return out, nil
}

// recordarAdjuntos apunta los nombres de los adjuntos vistos, para «Guardar en Drive».
func (a *Agregador) recordarAdjuntos(ms []api.RoomsMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range ms {
		for _, x := range m.Attachments {
			if a.nombres == nil {
				a.nombres = map[string]string{}
			}
			a.nombres[x.FileID] = x.Name
		}
	}
}

// ----- Hilos -----

// Follow sigue o deja de seguir un hilo, en las preferencias de quien mira.
func (a *Agregador) Follow(_ context.Context, raiz string, seguir bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs.Seguidos == nil {
		a.prefs.Seguidos = map[string]bool{}
	}
	a.prefs.Seguidos[raiz] = seguir
	a.guardar()
	return nil
}

// MaxHilos es cuántos hilos da «Hilos» (una sola página).
const MaxHilos = 100

// MyThreads son los hilos que tienen a mano las cuentas en los que quien mira escribió
// (la raíz o alguna respuesta) o que sigue a mano, menos los que dejó de seguir; del de
// actividad más reciente al más viejo. Sin páginas: todos de una vez, hasta MaxHilos.
func (a *Agregador) MyThreads(ctx context.Context, antes string) ([]api.RoomsThreadSummary, string, error) {
	if antes != "" {
		return nil, "", nil
	}
	var out []api.RoomsThreadSummary
	err := a.todos(func(cuenta string, p Proveedor) error {
		h, ok := p.(HilosPropios)
		if !ok {
			return nil
		}
		hs, err := h.MisHilos(ctx)
		for _, x := range hs {
			x.Root = prefijarMensaje(cuenta, x.Root)
			out = append(out, x)
		}
		return err
	})
	a.mu.Lock()
	out = slices.DeleteFunc(out, func(h api.RoomsThreadSummary) bool {
		if v, ok := a.prefs.Seguidos[h.Root.ID]; ok {
			return !v
		}
		propio := h.Root.AuthorSub == Yo || slices.ContainsFunc(h.Root.ThreadParticipants, func(p api.RoomsThreadParticipant) bool { return p.Sub == Yo })
		return !propio
	})
	a.mu.Unlock()
	slices.SortFunc(out, func(x, y api.RoomsThreadSummary) int { return y.LastActivity.Compare(x.LastActivity) })
	if len(out) > MaxHilos {
		out = out[:MaxHilos]
	}
	if len(out) == 0 && err != nil {
		return nil, "", err
	}
	return out, "", nil
}
