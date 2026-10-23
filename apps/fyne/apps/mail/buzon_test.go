// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/portapapeles"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// servicioFalso es el servidor de Mail en memoria. Apunta cada llamada para que el
// guardián compruebe QUÉ se pidió, no solo qué se pintó.
type servicioFalso struct {
	carpetas       []api.Mailbox
	paginas        map[string][]api.MailMessage // cursor → página
	siguiente      map[string]string            // cursor → cursor de la siguiente
	hilo           []api.MailMessage
	crudo          map[string]string
	buscados       []api.MailMessage
	alEscuchar     func(llegar func()) // lo que «llega» por el flujo de Llegadas
	errBuscar      error
	errLista       error
	errTriaje      error
	baja           api.MailBaja
	porEtiqueta    []api.MailMessage
	firma          string
	alias          []string
	enviado        string
	guardado       string
	errEnvio       error
	programados    []api.MailScheduled
	errProgramados error
	vacaciones     api.MailVacation
	errVacaciones  error
	motivoCarpeta  api.MailboxSetError
	errCarpeta     error
	errAlias       error
	reglas         []api.MailFilter
	errSmime       error
	compartidos    []api.MailShared
	deCompartido   []api.MailMessage
	sigCompartido  string
	errCompartidos error
	accesos        map[string][]api.MailGrant
	delegados      []api.MailGrant
	errAcceso      error
	errReglas      error

	llamadas []string
	marcados []string
}

func (s *servicioFalso) apuntar(f string, args ...string) {
	s.llamadas = append(s.llamadas, f+"("+strings.Join(args, ",")+")")
}

func (s *servicioFalso) Mailboxes(context.Context) ([]api.Mailbox, error) {
	s.apuntar("Mailboxes")
	return s.carpetas, nil
}

func (s *servicioFalso) Messages(_ context.Context, carpeta, cursor string, _ int) ([]api.MailMessage, string, error) {
	s.apuntar("Messages", carpeta, cursor)
	if s.errLista != nil {
		return nil, "", s.errLista
	}
	return s.paginas[cursor], s.siguiente[cursor], nil
}

func (s *servicioFalso) LabelMessages(_ context.Context, etiqueta, cursor string, _ int) ([]api.MailMessage, string, error) {
	s.apuntar("LabelMessages", etiqueta, cursor)
	return s.porEtiqueta, "", nil
}

func (s *servicioFalso) Search(_ context.Context, q string, desde, limite int) ([]api.MailMessage, error) {
	if desde > 0 {
		s.apuntar("Search", q, numero(desde))
	} else {
		s.apuntar("Search", q)
	}
	if desde >= len(s.buscados) {
		return nil, s.errBuscar
	}
	return slices.Clone(s.buscados[desde:min(desde+limite, len(s.buscados))]), s.errBuscar
}

func (s *servicioFalso) Llegadas(_ context.Context, alLlegar func()) error {
	s.apuntar("Llegadas")
	if s.alEscuchar != nil {
		s.alEscuchar(alLlegar)
	}
	return nil
}

func (s *servicioFalso) Thread(_ context.Context, hilo string) ([]api.MailMessage, error) {
	s.apuntar("Thread", hilo)
	return s.hilo, nil
}

func (s *servicioFalso) Raw(_ context.Context, id string) (io.ReadCloser, error) {
	s.apuntar("Raw", id)
	c, hay := s.crudo[id]
	if !hay {
		return nil, &api.Error{Status: http.StatusNotFound}
	}
	return io.NopCloser(strings.NewReader(c)), nil
}

func (s *servicioFalso) SetFlagMany(_ context.Context, ids []string, marca string, poner bool) error {
	s.apuntar("SetFlagMany", marca)
	if poner && s.errTriaje == nil {
		s.marcados = append(s.marcados, ids...)
	}
	return s.errTriaje
}

func (s *servicioFalso) Labels(context.Context) ([]api.MailLabel, error) {
	s.apuntar("Labels")
	return []api.MailLabel{{ID: "e1", Name: "Obra", Color: "teal", UnreadEmails: 2}}, nil
}

func (s *servicioFalso) Quota(context.Context) (api.MailQuota, error) {
	s.apuntar("Quota")
	return api.MailQuota{UsedBytes: 49254, QuotaBytes: 5 << 30}, nil
}

func (s *servicioFalso) Changes(context.Context, string) (api.MailCambios, error) {
	s.apuntar("Changes")
	return api.MailCambios{State: "s1"}, nil
}

func (s *servicioFalso) MoveMany(_ context.Context, ids []string, carpeta string) error {
	s.apuntar("MoveMany", append([]string{carpeta}, ids...)...)
	return s.errTriaje
}

func (s *servicioFalso) ApplyLabel(_ context.Context, etiqueta string, ids []string, poner bool) error {
	s.apuntar("ApplyLabel", append([]string{etiqueta, strconvBool(poner)}, ids...)...)
	return s.errTriaje
}

func (s *servicioFalso) Delete(_ context.Context, id string) error {
	s.apuntar("Delete", id)
	return s.errTriaje
}

func (s *servicioFalso) Unsubscribe(_ context.Context, id string) (api.MailBaja, error) {
	s.apuntar("Unsubscribe", id)
	return s.baja, nil
}

func (s *servicioFalso) DoUnsubscribe(_ context.Context, id string) (string, error) {
	s.apuntar("DoUnsubscribe", id)
	return api.BajaHecha, nil
}

func (s *servicioFalso) Signature(context.Context) (string, error) {
	s.apuntar("Signature")
	return s.firma, nil
}

func (s *servicioFalso) SetSignature(_ context.Context, f string) error {
	s.apuntar("SetSignature", f)
	s.firma = f
	return nil
}

func (s *servicioFalso) Aliases(context.Context) (string, []api.MailAlias, error) {
	s.apuntar("Aliases")
	xs := make([]api.MailAlias, 0, len(s.alias))
	for i, a := range s.alias {
		xs = append(xs, api.MailAlias{ID: "a" + strconvInt(i), Address: a})
	}
	return "ada@example.org", xs, s.errAlias
}

func (s *servicioFalso) CreateAlias(_ context.Context, d string) (api.MailAlias, error) {
	s.apuntar("CreateAlias", d)
	if s.errAlias != nil {
		return api.MailAlias{}, s.errAlias
	}
	s.alias = append(s.alias, d)
	return api.MailAlias{ID: "a" + strconvInt(len(s.alias)-1), Address: d}, nil
}

func (s *servicioFalso) DeleteAlias(_ context.Context, id string) error {
	s.apuntar("DeleteAlias", id)
	if s.errAlias != nil {
		return s.errAlias
	}
	i, _ := strconv.Atoi(strings.TrimPrefix(id, "a"))
	if i < len(s.alias) {
		s.alias = slices.Delete(s.alias, i, i+1)
	}
	return nil
}

func (s *servicioFalso) Send(_ context.Context, de string, dest []string, crudo []byte, undo int) (api.MailEnvio, error) {
	s.apuntar("Send", append([]string{de, strconvInt(undo)}, dest...)...)
	s.enviado = string(crudo)
	if s.errEnvio != nil {
		return api.MailEnvio{}, s.errEnvio
	}
	return api.MailEnvio{Status: "undo", ID: "o1"}, nil
}

func (s *servicioFalso) SaveDraft(_ context.Context, id, _ string, crudo []byte) (api.MailMessage, error) {
	s.apuntar("SaveDraft", id)
	s.guardado = string(crudo)
	return api.MailMessage{ID: "d" + strconvInt(s.pidio("SaveDraft"))}, nil
}

func (s *servicioFalso) CancelOutbound(_ context.Context, id string) error {
	s.apuntar("CancelOutbound", id)
	return nil
}

func (s *servicioFalso) Schedule(_ context.Context, de string, dest []string, _ []byte, cuando time.Time) (api.MailEnvio, error) {
	s.apuntar("Schedule", append([]string{de, cuando.UTC().Format(time.RFC3339)}, dest...)...)
	return api.MailEnvio{Status: "scheduled", ID: "p1", ReleaseAt: cuando}, nil
}

func (s *servicioFalso) Scheduled(context.Context) ([]api.MailScheduled, error) {
	s.apuntar("Scheduled")
	return s.programados, s.errProgramados
}

func (s *servicioFalso) Reschedule(_ context.Context, id string, cuando time.Time) error {
	s.apuntar("Reschedule", id, cuando.UTC().Format(time.RFC3339))
	return nil
}

func (s *servicioFalso) CreateLabel(_ context.Context, nombre, color string) (api.MailLabel, error) {
	s.apuntar("CreateLabel", nombre, color)
	return api.MailLabel{ID: "e9", Name: nombre, Color: color}, s.errTriaje
}

func (s *servicioFalso) UpdateLabel(_ context.Context, id, nombre, color string) error {
	s.apuntar("UpdateLabel", id, nombre, color)
	return s.errTriaje
}

func (s *servicioFalso) DeleteLabel(_ context.Context, id string) error {
	s.apuntar("DeleteLabel", id)
	return s.errTriaje
}

func (s *servicioFalso) Vacation(context.Context) (api.MailVacation, error) {
	s.apuntar("Vacation")
	return s.vacaciones, s.errVacaciones
}

func (s *servicioFalso) SetVacation(_ context.Context, v api.MailVacation) error {
	s.apuntar("SetVacation", strconvBool(v.Enabled), v.Subject)
	if s.errTriaje != nil {
		return s.errTriaje
	}
	s.vacaciones = v
	return nil
}

func (s *servicioFalso) CreateMailbox(_ context.Context, nombre string) (api.Mailbox, error) {
	s.apuntar("CreateMailbox", nombre)
	if s.errCarpeta != nil {
		return api.Mailbox{}, s.errCarpeta
	}
	m := api.Mailbox{ID: "c" + strconvInt(len(s.carpetas)), Name: nombre}
	s.carpetas = append(s.carpetas, m)
	return m, nil
}

func (s *servicioFalso) RenameMailbox(_ context.Context, id, nombre string) (api.MailboxSetError, error) {
	s.apuntar("RenameMailbox", id, nombre)
	if s.motivoCarpeta != "" || s.errCarpeta != nil {
		return s.motivoCarpeta, s.errCarpeta
	}
	for i := range s.carpetas {
		if s.carpetas[i].ID == id {
			s.carpetas[i].Name = nombre
		}
	}
	return "", nil
}

func (s *servicioFalso) DeleteMailbox(_ context.Context, id string) (api.MailboxSetError, error) {
	s.apuntar("DeleteMailbox", id)
	if s.motivoCarpeta != "" || s.errCarpeta != nil {
		return s.motivoCarpeta, s.errCarpeta
	}
	s.carpetas = slices.DeleteFunc(s.carpetas, func(m api.Mailbox) bool { return m.ID == id })
	return "", nil
}

func (s *servicioFalso) Filters(context.Context) ([]api.MailFilter, error) {
	s.apuntar("Filters")
	return slices.Clone(s.reglas), s.errReglas
}

func (s *servicioFalso) CreateFilter(_ context.Context, f api.MailFilter) (api.MailFilter, error) {
	s.apuntar("CreateFilter", f.Name, f.MatchFrom, f.MatchSubject, f.MoveTo, strconvBool(f.MarkRead), strconvBool(f.MarkFlagged))
	if s.errTriaje != nil {
		return api.MailFilter{}, s.errTriaje
	}
	f.ID = "r" + strconvInt(len(s.reglas)+1)
	s.reglas = append(s.reglas, f)
	return f, nil
}

func (s *servicioFalso) UpdateFilter(_ context.Context, f api.MailFilter) error {
	s.apuntar("UpdateFilter", f.ID, strconvBool(f.Enabled))
	if s.errTriaje != nil {
		return s.errTriaje
	}
	for i := range s.reglas {
		if s.reglas[i].ID == f.ID {
			s.reglas[i] = f
		}
	}
	return nil
}

func (s *servicioFalso) DeleteFilter(_ context.Context, id string) error {
	s.apuntar("DeleteFilter", id)
	if s.errTriaje != nil {
		return s.errTriaje
	}
	s.reglas = slices.DeleteFunc(s.reglas, func(f api.MailFilter) bool { return f.ID == id })
	return nil
}

func (s *servicioFalso) SmimeAnchor(context.Context) ([]byte, error) {
	s.apuntar("SmimeAnchor")
	return []byte("-----BEGIN CERTIFICATE-----"), s.errTriaje
}

func (s *servicioFalso) SetSmimeIdentity(_ context.Context, pem string) error {
	s.apuntar("SetSmimeIdentity", pem)
	return s.errSmime
}

func (s *servicioFalso) SharedMailboxes(context.Context) ([]api.MailShared, error) {
	s.apuntar("SharedMailboxes")
	return slices.Clone(s.compartidos), s.errCompartidos
}

func (s *servicioFalso) CreateSharedMailbox(_ context.Context, dir, nombre string) (api.MailShared, error) {
	s.apuntar("CreateSharedMailbox", dir, nombre)
	if s.errCompartidos != nil {
		return api.MailShared{}, s.errCompartidos
	}
	x := api.MailShared{AccountID: "s" + strconvInt(len(s.compartidos)+1), Address: dir, DisplayName: nombre, Kind: "shared", Permission: "manage"}
	s.compartidos = append(s.compartidos, x)
	return x, nil
}

func (s *servicioFalso) SharedFolders(_ context.Context, cuenta string) ([]api.Mailbox, error) {
	s.apuntar("SharedFolders", cuenta)
	return []api.Mailbox{{ID: cuenta + "-in", Name: "INBOX", Role: "inbox"}}, s.errCompartidos
}

func (s *servicioFalso) SharedMessages(_ context.Context, cuenta, carpeta, cursor string, _ int) ([]api.MailMessage, string, error) {
	s.apuntar("SharedMessages", cuenta, carpeta, cursor)
	if cursor != "" {
		return nil, "", s.errCompartidos
	}
	return s.deCompartido, s.sigCompartido, s.errCompartidos
}

func (s *servicioFalso) Grants(_ context.Context, cuenta string) ([]api.MailGrant, error) {
	s.apuntar("Grants", cuenta)
	return slices.Clone(s.accesos[cuenta]), s.errCompartidos
}

func (s *servicioFalso) Grant(_ context.Context, cuenta, quien, permiso string) error {
	s.apuntar("Grant", cuenta, quien, permiso)
	if s.errAcceso != nil {
		return s.errAcceso
	}
	if s.accesos == nil {
		s.accesos = map[string][]api.MailGrant{}
	}
	s.accesos[cuenta] = append(s.accesos[cuenta], api.MailGrant{Grantee: quien, Permission: permiso})
	return nil
}

func (s *servicioFalso) Revoke(_ context.Context, cuenta, quien string) error {
	s.apuntar("Revoke", cuenta, quien)
	return s.errAcceso
}

func (s *servicioFalso) Delegations(context.Context) ([]api.MailGrant, error) {
	s.apuntar("Delegations")
	return slices.Clone(s.delegados), s.errCompartidos
}

func (s *servicioFalso) Delegate(_ context.Context, quien, permiso string) error {
	s.apuntar("Delegate", quien, permiso)
	return s.errAcceso
}

func (s *servicioFalso) Undelegate(_ context.Context, quien string) error {
	s.apuntar("Undelegate", quien)
	return s.errAcceso
}

func strconvInt(n int) string { return strconv.Itoa(n) }

func strconvBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (s *servicioFalso) pidio(f string) int {
	n := 0
	for _, l := range s.llamadas {
		if strings.HasPrefix(l, f+"(") {
			n++
		}
	}
	return n
}

var hace = time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)

func sobre(id, hilo, carpeta, asunto string, leido bool, antes time.Duration) api.MailMessage {
	m := api.MailMessage{
		ID: id, ThreadID: hilo, MailboxID: carpeta, Subject: asunto,
		From: id + "@example.org", FromName: "Persona " + id, To: []string{"ada@example.org"},
		Preview: "extracto de " + id, ReceivedAt: hace.Add(-antes),
	}
	if leido {
		m.Keywords = []string{api.MailFlagSeen}
	}
	return m
}

func servicioDeEjemplo() *servicioFalso {
	return &servicioFalso{
		// Desordenadas a propósito: la de la persona primero y Papelera antes que
		// Recibidos. El orden lo pone el cliente, no el servidor.
		carpetas: []api.Mailbox{
			{ID: "pr", Name: "Proyectos"},
			{ID: "tr", Name: "Trash", Role: "trash"},
			{ID: "in", Name: "INBOX", Role: "inbox", UnreadEmails: 1},
			{ID: "ar", Name: "Archive", Role: "archive"},
		},
		paginas: map[string][]api.MailMessage{
			"": {
				sobre("m1", "h1", "in", "Calendario", false, 0),
				sobre("m2", "h2", "in", "Encuesta", true, time.Hour),
				sobre("m3", "h1", "in", "Calendario", true, 2*time.Hour),
			},
			"c2": {
				sobre("m3", "h1", "in", "Calendario", true, 2*time.Hour), // solapada
				sobre("m4", "h4", "in", "Factura", true, 3*time.Hour),
			},
		},
		siguiente: map[string]string{"": "c2"},
		crudo: map[string]string{
			"m1": "Subject: Calendario\r\n\r\nHola:\r\n\r\nOs paso las ventanas.\r\n",
		},
	}
}

// buzonDePrueba monta el buzón con las llamadas SÍNCRONAS y los temporizadores en
// una lista que el guardián dispara cuando quiere.
func buzonDePrueba(t *testing.T, s Servicio) (*Buzon, *[]func()) {
	t.Helper()
	p := NuevaPantalla(traductor(t), "es")
	p.Reloj(reloj())
	p.AvisosSinReloj = true
	b := NuevoBuzon(p, s)
	b.Lanzar = func(f func()) { f() }
	b.Fondo = func(func()) {} // el flujo de llegadas lo prueba TestLaLlegadaAdelantaElDelta
	// El portapapeles del sistema es de quien está delante: los guardianes llevan el suyo.
	b.LeerHTML = func() (string, error) { return "", portapapeles.ErrSinHTML }
	var pendientes []func()
	b.Esperar = func(_ time.Duration, f func()) func() {
		pendientes = append(pendientes, f)
		return func() {}
	}
	return b, &pendientes
}

// TestElBuzonArrancaEnRecibidos: carpetas ordenadas por rol, Recibidos elegida y su
// primera página plegada en conversaciones.
func TestElBuzonArrancaEnRecibidos(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())

	ids := make([]string, 0, len(b.P.Menu.Carpetas))
	for _, c := range b.P.Menu.Carpetas {
		ids = append(ids, c.ID)
	}
	if want := []string{"in", "ar", "tr", "pr"}; !slices.Equal(ids, want) {
		t.Errorf("las carpetas salen en el orden %v y la web las pone %v (ROLE_ORDER y luego por nombre)", ids, want)
	}
	if b.P.Menu.CarpetaID != "in" || b.P.Bandeja.Titulo != "Recibidos" {
		t.Errorf("se abre %q con título %q y tenía que ser Recibidos", b.P.Menu.CarpetaID, b.P.Bandeja.Titulo)
	}
	if s.pidio("Messages") != 1 {
		t.Errorf("la primera página se ha pedido %d veces", s.pidio("Messages"))
	}
	cs := b.P.Bandeja.Conversaciones
	if b.P.Bandeja.Estado != Listo || len(cs) != 2 {
		t.Fatalf("la bandeja está en %d con %d conversaciones; tenían que ser 2 hilos", b.P.Bandeja.Estado, len(cs))
	}
	if !cs[0].SinLeer || cs[0].HiloID != "h1" || len(cs[0].Mensajes) != 2 {
		t.Errorf("el primer hilo no es h1 sin leer con dos mensajes: %+v", cs[0])
	}
	if !b.P.Bandeja.HayMas {
		t.Error("el servidor ha dado cursor y la bandeja no sabe que hay más")
	}
	if len(b.P.Menu.Etiquetas) != 1 || b.P.Menu.Etiquetas[0].SinLeer != 2 {
		t.Errorf("las etiquetas no llegan al menú: %+v", b.P.Menu.Etiquetas)
	}
	if b.P.Menu.EstadoCuota != Listo || b.P.Menu.CuotaUso != "48 KB de 5,0 GB" {
		t.Errorf("la cuota dice %q (estado %d)", b.P.Menu.CuotaUso, b.P.Menu.EstadoCuota)
	}
}

// TestAbrirMarcaLeidoYTraeElCuerpo: abrir un hilo lo marca leído en UNA llamada,
// refresca los contadores del lateral y baja el .eml del último mensaje.
func TestAbrirMarcaLeidoYTraeElCuerpo(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	carpetasAntes := s.pidio("Mailboxes")

	b.Abrir(b.P.Bandeja.Conversaciones[0])

	if s.pidio("SetFlagMany") != 1 || !slices.Equal(s.marcados, []string{"m1"}) {
		t.Errorf("marcar leído: %d llamadas con %v; tenía que ser una con el único sin leer", s.pidio("SetFlagMany"), s.marcados)
	}
	if s.pidio("Mailboxes") != carpetasAntes+1 {
		t.Error("tras leer no se han vuelto a pedir las carpetas: el contador del lateral se queda con el número viejo")
	}
	if b.P.Bandeja.Conversaciones[0].SinLeer {
		t.Error("el hilo sigue sin leer en la lista después de abrirlo")
	}
	if s.pidio("Thread") != 1 {
		t.Error("no se ha pedido el hilo completo: faltaría lo contestado desde Enviados")
	}
	c, ya := b.P.Lector.Cuerpo("m1")
	if !ya || !strings.Contains(c.Texto, "Os paso las ventanas.") {
		t.Fatalf("el cuerpo de m1 no ha llegado al lector: %+v, %v", c, ya)
	}
	if strings.Contains(c.Texto, "\r") {
		t.Error("el cuerpo conserva CR: se pintarían como cajas")
	}
	if !b.P.Lector.abierto() {
		t.Error("el lector no enseña el hilo abierto")
	}
}

// TestLaBusquedaEsperaYNoConfundeFalloConVacio: se pregunta al dejar de teclear, y una
// búsqueda que falla se dice como fallo, no como «no hay resultados».
func TestLaBusquedaEsperaYNoConfundeFalloConVacio(t *testing.T) {
	s := servicioDeEjemplo()
	b, pendientes := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	*pendientes = nil

	b.Buscar("fac")
	b.Buscar("factura")
	if s.pidio("Search") != 0 {
		t.Fatal("se ha preguntado por cada tecla: falta el debounce")
	}
	(*pendientes)[len(*pendientes)-1]()
	if s.pidio("Search") != 1 || !slices.Contains(s.llamadas, "Search(factura)") {
		t.Errorf("búsquedas: %v", s.llamadas)
	}
	if !b.P.Bandeja.Buscando || b.P.Bandeja.Estado != Vacio {
		t.Errorf("sin resultados tenía que decir «no hay resultados»: buscando=%v estado=%d", b.P.Bandeja.Buscando, b.P.Bandeja.Estado)
	}

	s.errBuscar = errors.New("se ha caído la red")
	b.Buscar("otra")
	(*pendientes)[len(*pendientes)-1]()
	if b.P.Bandeja.Estado == Vacio {
		t.Error("una búsqueda que FALLA sale como «no hay resultados»: quien lo lee deja de buscar un correo que sí existe")
	}

	b.Buscar("")
	if b.P.Bandeja.Buscando || len(b.P.Bandeja.Conversaciones) != 2 {
		t.Errorf("vaciar la búsqueda no devuelve la carpeta: %d conversaciones", len(b.P.Bandeja.Conversaciones))
	}
}

// TestLaBusquedaPagina: con la primera página llena, el final de la lista pide la
// siguiente desde lo ya recibido (`position`); una página corta es la última y ya no se
// pide más. Es la rama de búsqueda de `handleLoadMore`.
func TestLaBusquedaPagina(t *testing.T) {
	s := servicioDeEjemplo()
	for i := range PorPagina + 7 {
		s.buscados = append(s.buscados, api.MailMessage{
			ID: "b" + numero(i), ThreadID: "hb" + numero(i), MailboxID: "in", Subject: "Factura " + numero(i),
			ReceivedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour),
		})
	}
	b, pendientes := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	b.Buscar("factura")
	(*pendientes)[len(*pendientes)-1]()
	if len(b.resultados) != PorPagina || !b.P.Bandeja.HayMas {
		t.Fatalf("primera página: %d resultados, hay más %v", len(b.resultados), b.P.Bandeja.HayMas)
	}
	b.MasPagina()
	if !slices.Contains(s.llamadas, "Search(factura,50)") || len(b.resultados) != PorPagina+7 {
		t.Fatalf("segunda página: %d resultados, %v", len(b.resultados), s.llamadas)
	}
	if b.P.Bandeja.HayMas {
		t.Error("con una página corta se sigue ofreciendo más")
	}
	n := s.pidio("Search")
	b.MasPagina()
	if s.pidio("Search") != n {
		t.Error("tras la última página se vuelve a pedir")
	}
}

// TestLaPaginaSiguienteNoRepite: el scroll infinito anexa sin duplicar y no pide más
// cuando el servidor ya no da cursor.
func TestLaPaginaSiguienteNoRepite(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())

	b.MasPagina()
	if !slices.Contains(s.llamadas, "Messages(in,c2)") {
		t.Fatalf("no se ha pedido la página siguiente con su cursor: %v", s.llamadas)
	}
	if n := len(b.mensajes); n != 4 {
		t.Errorf("tras dos páginas solapadas hay %d mensajes y tenían que ser 4", n)
	}
	if b.P.Bandeja.HayMas {
		t.Error("sin cursor la bandeja sigue creyendo que hay más")
	}
	antes := s.pidio("Messages")
	b.MasPagina()
	if s.pidio("Messages") != antes {
		t.Error("sin cursor se ha vuelto a pedir")
	}
}

// TestUnaRespuestaTardiaNoPisaLaVista: la carpeta de antes contesta cuando ya se ha
// elegido otra, y no se pinta.
func TestUnaRespuestaTardiaNoPisaLaVista(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())

	var colgadas []func()
	b.Lanzar = func(f func()) { colgadas = append(colgadas, f) }
	b.ElegirCarpeta(Carpeta{ID: "in", Nombre: "INBOX", Rol: "inbox"})
	s.paginas[""] = nil // la segunda carpeta está vacía
	b.ElegirCarpeta(Carpeta{ID: "ar", Nombre: "Archive", Rol: "archive"})
	colgadas[1]() // contesta primero la de Archivo…
	colgadas[0]() // …y después la de Recibidos, tarde
	if b.P.Bandeja.Estado != Vacio || b.P.Bandeja.Titulo != "Archivo" {
		t.Errorf("la respuesta de Recibidos ha pisado Archivo: estado %d, título %q", b.P.Bandeja.Estado, b.P.Bandeja.Titulo)
	}
}

// TestElErrorDiceLoQuePaso: un 403 no es «no hay red», y «no hay red» no es un fallo
// del servidor.
func TestElErrorDiceLoQuePaso(t *testing.T) {
	for _, c := range []struct {
		err  error
		want Estado
	}{
		{&api.Error{Status: http.StatusForbidden}, SinPermiso},
		{&api.Error{Status: http.StatusBadGateway}, Error},
		{errors.New("dial tcp: no route to host"), SinConexion},
	} {
		if got := estadoDe(c.err); got != c.want {
			t.Errorf("estadoDe(%v) = %d y tenía que ser %d", c.err, got, c.want)
		}
	}
	s := servicioDeEjemplo()
	s.errLista = &api.Error{Status: http.StatusForbidden}
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	if b.P.Bandeja.Estado != SinPermiso {
		t.Errorf("un 403 de la lista deja la bandeja en %d", b.P.Bandeja.Estado)
	}
}

// TestElDeltaSoloTocaLoCambiado es `apply` del sondeo: lo borrado sale, lo que entra
// en la vista se pone, lo que se va a otra carpeta sale, y todo por fecha.
func TestElDeltaSoloTocaLoCambiado(t *testing.T) {
	ms := []api.MailMessage{
		sobre("a", "h", "in", "", true, time.Hour),
		sobre("b", "h", "in", "", true, 2*time.Hour),
		sobre("c", "h", "in", "", true, 3*time.Hour),
	}
	movido := sobre("c", "h", "ar", "", true, 3*time.Hour)
	nuevo := sobre("n", "h", "in", "", false, 0)
	out := AplicarDelta(ms, api.MailCambios{
		Changed:   []api.MailMessage{movido, nuevo},
		Destroyed: []string{"b"},
	}, "in", "")
	ids := make([]string, 0, len(out))
	for _, m := range out {
		ids = append(ids, m.ID)
	}
	if want := []string{"n", "a"}; !slices.Equal(ids, want) {
		t.Errorf("tras el delta quedan %v y tenían que quedar %v", ids, want)
	}
}

// TestElSondeoArrancaSinCambiosYLuegoAplica: la primera vuelta solo fija el estado;
// la segunda aplica, y un error espacia la siguiente.
func TestElSondeoArrancaSinCambiosYLuegoAplica(t *testing.T) {
	s := servicioDeEjemplo()
	b, pendientes := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	if len(*pendientes) == 0 {
		t.Fatal("arrancar no programa el sondeo")
	}
	(*pendientes)[len(*pendientes)-1]()
	if b.estadoDelta != "s1" || s.pidio("Changes") != 1 {
		t.Errorf("la primera vuelta no fija el estado: %q, %d llamadas", b.estadoDelta, s.pidio("Changes"))
	}
	if b.esperaSondeo != SondeoBase {
		t.Errorf("tras un sondeo bueno la espera es %v y tenía que ser %v", b.esperaSondeo, SondeoBase)
	}
}

// TestLaLlegadaAdelantaElDelta: el aviso de llegada del flujo pregunta por el delta
// YA, sin esperar a la vuelta del sondeo; con un delta en camino no abre una segunda
// cadena de sondeo sino que pide otra vuelta al acabar. El flujo se abre al arrancar.
func TestLaLlegadaAdelantaElDelta(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	var fondo func()
	b.Fondo = func(f func()) { fondo = f }
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	b.Arrancar(ctx)
	if fondo == nil {
		t.Fatal("arrancar no abre el flujo de llegadas")
	}
	s.alEscuchar = func(llegar func()) { llegar(); cancelar() }
	fondo() // vuelve porque el contexto muere tras la primera llegada
	if s.pidio("Llegadas") != 1 || s.pidio("Changes") != 1 {
		t.Fatalf("la llegada no ha preguntado por el delta: %v", s.llamadas)
	}
	// La fase anterior canceló su vida para cerrar el SSE. Esta fase comprueba la
	// agrupación en una vida activa; una vida cancelada ya no debe hacer lecturas.
	b.ctx = context.Background()

	b.deltaEnCurso = true
	b.llegada()
	if s.pidio("Changes") != 1 || !b.otraVuelta {
		t.Fatalf("con un delta en camino se lanza otro: %v", s.llamadas)
	}
	b.deltaEnCurso, b.otraVuelta = false, false
	b.llegada()
	if s.pidio("Changes") != 2 {
		t.Errorf("sin delta en camino la llegada no pregunta: %v", s.llamadas)
	}
}

// TestElCuerpoPrefiereElTexto: el texto que el remitente escribió va antes que su
// maqueta desnuda, y si traía HTML se dice.
func TestElCuerpoPrefiereElTexto(t *testing.T) {
	c := CuerpoDe(eml.Mensaje{Texto: "Hola\r\nAda\r\n", HTML: "<p>Hola <b>Ada</b></p>"}, nil)
	if c.Texto != "Hola\nAda" || !c.ConFormato {
		t.Errorf("con las dos partes: %+v", c)
	}
	c = CuerpoDe(eml.Mensaje{HTML: "<p>Solo <b>HTML</b></p>"}, nil)
	if c.Texto != "Solo HTML" || !c.ConFormato {
		t.Errorf("solo con HTML: %+v", c)
	}
	c = CuerpoDe(eml.Mensaje{}, errors.New("404"))
	if c.Texto != "" || !c.Fallo {
		t.Errorf("sin nada: %+v", c)
	}
}

// TestLaCuotaDeMail: el uso, con el `formatBytes` de la suite, y el aviso al llenarse.
func TestLaCuotaDeMail(t *testing.T) {
	e, frac, pct, uso, nota := Cuota(traductor(t), "es", api.MailQuota{UsedBytes: 96, QuotaBytes: 100})
	if e != Listo || frac != 0.96 || pct != "96 %" || !strings.HasSuffix(uso, "de 100 B") || nota == "" {
		t.Errorf("una cuota al 96 %%: %d %v %q %q %q", e, frac, pct, uso, nota)
	}
	if e, _, _, _, _ := Cuota(traductor(t), "es", api.MailQuota{UsedBytes: 5}); e != Vacio {
		t.Error("sin límite configurado el medidor tenía que desaparecer, como en la web")
	}
}

// TestElCuerpoRespetaLosSaltos: `whitespace-pre-wrap`, y una URL más larga que la
// línea se parte en vez de salirse.
func TestElCuerpoRespetaLosSaltos(t *testing.T) {
	l := repartirPre("Hola:\n\nFirma", tema.TextoBase, 0, 400)
	if want := []string{"Hola:", "", "Firma"}; !slices.Equal(l, want) {
		t.Errorf("los saltos del correo se pierden: %q", l)
	}
	larga := "https://" + strings.Repeat("a", 200)
	partes := repartirPre(larga, tema.TextoBase, 0, 120)
	if len(partes) < 2 || strings.Join(partes, "") != larga {
		t.Errorf("la URL larga no se parte o se pierde algo: %d trozos", len(partes))
	}
}

// TestElEsqueletoMideLaFila ata el esqueleto al literal de la web. Estuvo en 83 aquí
// después de que la web lo corrigiera a 83, porque nadie lo comparaba.
func TestElEsqueletoMideLaFila(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/mail/src/components/MessageList.tsx")
	m := regexp.MustCompile(`h-\[([0-9.]+)px\] items-center gap-3 border-b`).FindStringSubmatch(fuente)
	if m == nil {
		t.Fatal("MessageList.tsx ya no escribe el alto del esqueleto como h-[Npx]: hay que rehacer el espejo")
	}
	if m[1] != "83" || AltoEsqueleto != 83 {
		t.Errorf("la web pinta el esqueleto a %spx y aquí a %v", m[1], AltoEsqueleto)
	}
	if AltoEsqueleto != AltoFilaConversacion() {
		t.Errorf("el esqueleto mide %v y la fila %v: el layout salta al llegar los datos", AltoEsqueleto, AltoFilaConversacion())
	}
}

// TestElLectorEnseñaElCuerpoYSuAviso: con el cuerpo leído, la tarjeta desplegada lo
// pinta, y si el correo traía formato lo dice.
func TestElLectorEnseñaElCuerpoYSuAviso(t *testing.T) {
	tr := traductor(t)
	s := servicioDeEjemplo()
	s.crudo["m1"] = "Subject: x\r\nContent-Type: text/html\r\n\r\n<p>Os paso <b>las ventanas</b>.</p>\r\n"
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])

	w := test.NewWindow(b.P.Lector)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(740, 744))
	w.Canvas().Capture()

	var textos []string
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
		if tx, ok := o.(*canvas.Text); ok && tx.Text != "" {
			textos = append(textos, tx.Text)
		}
		for _, h := range hijos(o) {
			mirar(h)
		}
	}
	mirar(b.P.Lector)
	todo := strings.Join(textos, "|")
	if !strings.Contains(todo, "Os paso las ventanas.") {
		t.Errorf("la tarjeta desplegada no pinta el cuerpo; pinta %q", todo)
	}
	if !strings.Contains(strings.Join(textos, " "), tr.T("reader.plainTextOnly", nil)) {
		t.Errorf("el correo traía formato y la tarjeta no lo avisa; pinta %q", todo)
	}
}

// TestLaCapturaConDatosSeGuarda deja el PNG de la pantalla con un hilo abierto cuando
// se pide (SYTUE_CAPTURA_BUZON=ruta.png): es como se mira al lado de la web.
func TestLaCapturaConDatosSeGuarda(t *testing.T) {
	destino := os.Getenv("SYTUE_CAPTURA_BUZON")
	if destino == "" {
		t.Skip("sin SYTUE_CAPTURA_BUZON")
	}
	s := servicioDeEjemplo()
	s.hilo = []api.MailMessage{s.paginas[""][0], s.paginas[""][2]}
	s.crudo["m1"] = "Subject: x\r\nContent-Type: text/html\r\n\r\n<p>Hola equipo:</p><p>Os paso las ventanas de mantenimiento de octubre y noviembre. " +
		"La primera es el martes 14 de 22:00 a 23:30 y la segunda el jueves 20.</p><p>Un saludo,<br>Sofía</p>\r\n"
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])

	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))
	b.P.Refrescar()
	img := w.Canvas().Capture()
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestTraeTuCorreoVaEnElPie: con una conexión de Google en la organización, el pie del
// menú lleva «Trae tus datos» (la pieza común `ui.ImportarDatos`) encima del medidor y
// pregunta por el correo Y los contactos, como `ImportData domains={['mail','contacts']}`;
// sin ella, solo el medidor.
func TestTraeTuCorreoVaEnElPie(t *testing.T) {
	var pedidos []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pedidos = append(pedidos, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/instances":
			_, _ = io.WriteString(w, `{"instances":[{"id":"g","name":"G","template_key":"google","status":"active"}]}`)
		case "/api/v1/migrations/self":
			_, _ = io.WriteString(w, `{"migrations":[{"id":"c","domain":"contacts","status":"done"},{"id":"d","domain":"drive","status":"done"}]}`)
		}
	}))
	defer srv.Close()
	// El pie con «Trae tus datos» es la columna de dos: el bloque y el medidor.
	conTraer := func(b *Buzon) bool {
		c, ok := b.P.Menu.pie().(*fyne.Container)
		if !ok {
			return false
		}
		_, columna := c.Layout.(ui.Columna)
		return columna && len(c.Objects) == 2
	}
	sin, _ := buzonDePrueba(t, servicioDeEjemplo())
	sin.Arrancar(context.Background())
	if conTraer(sin) {
		t.Fatal("sin pasarela el pie lleva algo encima del medidor")
	}
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	b.ConImportar(api.NewIntegraciones(srv.URL, nil, nil), nil)
	b.Arrancar(context.Background())
	for range 100 {
		fyne.DoAndWait(func() {}) // lo que la carga manda pintar corre en la rutina del test
		if conTraer(b) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !conTraer(b) {
		t.Fatalf("el pie no lleva «Trae tus datos» encima del medidor: %T", b.P.Menu.pie())
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(pedidos, "GET /api/v1/instances") || !slices.Contains(pedidos, "GET /api/v1/migrations/self") {
		t.Errorf("la pasarela no se consulta: %v", pedidos)
	}
}

func TestMailAjustesDesplegarYAbrir(t *testing.T) {
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	if b.P.Menu.AjustesAbiertos {
		t.Fatal("los ajustes deben arrancar plegados")
	}
	if b.P.Menu.AlPlegarAjustes == nil {
		t.Fatal("AlPlegarAjustes no está conectado")
	}

	// Plegar/desplegar
	b.P.Menu.AlPlegarAjustes()
	if !b.P.Menu.AjustesAbiertos {
		t.Fatal("AlPlegarAjustes no desplegó la sección")
	}

	// El menú construido debe contener las filas de ajustes
	sec := b.P.Menu.seccionAjustes(tema.Claro)
	if !sec.Abierta || len(sec.Filas) == 0 {
		t.Fatalf("la sección de ajustes abierta no contiene filas: %d", len(sec.Filas))
	}

	// Atajo de teclado Cmd+, / Ctrl+,
	b.P.Menu.AjustesAbiertos = false
	if !b.AtajoPersonalizado(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}) {
		t.Fatal("el atajo de ajustes Cmd+, no fue reconocido")
	}
	if !b.P.Menu.AjustesAbiertos {
		t.Fatal("el atajo de ajustes Cmd+, no desplegó los ajustes")
	}
}
