// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// servicioFalso es el servidor de Rooms en memoria. Apunta cada llamada para que el
// guardián compruebe QUÉ se pidió, no solo qué se pintó.
type servicioFalso struct {
	canales     []api.RoomsChannel
	pend        []api.RoomsUnread
	fuera       []api.RoomsChannel
	mensajes    map[string][]api.RoomsMessage
	errLista    error
	errMens     error
	errPost     error
	tramas      []api.RoomsEvent
	hilo        api.RoomsThread
	errHilo     error
	errAccion   error
	fijados     []api.RoomsMessage
	hallados    []api.RoomsMessage
	raizAround  string
	errCrear    error
	explorables []api.RoomsChannel
	errExplorar error
	equipo      []api.RoomsMember
	roster      []api.RoomsMember
	seguidos    []api.RoomsThreadSummary
	masSeguidos []api.RoomsThreadSummary
	sigSeguidos string
	errSeguidos error
	categorias  []api.RoomsCategory
	rolEquipo   string
	borradores  []api.RoomsDraft
	programados []api.RoomsScheduled
	// envios son las opciones de cada PostWith y ReplyWith; lectores, lo que da Readers.
	envios   []api.RoomsSend
	lectores []api.RoomsReader
	comandos []api.RoomsCommand
	prefs    api.RoomsNotifyPrefs
	votos    []api.RoomsCardVote

	llamadas []string
}

func (s *servicioFalso) apuntar(f string, args ...string) {
	s.llamadas = append(s.llamadas, f+"("+strings.Join(args, ",")+")")
}

func (s *servicioFalso) Teams(context.Context) ([]api.RoomsTeam, error) {
	s.apuntar("Teams")
	return []api.RoomsTeam{{ID: "t1", Role: s.rolEquipo}}, s.errLista
}

func (s *servicioFalso) Channels(_ context.Context, e string) ([]api.RoomsChannel, error) {
	s.apuntar("Channels", e)
	// Una copia, como el servicio de verdad: la Sala borra los archivados con
	// slices.DeleteFunc, que pone a cero la cola del array de quien se lo dio.
	return slices.Clone(s.canales), nil
}

func (s *servicioFalso) Unread(_ context.Context, e string) ([]api.RoomsUnread, error) {
	s.apuntar("Unread", e)
	return s.pend, nil
}

func (s *servicioFalso) ExternalChannels(context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	s.apuntar("ExternalChannels")
	return s.fuera, nil, nil
}

func (s *servicioFalso) Categories(context.Context) ([]api.RoomsCategory, error) {
	s.apuntar("Categories")
	return s.categorias, nil
}

func (s *servicioFalso) Messages(_ context.Context, c string, _ time.Time, _ int) ([]api.RoomsMessage, error) {
	s.apuntar("Messages", c)
	return s.mensajes[c], s.errMens
}

func (s *servicioFalso) Post(_ context.Context, c, texto string) (api.RoomsMessage, error) {
	s.apuntar("Post", c, texto)
	if s.errPost != nil {
		return api.RoomsMessage{}, s.errPost
	}
	m := api.RoomsMessage{ID: "nuevo", ChannelID: c, AuthorSub: "yo", Body: texto, CreatedAt: time.Now()}
	s.mensajes[c] = append(s.mensajes[c], m)
	return m, nil
}

func (s *servicioFalso) MarkRead(_ context.Context, c string) error {
	s.apuntar("MarkRead", c)
	return nil
}

func (s *servicioFalso) React(_ context.Context, m, e string, poner bool) error {
	s.apuntar("React", m, e, map[bool]string{true: "poner", false: "quitar"}[poner])
	return nil
}

func (s *servicioFalso) SetNotifyLevel(_ context.Context, c, n string) error {
	s.apuntar("SetNotifyLevel", c, n)
	return s.errAccion
}

func (s *servicioFalso) SetChannelCategory(_ context.Context, c, k string) error {
	s.apuntar("SetChannelCategory", c, k)
	return s.errAccion
}

func (s *servicioFalso) ArchiveChannel(_ context.Context, c string) error {
	s.apuntar("ArchiveChannel", c)
	return s.errAccion
}

func (s *servicioFalso) DeleteChannel(_ context.Context, c string) error {
	s.apuntar("DeleteChannel", c)
	return s.errAccion
}

func (s *servicioFalso) HideDirect(_ context.Context, c string) error {
	s.apuntar("HideDirect", c)
	return s.errAccion
}

func (s *servicioFalso) Scheduled(context.Context) ([]api.RoomsScheduled, error) {
	s.apuntar("Scheduled")
	return s.programados, nil
}

func (s *servicioFalso) CancelScheduled(_ context.Context, id string) error {
	s.apuntar("CancelScheduled", id)
	return s.errAccion
}

func (s *servicioFalso) Drafts(context.Context) ([]api.RoomsDraft, error) {
	s.apuntar("Drafts")
	return s.borradores, nil
}

func (s *servicioFalso) PutDraft(_ context.Context, c, raiz, texto string) error {
	s.apuntar("PutDraft", c, raiz, texto)
	return nil
}

func (s *servicioFalso) CreateCategory(_ context.Context, n string) (api.RoomsCategory, error) {
	s.apuntar("CreateCategory", n)
	return api.RoomsCategory{ID: "k-" + n, Name: n, SortOrder: len(s.categorias)}, s.errAccion
}

func (s *servicioFalso) UpdateCategory(_ context.Context, id string, c api.RoomsCategoryPatch) error {
	var partes []string
	if c.Name != nil {
		partes = append(partes, "nombre="+*c.Name)
	}
	if c.SortOrder != nil {
		partes = append(partes, "orden="+strconv.Itoa(*c.SortOrder))
	}
	if c.Collapsed != nil {
		partes = append(partes, "plegada="+strconv.FormatBool(*c.Collapsed))
	}
	s.apuntar("UpdateCategory", append([]string{id}, partes...)...)
	return s.errAccion
}

func (s *servicioFalso) DeleteCategory(_ context.Context, id string) error {
	s.apuntar("DeleteCategory", id)
	return s.errAccion
}

func (s *servicioFalso) SetFavorite(_ context.Context, c string, f bool) error {
	s.apuntar("SetFavorite", c, map[bool]string{true: "sí", false: "no"}[f])
	return errors.New("no")
}

func (s *servicioFalso) Edit(_ context.Context, m, texto string) (api.RoomsMessage, error) {
	s.apuntar("Edit", m, texto)
	if s.errAccion != nil {
		return api.RoomsMessage{}, s.errAccion
	}
	ahora := time.Now()
	return api.RoomsMessage{ID: m, Body: texto, EditedAt: &ahora}, nil
}

func (s *servicioFalso) Delete(_ context.Context, m string) error {
	s.apuntar("Delete", m)
	return s.errAccion
}

func (s *servicioFalso) Pin(_ context.Context, m string, poner bool) error {
	s.apuntar("Pin", m, map[bool]string{true: "sí", false: "no"}[poner])
	return s.errAccion
}

func (s *servicioFalso) Save(_ context.Context, m string, poner bool) error {
	s.apuntar("Save", m, map[bool]string{true: "sí", false: "no"}[poner])
	return s.errAccion
}

func (s *servicioFalso) MarkUnread(_ context.Context, m string) error {
	s.apuntar("MarkUnread", m)
	return s.errAccion
}

func (s *servicioFalso) Pins(_ context.Context, c string) ([]api.RoomsMessage, error) {
	s.apuntar("Pins", c)
	return s.fijados, nil
}

func (s *servicioFalso) Saved(context.Context) ([]api.RoomsMessage, error) {
	s.apuntar("Saved")
	return nil, nil
}

func (s *servicioFalso) MyThreads(_ context.Context, antes string) ([]api.RoomsThreadSummary, string, error) {
	s.apuntar("MyThreads", antes)
	if antes != "" {
		return s.masSeguidos, "", s.errSeguidos
	}
	return s.seguidos, s.sigSeguidos, s.errSeguidos
}

func (s *servicioFalso) Search(_ context.Context, q string, _ int) ([]api.RoomsMessage, error) {
	s.apuntar("Search", q)
	return s.hallados, nil
}

func (s *servicioFalso) Around(_ context.Context, c, m string, _ int) ([]api.RoomsMessage, string, error) {
	s.apuntar("Around", c, m)
	return s.mensajes[c], s.raizAround, nil
}

func (s *servicioFalso) CreateChannel(_ context.Context, e, nombre, asunto string, privado bool) (api.RoomsChannel, error) {
	s.apuntar("CreateChannel", e, nombre, asunto, map[bool]string{true: "privado", false: "público"}[privado])
	if s.errCrear != nil {
		return api.RoomsChannel{}, s.errCrear
	}
	return api.RoomsChannel{ID: "nuevo", Name: SlugCanal(nombre), Kind: "public", Topic: asunto}, nil
}

func (s *servicioFalso) Browse(_ context.Context, e, q string) ([]api.RoomsChannel, error) {
	s.apuntar("Browse", e, q)
	return s.explorables, s.errExplorar
}

func (s *servicioFalso) Join(_ context.Context, c string) (api.RoomsChannel, error) {
	s.apuntar("Join", c)
	for _, x := range s.explorables {
		if x.ID == c {
			return x, nil
		}
	}
	return api.RoomsChannel{}, &api.Error{Status: 404}
}

func (s *servicioFalso) OpenDirect(_ context.Context, e string, ps []string) (api.RoomsChannel, error) {
	s.apuntar("OpenDirect", append([]string{e}, ps...)...)
	return api.RoomsChannel{ID: "dm-" + strings.Join(ps, "-"), Kind: "direct"}, nil
}

func (s *servicioFalso) TeamMembers(_ context.Context, e string) ([]api.RoomsMember, error) {
	s.apuntar("TeamMembers", e)
	return s.equipo, nil
}

func (s *servicioFalso) ResolvePerson(_ context.Context, q string) (api.RoomsPerson, error) {
	s.apuntar("ResolvePerson", q)
	if q == "@nadie" {
		return api.RoomsPerson{}, &api.Error{Status: 404}
	}
	return api.RoomsPerson{UserID: "g1", Name: "Grace", Username: "grace"}, nil
}

func (s *servicioFalso) ChannelMembers(_ context.Context, c string) ([]api.RoomsMember, error) {
	s.apuntar("ChannelMembers", c)
	return s.roster, nil
}

func (s *servicioFalso) AddChannelMember(_ context.Context, c, p, n string) (api.RoomsMember, error) {
	s.apuntar("AddChannelMember", c, p, n)
	return api.RoomsMember{Subject: p, Name: n, Role: "member"}, s.errAccion
}

func (s *servicioFalso) RemoveChannelMember(_ context.Context, c, p string) error {
	s.apuntar("RemoveChannelMember", c, p)
	return s.errAccion
}

func (s *servicioFalso) SetChannelMemberRole(_ context.Context, c, p, rol string) (api.RoomsMember, error) {
	s.apuntar("SetChannelMemberRole", c, p, rol)
	return api.RoomsMember{Subject: p, Name: p, Role: rol}, s.errAccion
}

func (s *servicioFalso) Thread(_ context.Context, raiz string) (api.RoomsThread, error) {
	s.apuntar("Thread", raiz)
	if s.errHilo != nil {
		return api.RoomsThread{}, s.errHilo
	}
	return s.hilo, nil
}

func (s *servicioFalso) Reply(_ context.Context, raiz, texto string) (api.RoomsMessage, error) {
	s.apuntar("Reply", raiz, texto)
	m := api.RoomsMessage{ID: "r-nueva", AuthorSub: "yo", AuthorName: "Yo", Body: texto, CreatedAt: time.Now()}
	s.hilo.Replies = append(s.hilo.Replies, m)
	return m, nil
}

func (s *servicioFalso) Follow(_ context.Context, raiz string, seguir bool) error {
	s.apuntar("Follow", raiz, map[bool]string{true: "sí", false: "no"}[seguir])
	return s.errAccion
}

func (s *servicioFalso) Listen(_ context.Context, cs []string, al func(api.RoomsEvent)) error {
	s.apuntar("Listen", cs...)
	for _, ev := range s.tramas {
		al(ev)
	}
	return nil
}

func (s *servicioFalso) Typing(_ context.Context, c string) error {
	s.apuntar("Typing", c)
	return nil
}

func (s *servicioFalso) veces(f string) int {
	n := 0
	for _, l := range s.llamadas {
		if l == f {
			n++
		}
	}
	return n
}

func (s *servicioFalso) pidio(f string) bool { return s.veces(f) > 0 }

// salaDePrueba monta la Sala con el doble, síncrona: `Lanzar` corre en el acto y los
// temporizadores se apuntan en vez de dormir.
func salaDePrueba(t *testing.T, s *servicioFalso) (*Sala, map[time.Duration]func()) {
	t.Helper()
	sa, relojes, _ := salaEn(t, s, 1280)
	return sa, relojes
}

// salaEn es salaDePrueba en una ventana de ese ancho, que devuelve para capturarla.
func salaEn(t *testing.T, s *servicioFalso, ancho float32) (*Sala, map[time.Duration]func(), fyne.Window) {
	t.Helper()
	p := NuevaPantalla(traductor(t), "es")
	p.Reloj(reloj())
	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, 744))
	// El historial y la caja nacen escondidos (sin canal): se les monta el renderer ya,
	// que es donde viven el desplazador y el editor que el guardián mira.
	test.WidgetRenderer(p.Canal.Historial)
	test.WidgetRenderer(p.Canal.Compositor)
	sa := NuevaSala(p, s, "yo")
	sa.Lanzar = func(f func()) { f() }
	temporizadores := map[time.Duration]func(){}
	sa.Esperar = func(d time.Duration, f func()) func() {
		temporizadores[d] = f
		return func() { delete(temporizadores, d) }
	}
	return sa, temporizadores, w
}

func servicioConDos() *servicioFalso {
	ayer := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	return &servicioFalso{
		canales: []api.RoomsChannel{
			{ID: "c1", Name: "equipo", Kind: "public", Topic: "Guardias"},
			{ID: "d1", Name: "dm-x", Kind: "direct", Peers: []string{"Ana Ruiz"}},
			{ID: "viejo", Name: "archivado", Kind: "public", Archived: true},
		},
		pend: []api.RoomsUnread{{ChannelID: "c1", Unread: 1}, {ChannelID: "d1", Unread: 4, Mentions: 1}},
		mensajes: map[string][]api.RoomsMessage{
			"c1": {
				{ID: "m1", AuthorSub: "ana", AuthorName: "Ana", Body: "hola", CreatedAt: ayer},
				{ID: "m2", AuthorSub: "ana", Body: "", Type: "call", CreatedAt: ayer.Add(time.Minute)},
				{ID: "m3", AuthorSub: "beto", AuthorName: "Beto", Body: "90", Type: "call", CreatedAt: ayer.Add(2 * time.Minute)},
				{
					ID: "m4", AuthorSub: "beto", AuthorName: "Beto", Body: "vale", Priority: "urgent", CreatedAt: ayer.Add(3 * time.Minute),
					Reactions: []api.RoomsReaction{{Emoji: "👍", Count: 2, Subs: []string{"ana", "yo"}}},
				},
			},
		},
	}
}

// TestLaSalaAbreElPrimerCanalYLoDaPorLeido es el efecto de canales y el de mensajes
// de `ChatApp.tsx`: equipo → canales → pendientes; el primero de la lista del
// servicio se abre, su separador de no leídos se ancla con el contador que tenía AL
// ABRIRLO, y se da por leído. Lo archivado no sale.
//
// VISTO EN ROJO (2026-09-22, en el frontal): contando los no leídos a 0 al abrir, el
// separador sale en −1 en vez de ante la fila 3. Deshecho.
func TestLaSalaAbreElPrimerCanalYLoDaPorLeido(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())

	p := sa.P
	if p.Lateral.EstadoCanales != Listo || len(p.Lateral.Canales) != 2 {
		t.Fatalf("el carril quedó en %v con %d canales (el archivado no cuenta)", p.Lateral.EstadoCanales, len(p.Lateral.Canales))
	}
	if p.Lateral.CanalID != "c1" || p.Canal.Cabecera.Canal.Tema != "Guardias" {
		t.Fatalf("se abrió %q con tema %q y tenía que ser el primero, «equipo»", p.Lateral.CanalID, p.Canal.Cabecera.Canal.Tema)
	}
	for _, c := range p.Lateral.Canales {
		switch c.ID {
		case "c1":
			if c.SinLeer != 0 {
				t.Errorf("el canal abierto sigue con %d sin leer en el carril", c.SinLeer)
			}
		case "d1":
			if c.Clase != Directo || c.Nombre != "Ana Ruiz" || c.SinLeer != 4 || c.Menciones != 1 {
				t.Errorf("el directo sale como %+v", c)
			}
		}
	}
	h := p.Canal.Historial
	if h.Estado != Listo || len(h.Mensajes) != 4 {
		t.Fatalf("historial en %v con %d mensajes", h.Estado, len(h.Mensajes))
	}
	if h.IndiceNoLeidos != 3 {
		t.Errorf("el separador va ante la fila %d y con 1 sin leer de 4 va ante la 3", h.IndiceNoLeidos)
	}
	if !s.pidio("MarkRead(c1)") {
		t.Errorf("el canal abierto no se dio por leído: %v", s.llamadas)
	}
	// La traducción de `toChatMessage`: autor sin nombre, las dos llamadas, la
	// prioridad y la reacción propia.
	m := h.Mensajes
	if m[1].Autor.Nombre != "ana" || m[1].Tipo != Llamada || m[1].Terminada {
		t.Errorf("la llamada iniciada sale como %+v", m[1])
	}
	if !m[2].Terminada || m[2].Segundos != 90 {
		t.Errorf("la llamada terminada sale como %+v", m[2])
	}
	if h.UltimaLlamadaID != "m3" {
		t.Errorf("la última llamada es %q y es m3", h.UltimaLlamadaID)
	}
	if m[3].Prioridad != Urgente || len(m[3].Reacciones) != 1 || !m[3].Reacciones[0].Mia || m[3].Reacciones[0].Veces != 2 {
		t.Errorf("el urgente con reacción sale como %+v", m[3])
	}
}

// TestLosEstadosDeLaSala: un fallo al pedir los canales es el aviso con reintento; un
// 403 al abrir uno es «sin permiso», otra respuesta del servidor «error» y una
// petición sin respuesta, «sin conexión».
func TestLosEstadosDeLaSala(t *testing.T) {
	s := servicioConDos()
	s.errLista = &api.Error{Status: 500}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	if sa.P.Lateral.EstadoCanales != Error {
		t.Fatalf("con el servicio caído el carril queda en %v", sa.P.Lateral.EstadoCanales)
	}
	s.errLista = nil
	sa.P.Lateral.AlReintentar()
	if sa.P.Lateral.EstadoCanales != Listo {
		t.Fatalf("reintentar deja el carril en %v", sa.P.Lateral.EstadoCanales)
	}
	for err, quiero := range map[error]Estado{
		&api.Error{Status: 403}:                           SinPermiso,
		&api.Error{Status: 502}:                           Error,
		&url.Error{Op: "Get", Err: errors.New("sin red")}: SinConexion,
	} {
		s.errMens = err
		sa.Elegir(sa.P.Lateral.Canales[0])
		if got := sa.P.Canal.Historial.Estado; got != quiero {
			t.Errorf("con %v el historial queda en %v y es %v", err, got, quiero)
		}
	}
}

// TestEnviarVaciaLaCajaYSiFallaDevuelveElTexto: el mensaje entra cuando el servicio
// lo confirma; si no, lo escrito vuelve a la caja.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin devolver el texto, la caja queda
// vacía y «otra» se pierde. Deshecho.
func TestEnviarVaciaLaCajaYSiFallaDevuelveElTexto(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor

	c.PonerTexto("buenos días")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("Post(c1,buenos días)") || c.Texto() != "" {
		t.Fatalf("Intro no envió o no vació la caja: %v, %q", s.llamadas, c.Texto())
	}
	h := sa.P.Canal.Historial
	if got := h.Mensajes[len(h.Mensajes)-1]; got.Cuerpo != "buenos días" {
		t.Errorf("el mensaje enviado no entró al final: %+v", got)
	}

	s.errPost = errors.New("caído")
	c.PonerTexto("otra")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if c.Texto() != "otra" {
		t.Errorf("con el envío caído la caja queda con %q y tenía que recuperar «otra»", c.Texto())
	}
}

// TestElTiempoRealRecargaYApuntaQuienEscribe: una trama del canal abierto recarga su
// final, una de otro canal pide los contadores, y «escribiendo» dura cinco segundos.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin descartar el eco propio, sale
// «[Ana Yo]» escribiendo. Deshecho.
func TestElTiempoRealRecargaYApuntaQuienEscribe(t *testing.T) {
	s := servicioConDos()
	s.tramas = []api.RoomsEvent{
		{Type: api.RoomsReady},
		{Type: "typing", ChannelID: "c1", Subject: "ana", Name: "Ana"},
		{Type: "typing", ChannelID: "c1", Subject: "yo", Name: "Yo"},
		{Type: "message", ChannelID: "d1"},
	}
	sa, relojes := salaDePrueba(t, s)
	sa.Arrancar(context.Background())

	if !s.pidio("Listen(c1,d1)") {
		t.Fatalf("no se suscribió a los dos canales vivos: %v", s.llamadas)
	}
	if got := sa.P.Canal.Escribiendo; len(got) != 1 || got[0] != "Ana" {
		t.Fatalf("escribiendo = %v y es solo Ana (el eco propio no cuenta)", got)
	}
	if n := strings.Count(strings.Join(s.llamadas, " "), "Unread(t1)"); n != 2 {
		t.Errorf("una trama de otro canal tenía que volver a pedir los contadores: %v", s.llamadas)
	}
	caduca := relojes[VidaEscribiendo]
	if caduca == nil {
		t.Fatal("«escribiendo» no se programó para caducar")
	}
	caduca()
	if len(sa.P.Canal.Escribiendo) != 0 {
		t.Errorf("a los 5 s sigue %v escribiendo", sa.P.Canal.Escribiendo)
	}
	// La conexión se cerró (el doble vuelve con nil al acabar las tramas): nil es un
	// cierre limpio y no se reconecta.
	if relojes[ReconectarBase] != nil {
		t.Error("un cierre limpio no reconecta")
	}
}

// TestElHistorialSeAbrePorElFinal: un chat se lee de abajo arriba.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin `ScrollToBottom`, 1241 de contenido
// en 566 de alto se abren en y = 0, con lo más viejo delante. Deshecho.
func TestElHistorialSeAbrePorElFinal(t *testing.T) {
	s := servicioConDos()
	for i := range 40 {
		s.mensajes["c1"] = append(s.mensajes["c1"], api.RoomsMessage{
			ID: "x" + string(rune('a'+i)), AuthorSub: "ana", Body: "línea", CreatedAt: time.Date(2026, 9, 22, 9, i, 0, 0, time.UTC),
		})
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sc, ok := sa.P.Canal.Historial.cuerpo.(*container.Scroll)
	if !ok {
		t.Fatal("el historial con datos no es un desplazador")
	}
	if !alFondo(sc) || sc.Offset.Y == 0 {
		t.Errorf("el historial se abre en y = %v y tenía que abrirse abajo (%v de contenido en %v)",
			sc.Offset.Y, sc.Content.MinSize().Height, sc.Size().Height)
	}
}

// TestLaSalaSeVe deja los PNG de la Sala con datos, cuando se piden: el canal abierto
// al arrancar y el directo después, a 1280 y a 360 de ancho. Es como se mira al lado
// de las capturas de la web.
func TestLaSalaSeVe(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_SALA")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_SALA")
	}
	for _, ancho := range []float32{1280, 360} {
		sa, _, w := salaEn(t, servicioConHilo(), ancho)
		sa.Arrancar(context.Background())
		guardar := func(nombre string) {
			f, err := os.Create(filepath.Join(dir, nombre))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, w.Canvas().Capture()); err != nil {
				t.Fatal(err)
			}
		}
		n := strconv.Itoa(int(ancho))
		guardar("sala-canal-" + n + ".png")
		sa.Elegir(sa.P.Lateral.Canales[1])
		guardar("sala-directo-" + n + ".png")
		sa.Elegir(sa.P.Lateral.Canales[0])
		sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
		guardar("sala-hilo-" + n + ".png")
		sa.CerrarHilo()
		filaDe(t, sa.P.Canal.Historial, "m1").MouseIn(nil)
		guardar("sala-barra-" + n + ".png")
		sa.AbrirPanel(PanelBusqueda)
		guardar("sala-buscar-vacio-" + n + ".png")
		sa.lista.Mensajes, sa.lista.Estado, sa.lista.consulta = sa.lista.Mensajes[:0], Listo, "hola"
		sa.lista.Mensajes = append(sa.lista.Mensajes, sa.P.Canal.Historial.Mensajes[:1]...)
		sa.lista.Refresh()
		guardar("sala-buscar-" + n + ".png")
		sa.IrAMensaje("c1", "m1")
		guardar("sala-salto-" + n + ".png")
		sa.IrAlFinal()
		sa.mensajes = append(sa.mensajes, api.RoomsMessage{
			ID: "md", AuthorSub: "beto", AuthorName: "Beto", CreatedAt: time.Date(2026, 9, 21, 9, 30, 0, 0, time.UTC),
			Body: "**Negrita**, _cursiva_, ~~tachado~~, `código` y https://sytue.eu :rocket:\n@Ana Ruiz y @here, mirad\n" +
				"# Título\n- uno\n- dos\n1. primero\n> citado\n```go\nfmt.Println(\"hola\")\n```",
			Mentions: []string{"ana"},
		})
		sa.roster = []Miembro{{ID: "ana", Nombre: "Ana Ruiz"}}
		sa.pintarMensajes()
		sa.P.Canal.Historial.Refresh()
		guardar("sala-markdown-" + n + ".png")
		sa.AbrirPanel(PanelHilos)
		sa.seguidos = []api.RoomsThreadSummary{
			{Root: sa.mensajes[0], LastActivity: time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC), UnreadReplies: 2},
			{
				Root: api.RoomsMessage{
					ID: "h2", ChannelID: "d1", AuthorSub: "beto", AuthorName: "Beto", ReplyCount: 1,
					Body: strings.Repeat("Un texto largo que ocupa más de dos líneas en el panel de hilos. ", 3),
				},
				LastActivity: time.Date(2026, 9, 21, 18, 40, 0, 0, time.UTC),
			},
		}
		sa.seguidosSig = "x"
		sa.pintarSeguidos()
		guardar("sala-hilos-" + n + ".png")
		sa.cerrarDerecho()
		sa.borradores[ambito{canal: "c1"}] = "Lo dejo para mañana, que hoy no llego"
		sa.borradores[ambito{canal: "d1", raiz: "x"}] = strings.Repeat("Una respuesta larga a medio escribir. ", 4)
		sa.contarBorradores()
		sa.AbrirPanel(PanelBorradores)
		guardar("sala-borradores-" + n + ".png")
		sa.cerrarDerecho()
	}
}

// servicioConHilo es servicioConDos con respuestas colgando de «vale».
func servicioConHilo() *servicioFalso {
	s := servicioConDos()
	raiz := &s.mensajes["c1"][3]
	raiz.ReplyCount = 2
	s.hilo = api.RoomsThread{
		Root: *raiz, Following: true,
		Replies: []api.RoomsMessage{
			{ID: "r1", AuthorSub: "ana", AuthorName: "Ana", Body: "¿seguro?", CreatedAt: raiz.CreatedAt.Add(time.Minute)},
			{ID: "r2", AuthorSub: "beto", AuthorName: "Beto", Body: "Sí, ya está.", CreatedAt: raiz.CreatedAt.Add(2 * time.Minute)},
		},
	}
	return s
}

// TestElHiloSeAbreALaDerechaYContesta es `openThread` y `handleSendReply`: el hilo
// ocupa los 308 de la derecha y el canal se queda con el resto; contestar entra en el
// hilo; y cerrarlo devuelve el ancho entero al canal. En ventana estrecha el hilo tapa
// el canal, que es el `max-md:hidden` de la web.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con el panel a 20rem, el hilo va en x =
// 740 con 280 de ancho y la cabecera del canal mide 740 y no 712. Deshecho.
func TestElHiloSeAbreALaDerechaYContesta(t *testing.T) {
	s := servicioConHilo()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal
	raiz := c.Historial.Mensajes[3]
	if raiz.Respuestas != 2 {
		t.Fatalf("la raíz llega con %d respuestas", raiz.Respuestas)
	}
	c.Historial.AlAbrirHilo(raiz)

	h := sa.hilo
	if c.Derecho != h || h.Estado != Listo || len(h.Respuestas) != 2 || !h.Siguiendo {
		t.Fatalf("el hilo no se abrió entero: derecho %v, estado %v, %d respuestas, siguiendo %v",
			c.Derecho != nil, h.Estado, len(h.Respuestas), h.Siguiendo)
	}
	// 352 es --rooms-rhs-width (22rem) con el rem de 16. Se escribe el número y no la
	// constante: un guardián que compara la implementación consigo misma pasa siempre.
	const rhs = 352
	anchoCanal := float32(1280) - tema.AnchoLateral
	if x, w := h.Position().X, h.Size().Width; x != anchoCanal-rhs || w != rhs {
		t.Errorf("el hilo va en x = %v con %v de ancho y en la web es el último `22rem` (352)", x, w)
	}
	if w := c.Cabecera.Size().Width; w != anchoCanal-rhs {
		t.Errorf("la cabecera del canal mide %v con el hilo abierto y le quedan %v", w, anchoCanal-rhs)
	}

	test.WidgetRenderer(h.Compositor)
	h.Compositor.PonerTexto("Gracias")
	h.Compositor.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("Reply(m4,Gracias)") || len(h.Respuestas) != 3 || h.Compositor.Texto() != "" {
		t.Fatalf("contestar: %v, %d respuestas, caja %q", s.llamadas, len(h.Respuestas), h.Compositor.Texto())
	}
	h.AlSeguir()
	if h.Siguiendo || !s.pidio("Follow(m4,no)") {
		t.Errorf("dejar de seguir: siguiendo %v, %v", h.Siguiendo, s.llamadas)
	}
	h.AlCerrar()
	if c.Derecho != nil || c.Cabecera.Size().Width != anchoCanal {
		t.Errorf("cerrado el hilo, el canal mide %v y tenía que volver a %v", c.Cabecera.Size().Width, anchoCanal)
	}
}

// TestElHiloTapaElCanalEnElTelefono: a 360 el panel ocupa la columna entera.
func TestElHiloTapaElCanalEnElTelefono(t *testing.T) {
	sa, _, _ := salaEn(t, servicioConHilo(), 360)
	sa.Arrancar(context.Background())
	sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
	c := sa.P.Canal
	if c.Cabecera.Visible() || !sa.hilo.Visible() || sa.hilo.Size().Width != c.Size().Width {
		t.Errorf("a 360 el hilo mide %v de %v y la cabecera del canal se ve: %v",
			sa.hilo.Size().Width, c.Size().Width, c.Cabecera.Visible())
	}
}

// TestUnHiloQueNoCargaLoDice: el aviso con su «Reintentar», que vuelve a pedir.
func TestUnHiloQueNoCargaLoDice(t *testing.T) {
	s := servicioConHilo()
	s.errHilo = &api.Error{Status: 500}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirHilo(sa.P.Canal.Historial.Mensajes[3])
	if sa.hilo.Estado != Error {
		t.Fatalf("con el hilo caído el panel queda en %v", sa.hilo.Estado)
	}
	s.errHilo = nil
	sa.hilo.AlReintentar()
	if sa.hilo.Estado != Listo || len(sa.hilo.Respuestas) != 2 {
		t.Errorf("reintentar deja el hilo en %v con %d respuestas", sa.hilo.Estado, len(sa.hilo.Respuestas))
	}
}

// TestLaCabeceraEstrechaLlevaHamburguesa: por debajo de `md` la banda lleva la
// hamburguesa delante y solo «Ponme al día», buscar y llamar; lo demás es
// `max-md:hidden`. La hamburguesa saca el carril como cajón, y elegir canal lo recoge.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin esconder las acciones salen 7 en vez
// de 2. Deshecho. El «Aa» del compositor también cabe en la tira desplazable móvil.
func TestLaCabeceraEstrechaLlevaHamburguesa(t *testing.T) {
	sa, _, _ := salaEn(t, servicioConDos(), 360)
	sa.Arrancar(context.Background())
	cab := sa.P.Canal.Cabecera
	if !cab.Estrecha || cab.menu == nil || len(cab.acciones) != 2 {
		t.Fatalf("a 360 la banda: estrecha %v, hamburguesa %v, %d acciones (son 2)",
			cab.Estrecha, cab.menu != nil, len(cab.acciones))
	}
	if cab.nombre.Position().X+ui.Medir("equipo", escalonNombre, ui.SemiNegrita).Width > cab.acciones[0].Position().X {
		t.Error("el nombre del canal no cabe delante de las acciones")
	}
	// El «Aa» también se alcanza en el teléfono, sin desplazar Enviar fuera.
	if n := len(sa.P.Canal.Compositor.botones); n != 5 || sa.P.Canal.Compositor.SinFormato {
		t.Errorf("a 360 la barra del compositor lleva %d botones y son 5, incluido «Aa»", n)
	}
	test.Tap(cab.menu)
	if !sa.P.armazon.CajonAbierto || !sa.P.armazon.Menu.Visible() {
		t.Fatal("la hamburguesa no saca el carril")
	}
	sa.Elegir(sa.P.Lateral.Canales[1])
	if sa.P.armazon.CajonAbierto {
		t.Error("elegir un canal no recoge el cajón")
	}
}

// TestPresenciaDe traduce el estado del servicio al punto del carril.
func TestPresenciaDe(t *testing.T) {
	casos := map[api.EstadoPresencia]ui.Presencia{
		api.EnLinea: ui.EnLinea, api.Ausente: ui.Ausente, api.NoMolestar: ui.NoMolestar,
		api.Desconectado: ui.Desconectado, "": ui.SinPresencia, "raro": ui.SinPresencia,
	}
	for e, quiero := range casos {
		if got := PresenciaDe(e); got != quiero {
			t.Errorf("PresenciaDe(%q) = %v y es %v", e, got, quiero)
		}
	}
}

// TestElMenuDeUnMensajeEsElDeLaWeb es `menuItems`: fijar, guardar, seguir, copiar el texto,
// copiar el enlace y marcar como no leído, y en los PROPIOS editar y borrar tras un
// separador. Fijar se ve en la chincheta de la cabecera al momento.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin separar los propios, el menú de un
// mensaje mío sale sin «Editar» ni «Borrar». Deshecho.
func TestElMenuDeUnMensajeEsElDeLaWeb(t *testing.T) {
	s := servicioConDos()
	s.fijados = []api.RoomsMessage{{ID: "m1"}}
	s.mensajes["c1"] = append(s.mensajes["c1"], mensajeMio())
	sa, _ := salaDePrueba(t, s)
	sa.Apex = "sytue.eu"
	var copiado string
	sa.Copiar = func(x string) { copiado = x }
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial

	rotulos := func(es []fyne.CanvasObject) []string {
		var out []string
		for _, e := range es {
			switch x := e.(type) {
			case *ui.ElementoMenu:
				out = append(out, x.Texto)
			case *ui.SeparadorMenu:
				out = append(out, "—")
			}
		}
		return out
	}
	ajeno, mio := h.Mensajes[0], h.Mensajes[len(h.Mensajes)-1]
	if got := strings.Join(rotulos(h.Menu(ajeno)), " | "); got != "Desfijar | Guardar | Seguir el hilo | Copiar el texto | Copiar enlace | "+
		"Guardar en Sytue Notepads | Reenviar… | Marcar como no leído" {
		t.Errorf("el menú de un mensaje ajeno (ya fijado) es %q", got)
	}
	// «Visto por» solo en los propios. El rótulo es el de la web tal cual: la clave es
	// un plural y la web la pide sin `count` (ver el informe).
	if got := strings.Join(rotulos(h.Menu(mio)), " | "); !strings.HasSuffix(got, "Marcar como no leído | "+sa.P.T.T("message.readersTitle", nil)+" | — | Editar | Borrar") {
		t.Errorf("el menú de un mensaje propio es %q", got)
	}
	if sa.P.Canal.Cabecera.Fijados != 1 {
		t.Errorf("la chincheta dice %d y hay un fijado", sa.P.Canal.Cabecera.Fijados)
	}
	test.Tap(h.Menu(mio)[0].(*ui.ElementoMenu))
	if !s.pidio("Pin(mio,sí)") || sa.P.Canal.Cabecera.Fijados != 2 {
		t.Errorf("fijar: %v, chincheta %d", s.llamadas, sa.P.Canal.Cabecera.Fijados)
	}
	test.Tap(h.Menu(mio)[4].(*ui.ElementoMenu))
	// Con el prefijo `/rooms` de la app (`api.AppURL`): la web lo escribe sin él
	// (`messageLink` usa `location.origin` a secas) y un enlace profundo sin prefijo da
	// el 404 de Astro. Está dicho en el informe.
	if copiado != "https://rooms.sytue.eu/rooms/channels/c1?message=mio" {
		t.Errorf("el enlace copiado es %q", copiado)
	}
	// Un fallo se deshace en silencio, como `togglePin`.
	s.errAccion = &api.Error{Status: 500}
	sa.Fijar(mio, false)
	if !sa.fijados["mio"] || sa.P.Canal.Cabecera.Fijados != 2 {
		t.Error("desfijar con el servicio caído no se deshizo")
	}
}

// TestEditarYBorrarUnMensajePropio: la caja de editar sustituye al texto, Intro guarda
// —el texto nuevo y «(editado)» se ven ya— y Escape cancela; borrar lo quita cuando
// el servicio lo confirma.
func TestEditarYBorrarUnMensajePropio(t *testing.T) {
	s := servicioConDos()
	s.mensajes["c1"] = append(s.mensajes["c1"], mensajeMio())
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	mio := h.Mensajes[len(h.Mensajes)-1]

	sa.Editar(mio)
	f := filaDe(t, h, "mio")
	if !f.Editando || f.edicion == nil {
		t.Fatal("editar no pone la caja en la fila")
	}
	if got, quiero := MedidaFilaMensaje(f, 900).Height, AireFila+2*RellenoFila+tema.TextoSM.Linea+AireCuerpo+MedidaEdicion(1).Height; got != quiero {
		t.Errorf("la fila en edición mide %v y son %v", got, quiero)
	}
	test.WidgetRenderer(f.edicion)
	f.edicion.area.PonerTexto("lo mío, corregido")
	f.edicion.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("Edit(mio,lo mío, corregido)") || h.EditandoID != "" {
		t.Fatalf("Intro no guardó: %v, editando %q", s.llamadas, h.EditandoID)
	}
	if m := h.Mensajes[len(h.Mensajes)-1]; m.Cuerpo != "lo mío, corregido" || !m.Editado {
		t.Errorf("tras editar el mensaje es %+v", m)
	}

	sa.Editar(h.Mensajes[len(h.Mensajes)-1])
	f = filaDe(t, h, "mio")
	test.WidgetRenderer(f.edicion)
	f.edicion.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if h.EditandoID != "" || strings.Count(strings.Join(s.llamadas, " "), "Edit(") != 1 {
		t.Errorf("Escape no canceló sin mandar nada: editando %q, %v", h.EditandoID, s.llamadas)
	}

	sa.Borrar(mio)
	if !s.pidio("Delete(mio)") || h.Mensajes[len(h.Mensajes)-1].ID == "mio" {
		t.Errorf("borrar: %v", s.llamadas)
	}
}

// TestLaBarraDeUnMensajeSaleAlPasarPorEncima: la barra de acciones está apagada y se
// enciende con el puntero sobre la fila, sigue mientras el puntero pasa a la barra y se va
// al salir. Mide lo que suman sus seis botones de 24,5 con `p-0.5 gap-0.5` y borde.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin enseñarla en el `hover`, «con el
// puntero encima no sale la barra». Deshecho.
func TestLaBarraDeUnMensajeSaleAlPasarPorEncima(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Arrancar(context.Background())
	f := filaDe(t, sa.P.Canal.Historial, "m1")
	b := f.barra
	if b == nil || !b.Apagada {
		t.Fatal("la fila no lleva barra, o se ve sin pasar por encima")
	}
	f.MouseIn(nil)
	if b.Apagada {
		t.Fatal("con el puntero encima no sale la barra")
	}
	f.MouseOut()
	b.MouseIn(&desktop.MouseEvent{})
	if b.Apagada {
		t.Error("al pasar de la fila a su barra, la barra se esconde debajo del puntero")
	}
	b.MouseOut()
	if !b.Apagada {
		t.Error("al salir de la barra sigue puesta")
	}
	// Siete botones: las cuatro rápidas, el selector de emoji, «Responder» y el «…».
	if got := b.MinSize(); got.Width != 214 || got.Height != 34 {
		t.Errorf("la barra mide %v y son 214 × 34", got)
	}
}

// filaDe busca la fila de ese mensaje en el historial ya pintado.
func filaDe(t *testing.T, h *Historial, id string) *FilaMensaje {
	t.Helper()
	sc, ok := h.cuerpo.(*container.Scroll)
	if !ok {
		t.Fatal("el historial no es un desplazador")
	}
	for _, o := range sc.Content.(*fyne.Container).Objects {
		if f, es := o.(*FilaMensaje); es && f.Mensaje.ID == id {
			return f
		}
	}
	t.Fatalf("no hay fila para %q", id)
	return nil
}

// mensajeMio es un mensaje de quien mira, al final del canal.
func mensajeMio() api.RoomsMessage {
	return api.RoomsMessage{
		ID: "mio", AuthorSub: "yo", AuthorName: "Yo", Body: "lo mío",
		CreatedAt: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
	}
}

// TestLosPanelesDeListaSeAbrenYSeCierran: «Fijados» y «Guardados» ocupan la derecha
// con su lista, el botón de la cabecera queda pulsado, y volver a pulsarlo lo cierra.
// Los guardados dicen de qué canal es cada uno.
func TestLosPanelesDeListaSeAbrenYSeCierran(t *testing.T) {
	s := servicioConDos()
	s.fijados = []api.RoomsMessage{{ID: "m1", ChannelID: "c1", AuthorSub: "ana", AuthorName: "Ana", Body: "hola"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal

	sa.AbrirPanel(PanelFijados)
	l := sa.lista
	if c.Derecho != l || l.Estado != Listo || len(l.Mensajes) != 1 || c.Cabecera.Panel != PanelFijados {
		t.Fatalf("fijados: derecho %v, estado %v, %d mensajes, pulsado %q", c.Derecho == l, l.Estado, len(l.Mensajes), c.Cabecera.Panel)
	}
	if l.Titulo != "Fijados" || l.Subtitulo != "#equipo" || l.RotuloCerrar != "Cerrar Fijados" {
		t.Errorf("la banda dice %q / %q / %q", l.Titulo, l.Subtitulo, l.RotuloCerrar)
	}
	sa.AbrirPanel(PanelFijados)
	if c.Derecho != nil || c.Cabecera.Panel != "" {
		t.Fatal("volver a pulsar «Fijados» no cierra el panel")
	}

	sa.AbrirPanel(PanelGuardados)
	f := sa.lista.fila(Mensaje{ID: "x", CanalID: "c1", Autor: Autor{Nombre: "Ana"}})
	if !f.ConCanal || f.rotuloCanal() != "#equipo" {
		t.Errorf("un guardado no dice su canal: %v %q", f.ConCanal, f.rotuloCanal())
	}
	if f.AlQuitar == nil || f.RotuloQuitar != "Quitar de guardados" || f.AlCopiar == nil {
		t.Error("la fila de un guardado no lleva «copiar el enlace» y «quitar»")
	}
	// El hilo y los paneles se excluyen.
	sa.AbrirHilo(c.Historial.Mensajes[0])
	if sa.lista != nil || c.Cabecera.Panel != "" || c.Derecho != sa.hilo {
		t.Error("abrir un hilo no cierra el panel de guardados")
	}
}

// TestLaBusquedaEsperaYPregunta: con menos de dos letras no se pregunta; con más se
// espera 250 ms sin teclear y la línea de estado cuenta los resultados.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el mínimo de dos letras y sin pasar
// la consulta por la caja, «la búsqueda no espera». Deshecho.
func TestLaBusquedaEsperaYPregunta(t *testing.T) {
	s := servicioConDos()
	s.hallados = []api.RoomsMessage{{ID: "m1", ChannelID: "c1", AuthorSub: "ana", AuthorName: "Ana", Body: "hola"}}
	sa, relojes := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirPanel(PanelBusqueda)
	l := sa.lista
	test.WidgetRenderer(l)

	l.PonerConsulta("h")
	if relojes[EsperaBusqueda] != nil || l.textoEstado() != "" {
		t.Fatal("con una letra ya se programa la búsqueda")
	}
	l.PonerConsulta("hola")
	espera := relojes[EsperaBusqueda]
	if espera == nil || l.Estado != Cargando || l.textoEstado() != "Buscando…" || s.pidio("Search(hola)") {
		t.Fatalf("la búsqueda no espera: reloj %v, estado %v, %q", espera != nil, l.Estado, l.textoEstado())
	}
	espera()
	if !s.pidio("Search(hola)") || len(l.Mensajes) != 1 || l.textoEstado() != "1 resultado" {
		t.Errorf("tras la espera: %v, %d resultados, %q", s.llamadas, len(l.Mensajes), l.textoEstado())
	}
	sa.VerArchivos()
	if sa.lista.Consulta() != "in:#equipo has:file " {
		t.Errorf("«Archivos del canal» busca %q", sa.lista.Consulta())
	}
}

// TestIrAUnMensajeLoResaltaSinDarPorLeido es `goToMessage`: la ventana centrada en el
// mensaje, resaltado 2,5 s, «Ir a mensajes recientes» y el canal SIN dar por leído; la
// barra vuelve al final y entonces sí.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin repintar el resalte al montar la
// fila, la fila está marcada pero su fondo sale transparente. Lo encontró la captura,
// no el guardián, que solo miraba la bandera; ahora mira el color. Deshecho.
func TestIrAUnMensajeLoResaltaSinDarPorLeido(t *testing.T) {
	s := servicioConDos()
	sa, relojes := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	leidos := func() int { return strings.Count(strings.Join(s.llamadas, " "), "MarkRead(") }
	antes := leidos()

	sa.IrAMensaje("c1", "m1")
	c := sa.P.Canal
	if !s.pidio("Around(c1,m1)") || c.Historial.Resaltado != "m1" || !c.ViendoHistorial || leidos() != antes {
		t.Fatalf("saltar: %v, resaltado %q, viendo historial %v", s.llamadas, c.Historial.Resaltado, c.ViendoHistorial)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if f := filaDe(t, c.Historial, "m1"); !f.Resaltada || f.fondo.FillColor != tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v)) {
		t.Errorf("la fila del mensaje no sale resaltada: %v, fondo %v", f.Resaltada, f.fondo.FillColor)
	}
	if !c.alFinal.Visible() {
		t.Error("no sale «Ir a mensajes recientes»")
	}
	relojes[DuracionResalte]()
	if c.Historial.Resaltado != "" || filaDe(t, c.Historial, "m1").Resaltada {
		t.Error("a los 2,5 s sigue resaltado")
	}
	test.Tap(c.alFinal)
	if c.ViendoHistorial || c.alFinal.Visible() || leidos() != antes+1 {
		t.Errorf("volver al final: viendo historial %v, %v", c.ViendoHistorial, s.llamadas)
	}
}

// TestIrAUnCanalFiltraYAbre es el conmutador (`ordenarCanales` y `QuickSwitcher`): sin
// acentos ni mayúsculas, primero lo que EMPIEZA por lo escrito, luego lo que lo
// contiene, lo pendiente por delante, e Intro abre el señalado.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el peso del prefijo, «fact» pone
// primero «Pre-facturas» (tiene sin leer) en vez de «Facturación». Deshecho.
func TestIrAUnCanalFiltraYAbre(t *testing.T) {
	canales := []Canal{
		{ID: "a", Nombre: "Ventas"},
		{ID: "b", Nombre: "Facturación"},
		{ID: "c", Nombre: "Pre-facturas", SinLeer: 2},
		{ID: "d", Nombre: "facturas viejas"},
	}
	nombres := func(cs []Canal) string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.Nombre)
		}
		return strings.Join(out, " | ")
	}
	if got := nombres(OrdenarConmutador("es", canales, "  FACTURACION ")); got != "Facturación" {
		t.Errorf("«facturacion» encuentra %q", got)
	}
	if got := nombres(OrdenarConmutador("es", canales, "fact")); got != "Facturación | facturas viejas | Pre-facturas" {
		t.Errorf("«fact» ordena %q", got)
	}
	if got := nombres(OrdenarConmutador("es", canales, "")); got != "Pre-facturas | Facturación | facturas viejas | Ventas" {
		t.Errorf("sin escribir ordena %q", got)
	}

	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Arrancar(context.Background())
	sa.AbrirConmutador()
	con := sa.conmutador
	if con == nil || len(con.Filas()) != 2 {
		t.Fatalf("el conmutador no se abrió con los dos canales: %+v", con)
	}
	test.WidgetRenderer(con)
	con.Caja().PonerTexto("ana")
	if len(con.Filas()) != 1 || con.Filas()[0].ID != "d1" {
		t.Fatalf("«ana» deja %+v", con.Filas())
	}
	con.Caja().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if sa.P.Lateral.CanalID != "d1" {
		t.Errorf("Intro abre %q y el señalado era el directo", sa.P.Lateral.CanalID)
	}
}

// TestCrearCanalEsElDeLaWeb es `CreateChannelModal`: el slug que enseña la ayuda, el
// botón apagado sin nombre, el 409 junto al campo y, al crear, el canal abierto. La
// «ñ» no es ni letra ASCII ni separador y se cae, igual que en `channelSlug`.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el caso del 409, el duplicado sale
// como fallo genérico debajo y no junto al nombre. Deshecho.
func TestCrearCanalEsElDeLaWeb(t *testing.T) {
	for _, caso := range []struct{ nombre, quiero string }{
		{"  Diseño de Producto ", "diseo-de-producto"}, {"a__b..c", "a-b-c"}, {"--x--", "x"}, {"ñ", ""},
	} {
		if got, nombre, quiero := SlugCanal(caso.nombre), caso.nombre, caso.quiero; got != quiero {
			t.Errorf("SlugCanal(%q) = %q y la web da %q", nombre, got, quiero)
		}
	}
	s := servicioConDos()
	s.errCrear = &api.Error{Status: 409}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirCrearCanal()
	f := sa.crearCanal
	test.WidgetRenderer(f)
	if !f.BotonCrear().Inerte {
		t.Error("sin nombre el botón de crear está encendido")
	}
	f.Nombre().PonerTexto("Ventas Sur")
	if f.BotonCrear().Inerte {
		t.Fatal("con nombre el botón sigue apagado")
	}
	f.Enviar()
	if f.ErrorNombre != "Ya existe un canal con ese nombre." || f.Fallo || f.Ocupado {
		t.Fatalf("el 409 deja %q, fallo %v, ocupado %v", f.ErrorNombre, f.Fallo, f.Ocupado)
	}
	s.errCrear = nil
	f.privado.Tapped(nil)
	f.Enviar()
	if !s.pidio("CreateChannel(t1,Ventas Sur,,privado)") || sa.P.Lateral.CanalID != "nuevo" {
		t.Errorf("crear: %v, abierto %q", s.llamadas, sa.P.Lateral.CanalID)
	}
}

// TestExplorarBuscaYUneSinAbrir es `BrowseChannelsModal`: pregunta al abrir (tras los
// 250 ms), un fallo ofrece reintentar, y «Unirse» suma el canal al carril sin abrirlo
// ni cerrar el diálogo.
//
// VISTO EN ROJO (2026-09-23, en el frontal): entrando en el canal al unirse (como al
// crearlo), el abierto pasa a ser «v1». Deshecho.
func TestExplorarBuscaYUneSinAbrir(t *testing.T) {
	s := servicioConDos()
	s.explorables = []api.RoomsChannel{{ID: "v1", Name: "ventas", Kind: "public", Purpose: "Pedidos y clientes"}}
	s.errExplorar = &api.Error{Status: 500}
	sa, relojes := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirExplorar()
	e := sa.explorar
	test.WidgetRenderer(e)
	if e.Estado != Cargando || relojes[EsperaBusqueda] == nil {
		t.Fatalf("al abrir no espera antes de preguntar: %v", e.Estado)
	}
	relojes[EsperaBusqueda]()
	if e.Estado != Error {
		t.Fatalf("con el servicio caído queda en %v", e.Estado)
	}
	s.errExplorar = nil
	e.AlReintentar()
	relojes[EsperaBusqueda]()
	if e.Estado != Listo || len(e.Canales) != 1 || e.Canales[0].Proposito != "Pedidos y clientes" {
		t.Fatalf("reintentar deja %v con %+v", e.Estado, e.Canales)
	}
	abierto := sa.P.Lateral.CanalID
	e.AlUnirse(e.Canales[0])
	if len(e.Canales) != 0 || len(sa.P.Lateral.Canales) != 3 || sa.P.Lateral.CanalID != abierto {
		t.Errorf("unirse: %d en la lista, %d en el carril, abierto %q", len(e.Canales), len(sa.P.Lateral.Canales), sa.P.Lateral.CanalID)
	}
}

// TestNuevoMensajeConAlguienDeFuera es `NewDmModal` con `BuscarPersona` (ADR 0040):
// quien mira no sale en la lista, un correo o un @usuario ofrece buscarlo en Sytue One,
// quien no es del equipo va marcado «otra organización», y abrir entra en el directo
// con el nombre de las personas elegidas.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin marcar a quien no es del equipo,
// Grace entra como una más y la píldora no dice «otra organización». Deshecho.
func TestNuevoMensajeConAlguienDeFuera(t *testing.T) {
	if !PareceIdentificador(" @grace ") || PareceIdentificador("Ana Ruiz") || PareceIdentificador("@") ||
		PareceIdentificador("a@b c") {
		t.Error("PareceIdentificador no es el de la web")
	}
	s := servicioConDos()
	s.equipo = []api.RoomsMember{{Subject: "yo", Name: "Yo"}, {Subject: "ana", Name: "Ana Ruiz"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirNuevoDirecto()
	nd := sa.directo
	test.WidgetRenderer(nd)
	if len(nd.Equipo) != 1 || nd.Equipo[0].ID != "ana" {
		t.Fatalf("la lista del equipo es %+v y quien mira no sale", nd.Equipo)
	}
	nd.Caja().PonerTexto("@nadie")
	nd.AlBuscarPersona("@nadie")
	if nd.ErrorBuscar != "Nadie en Sytue One usa ese correo o @usuario." {
		t.Errorf("un 404 dice %q", nd.ErrorBuscar)
	}
	nd.Caja().PonerTexto("@grace")
	nd.AlBuscarPersona("@grace")
	nd.Conmutar(nd.Equipo[0])
	if len(nd.Elegidas) != 2 || !nd.Elegidas[0].Externa || nd.Caja().Texto() != "" {
		t.Fatalf("elegidas %+v, caja %q", nd.Elegidas, nd.Caja().Texto())
	}
	if nd.BotonAbrir().Texto != "Crear grupo (2)" {
		t.Errorf("el botón dice %q", nd.BotonAbrir().Texto)
	}
	nd.Confirmar()
	if !s.pidio("OpenDirect(t1,g1,ana)") || sa.P.Lateral.CanalID != "dm-g1-ana" || sa.P.Canal.Cabecera.Canal.Nombre != "Grace, Ana Ruiz" {
		t.Errorf("abrir: %v, abierto %q, «%s»", s.llamadas, sa.P.Lateral.CanalID, sa.P.Canal.Cabecera.Canal.Nombre)
	}
}

// TestLosAnterioresEntranArribaSinMoverLaVista es `loadOlder`: acercarse al principio
// pide la página de antes, entra arriba sin repetir y la vista sigue en lo que se
// leía; el separador de no leídos sigue ante SU mensaje; y una página corta dice que
// no hay más.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin desplazar la vista, «quedó en y = 0 y
// el contenido creció 73,5 por arriba»; sin quitar los repetidos, entran 53 y no 52.
// Deshecho.
func TestLosAnterioresEntranArribaSinMoverLaVista(t *testing.T) {
	s := servicioConDos()
	base := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	pagina := make([]api.RoomsMessage, 0, PorPagina)
	for i := range PorPagina {
		pagina = append(pagina, api.RoomsMessage{
			ID: "p" + strconv.Itoa(i), AuthorSub: "ana", AuthorName: "Ana", Body: "línea", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	s.mensajes["c1"] = pagina
	s.pend = []api.RoomsUnread{{ChannelID: "c1", Unread: 2}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	separador := func() string {
		if h.IndiceNoLeidos < 0 || h.IndiceNoLeidos >= len(h.Mensajes) {
			return ""
		}
		return h.Mensajes[h.IndiceNoLeidos].ID
	}
	if !h.HayAnteriores || separador() != "p48" {
		t.Fatalf("página llena: hay anteriores %v, separador ante %q", h.HayAnteriores, separador())
	}
	// La página de antes: 3 mensajes (menos de una página) y uno repetido.
	s.mensajes["c1"] = []api.RoomsMessage{
		{ID: "v1", AuthorSub: "beto", Body: "viejo", CreatedAt: base.Add(-3 * time.Hour)},
		{ID: "v2", AuthorSub: "beto", Body: "viejo", CreatedAt: base.Add(-2 * time.Hour)},
		pagina[0],
	}
	sc := h.cuerpo.(*container.Scroll)
	sc.Offset.Y = 0
	antes := sc.Content.MinSize().Height
	sc.OnScrolled(sc.Offset)
	if len(h.Mensajes) != PorPagina+2 || h.Mensajes[0].ID != "v1" || h.HayAnteriores {
		t.Fatalf("tras cargar anteriores: %d mensajes, primero %q, hay más %v", len(h.Mensajes), h.Mensajes[0].ID, h.HayAnteriores)
	}
	if separador() != "p48" {
		t.Errorf("el separador se movió a %q", separador())
	}
	sc = h.cuerpo.(*container.Scroll)
	if crecio := sc.Content.MinSize().Height - antes; sc.Offset.Y != crecio {
		t.Errorf("la vista quedó en y = %v y el contenido creció %v por arriba", sc.Offset.Y, crecio)
	}
}

// TestTuEstadoCambiaYCaduca es `StatusMenu`: elegir «Ausente» cambia el punto y se
// publica; el estado personalizado con «30 minutos» se borra solo a los 30; y «Marcar
// todo como leído» deja los contadores a cero.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin programar la caducidad, «el estado de
// 30 minutos no se programa para borrarse». Deshecho.
func TestTuEstadoCambiaYCaduca(t *testing.T) {
	ahora := time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local) // miércoles
	if got := CaducaEn(CaducaHoy, ahora); !got.Equal(time.Date(2026, 9, 24, 0, 0, 0, 0, time.Local)) {
		t.Errorf("«hoy» caduca %v", got)
	}
	if got := CaducaEn(CaducaSemana, ahora); !got.Equal(time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)) {
		t.Errorf("«esta semana» caduca %v y es la medianoche del lunes", got)
	}
	if !CaducaEn(NoCaduca, ahora).IsZero() {
		t.Error("«no quitar» caduca")
	}

	s := servicioConDos()
	sa, relojes := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return ahora }
	var publicado []string
	sa.AlPonerPresencia = func(_ context.Context, e api.EstadoPresencia, texto string) {
		publicado = append(publicado, string(e)+"|"+texto)
	}
	sa.Arrancar(context.Background())
	l := sa.P.Lateral

	sa.CambiarPresencia(api.Ausente, "", false, time.Time{})
	if l.Presencia != ui.Ausente || publicado[len(publicado)-1] != "away|" {
		t.Fatalf("ausente: %v, %v", l.Presencia, publicado)
	}
	sa.AbrirEstado(sa.P.Objeto())
	b := sa.bloqueEstado
	b.Caja().PonerTexto("En una reunión")
	b.Plazo().Elegida = int(Caduca30m)
	b.Caja().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if l.EstadoPersonal != "En una reunión" || publicado[len(publicado)-1] != "away|En una reunión" {
		t.Fatalf("estado: %q, %v", l.EstadoPersonal, publicado)
	}
	caduca := relojes[30*time.Minute]
	if caduca == nil {
		t.Fatal("el estado de 30 minutos no se programa para borrarse")
	}
	caduca()
	if l.EstadoPersonal != "" || l.Presencia != ui.Ausente {
		t.Errorf("a los 30 minutos: estado %q, presencia %v", l.EstadoPersonal, l.Presencia)
	}

	sa.MarcarTodoLeido()
	for _, c := range l.Canales {
		if c.SinLeer+c.Menciones != 0 {
			t.Errorf("tras marcar todo, %s sigue con %d", c.Nombre, c.SinLeer)
		}
	}
	if !s.pidio("MarkRead(d1)") {
		t.Errorf("no se dio por leído el directo pendiente: %v", s.llamadas)
	}
}

// TestLosMiembrosDelCanal es `MembersPanel`: la pila de caras de la cabecera sale del
// roster; quien administra el canal ve «Añadir personas» y el «…» de los demás (no el
// suyo); añadir a alguien del equipo lo suma y lo dice; y un 403 dice por qué no.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin contar el rol de admin en el canal, a
// una admin del canal que no lo es del equipo el panel no la deja administrar. Deshecho.
func TestLosMiembrosDelCanal(t *testing.T) {
	s := servicioConDos()
	s.roster = []api.RoomsMember{
		{Subject: "yo", Name: "Yo", Role: "admin"},
		{Subject: "ana", Name: "Ana Ruiz", Role: "member"},
		{Subject: "gr", Name: "Grace", Role: "member", External: true},
	}
	s.equipo = []api.RoomsMember{{Subject: "yo", Name: "Yo"}, {Subject: "ana", Name: "Ana Ruiz"}, {Subject: "be", Name: "Beto"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	cab := sa.P.Canal.Cabecera
	if cab.Miembros != 3 || strings.Join(cab.Caras, ",") != "Ana Ruiz,Grace,Yo" {
		t.Fatalf("la cabecera: %d miembros, caras %v", cab.Miembros, cab.Caras)
	}
	sa.AbrirPanel(PanelMiembros)
	p := sa.miembros
	test.WidgetRenderer(p)
	if p == nil || !p.Gestiona || p.anadir == nil || cab.Panel != PanelMiembros {
		t.Fatalf("el panel no deja administrar a una admin del canal")
	}
	if texto, _ := p.textoEstado(); texto != "3 personas" {
		t.Errorf("el recuento dice %q", texto)
	}
	if f := p.fila(Miembro{ID: "yo", Nombre: "Yo", Admin: true}).(*FilaMiembro); f.Menu != nil || f.Tu != "(tú)" {
		t.Error("la fila propia lleva «…» o no dice «(tú)»")
	}
	if f := p.fila(Miembro{ID: "gr", Nombre: "Grace", Externo: true}).(*FilaMiembro); f.Menu == nil ||
		strings.Join(f.Insignias, ",") != "Otra organización" {
		t.Errorf("la fila de Grace: menú %v, insignias %v", f.Menu != nil, f.Insignias)
	}
	p.CambiarModo(true)
	if vis := p.Visibles(); len(vis) != 1 || vis[0].ID != "be" {
		t.Fatalf("para añadir quedan %+v y es solo Beto", vis)
	}
	p.AlAnadir(p.Visibles()[0])
	if !s.pidio("AddChannelMember(c1,be,Beto)") || cab.Miembros != 4 || p.Aviso != "Beto añadido" {
		t.Errorf("añadir: %v, %d miembros, aviso %q", s.llamadas, cab.Miembros, p.Aviso)
	}
	s.errAccion = &api.Error{Status: 403}
	p.AlQuitar(Miembro{ID: "ana", Nombre: "Ana Ruiz"})
	if p.ErrorAccion != "No tienes permiso para hacer eso." || cab.Miembros != 4 {
		t.Errorf("un 403 deja %q y %d miembros", p.ErrorAccion, cab.Miembros)
	}
}

// TestElNombreDelCanalAbreSuMenu: en un canal el nombre es un botón de 24 de alto con
// el chevron, que abre el menú con Miembros, Fijados, Guardados, Programados y los ajustes de
// notificación; en un directo es
// texto. El texto del nombre no se mueve por ser botón (`-mx-1.5`).
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `-mx-1.5`, el disparador corre el
// nombre de 52,5 a 57,75. Deshecho.
func TestElNombreDelCanalAbreSuMenu(t *testing.T) {
	s := servicioConDos()
	s.roster = []api.RoomsMember{{Subject: "yo", Name: "Yo"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	cab := sa.P.Canal.Cabecera
	d := cab.disparador
	if d == nil || d.Size().Height != 24 {
		t.Fatalf("el nombre de un canal no es un disparador de 24: %+v", d)
	}
	if got, quiero := d.Position().X+CanalonDisparador, cab.nombre.Position().X; got > quiero+1 || got < quiero-1 {
		t.Errorf("el texto del nombre empieza en %v y el disparador lo corre a %v", quiero, got)
	}
	test.Tap(d)
	if sa.menuCanal == nil || len(sa.menuCanal.Elementos) != 7 { // «Ajustes de notificación», — y «Salir del canal»
		t.Fatalf("el menú del canal: %+v", sa.menuCanal)
	}
	test.Tap(sa.menuCanal.Elementos[0].(*ui.ElementoMenu))
	if sa.miembros == nil {
		t.Error("«Miembros» del menú no abre el panel")
	}
	sa.Elegir(sa.P.Lateral.Canales[1])
	if cab.disparador != nil {
		t.Error("en un directo el nombre sigue siendo un disparador")
	}
}

// TestHilosSeguidos es la vista «Hilos»: se carga al arrancar para la pastilla del
// carril (que cuenta HILOS con no leídas, no respuestas), el carril la abre como panel
// con el #canal de cada uno, «Cargar más» añade sin repetir, abrir uno lo da por visto
// y lo abre con el nombre de SU canal, y una respuesta en vivo lo sube.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `verSeguido` al abrir el hilo, la
// pastilla se quedaba en 1; sin el orden por actividad, «b» seguía debajo. Deshecho.
func TestHilosSeguidos(t *testing.T) {
	s := servicioConDos()
	raiz := func(id, canal string, n int) api.RoomsMessage {
		return api.RoomsMessage{ID: id, ChannelID: canal, AuthorSub: "ana", AuthorName: "Ana", Body: "raíz " + id, ReplyCount: n}
	}
	hora := func(h int) time.Time { return time.Date(2026, 9, 22, h, 0, 0, 0, time.UTC) }
	s.seguidos = []api.RoomsThreadSummary{
		{Root: raiz("a", "d1", 3), LastActivity: hora(10), UnreadReplies: 2},
		{Root: raiz("b", "c1", 1), LastActivity: hora(9)},
	}
	s.sigSeguidos = "2026-09-22T09:00:00Z"
	s.masSeguidos = []api.RoomsThreadSummary{{Root: raiz("b", "c1", 1), LastActivity: hora(9)}, {Root: raiz("c", "c1", 1), LastActivity: hora(8)}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	if !s.pidio("MyThreads()") || l.HilosSinLeer != 1 {
		t.Fatalf("al arrancar: %v, pastilla %d", s.llamadas, l.HilosSinLeer)
	}

	l.AlAbrirPanel(PanelHilos)
	p := sa.lista
	if p == nil || !p.DeHilos || l.Panel != PanelHilos || len(p.Hilos) != 2 || p.AlCargarMas == nil {
		t.Fatalf("el panel «Hilos»: %+v, carril en %q", p, l.Panel)
	}
	if p.Titulo != "Hilos" || p.Canales["d1"] == "" || p.Hilos[0].SinLeer != 2 || p.Hilos[0].Raiz.Respuestas != 3 {
		t.Errorf("la hoja dice %q, canales %v, primero %+v", p.Titulo, p.Canales, p.Hilos[0])
	}
	p.AlCargarMas()
	if len(sa.seguidos) != 3 || p.AlCargarMas != nil || !s.pidio("MyThreads(2026-09-22T09:00:00Z)") {
		t.Errorf("«Cargar más»: %d hilos, más %v", len(sa.seguidos), p.AlCargarMas != nil)
	}

	p.AlElegir(p.Hilos[0].Raiz)
	if sa.raizHilo != "a" || sa.lista != nil || l.Panel != "" || l.HilosSinLeer != 0 {
		t.Fatalf("abrir un hilo: raíz %q, panel %v, carril %q, pastilla %d", sa.raizHilo, sa.lista != nil, l.Panel, l.HilosSinLeer)
	}
	if sa.hilo.NombreCanal != sa.nombresDeCanal()["d1"] {
		t.Errorf("el hilo de otro canal dice %q", sa.hilo.NombreCanal)
	}

	sa.CerrarHilo()
	sa.alLlegar(api.RoomsEvent{
		Type: "message", ChannelID: "c1",
		Payload: []byte(`{"id":"r9","channel_id":"c1","parent_id":"b","created_at":"2026-09-22T11:00:00Z"}`),
	})
	if h := sa.seguidos[0]; h.Root.ID != "b" || h.UnreadReplies != 1 || h.Root.ReplyCount != 2 || l.HilosSinLeer != 1 {
		t.Errorf("una respuesta en vivo deja primero %+v y la pastilla en %d", h, l.HilosSinLeer)
	}
}

// TestLaFilaDeUnHiloEsLaDeLaWeb: la última actividad va a la derecha (`ml-auto`), el
// texto se corta en dos líneas con «…» (`line-clamp-2`) y debajo van «N respuestas» y
// la pastilla. El alto es la cuenta de MedidaFilaLista.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el recorte, el texto largo daba doce
// líneas. Deshecho.
func TestLaFilaDeUnHiloEsLaDeLaWeb(t *testing.T) {
	test.NewTempApp(t)
	largo := strings.Repeat("palabra ", 60)
	h := ResumenHilo{
		Raiz:      Mensaje{ID: "a", Autor: Autor{Nombre: "Ana"}, Cuerpo: largo, Respuestas: 3},
		Actividad: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC), SinLeer: 2,
	}
	f := NuevaFilaLista(traductor(t), "es", h.Raiz, "equipo")
	f.Resumen = &h
	const ancho = 308
	test.WidgetRenderer(f)
	f.Resize(fyne.NewSize(ancho, MedidaFilaLista(f, ancho).Height))
	lineas := f.lineas(ancho)
	if len(lineas) != LineasResumenHilo || !strings.HasSuffix(lineas[1], "…") {
		t.Fatalf("el texto no se corta en dos líneas: %q", lineas)
	}
	quiero := 2*RellenoFilaHilo + tema.TextoSM.Linea + 2*tema.TextoBase.Linea + AltoPieResumen
	if got := MedidaFilaLista(f, ancho).Height; got != quiero {
		t.Errorf("la fila mide %v y son %v", got, quiero)
	}
	x := CanalonFilaHilo + ColumnaAvatar + HuecoFila
	fin := f.cuando.Position().X + ui.Medir(f.cuando.Text, escalonMeta, ui.Normal).Width
	if d := fin - (x + f.cabe(ancho)); d > 0.5 || d < -0.5 {
		t.Errorf("la actividad acaba en %v y no en el borde de la columna (%v)", fin, x+f.cabe(ancho))
	}
	if f.respuestas.Text != "3 respuestas" || f.sinLeer == nil || f.sinLeer.Texto != "2" {
		t.Errorf("el pie dice %q y la pastilla %+v", f.respuestas.Text, f.sinLeer)
	}
	if f.canal.Text != "#equipo" || !f.canal.Visible() {
		t.Errorf("el canal dice %q", f.canal.Text)
	}
}

// TestSeguirUnMensaje es «Seguir el hilo» del «…» y el «Siguiendo» de la tira: lo que
// está en «Hilos» sale seguido, alternarlo se ve ya y recarga «Hilos», un fallo lo
// deshace, y lo que dice el hilo al abrirse manda sobre la página cargada.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin repintar el canal cuando llega
// «Hilos», «m1» salía sin «Siguiendo» aunque estaba en la lista. Deshecho.
func TestSeguirUnMensaje(t *testing.T) {
	s := servicioConDos()
	s.seguidos = []api.RoomsThreadSummary{{Root: api.RoomsMessage{ID: "m1", ChannelID: "c1"}}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	fila := func() *FilaMensaje {
		f := NuevaFilaMensaje(sa.P.T, sa.P.Locale, h.Mensajes[0], false, nil)
		f.AlAbrirHilo = func(Mensaje) {}
		test.WidgetRenderer(f)
		return f
	}
	if f := fila(); !h.Mensajes[0].Siguiendo || f.altoTira() != AltoEnlaceHilo || f.siguiendo == nil || f.siguiendo.Text != "Siguiendo" {
		t.Fatalf("un hilo de «Hilos» no sale seguido: %+v", h.Mensajes[0])
	}
	elemento := func() *ui.ElementoMenu { return h.Menu(h.Mensajes[0])[2].(*ui.ElementoMenu) }
	if elemento().Texto != "Dejar de seguir" {
		t.Errorf("el «…» de uno seguido dice %q", elemento().Texto)
	}
	s.llamadas = nil
	test.Tap(elemento())
	if !s.pidio("Follow(m1,no)") || h.Mensajes[0].Siguiendo || !s.pidio("MyThreads()") {
		t.Errorf("dejar de seguir: %v, sigue %v", s.llamadas, h.Mensajes[0].Siguiendo)
	}
	s.errAccion = &api.Error{Status: 500}
	test.Tap(elemento())
	if h.Mensajes[0].Siguiendo {
		t.Error("seguir con el servicio caído no se deshizo")
	}
	s.errAccion = nil
	s.hilo = api.RoomsThread{Root: api.RoomsMessage{ID: "m1", ChannelID: "c1"}, Following: true}
	sa.AbrirHilo(h.Mensajes[0])
	if !sa.hilo.Siguiendo || !h.Mensajes[0].Siguiendo {
		t.Error("lo que dice el hilo al abrirse no llega al canal")
	}
}

// TestEscribirAvisaALosDemas es `onTyping`: escribir en la caja manda «escribiendo» al
// canal abierto, como mucho una vez cada 3 s, y un texto en blanco no lo manda.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin la espera de 3 s salían tres avisos
// en tres pulsaciones. Deshecho.
func TestEscribirAvisaALosDemas(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	ahora := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	sa.Ahora = func() time.Time { return ahora }
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	c.PonerTexto("   ")
	if s.pidio("Typing(c1)") {
		t.Fatal("un texto en blanco avisa de que se escribe")
	}
	c.PonerTexto("h")
	c.PonerTexto("ho")
	ahora = ahora.Add(2 * time.Second)
	c.PonerTexto("hol")
	if n := s.veces("Typing(c1)"); n != 1 {
		t.Fatalf("en 2 s salen %d avisos y es uno", n)
	}
	ahora = ahora.Add(EsperaEscribiendo)
	c.PonerTexto("hola")
	if n := s.veces("Typing(c1)"); n != 2 {
		t.Errorf("pasados 3 s salen %d avisos en total y son dos", n)
	}
}

// TestElMenuDeLaFilaDelCanal es `renderChannelRow`: favorito, los tres niveles de aviso
// con el tic en el vigente, mover a las otras categorías y, a quien administra,
// archivar tras confirmarlo. Cada cambio se ve ya y se deshace si el servicio dice que
// no; archivar el abierto abre el siguiente.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin deshacer, un fallo al mover dejaba el
// canal en «Clientes». Deshecho.
func TestElMenuDeLaFilaDelCanal(t *testing.T) {
	s := servicioConDos()
	s.categorias = []api.RoomsCategory{{ID: "k2", Name: "Zeta", SortOrder: 2}, {ID: "k1", Name: "Clientes", SortOrder: 1}}
	s.rolEquipo = "admin"
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	rotulos := func() string {
		var out []string
		for _, e := range sa.menuFila.Elementos {
			switch x := e.(type) {
			case *ui.ElementoMenu:
				r := x.Texto
				if x.Marcada {
					r += " ✓"
				}
				out = append(out, r)
			case *ui.SeparadorMenu:
				out = append(out, "—")
			}
		}
		return strings.Join(out, " | ")
	}
	sa.MenuFila(l.Canales[0], sa.P.Canal.Cabecera)
	if got := rotulos(); got != "Añadir a favoritos | — | Todas las notificaciones ✓ | Solo menciones y palabras clave | "+
		"Silenciar el canal | — | Mover a «Clientes» | Mover a «Zeta» | — | Salir del canal | Archivar canal | Borrar canal" {
		t.Fatalf("el menú de la fila es %q", got)
	}
	test.Tap(sa.menuFila.Elementos[4].(*ui.ElementoMenu))
	if c := l.Canales[0]; !s.pidio("SetNotifyLevel(c1,none)") || c.Nivel != AvisarNada || !c.Silenciado {
		t.Errorf("silenciar: %v, nivel %v, silenciado %v", s.llamadas, c.Nivel, c.Silenciado)
	}
	s.errAccion = &api.Error{Status: 500}
	sa.MoverA(l.Canales[0], "k1")
	if c := l.Canales[0]; c.CategoriaID != "" || c.Nivel != AvisarNada || !s.pidio("SetChannelCategory(c1,k1)") {
		t.Errorf("mover con el servicio caído deja la categoría %q y el nivel %v", c.CategoriaID, c.Nivel)
	}
	s.errAccion = nil
	sa.MoverA(l.Canales[0], "k1")
	sa.MenuFila(l.Canales[0], sa.P.Canal.Cabecera)
	if got := rotulos(); !strings.Contains(got, "Mover a «Zeta» | Quitar de la categoría") || strings.Contains(got, "«Clientes»") {
		t.Errorf("en una categoría el menú dice %q", got)
	}
	// Un directo no se archiva ni se borra para todos: se borra el chat, solo para ti.
	sa.MenuFila(l.Canales[1], sa.P.Canal.Cabecera)
	if got := rotulos(); strings.Contains(got, "Archivar") || strings.Contains(got, "Borrar canal") ||
		strings.Contains(got, "Salir") || !strings.HasSuffix(got, "— | Borrar chat") {
		t.Errorf("el menú de un directo es %q", got)
	}

	sa.Archivar(l.Canales[0])
	if !s.pidio("ArchiveChannel(c1)") || len(l.Canales) != 1 || sa.abierto != "d1" {
		t.Errorf("archivar el abierto: %v, quedan %d, abierto %q", s.llamadas, len(l.Canales), sa.abierto)
	}
	s.errAccion = &api.Error{Status: 403}
	sa.Archivar(l.Canales[0])
	if len(l.Canales) != 1 {
		t.Error("archivar sin permiso no devolvió el canal al carril")
	}
}

// TestSalirYBorrarDesdeElMenu: «Salir del canal» está para cualquiera y «Borrar canal»
// solo para quien administra; los dos confirman antes. Salir comparte camino con
// /leave y un 409 dice que eres el último admin; borrar un canal pide DELETE, «Borrar
// chat» en un directo lo oculta solo para ti; si el servicio dice que no, vuelve al
// carril. Y un canal que borra otro desaparece del carril al llegar la trama.
func TestSalirYBorrarDesdeElMenu(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	tr := sa.P.T
	rotulos := func() []string {
		var out []string
		for _, e := range sa.menuFila.Elementos {
			if x, ok := e.(*ui.ElementoMenu); ok {
				out = append(out, x.Texto)
			}
		}
		return out
	}
	sa.MenuFila(l.Canales[0], sa.P.Canal.Cabecera)
	if got := rotulos(); !slices.Contains(got, "Salir del canal") || slices.Contains(got, "Borrar canal") {
		t.Fatalf("sin administrar, el menú es %q", got)
	}
	sa.PedirSalir(l.Canales[0])
	if sa.pregunta == nil || s.pidio("LeaveChannel(c1)") {
		t.Fatal("«Salir del canal» sale sin confirmar")
	}
	sa.pregunta.Cerrar()

	s.errAccion = &api.Error{Status: 409}
	sa.SalirDe("c1")
	avisos := sa.P.Avisos.Avisos()
	if !s.pidio("LeaveChannel(c1)") || len(l.Canales) != 2 || len(avisos) == 0 ||
		avisos[len(avisos)-1].Titulo != tr.T("channel.lastAdmin", nil) {
		t.Fatalf("salir siendo el último admin: %v, quedan %d", s.llamadas, len(l.Canales))
	}
	s.errAccion = nil

	sa.BorrarCanal(l.Canales[1])
	if !s.pidio("HideDirect(d1)") || s.pidio("DeleteChannel(d1)") || len(l.Canales) != 1 {
		t.Fatalf("«Borrar chat»: %v, quedan %d", s.llamadas, len(l.Canales))
	}
	s.errAccion = &api.Error{Status: 403}
	sa.BorrarCanal(l.Canales[0])
	if !s.pidio("DeleteChannel(c1)") || len(l.Canales) != 1 {
		t.Fatalf("borrar sin permiso no devolvió el canal: %v, quedan %d", s.llamadas, len(l.Canales))
	}
	s.errAccion = nil

	sa.alLlegar(api.RoomsEvent{Type: "channel_deleted", ChannelID: "c1"})
	if len(l.Canales) != 0 || sa.abierto != "" {
		t.Errorf("tras borrarlo otro quedan %d canales y abierto %q", len(l.Canales), sa.abierto)
	}
}

// TestLasCategoriasDelCarril son `toggleGroupCollapse`, `createNewCategory` y
// `categoryMenu`: plegar una por defecto queda en la ventana y una personalizada va al
// servidor (y vuelve si falla); «Nueva categoría» y «Renombrar» preguntan el nombre;
// subir y bajar renumeran desde cero guardando solo las que cambian; y borrar devuelve
// sus canales a su sección, o todo si el servicio dice que no.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin renumerar, subir «Zeta» solo guardaba
// su orden y «Clientes» se quedaba en el mismo puesto. Deshecho.
func TestLasCategoriasDelCarril(t *testing.T) {
	s := servicioConDos()
	s.canales[0].CategoryID = "k1"
	s.categorias = []api.RoomsCategory{{ID: "k1", Name: "Clientes", SortOrder: 0}, {ID: "k2", Name: "Zeta", SortOrder: 1}}
	sa, _, w := salaEn(t, s, 1280)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	cat := func(id string) *Categoria {
		i := slices.IndexFunc(l.Categorias, func(k Categoria) bool { return k.ID == id })
		if i < 0 {
			t.Fatalf("no hay categoría %q: %+v", id, l.Categorias)
		}
		return &l.Categorias[i]
	}

	sa.PlegarSeccion(Seccion{Clave: ClaveDirectos})
	if !l.Plegadas[ClaveDirectos] || s.pidio("UpdateCategory(dms,plegada=true)") {
		t.Errorf("plegar «Mensajes directos»: %v, %v", l.Plegadas, s.llamadas)
	}
	sa.PlegarSeccion(Seccion{Clave: "k1", Categoria: cat("k1")})
	if !cat("k1").Plegada || !s.pidio("UpdateCategory(k1,plegada=true)") {
		t.Errorf("plegar «Clientes»: %+v, %v", *cat("k1"), s.llamadas)
	}
	s.errAccion = &api.Error{Status: 500}
	sa.PlegarSeccion(Seccion{Clave: "k1", Categoria: cat("k1")})
	if !cat("k1").Plegada {
		t.Error("desplegar con el servicio caído no se deshizo")
	}
	s.errAccion = nil

	sec, _ := l.seccion(Seccion{Clave: "k1", Titulo: "Clientes", Categoria: cat("k1")}, tema.Claro)
	if len(sec.Acciones) != 1 || sec.Acciones[0].(*ui.AccionLateral).Nombre != "Acciones de Clientes" {
		t.Fatalf("la categoría no lleva su «…»: %+v", sec.Acciones)
	}
	test.Tap(sec.Acciones[0].(*ui.AccionLateral))
	var rotulos []string
	for _, e := range sa.menuCat.Elementos {
		if x, ok := e.(*ui.ElementoMenu); ok {
			rotulos = append(rotulos, x.Texto)
		}
	}
	if got := strings.Join(rotulos, " | "); got != "Renombrar | Subir | Bajar | Borrar categoría" {
		t.Errorf("el «…» de la categoría es %q", got)
	}

	escribir := func(texto string) {
		t.Helper()
		campo, ok := w.Canvas().Focused().(*ui.Entrada)
		if !ok {
			t.Fatalf("la pregunta no pone el foco en su campo: %T", w.Canvas().Focused())
		}
		campo.PonerTexto(texto)
		campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	}
	sa.NuevaCategoria()
	escribir("  Ventas ")
	if !s.pidio("CreateCategory(Ventas)") || len(l.Categorias) != 3 {
		t.Errorf("«Nueva categoría»: %v, %d categorías", s.llamadas, len(l.Categorias))
	}
	sa.RenombrarCategoria(*cat("k1"))
	escribir("Clientes VIP")
	if cat("k1").Nombre != "Clientes VIP" || !s.pidio("UpdateCategory(k1,nombre=Clientes VIP)") {
		t.Errorf("renombrar: %+v, %v", *cat("k1"), s.llamadas)
	}

	s.llamadas = nil
	sa.MoverCategoria(*cat("k2"), -1)
	if cat("k2").Orden != 0 || cat("k1").Orden != 1 || !s.pidio("UpdateCategory(k2,orden=0)") ||
		!s.pidio("UpdateCategory(k1,orden=1)") || s.veces("UpdateCategory(k-Ventas,orden=2)") != 0 {
		t.Errorf("subir «Zeta»: %v", s.llamadas)
	}

	s.errAccion = &api.Error{Status: 500}
	sa.BorrarCategoria(*cat("k1"))
	if len(l.Categorias) != 3 || l.Canales[0].CategoriaID != "k1" {
		t.Error("borrar con el servicio caído no devolvió la categoría y su canal")
	}
	s.errAccion = nil
	sa.BorrarCategoria(*cat("k1"))
	if !s.pidio("DeleteCategory(k1)") || len(l.Categorias) != 2 || l.Canales[0].CategoriaID != "" {
		t.Errorf("borrar: %v, %d categorías, el canal en %q", s.llamadas, len(l.Categorias), l.Canales[0].CategoriaID)
	}
}

// TestLosBorradores es `lib/drafts.ts` y `DraftsPanel`: lo que viene del servidor sale
// en la caja de su canal sin avisar de que se escribe; cada canal guarda el suyo al
// cambiar (antes el texto se quedaba en la caja y se mandaba al canal siguiente); una
// ráfaga es UNA subida; enviar lo borra; y el panel lleva al hilo con su texto y lo
// descarta.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin restaurar al cambiar de canal, «otra
// cosa» seguía en la caja de «Ana Ruiz»; y sin escribir lo pendiente al crear el área,
// la caja del hilo recién abierto salía vacía. Deshecho.
func TestLosBorradores(t *testing.T) {
	s := servicioConDos()
	raiz := "m4"
	s.borradores = []api.RoomsDraft{{ChannelID: "c1", Body: "a medias"}, {ChannelID: "c1", ParentID: &raiz, Body: "en el hilo"}}
	sa, relojes := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c, l := sa.P.Canal.Compositor, sa.P.Lateral
	if c.Texto() != "a medias" || l.Borradores != 2 || s.pidio("Typing(c1)") {
		t.Fatalf("al arrancar la caja dice %q, %d borradores, %v", c.Texto(), l.Borradores, s.llamadas)
	}
	c.PonerTexto("otra")
	c.PonerTexto("otra cosa")
	relojes[EsperaBorrador]()
	if s.veces("PutDraft(c1,,otra cosa)") != 1 || s.pidio("PutDraft(c1,,otra)") {
		t.Errorf("una ráfaga sube %v", s.llamadas)
	}
	sa.Elegir(l.Canales[1])
	if c.Texto() != "" {
		t.Fatalf("en «%s» la caja trae %q del canal anterior", l.Canales[1].Nombre, c.Texto())
	}
	sa.Elegir(l.Canales[0])
	if c.Texto() != "otra cosa" {
		t.Errorf("al volver la caja dice %q", c.Texto())
	}
	sa.Enviar(c.Texto())
	relojes[EsperaBorrador]()
	if l.Borradores != 1 || !s.pidio("PutDraft(c1,,)") {
		t.Errorf("enviar deja %d borradores y %v", l.Borradores, s.llamadas)
	}

	sa.AbrirPanel(PanelBorradores)
	p := sa.lista
	if p == nil || !p.DeBorradores || l.Panel != PanelBorradores || len(p.Borradores) != 1 || p.Borradores[0].Raiz != "m4" {
		t.Fatalf("el panel «Borradores»: %+v, carril en %q", p, l.Panel)
	}
	p.AlIrABorrador(p.Borradores[0])
	if sa.raizHilo != "m4" || sa.hilo.Compositor.Texto() != "en el hilo" || l.Panel != "" {
		t.Fatalf("ir al borrador del hilo: raíz %q, caja %q, carril %q", sa.raizHilo, sa.hilo.Compositor.Texto(), l.Panel)
	}
	sa.DescartarBorrador(Borrador{Canal: "c1", Raiz: "m4"})
	if l.Borradores != 0 || sa.hilo.Compositor.Texto() != "" {
		t.Errorf("descartar deja %d borradores y la caja del hilo en %q", l.Borradores, sa.hilo.Compositor.Texto())
	}
}

// TestLaFilaDeUnBorradorEsLaDeLaWeb: «#canal» y «en un hilo», el texto en dos líneas,
// «Descartar» a la derecha fuera del botón y el filete salvo en la última.
func TestLaFilaDeUnBorradorEsLaDeLaWeb(t *testing.T) {
	test.NewTempApp(t)
	var ido, descartado bool
	f := NuevaFilaBorrador(traductor(t), Borrador{Canal: "c1", Raiz: "m4", Texto: strings.Repeat("texto a medias ", 30)}, "equipo",
		func(Borrador) { ido = true }, func(Borrador) { descartado = true })
	const ancho = 308
	test.WidgetRenderer(f)
	f.Resize(fyne.NewSize(ancho, MedidaFilaBorrador(f, ancho).Height))
	quiero := 2*RellenoBorrador + 2*RellenoBotonBorrador + tema.TextoSM.Linea + tema.Espacio(0.5) + 2*tema.TextoSM.Linea + tema.Borde
	if got := MedidaFilaBorrador(f, ancho).Height; got != quiero {
		t.Errorf("la fila mide %v y son %v", got, quiero)
	}
	if f.nombre.Text != "#equipo" || f.hilo == nil || f.hilo.Text != "en un hilo" {
		t.Errorf("la cabeza dice %q / %v", f.nombre.Text, f.hilo)
	}
	if x := f.descarta.Position().X + LadoDescartar; x != ancho-CanalonBorrador {
		t.Errorf("«Descartar» acaba en %v y no en el `px-2` (%v)", x, ancho-CanalonBorrador)
	}
	test.Tap(f.descarta)
	test.Tap(f)
	if !descartado || !ido {
		t.Errorf("descartar %v, ir %v", descartado, ido)
	}
	f.Ultima = true
	if MedidaFilaBorrador(f, ancho).Height != quiero-tema.Borde {
		t.Error("la última fila sigue llevando filete")
	}
}

// TestLosProgramados es `ScheduledPanel`: el reloj de la cabecera abre la lista con su
// canal; «Enviar ya» cancela y publica, también uno con adjuntos, que dice «Solo
// adjuntos»; y si cancelar falla, la lista se vuelve a pedir.
//
// VISTO EN ROJO (2026-09-23, en el frontal): publicando sin cancelar antes, «Enviar
// ya» dejaba el programado vivo y salía dos veces. Deshecho.
func TestLosProgramados(t *testing.T) {
	s := servicioConDos()
	cuando := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	s.programados = []api.RoomsScheduled{
		{ID: "p1", ChannelID: "c1", AuthorSub: "yo", AuthorName: "Yo", Body: "**luego**", SendAt: cuando},
		{ID: "p2", ChannelID: "d1", AuthorSub: "yo", FileIDs: []string{"f1"}, SendAt: cuando},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirPanel(PanelProgramado)
	l := sa.lista
	if l == nil || !l.DeProgramados || l.Estado != Listo || len(l.Programados) != 2 || sa.P.Canal.Cabecera.Panel != PanelProgramado {
		t.Fatalf("el panel «Programados»: %+v", l)
	}
	if l.Titulo != "Programados" || l.RotuloCerrar != "Cerrar los mensajes programados" || l.Programados[1].Autor != "yo" {
		t.Errorf("la hoja dice %q / %q, y el segundo es de %q", l.Titulo, l.RotuloCerrar, l.Programados[1].Autor)
	}
	f := NuevaFilaProgramado(sa.P.T, sa.P.Locale, l.Programados[1], "dm-x", nil, nil)
	test.WidgetRenderer(f)
	if f.enviar == nil || f.adjuntos == nil || f.adjuntos.Text != "Solo adjuntos" {
		t.Errorf("uno con adjuntos: enviar %v, texto %v", f.enviar != nil, f.adjuntos)
	}
	// «Enviar ya» de uno con adjuntos los publica con él.
	sa.EnviarYa(Programado{ID: "p9", Canal: "d1", Adjuntos: []string{"f1"}})
	if n := len(s.envios); n == 0 || len(s.envios[n-1].FileIDs) != 1 || s.envios[n-1].FileIDs[0] != "f1" {
		t.Errorf("«Enviar ya» con adjuntos manda %+v", s.envios)
	}
	l.AlEnviarYa(l.Programados[0])
	i, j := slices.Index(s.llamadas, "CancelScheduled(p1)"), slices.Index(s.llamadas, "Post(c1,**luego**)")
	if i < 0 || j < i || len(l.Programados) != 1 {
		t.Errorf("«Enviar ya»: %v, quedan %d", s.llamadas, len(l.Programados))
	}
	s.errAccion = &api.Error{Status: 500}
	l.AlCancelarYa(l.Programados[0])
	if s.veces("Scheduled()") != 2 || len(l.Programados) != 2 {
		t.Errorf("cancelar con el servicio caído: %v, %d en la lista", s.llamadas, len(l.Programados))
	}
}

// TestLaFilaDeUnProgramadoMideLoQueDice: el alto es el `py-3`, el nombre, el texto con
// su Markdown, «Se envía el …» con su `mt-1` y los botones con su `mt-2`.
func TestLaFilaDeUnProgramadoMideLoQueDice(t *testing.T) {
	test.NewTempApp(t)
	m := Programado{ID: "p1", Canal: "c1", Autor: "Ana", Cuerpo: "hola", Cuando: time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)}
	f := NuevaFilaProgramado(traductor(t), "es", m, "equipo", nil, nil)
	test.WidgetRenderer(f)
	const ancho = 308
	quiero := 2*RellenoProgramado + tema.TextoSM.Linea + f.altoTexto(ancho) + tema.Espacio(1) + tema.TextoXS.Linea +
		tema.Espacio(2) + f.altoBotones(ancho) + tema.Borde
	if got := MedidaFilaProgramado(f, ancho).Height; got != quiero {
		t.Errorf("la fila mide %v y son %v", got, quiero)
	}
	if !strings.HasPrefix(f.cuando.Text, "Se envía el ") || f.enviar == nil || f.enviar.Texto != "Enviar ya" {
		t.Errorf("dice %q y el botón %+v", f.cuando.Text, f.enviar)
	}
}

// TestLasFilasDelPanelSeMidenAlAnchoDelPanel: una fila que parte su texto se coloca con
// el alto que tiene al ancho del panel, y la siguiente empieza justo debajo.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `ui.Columna` la fila del texto largo
// se medía con su tamaño de antes de estirarla y no casaba con su medida. Deshecho.
func TestLasFilasDelPanelSeMidenAlAnchoDelPanel(t *testing.T) {
	test.NewTempApp(t)
	p := NuevoPanelLista(traductor(t), "es", "Hilos")
	p.DeHilos, p.Estado = true, Listo
	p.Hilos = []ResumenHilo{
		{Raiz: Mensaje{ID: "a", Autor: Autor{Nombre: "Ana"}, Cuerpo: strings.Repeat("palabra ", 30), Respuestas: 1}},
		{Raiz: Mensaje{ID: "b", Autor: Autor{Nombre: "Beto"}, Cuerpo: "hola"}},
	}
	w := test.NewWindow(p)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(AnchoPanelDerecho, 600))
	filas := p.cuerpo.(*container.Scroll).Content.(*fyne.Container).Objects
	primera, segunda := filas[1].(*FilaLista), filas[2].(*FilaLista)
	if got, quiero := primera.Size().Height, MedidaFilaLista(primera, primera.Size().Width).Height; got != quiero {
		t.Errorf("la fila del texto largo mide %v y a su ancho son %v", got, quiero)
	}
	if got, quiero := segunda.Position().Y, primera.Position().Y+primera.Size().Height; got != quiero {
		t.Errorf("la segunda empieza en %v y la primera acaba en %v", got, quiero)
	}
}

// TestElSelectorDeEmoji es `EmojiPicker` y `searchEmoji`: un emoji con varios nombres
// sale una vez y se busca por cualquiera; las flechas recorren la rejilla de ocho sin
// salirse; Intro elige; sin resultados lo dice. Desde la caja inserta en el cursor, y
// desde la barra de un mensaje reacciona.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin juntar por emoji, buscar «+1» daba un
// 👍 aparte con solo :+1:, y la rejilla lo enseñaba dos veces. Deshecho.
func TestElSelectorDeEmoji(t *testing.T) {
	pulgar := BuscarEmoji("+1")
	if len(pulgar) != 1 || pulgar[0].Glifo != "👍" || strings.Join(pulgar[0].Nombres, " ") != "thumbsup +1" {
		t.Fatalf("buscar «+1» da %+v", pulgar)
	}
	if n := len(BuscarEmoji("THUMBS")); n != 2 { // thumbsup y thumbsdown
		t.Errorf("buscar «THUMBS» da %d", n)
	}

	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	c.PonerTexto("hola ")
	c.AlEmoji()
	sel := sa.emojis
	if sel == nil || len(sel.Resultados()) != len(entradasEmoji) || sel.Activo() != 0 {
		t.Fatalf("el selector no se abre con todos: %+v", sel)
	}
	tecla := func(k fyne.KeyName) { sel.Caja().AlTeclear(&fyne.KeyEvent{Name: k}) }
	tecla(fyne.KeyUp)
	tecla(fyne.KeyLeft)
	if sel.Activo() != 0 {
		t.Errorf("arriba y a la izquierda del primero dejan el activo en %d", sel.Activo())
	}
	tecla(fyne.KeyDown)
	tecla(fyne.KeyRight)
	tecla(fyne.KeyUp)
	if sel.Activo() != 1 {
		t.Errorf("abajo, derecha y arriba dejan el activo en %d y es 1", sel.Activo())
	}
	tecla(fyne.KeyReturn)
	if got := c.Texto(); got != "hola "+entradasEmoji[1].Glifo {
		t.Errorf("elegir inserta y la caja dice %q", got)
	}

	sa.P.Canal.Historial.AlEmojis(sa.P.Canal.Historial.Mensajes[0], c.BotonEmoji())
	sa.emojis.Caja().PonerTexto("zzz")
	if len(sa.emojis.Resultados()) != 0 {
		t.Error("«zzz» encuentra algo")
	}
	sa.emojis.Caja().PonerTexto("tada")
	sa.emojis.Caja().AlTeclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("React(m1,🎉,poner)") {
		t.Errorf("reaccionar desde el selector: %v", s.llamadas)
	}
}

// TestLaBarraApagadaNoDejaSuSombra: la barra apagada no se ve, y su sombra tampoco. En
// una ventana real quedaba una caja gris de 187,5 × 30 junto a cada mensaje: la sombra,
// que Ajustar volvía a enseñar al pasar la barra de no tener tamaño a tenerlo (el pintor
// de software no pinta sombras y los tests no la veían). VISTO EN ROJO sin el Hide de
// barraMensajeRenderer.Layout.
func TestLaBarraApagadaNoDejaSuSombra(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Arrancar(context.Background())
	b := filaDe(t, sa.P.Canal.Historial, "m1").barra
	b.Resize(fyne.NewSize(0, 0))
	test.WidgetRenderer(b).Layout(fyne.NewSize(0, 0))
	b.Resize(b.MinSize())
	test.WidgetRenderer(b).Layout(b.MinSize())
	if b.sombra.Objeto().Visible() {
		t.Error("la barra está apagada y su sombra se ve")
	}
	b.MouseIn(&desktop.MouseEvent{})
	if !b.sombra.Objeto().Visible() {
		t.Error("con el puntero encima la barra sale sin su sombra")
	}
}
