// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// hubDeMentira es el Hub de la campana: lista, contador, flujo SSE, «leído» y «todas».
type hubDeMentira struct {
	mu       sync.Mutex
	sinLeer  int
	leidos   []string
	todas    int
	listas   int
	enviar   chan string
	conexion chan struct{}
}

func (h *hubDeMentira) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch p := r.URL.Path; {
	case p == "/api/v1/notifications":
		h.leer(func() { h.listas++ })
		_, _ = fmt.Fprint(w, `{"notifications":[]}`)
	case p == "/api/v1/notifications/unread_count":
		var n int
		h.leer(func() { n = h.sinLeer })
		_, _ = fmt.Fprintf(w, `{"unread":%d}`, n)
	case p == "/api/v1/notifications/stream":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		select { // sin bloquear: tras un corte reconecta una vez por segundo
		case h.conexion <- struct{}{}:
		default:
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case d, ok := <-h.enviar:
				if !ok {
					return // corta el flujo: la campana tiene que reconectar
				}
				if !strings.HasPrefix(d, "event: ") { // un aviso; si trae su `event:`, va tal cual
					d = "event: notification\ndata: " + d
				}
				_, _ = fmt.Fprintf(w, "%s\n\n", d)
				w.(http.Flusher).Flush()
			}
		}
	case p == "/api/v1/notifications/read_all":
		h.leer(func() { h.todas, h.sinLeer = h.todas+1, 0 })
	case strings.HasSuffix(p, "/read"):
		h.leer(func() {
			h.leidos = append(h.leidos, strings.Split(p, "/")[4])
			h.sinLeer = max(h.sinLeer-1, 0)
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *hubDeMentira) leer(f func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f()
}

// La campana de la barra, de punta a punta contra un Hub de mentira: un aviso que llega
// por el flujo se ve en el panel ABIERTO con el título en el idioma de la app, cuenta,
// al pulsarlo deja de contar y abre su elemento en la app que toca; «marcar todas»
// deja el contador a cero; y tras un corte del flujo se recarga la lista. El ancho del
// panel es el de la web a 1280 y a 360.
//
// VISTO EN ROJO: sin `Reemplazar` (con el `Elementos =` de antes) el panel abierto no
// enseña el aviso nuevo; sin el `cargar` tras reconectar, la lista no se vuelve a pedir;
// sin `TituloAviso`, la fila lleva el título del emisor.
func TestLaCampanaRecibeMarcaYAbreLosAvisos(t *testing.T) {
	hub := &hubDeMentira{enviar: make(chan string, 4), conexion: make(chan struct{}, 4)}
	srv := httptest.NewServer(hub)
	// Con t.Cleanup y ANTES que `a.parar`: las limpiezas van al revés, así que la campana
	// suelta el flujo primero y el servidor no se queda esperando a un manejador vivo.
	t.Cleanup(srv.Close)

	s := suiteConSesion(t)
	s.Hub = api.NewHub(srv.URL, sesionDePrueba{}, nil)
	tr := traductorPrueba(t)
	a := nuevoAvisosBarra(tr, s)
	var idas []string
	a.irA = func(producto, u string) { idas = append(idas, producto+" "+u) }
	t.Cleanup(a.parar)
	w := test.NewWindow(a.boton)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)

	a.sesionCambio(s.Estado())
	esperarConexion(t, hub)
	esperar(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.estado == listo })
	fyne.DoAndWait(a.abrir)
	if !a.menu.Abierto() {
		t.Fatal("el panel no se abre")
	}

	n := api.HubNotification{
		ID: "n1", Source: "sheets", Type: "docs.comment", Title: "Comentario en Presupuesto",
		Body: "¿Revisamos la fila 12?", URL: "https://calcus.sytue.eu/calcus/edit?id=d1",
		Params: map[string]string{"actor": "Ana", "name": "Presupuesto"}, CreatedAt: time.Now(),
	}
	// Y le llega a la app que lo escuche (`AlLlegarAviso`): es lo que desbloquea en vivo
	// la franja de solo lectura. VISTO EN ROJO: sin `avisarOyentes` en `llega`, no oye nada.
	var oido sync.Map
	quitar := AlLlegarAviso(func(h api.HubNotification) { oido.Store(h.ID, true) })
	t.Cleanup(quitar)
	datos, _ := json.Marshal(n)
	hub.leer(func() { hub.sinLeer = 1 })
	hub.enviar <- string(datos)
	fila := func() *FilaLista {
		for _, o := range test.WidgetRenderer(a.menu).Objects() {
			if f, es := o.(*FilaLista); es {
				return f
			}
		}
		return nil
	}
	esperar(t, func() bool { return fila() != nil })
	f := fila()
	if f == nil {
		t.Fatal("el aviso que llega por el flujo no se ve en el panel abierto")
	}
	if _, ok := oido.Load("n1"); !ok {
		t.Error("el aviso nuevo no le llega a la app que escucha el flujo (AlLlegarAviso)")
	}
	quiero := TituloAviso(tr, n)
	if quiero == n.Title || !strings.Contains(quiero, "Ana") {
		t.Fatalf("el título no sale del catálogo: %q", quiero)
	}
	if f.Titulo != quiero || f.Subtitulo != n.Body || !f.SinLeer {
		t.Fatalf("fila = %q / %q / sin leer %v", f.Titulo, f.Subtitulo, f.SinLeer)
	}
	if cuenta := sinLeerDe(a); cuenta != 1 {
		t.Fatalf("sin leer = %d tras llegar uno", cuenta)
	}

	// Pulsarlo: leído en el Hub, contador abajo y su elemento abierto en Calcus.
	fyne.DoAndWait(f.AlPulsar)
	esperar(t, func() bool { return sinLeerDe(a) == 0 })
	hub.leer(func() {
		if len(hub.leidos) != 1 || hub.leidos[0] != "n1" {
			t.Errorf("leídos en el Hub = %v", hub.leidos)
		}
	})
	if sinLeerDe(a) != 0 || len(idas) != 1 || idas[0] != "sheets https://calcus.sytue.eu/calcus/edit?id=d1" {
		t.Fatalf("sin leer = %d, abierto = %v", sinLeerDe(a), idas)
	}

	// Otro, y «marcar todas».
	n.ID = "n2"
	datos, _ = json.Marshal(n)
	hub.enviar <- string(datos)
	esperar(t, func() bool { return sinLeerDe(a) == 1 })
	a.leerTodos()
	hub.leer(func() {
		if hub.todas != 1 {
			t.Errorf("«marcar todas» no llegó al Hub (%d)", hub.todas)
		}
	})
	if sinLeerDe(a) != 0 {
		t.Fatalf("tras «marcar todas», sin leer = %d", sinLeerDe(a))
	}

	// El ancho, el de `w-80 max-w-full` dentro del `p-1` con borde del panel; por
	// debajo de 768, la hoja a todo el ancho (`Dropdown` en el teléfono).
	panel := AnchoAvisos + 2*(rellenoPanelMenu+tema.Borde)
	for _, c := range []struct{ ventana, quiero float32 }{{1280, panel}, {800, panel}, {360, 360}, {280, 280}} {
		if got := MedidaMenu(a.menu, fyne.NewSize(c.ventana, 800)).Width; got != c.quiero {
			t.Errorf("a %v de ancho el panel mide %v y en la web %v", c.ventana, got, c.quiero)
		}
	}

	// Un corte del flujo: se reconecta y se vuelve a pedir la lista.
	var antes int
	hub.leer(func() { antes = hub.listas })
	close(hub.enviar)
	esperarConexion(t, hub)
	esperar(t, func() bool {
		var ahora int
		hub.leer(func() { ahora = hub.listas })
		return ahora > antes
	})
	hub.leer(func() {
		if hub.listas <= antes {
			t.Errorf("tras el corte no se ha vuelto a pedir la lista (%d)", hub.listas)
		}
	})
}

// esperarConexion espera a que la campana abra el flujo.
func esperarConexion(t *testing.T, h *hubDeMentira) {
	t.Helper()
	limite := time.After(5 * time.Second)
	for {
		select {
		case <-h.conexion:
			return
		case <-limite:
			t.Fatal("la campana no abre el flujo del Hub")
		default:
			fyne.DoAndWait(func() {})
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func sinLeerDe(a *avisosBarra) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sinLeer
}

// Los correos sin leer que cuenta el Hub salen ENCIMA de los avisos, como en la web, y
// el correo que llega por el flujo suma. Sin `mail_unread` (Mail caído) no hay fila.
func TestLaCampanaEnseñaElCorreoSinLeer(t *testing.T) {
	cuenta := `{"unread":0,"mail_unread":3}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/notifications/unread_count" {
			_, _ = fmt.Fprint(w, cuenta)
			return
		}
		_, _ = fmt.Fprint(w, `{"notifications":[]}`)
	}))
	t.Cleanup(srv.Close)
	a := nuevoAvisosBarra(traductorPrueba(t), nil)
	a.hub = api.NewHub(srv.URL, sesionDePrueba{}, nil)
	primera := func() string {
		if f, es := a.contenido()[0].(*FilaLista); es {
			return f.Titulo
		}
		return ""
	}

	a.cargar(context.Background(), false)
	if tit := primera(); !strings.Contains(tit, "3") {
		t.Fatalf("la primera fila es %q, quiero la de los 3 correos sin leer", tit)
	}
	a.llega(api.HubNotification{ID: "m1", Source: "mail", Type: "mail.message.received", Title: "Nuevo correo", CreatedAt: time.Now()})
	if tit := primera(); !strings.Contains(tit, "4") {
		t.Fatalf("tras un correo nuevo la fila dice %q, quiero 4", tit)
	}
	cuenta = `{"unread":0}`
	a.cargar(context.Background(), false)
	if tit := primera(); tit != "" {
		t.Fatalf("sin saber el correo sale la fila %q", tit)
	}
}
