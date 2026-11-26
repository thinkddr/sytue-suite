// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

type libretaDAV struct {
	srv   *davprueba.Servidor
	repo  *libreta.Libreta
	s     *agenda.Sincronizador
	col   string
	libro string
	t     *testing.T
}

func montarLibreta(t *testing.T) *libretaDAV {
	t.Helper()
	srv := davprueba.Nuevo()
	t.Cleanup(srv.Close)
	col := davprueba.CasaCard + "libreta/"
	srv.Crear(col, "Mis contactos", false)
	dir := t.TempDir()
	repo := abrirContactos(t, dir)
	cli, err := dav.Nuevo(dav.Cuenta{Direccion: srv.URL + "/card/", Usuario: srv.Usuario, Clave: srv.Clave}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	s := &agenda.Sincronizador{
		Remoto: &agenda.RemotoDAV{Cliente: cli, Clase: dav.Libreta}, Local: agenda.LocalContactos{Repo: repo},
		Estado: filepath.Join(dir, "agenda", "c.json"), MarcaCopia: "(copia en conflicto)",
	}
	return &libretaDAV{srv: srv, repo: repo, s: s, col: col, t: t}
}

func (l *libretaDAV) sinc() agenda.Resultado {
	l.t.Helper()
	res, err := l.s.Sincronizar(l.t.Context())
	if err != nil || len(res.Errores) > 0 {
		l.t.Fatalf("%v %v", err, res.Errores)
	}
	if l.libro == "" {
		l.libro = l.s.Colecciones()[0].ID
	}
	return res
}

func (l *libretaDAV) grupos() map[string]api.ContactGroup {
	l.t.Helper()
	gs, err := l.repo.Groups(l.t.Context(), l.libro)
	if err != nil {
		l.t.Fatal(err)
	}
	out := map[string]api.ContactGroup{}
	for _, g := range gs {
		out[g.Name] = g
	}
	return out
}

// Los grupos de CardDAV (KIND:group de la 4.0 y X-ADDRESSBOOKSERVER-KIND de iCloud) son
// las etiquetas de la libreta: bajan con sus miembros y su UID, y las etiquetas de aquí
// suben como grupos. Un borrado aquí se borra allí.
func TestGruposDeCardDAVSonEtiquetas(t *testing.T) {
	const (
		ana     = "0a0a0a0a-0000-4000-8000-00000000000a"
		bea     = "0b0b0b0b-0000-4000-8000-00000000000b"
		equipo  = "0e0e0e0e-0000-4000-8000-00000000000e"
		familia = "0f0f0f0f-0000-4000-8000-00000000000f"
	)
	l := montarLibreta(t)
	l.srv.Poner(l.col+"ana.vcf", []byte(vcf("urn:uuid:"+ana, "Ana")))
	l.srv.Poner(l.col+"bea.vcf", []byte(vcf("urn:uuid:"+bea, "Bea")))
	l.srv.Poner(l.col+"equipo.vcf", []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:urn:uuid:"+equipo+"\r\nKIND:group\r\nFN:Equipo\r\nMEMBER:urn:uuid:"+ana+"\r\nX-PROPIO:se queda\r\nEND:VCARD\r\n"))
	l.srv.Poner(l.col+"familia.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:"+familia+"\r\nN:Familia;;;;\r\nFN:Familia\r\nX-ADDRESSBOOKSERVER-KIND:group\r\n"+
		"X-ADDRESSBOOKSERVER-MEMBER:urn:uuid:"+ana+"\r\nX-ADDRESSBOOKSERVER-MEMBER:urn:uuid:"+bea+"\r\nEND:VCARD\r\n"))
	if res := l.sinc(); res.Bajados != 4 {
		t.Fatalf("bajada: %+v", res)
	}
	gs := l.grupos()
	if g := gs["Equipo"]; g.UID != equipo || !slices.Equal(g.Members, []string{ana}) {
		t.Fatalf("Equipo: %+v", gs)
	}
	if g := gs["Familia"]; g.UID != familia || len(g.Members) != 2 {
		t.Fatalf("Familia (de iCloud): %+v", gs)
	}
	if ts, _ := l.repo.TarjetasCrudas(t.Context(), l.libro); len(ts) != 2 {
		t.Fatalf("los grupos no son contactos: %+v", ts)
	}
	// Una vuelta sin cambios: nada.
	if res := l.sinc(); res.Subidos+res.Bajados+res.Borrados != 0 {
		t.Fatalf("en vacío: %+v", res)
	}

	// Aquí: Bea entra en Equipo y se crea la etiqueta «Clientes» con Ana.
	if _, err := l.repo.RenameGroup(t.Context(), gs["Equipo"].ID, "Equipo", []string{ana, bea}); err != nil {
		t.Fatal(err)
	}
	cl, err := l.repo.CreateGroup(t.Context(), l.libro, "Clientes")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.repo.BatchGroupMembers(t.Context(), cl.ID, []string{ana}, true); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Subidos != 2 {
		t.Fatalf("subida: %+v", res)
	}
	eq := string(l.srv.Datos(l.col + "equipo.vcf"))
	if !strings.Contains(eq, "MEMBER:urn:uuid:"+bea) || !strings.Contains(eq, "UID:urn:uuid:"+equipo) || !strings.Contains(eq, "X-PROPIO:se queda") {
		t.Fatalf("Equipo en el servidor: %q", eq)
	}
	nuevo := ""
	for _, r := range l.srv.Rutas(l.col) {
		if d := string(l.srv.Datos(r)); strings.Contains(d, "FN:Clientes") {
			nuevo = d
		}
	}
	if !strings.Contains(nuevo, "KIND:group") || !strings.Contains(nuevo, "MEMBER:urn:uuid:"+ana) || !strings.Contains(nuevo, cl.UID) {
		t.Fatalf("Clientes en el servidor: %q", nuevo)
	}

	// Allí: Familia pierde a Bea. Aquí se ve.
	l.srv.Poner(l.col+"familia.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:"+familia+"\r\nN:Familia;;;;\r\nFN:Familia\r\nX-ADDRESSBOOKSERVER-KIND:group\r\n"+
		"X-ADDRESSBOOKSERVER-MEMBER:urn:uuid:"+ana+"\r\nEND:VCARD\r\n"))
	l.sinc()
	if g := l.grupos()["Familia"]; !slices.Equal(g.Members, []string{ana}) {
		t.Fatalf("Familia tras el cambio: %+v", g)
	}

	// Se borra la etiqueta aquí: el grupo se va del servidor.
	if err := l.repo.DeleteGroup(t.Context(), l.grupos()["Equipo"].ID); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Borrados != 1 || l.srv.Datos(l.col+"equipo.vcf") != nil {
		t.Fatalf("borrado: %+v", res)
	}
}

// Una libreta que no quiere vCard 4.0 la recibe en 3.0, y lo que se baja sigue siendo
// el mismo contacto aquí.
func TestLibretaQueSoloQuiereVCard3(t *testing.T) {
	l := montarLibreta(t)
	l.srv.Solo3 = true
	l.sinc()
	c, err := l.repo.Create(t.Context(), l.libro, api.ContactInput{
		FormattedName: "Iker Sanz", GivenName: "Iker", FamilyName: "Sanz",
		Emails: []api.ContactEmail{{Email: "iker@example.org", IsPrimary: true}}, Phones: []api.ContactPhone{{Phone: "+34600111222", Type: "cell", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	g, err := l.repo.CreateGroup(t.Context(), l.libro, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.repo.BatchGroupMembers(t.Context(), g.ID, []string{c.UID}, true); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Subidos != 2 || res.Pendientes != 0 {
		t.Fatalf("subida: %+v", res)
	}
	var tarjeta, grupo string
	for _, r := range l.srv.Rutas(l.col) {
		d := string(l.srv.Datos(r))
		if strings.Contains(d, "Iker") {
			tarjeta = d
		}
		if strings.Contains(d, "Amigos") {
			grupo = d
		}
	}
	if !strings.Contains(tarjeta, "VERSION:3.0") || !strings.Contains(tarjeta, "iker@example.org") || !strings.Contains(tarjeta, "N:Sanz;Iker") {
		t.Fatalf("tarjeta: %q", tarjeta)
	}
	if !strings.Contains(grupo, "VERSION:3.0") || !strings.Contains(grupo, "X-ADDRESSBOOKSERVER-KIND:group") ||
		!strings.Contains(grupo, "X-ADDRESSBOOKSERVER-MEMBER:urn:uuid:"+c.UID) || strings.Contains(grupo, "KIND:group\r\nMEMBER") {
		t.Fatalf("grupo: %q", grupo)
	}
	// Se cambia aquí: sube otra vez en 3.0, sin conflicto.
	if _, err := l.repo.Update(t.Context(), c.ID, api.ContactInput{FormattedName: "Iker Sanz Gil", GivenName: "Iker", FamilyName: "Sanz Gil"}, ""); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Subidos != 1 || len(res.Conflictos) != 0 {
		t.Fatalf("cambio: %+v", res)
	}
	if res := l.sinc(); res.Subidos+res.Bajados != 0 || len(res.Conflictos) != 0 {
		t.Fatalf("en vacío: %+v", res)
	}
}

// Crear un calendario en la cuenta (MKCALENDAR) desde Calendar: aparece aquí con la
// sincronización siguiente, en la sección de su cuenta.
func TestCrearCalendarioEnLaCuenta(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	srv.Crear(davprueba.CasaCal+"personal/", "Personal", false)
	ajustes := t.TempDir()
	r := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), nil)
	r.HTTP = func(red.Proposito) *http.Client { return srv.Client() }
	c, err := r.AnadirDAV(t.Context(), agenda.Alta{Rotulo: "Nextcloud", Usuario: srv.Usuario, Clave: srv.Clave, Calendario: srv.URL + "/cal/"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.PuedeCrear(c, agenda.Calendario) || r.PuedeCrear(c, agenda.Contactos) {
		t.Fatal("PuedeCrear")
	}
	if err := r.CrearCalendario(t.Context(), c.ID, "Viajes", "#AA3300"); err != nil {
		t.Fatal(err)
	}
	if err := r.CrearCalendario(t.Context(), "otra", "x", ""); err == nil {
		t.Fatal("creó en una cuenta que no existe")
	}
	cal := abrirCalendario(t, t.TempDir())
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, agenda.LocalCalendario{Repo: cal}, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	gs := r.Grupos(agenda.Calendario, agenda.LocalCalendario{Repo: cal}, dir)
	nombres := make([]string, 0, len(gs[0].Colecciones))
	for _, x := range gs[0].Colecciones {
		nombres = append(nombres, x.Nombre)
	}
	if !slices.Equal(nombres, []string{"Personal", "Viajes"}) {
		t.Fatalf("calendarios: %v", nombres)
	}
	cals, _ := cal.List(t.Context())
	for _, x := range cals {
		if x.DisplayName == "Viajes" && (x.ReadOnly || !strings.EqualFold(x.Color, "#AA3300")) {
			t.Fatalf("el nuevo: %+v", x)
		}
	}
}
