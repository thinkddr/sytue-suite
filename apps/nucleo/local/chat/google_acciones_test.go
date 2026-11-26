// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// chatFalso es la parte de Google Chat (y de People) de los directos nuevos, los
// ficheros, los nombres y los espacios nuevos.
type chatFalso struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	personas   []string
	preparados []map[string]any
	subidos    []string // nombre|tipo|contenido
	enviados   []map[string]any
	espacioNuevo,
	peopleCerrada bool
}

func nuevoChatFalso(t *testing.T) *chatFalso {
	f := &chatFalso{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *chatFalso) google(lectura, directos bool) *GoogleChat {
	tok, hc := cchat.TokenFijo("g1"), f.srv.Client()
	usuario := func(context.Context) (cchat.UsuarioGoogle, error) {
		return cchat.UsuarioGoogle{Sujeto: "123", Correo: "ana@empresa.eu", Nombre: "Ana"}, nil
	}
	g := NuevaGoogleChat(&cchat.GoogleChat{Base: f.srv.URL + "/v1", Token: tok, HTTP: hc}, nil, usuario, "Google Chat · ana@empresa.eu", lectura,
		Avisos{PersonaGoogle: "Persona de Google", SinLectura: "para leer, un cliente propio"})
	g.P = &cchat.PersonasGoogle{Base: f.srv.URL + "/people/v1", Token: tok, HTTP: hc}
	g.Directos = directos
	return g
}

func (f *chatFalso) servir(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer g1" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	switch ruta := r.URL.Path; {
	case ruta == "/v1/spaces":
		v := []any{map[string]any{"name": "spaces/AAA", "displayName": "Equipo", "spaceType": "SPACE", "lastActiveTime": "2026-11-20T09:00:00Z"}}
		if f.espacioNuevo {
			v = append(v, map[string]any{"name": "spaces/NUE", "displayName": "Lanzamiento", "spaceType": "SPACE"})
		}
		if len(f.preparados) > 0 {
			v = append(v, map[string]any{"name": "spaces/DMCARLA", "spaceType": "DIRECT_MESSAGE"})
		}
		escribir(w, map[string]any{"spaces": v})
	case ruta == "/v1/spaces:findDirectMessage":
		f.personas = append(f.personas, "buscar "+q.Get("name"))
		if q.Get("name") == "users/456" {
			escribir(w, map[string]any{"name": "spaces/DMBEA", "spaceType": "DIRECT_MESSAGE"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case ruta == "/v1/spaces:setup" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.preparados = append(f.preparados, c)
		escribir(w, map[string]any{"name": "spaces/DMCARLA", "spaceType": "DIRECT_MESSAGE"})
	case strings.HasPrefix(ruta, "/people/v1/people/") && f.peopleCerrada:
		f.personas = append(f.personas, "403")
		w.WriteHeader(http.StatusForbidden)
	case ruta == "/people/v1/people/456":
		f.personas = append(f.personas, "people 456")
		escribir(w, map[string]any{"names": []any{map[string]string{"displayName": "Bea Gil"}}})
	case strings.HasPrefix(ruta, "/people/v1/people/"):
		f.personas = append(f.personas, "people "+strings.TrimPrefix(ruta, "/people/v1/people/"))
		w.WriteHeader(http.StatusNotFound)
	case ruta == "/upload/v1/spaces/AAA/attachments:upload" && r.Method == http.MethodPost:
		if q.Get("uploadType") != "multipart" {
			f.t.Errorf("subida: %s", r.URL)
		}
		tipo, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || tipo != "multipart/related" {
			f.t.Errorf("tipo de la subida: %q", r.Header.Get("Content-Type"))
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		meta, _ := mr.NextPart()
		var m map[string]string
		_ = json.NewDecoder(meta).Decode(&m)
		datos, _ := mr.NextPart()
		b, _ := io.ReadAll(datos)
		f.subidos = append(f.subidos, m["filename"]+"|"+datos.Header.Get("Content-Type")+"|"+string(b))
		escribir(w, map[string]any{"attachmentDataRef": map[string]string{"resourceName": "ref/acta-1", "attachmentUploadToken": "tok-1"}})
	case ruta == "/v1/spaces/AAA/messages" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.enviados = append(f.enviados, c)
		escribir(w, map[string]any{
			"name": "spaces/AAA/messages/n1", "text": c["text"], "createTime": "2026-11-20T09:10:00Z", "sender": map[string]string{"name": "users/123"},
			"thread": map[string]string{"name": "spaces/AAA/threads/n1"},
			"attachment": []any{map[string]any{
				"name": "spaces/AAA/messages/n1/attachments/a1", "contentName": "acta.pdf", "contentType": "application/pdf", "source": "UPLOADED_CONTENT",
				"attachmentDataRef": map[string]string{"resourceName": "ref/acta-1"},
			}},
		})
	case ruta == "/v1/spaces/AAA/messages" && r.Method == http.MethodGet:
		escribir(w, map[string]any{"messages": []any{
			map[string]any{"name": "spaces/AAA/messages/m2", "text": "¿Vemos el presupuesto?", "createTime": "2026-11-20T08:10:00Z", "sender": map[string]string{"name": "users/789", "type": "HUMAN"}, "thread": map[string]string{"name": "spaces/AAA/threads/h2"}},
			map[string]any{
				"name": "spaces/AAA/messages/m1", "text": "el plano", "createTime": "2026-11-20T08:00:00Z", "sender": map[string]string{"name": "users/456", "type": "HUMAN"}, "thread": map[string]string{"name": "spaces/AAA/threads/h1"},
				"attachment": []any{
					map[string]any{"name": "spaces/AAA/messages/m1/attachments/a1", "contentName": "plano.png", "contentType": "image/png", "attachmentDataRef": map[string]string{"resourceName": "ref/plano"}},
					map[string]any{"name": "spaces/AAA/messages/m1/attachments/a2", "contentName": "Hoja", "driveDataRef": map[string]string{"driveFileId": "d1"}, "source": "DRIVE_FILE"},
				},
			},
		}})
	case ruta == "/v1/spaces/AAA/members":
		escribir(w, map[string]any{"memberships": []any{map[string]any{"member": map[string]string{"name": "users/123"}}}})
	case strings.HasPrefix(ruta, "/v1/media/ref/") && q.Get("alt") == "media":
		_, _ = io.WriteString(w, "contenido de "+strings.TrimPrefix(ruta, "/v1/media/"))
	default:
		f.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

// Un directo: el que ya hay se busca con el permiso de siempre; uno nuevo pide el de los
// directos (sin él, se dice) y se crea con `spaces.setup`, con el correo de la persona,
// que es como se llama el directo.
func TestGoogleAbreDirectos(t *testing.T) {
	f := nuevoChatFalso(t)
	ctx := t.Context()
	g := f.google(false, false)
	ch, err := g.AbrirDirecto(ctx, []string{"users/456"})
	if err != nil || ch.ID != "spaces/DMBEA" || ch.Kind != "direct" || ch.Name != "Bea Gil" {
		t.Fatal(ch, err)
	}
	p, err := g.Resolver(ctx, "Carla@Empresa.eu")
	if err != nil || p.UserID != "users/carla@empresa.eu" {
		t.Fatal(p, err)
	}
	if _, err := g.AbrirDirecto(ctx, []string{p.UserID}); !errors.Is(err, ErrSinDirectosGoogle) {
		t.Fatal(err)
	}
	g = f.google(false, true)
	ch, err = g.AbrirDirecto(ctx, []string{p.UserID})
	if err != nil || ch.ID != "spaces/DMCARLA" || ch.Name != "carla@empresa.eu" {
		t.Fatal(ch, err)
	}
	if cs, _, _ := g.Canales(ctx); len(cs) != 2 || cs[1].ID != "spaces/DMCARLA" || cs[1].Name != "carla@empresa.eu" {
		t.Fatal("el directo abierto sigue en la lista con su nombre:", cs)
	}
	if _, err := g.AbrirDirecto(ctx, []string{"users/x?y"}); !errors.Is(err, ErrNoEncontrada) {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.preparados) != 1 {
		t.Fatal(f.preparados)
	}
	c, _ := json.Marshal(f.preparados[0])
	if string(c) != `{"memberships":[{"member":{"name":"users/carla@empresa.eu","type":"HUMAN"}}],"space":{"spaceType":"DIRECT_MESSAGE"}}` {
		t.Fatal(string(c))
	}
}

// Los ficheros: se suben al espacio (multipart con su nombre) y van en el mensaje con su
// referencia; los que llegan se bajan por `media` (con Lectura) y los de Drive no.
func TestGoogleAdjuntosSeMandanYSeBajan(t *testing.T) {
	f := nuevoChatFalso(t)
	ctx := t.Context()
	g := f.google(true, false)
	fichero := Fichero{Nombre: "acta.pdf", Tipo: "application/pdf", Tamano: 4, Abrir: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("%PDF")), nil }}
	m, err := g.EnviarFicheros(ctx, "spaces/AAA", "", "el acta", []Fichero{fichero})
	if err != nil || m.Body != "el acta" || len(m.Attachments) != 1 || m.Attachments[0].Name != "acta.pdf" || m.AuthorSub != Yo {
		t.Fatal(m, err)
	}
	f.mu.Lock()
	if strings.Join(f.subidos, ";") != "acta.pdf|application/pdf|%PDF" || len(f.enviados) != 1 {
		t.Fatal(f.subidos, f.enviados)
	}
	c, _ := json.Marshal(f.enviados[0])
	if string(c) != `{"attachment":[{"attachmentDataRef":{"attachmentUploadToken":"tok-1","resourceName":"ref/acta-1"}}],"text":"el acta"}` {
		t.Fatal(string(c))
	}
	f.mu.Unlock()
	leer := func(id string) (string, error) {
		rc, err := g.Bajar(ctx, "spaces/AAA", id, false)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		return string(b), err
	}
	if s, err := leer(m.Attachments[0].FileID); err != nil || s != "contenido de ref/acta-1" {
		t.Fatal(s, err)
	}
	ms, err := g.Mensajes(ctx, "spaces/AAA", time.Time{}, 50)
	if err != nil || len(ms) != 2 || len(ms[0].Attachments) != 2 || ms[0].Attachments[0].ContentType != "image/png" {
		t.Fatal(ms, err)
	}
	if s, err := leer(ms[0].Attachments[0].FileID); err != nil || s != "contenido de ref/plano" {
		t.Fatal(s, err)
	}
	if _, err := leer(ms[0].Attachments[1].FileID); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("un fichero de Drive:", err)
	}
	if _, err := g.C.Bajar(ctx, "../../otra"); !errors.Is(err, cchat.ErrEntrada) {
		t.Fatal(err)
	}
}

// Los nombres: Google no los da con la autenticación de la persona; la People API sí,
// si los enseña (Bea); si no (404), «Persona de Google». No se pregunta dos veces por
// nadie, y tras un 403 no se pregunta más. La búsqueda encuentra por el nombre sabido.
func TestGoogleNombresPorPeopleYBusqueda(t *testing.T) {
	f := nuevoChatFalso(t)
	ctx := t.Context()
	g := f.google(true, false)
	ms, err := g.Mensajes(ctx, "spaces/AAA", time.Time{}, 50)
	if err != nil || len(ms) != 2 || ms[0].AuthorName != "Bea Gil" || ms[1].AuthorName != "Persona de Google" {
		t.Fatal(ms, err)
	}
	if _, err := g.Mensajes(ctx, "spaces/AAA", time.Time{}, 50); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	slices.Sort(f.personas)
	if strings.Join(f.personas, ",") != "people 456,people 789" {
		t.Fatal(f.personas)
	}
	f.personas = nil
	f.peopleCerrada = true
	f.mu.Unlock()
	if r, _ := g.Buscar(ctx, "bea plano", 10); len(r) != 1 || r[0].ID != "m1" {
		t.Fatal(r)
	}
	if r, _ := g.Buscar(ctx, "presupuesto", 10); len(r) != 1 || r[0].ID != "m2" {
		t.Fatal(r)
	}
	otra := f.google(true, false)
	if ms, _ := otra.Mensajes(ctx, "spaces/AAA", time.Time{}, 50); ms[0].AuthorName != "Persona de Google" {
		t.Fatal(ms)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.personas, ",") != "403" {
		t.Fatal("tras el 403 se siguió preguntando:", f.personas)
	}
}

// Un espacio nuevo aparece sin recargar también sin Lectura: se mira la lista de
// espacios (que se ve con el permiso de siempre) y se avisa una vez.
func TestGoogleEspacioNuevoApareceSolo(t *testing.T) {
	f := nuevoChatFalso(t)
	g := f.google(false, false)
	g.SondeoMax = 10 * time.Millisecond
	ctx, parar := context.WithCancel(t.Context())
	defer parar()
	if _, _, err := g.Canales(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.espacioNuevo = true
	f.mu.Unlock()
	avisos := make(chan string, 4)
	go func() { _ = g.Escuchar(ctx, func(ev api.RoomsEvent) { avisos <- ev.Type }) }()
	if ev := <-avisos; ev != EventoCanales {
		t.Fatal(ev)
	}
	cs, _, err := g.Canales(ctx)
	if err != nil || len(cs) != 2 || cs[1].Name != "Lanzamiento" {
		t.Fatal(cs, err)
	}
	select {
	case ev := <-avisos:
		t.Fatal("se avisó otra vez:", ev)
	case <-time.After(60 * time.Millisecond):
	}
}
