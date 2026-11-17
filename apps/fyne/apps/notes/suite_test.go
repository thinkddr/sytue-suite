// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// conSuite le da a la pantalla los clientes de Drive y Rooms contra el mismo servicio
// falso, como hace el `main` contra los de verdad.
func conSuite(t *testing.T, p *Pantalla, respuestas map[string]string) *servicioFalso {
	t.Helper()
	falso := &servicioFalso{respuesta: respuestas}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	sesion := auth.NewSession(auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "owner", Access: "synthetic", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error { return nil })
	p.Suite = &api.Elegir{Apex: api.ApexProd, Drive: api.NewDrive(srv.URL, sesion, nil), Rooms: api.NewRooms(srv.URL, sesion, nil)}
	return falso
}

// «Compartir en Rooms» va entre «Exportar» e «Información», en la tarjeta y en el
// árbol, como en NoteCard.tsx y NoteTree.tsx. Sin con qué publicar, no se ofrece.
func TestCompartirEnRoomsEstaEnSuSitio(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1"})
	n, _ := p.nota("a")
	tr := p.tr.T
	textos := func(m *ui.Menu) []string {
		var out []string
		for _, e := range m.Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok {
				out = append(out, it.Texto)
			}
		}
		return out
	}
	if slices.Contains(textos(p.menuNota(n, false)), tr("share.toRoom", nil)) {
		t.Error("sin clientes de la suite se ofrece «Compartir en Rooms», que siempre fallaría")
	}
	conSuite(t, p, nil)
	for _, arbol := range []bool{false, true} {
		got := textos(p.menuNota(n, arbol))
		i := slices.Index(got, tr("share.toRoom", nil))
		if i < 1 || got[i-1] != tr("note.export", nil) || got[i+1] != tr("note.info", nil) {
			t.Errorf("árbol=%v: «Compartir en Rooms» no va entre «Exportar» e «Información»: %v", arbol, got)
		}
	}
}

// Lo que llega a la sala es el ENLACE de la nota con su título, nunca su contenido, y el
// aviso dice dónde se ha compartido.
func TestCompartirEnSalaPublicaElEnlaceYNoElContenido(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Title: "Presupuesto", Content: "<p>secreto</p>"},
		api.Note{ID: "b", NotebookID: "c1"})
	falso := conSuite(t, p, map[string]string{"POST /channels/s1/messages": `{"id":"m1"}`})
	n, _ := p.nota("a")
	p.publicarEnSala("c1", n, api.Elegible{ID: "s1", Nombre: "General"})
	cuerpo := falso.ultima(t, "POST /channels/s1/messages")
	quiero := "📝 Presupuesto\nhttps://notes.sytue.eu/notes/?notebook=c1&note=a"
	if cuerpo["body"] != quiero {
		t.Errorf("a la sala llega %q y tiene que ser %q", cuerpo["body"], quiero)
	}
	if strings.Contains(cuerpo["body"].(string), "secreto") {
		t.Error("el contenido de la nota ha viajado a la sala")
	}
	// Una nota sin título se publica como «Sin título», del catálogo.
	sin, _ := p.nota("b")
	p.publicarEnSala("c1", sin, api.Elegible{ID: "s1", Nombre: "General"})
	if got := falso.ultima(t, "POST /channels/s1/messages")["body"].(string); !strings.HasPrefix(got, "📝 "+p.tr.T("note.untitled", nil)+"\n") {
		t.Errorf("sin título se publica %q", got)
	}
}

// La herramienta de enlaces ofrece los ficheros de Drive que casan —no las carpetas—,
// con su enlace canónico y rotulados «Sytue Drive».
func TestLaHerramientaDeEnlacesOfreceFicherosDeDrive(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	conSuite(t, p, map[string]string{
		"QUERY /search": `{"files":[{"id":"F9","name":"Actas","is_directory":true},
			{"id":"f1","name":"acta.pdf","is_directory":false,"size_bytes":10}]}`,
	})
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	ops := p.opcionesDrive(ctx, "acta")
	if len(ops) != 1 || ops[0].url != "https://drive.sytue.eu/drive/files/f1" || ops[0].rotulo != "acta.pdf" ||
		ops[0].cuaderno != MarcaDrive || ops[0].tipo != enlaceDrive {
		t.Fatalf("la herramienta de enlaces ofrece %+v", ops)
	}
	// Van detrás de las notas y bajo su rótulo, como en LinkTool.tsx.
	nota := opcionEnlace{url: "u", rotulo: "Mudanza", nota: "Mudanza", tipo: enlaceLocal}
	col := p.filasEnlace([]opcionEnlace{nota, ops[0]}, 0, false, func(opcionEnlace) {}, p.variante())
	// «Este cuaderno», la nota, «Sytue Drive» y el fichero: cuatro filas.
	if n := len(col.(*fyne.Container).Objects); n != 4 {
		t.Errorf("la lista tiene %d filas y son cuatro: cada grupo con su rótulo", n)
	}
}

// Un presentador privado no puede usar los clientes HTTP de Rooms o Calendar,
// aunque un montaje futuro les conecte una Suite y el cuaderno diga encrypted=false.
func TestContenidoPrivadoNoSalePorSuiteHTTP(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	work, err := notas.NewLocalWork(t.Context(), scope, almacenDePrueba(t), func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.activo = DeCuaderno(api.Notebook{ID: "c1"})
	n := Nota{ID: "n1", Cuaderno: "c1", Titulo: "secreto"}
	falso := conSuite(t, p, nil)
	p.Suite.Calendar = api.NewCalendars("https://calendar.invalid", nil, nil)
	if p.permiteEnviarContenidoExterno("c1", n) {
		t.Fatal("un repositorio privado se trató como fuente pública")
	}
	p.crearEvento(n)
	p.compartirEnSala(n)
	if got := falso.rutas(); len(got) != 0 {
		t.Fatalf("el título privado llegó a la red: %v", got)
	}
}

func TestPermisoDeEnvioCaducaAlCambiarCuaderno(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1"})
	n, _ := p.nota("n1")
	conSuite(t, p, nil)
	if !p.permiteEnviarContenidoExterno("c1", n) {
		t.Fatal("se bloqueó un cuaderno ordinario")
	}
	p.activo = p.cuadernos[1]
	if p.permiteEnviarContenidoExterno("c1", n) {
		t.Fatal("una selección antigua sobrevivió al cambio de cuaderno")
	}
	p.activo = p.cuadernos[0]
	p.activo.Cifrado = true
	if p.permiteEnviarContenidoExterno("c1", n) {
		t.Fatal("un cuaderno cifrado pudo enviar su título")
	}
}

func TestSuiteNoEnviaTrasCambioDeCuentaInclusoABA(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1", Title: "secreto"})
	falso := &servicioFalso{}
	servidor := httptest.NewServer(falso)
	t.Cleanup(servidor.Close)
	primera := auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "alice", Access: "a", Expiry: time.Now().Add(time.Hour)}
	otra := primera
	otra.Subject, otra.Access = "bob", "b"
	sesion := auth.NewSession(primera, nil, nil)
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error { return nil })
	p.Suite = &api.Elegir{Apex: api.ApexProd, Rooms: api.NewRooms(servidor.URL, sesion, nil)}
	ctx, suite := p.capturarAutorizacionSuite(), p.Suite
	sesion.Sincronizar(otra)
	sesion.Sincronizar(primera)
	n, _ := p.nota("n1")
	p.publicarEnSalaConContexto(ctx, suite, "", "c1", n, api.Elegible{ID: "s1"})
	if got := falso.rutas(); len(got) != 0 {
		t.Fatalf("una intención retirada envió contenido: %v", got)
	}
	if auth.RequestAuthorizationIntent(ctx).Current() {
		t.Fatal("ABA revivió la intención")
	}
}

func TestIntencionSuiteNuevaPorGestoConTestigo(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1"})
	tokens := auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "alice", Access: "first", Expiry: time.Now().Add(time.Hour)}
	sesion := auth.NewSession(tokens, nil, nil)
	vigente := true
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error {
		if !vigente {
			return auth.ErrAuthorizationChanged
		}
		return nil
	})
	old := p.capturarAutorizacionSuite()
	tokens.Access = "renewed"
	sesion.Sincronizar(tokens)
	if auth.RequestAuthorizationIntent(old).Current() || p.capturarAutorizacionSuite() == nil {
		t.Fatal("la renovación de la misma cuenta impidió un nuevo gesto")
	}
	vigente = false // a new login invalidates the persisted witness, including A-B-A.
	sesion.Sincronizar(auth.Tokens{Issuer: tokens.Issuer, Cliente: tokens.Cliente, Subject: "bob", Access: "b", Expiry: tokens.Expiry})
	sesion.Sincronizar(tokens)
	if p.capturarAutorizacionSuite() != nil {
		t.Fatal("ABA adoptó una vida retirada")
	}
}

func TestEnvioSuiteCancelaPeticionConVida(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sin leer el cuerpo del POST, net/http no vigila la conexión y r.Context() no
		// se cancela cuando el cliente la corta: server.Close esperaría para siempre.
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1", Title: "secret"})
	tokens := auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "alice", Access: "a", Expiry: time.Now().Add(time.Hour)}
	sesion := auth.NewSession(tokens, nil, nil)
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error { return nil })
	suite := &api.Elegir{Apex: api.ApexProd, Rooms: api.NewRooms(server.URL, sesion, nil)}
	ctx, cancelar := context.WithCancel(p.capturarAutorizacionSuite())
	n, _ := p.nota("n1")
	done := make(chan struct{})
	go func() { p.publicarEnSalaConContexto(ctx, suite, "", "c1", n, api.Elegible{ID: "s1"}); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		cancelar()
		t.Fatal("request did not begin")
	}
	cancelar()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not stop after cancellation")
	}
}

func TestEdicionDeCalendarRetiradaNoPintaNiEnvia(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1", Title: "original"})
	tokens := auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "alice", Access: "a", Expiry: time.Now().Add(time.Hour)}
	sesion := auth.NewSession(tokens, nil, nil)
	vigente := true
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error {
		if !vigente {
			return auth.ErrAuthorizationChanged
		}
		return nil
	})
	ctx := p.capturarAutorizacionSuite()
	queued := make(chan func(), 1)
	release := make(chan struct{})
	p.vida().dispatchWait = func(fn func()) { queued <- fn; <-release }
	done := make(chan error, 1)
	go func() {
		_, err := p.editarConEntrada(ctx, "n1", Parche{Enlaces: []api.NoteLink{{TargetKind: "calendar_event", TargetID: "event"}}}, false, "", nil)
		done <- err
	}()
	var callback func()
	select {
	case callback = <-queued:
	case <-time.After(time.Second):
		t.Fatal("UI callback not scheduled")
	}
	tokens.Subject, tokens.Access = "bob", "b"
	vigente = false
	sesion.Sincronizar(tokens)
	callback()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, auth.ErrAuthorizationChanged) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("edit did not finish")
	}
	if len(falso.rutas()) != 0 || len(p.notas[0].Enlaces) != 0 {
		t.Fatal("retired event modified the new presenter")
	}
}

func TestUITrasRefreshUsaTestigoDeVida(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	tokens := auth.Tokens{Issuer: "https://id.invalid", Cliente: "notes", Subject: "alice", Access: "a", Expiry: time.Now().Add(time.Hour)}
	sesion := auth.NewSession(tokens, nil, nil)
	vigente := true
	p.VincularAutorizacionSuite(sesion.WithAuthorizationIntent, func(context.Context) error {
		if !vigente {
			return auth.ErrAuthorizationChanged
		}
		return nil
	})
	ctx := p.capturarAutorizacionSuite()
	tokens.Access = "renewed"
	sesion.Sincronizar(tokens)
	if p.suiteSigueVigente(ctx) || !p.suiteVidaVigente(ctx) {
		t.Fatal("refresh should retire the network intent, not the UI owner")
	}
	vigente = false
	if p.suiteVidaVigente(ctx) {
		t.Fatal("UI callback survived a new login")
	}
}
