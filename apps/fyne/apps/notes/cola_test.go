// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func almacenDePrueba(t *testing.T) *offline.Store {
	t.Helper()
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// envioFalso apunta lo que sale y falla con lo que se le diga por client_id.
type envioFalso struct {
	enviadas []string
	dias     []string
	fallos   map[string]error
}

func (f *envioFalso) envio() envio {
	return envio{
		Hoy: "2026-10-02",
		Crear: func(_ context.Context, nb string, in api.NoteInput) (api.Note, error) {
			if err := f.fallos[in.ClientID()]; err != nil {
				return api.Note{}, err
			}
			f.enviadas = append(f.enviadas, nb+":"+in.Title)
			return api.Note{ID: "n"}, nil
		},
		Diario: func(_ context.Context, date, _ string, carry bool) (api.Notebook, error) {
			f.dias = append(f.dias, date)
			if !carry {
				f.dias = append(f.dias, "sin-arrastre")
			}
			return api.Notebook{ID: "nb-" + date}, nil
		},
	}
}

func encolarDePrueba(t *testing.T, st *offline.Store, c capturaPendiente) {
	t.Helper()
	c.In = c.In.ConClientID(c.ClientID)
	if err := st.Write("ada", espacioCola, c.ClientID, c); err != nil {
		t.Fatal(err)
	}
}

func idsCola(cs []capturaPendiente) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ClientID)
	}
	return out
}

func TestLaColaSaleEnOrdenUnaVez(t *testing.T) {
	st := almacenDePrueba(t)
	encolarDePrueba(t, st, capturaPendiente{ClientID: "b", Cuaderno: "c1", In: api.NoteInput{Title: "dos"}, At: 2})
	encolarDePrueba(t, st, capturaPendiente{ClientID: "a", Cuaderno: "c1", In: api.NoteInput{Title: "uno"}, At: 1})
	f := &envioFalso{}
	tocados := vaciarCola(context.Background(), st, "ada", f.envio())
	if got := f.enviadas; len(got) != 2 || got[0] != "c1:uno" || got[1] != "c1:dos" || !tocados["c1"] {
		t.Fatalf("enviadas %v, tocados %v", got, tocados)
	}
	if q := pendientesDe(st, "ada"); len(q) != 0 {
		t.Fatalf("quedan %v", idsCola(q))
	}
	vaciarCola(context.Background(), st, "ada", f.envio())
	if len(f.enviadas) != 2 {
		t.Fatal("una captura enviada se ha vuelto a enviar")
	}
}

func TestLaColaParaSinRedYApartaLosRechazos(t *testing.T) {
	st := almacenDePrueba(t)
	encolarDePrueba(t, st, capturaPendiente{ClientID: "a", Cuaderno: "c1", In: api.NoteInput{Title: "uno"}, At: 1})
	encolarDePrueba(t, st, capturaPendiente{ClientID: "b", Cuaderno: "c1", In: api.NoteInput{Title: "dos"}, At: 2})
	sinRed := &envioFalso{fallos: map[string]error{"a": errors.New("dial tcp: connection refused")}}
	vaciarCola(context.Background(), st, "ada", sinRed.envio())
	if len(sinRed.enviadas) != 0 || len(pendientesDe(st, "ada")) != 2 {
		t.Fatalf("sin red ha salido %v", sinRed.enviadas)
	}
	caido := &envioFalso{fallos: map[string]error{"a": &api.Error{Status: http.StatusServiceUnavailable}}}
	vaciarCola(context.Background(), st, "ada", caido.envio())
	if len(caido.enviadas) != 0 {
		t.Fatal("un 503 no para la cola")
	}
	borrado := &envioFalso{fallos: map[string]error{"a": &api.Error{Status: http.StatusNotFound}}}
	vaciarCola(context.Background(), st, "ada", borrado.envio())
	q := pendientesDe(st, "ada")
	if len(borrado.enviadas) != 1 || borrado.enviadas[0] != "c1:dos" || len(q) != 1 || !q[0].Rechazada {
		t.Fatalf("un 404 bloquea a las demás: enviadas %v, quedan %+v", borrado.enviadas, q)
	}
	otra := &envioFalso{}
	vaciarCola(context.Background(), st, "ada", otra.envio())
	if len(otra.enviadas) != 0 {
		t.Fatal("una rechazada se ha vuelto a intentar")
	}
}

func TestUnDiaSinCuadernoSeAbreAlEnviar(t *testing.T) {
	st := almacenDePrueba(t)
	encolarDePrueba(t, st, capturaPendiente{ClientID: "a", Fecha: "2026-10-02", Titulo: "2 de octubre", In: api.NoteInput{Title: "uno"}, At: 1})
	f := &envioFalso{}
	tocados := vaciarCola(context.Background(), st, "ada", f.envio())
	if len(f.dias) != 1 || f.dias[0] != "2026-10-02" || f.enviadas[0] != "nb-2026-10-02:uno" || !tocados[prefijoDia+"2026-10-02"] {
		t.Fatalf("días %v, enviadas %v, tocados %v", f.dias, f.enviadas, tocados)
	}
}

// Sin red, capturar encola con su client_id y vacía el campo; al volver la red sale UNA
// vez, y el borrador ya no está.
func TestCapturarSinRedEncolaYSaleAlVolver(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"text"}`,
	})
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "ada" }
	p.red = &offline.Monitor{}
	p.red.Report(offline.Offline)
	p.capturaDiaria(tema.Claro, false)
	p.campoCaptura.PonerTexto("Llamar a Marta")
	p.guardarBorrador("c1", "Llamar a Marta")
	p.enviarCaptura()
	esperar(t, func() bool { return len(p.cola) == 1 && !p.guardandoCaptura })
	if len(falso.rutas()) != 0 {
		t.Fatalf("sin red ha salido %v", falso.rutas())
	}
	if len(p.cola) != 1 || p.cola[0].In.ClientID() == "" || p.cola[0].Cuaderno != "c1" {
		t.Fatalf("la cola es %+v", p.cola)
	}
	if p.campoCaptura.Texto() != "" || p.borradorGuardado("c1") != "" {
		t.Fatal("el campo no se ha vaciado al encolar")
	}
	if len(p.pendientesAqui()) != 1 {
		t.Fatal("lo pendiente no se ve en su cuaderno")
	}
	p.red.Report(offline.Online)
	p.vaciar()
	esperar(t, func() bool { return len(p.cola) == 0 })
	p.vaciar()
	var posts []string
	for _, r := range falso.rutas() {
		if strings.HasPrefix(r, "POST") {
			posts = append(posts, r)
		}
	}
	if len(posts) != 1 || posts[0] != "POST /notebooks/c1/notes" {
		t.Fatalf("al volver la red ha salido %v", posts)
	}
	if falso.ultima(t, "POST /notebooks/c1/notes")["data"].(map[string]any)["client_id"] == "" {
		t.Fatal("la nota sale sin client_id")
	}
}

// El borrador de la captura sobrevive a cerrar la app: está en el almacén.
func TestElBorradorSeGuarda(t *testing.T) {
	st := almacenDePrueba(t)
	p, _ := pantallaConServicio(t, nil)
	p.Almacen, p.Yo = st, func() string { return "ada" }
	p.activo = DeCuaderno(api.Notebook{ID: "c1"})
	campo := p.nuevoCampoCaptura()
	campo.PonerTexto("a medias")
	otra, _ := pantallaConServicio(t, nil)
	otra.Almacen, otra.Yo = st, func() string { return "ada" }
	if otra.borradorGuardado("c1") != "a medias" {
		t.Fatal("el borrador no ha sobrevivido")
	}
	otra.Yo = func() string { return "bob" }
	otra.borradores = nil
	if otra.borradorGuardado("c1") != "" {
		t.Fatal("el borrador cruza personas")
	}
}

func TestElDiaSinRed(t *testing.T) {
	conocido := DeCuaderno(api.Notebook{ID: "d1", DailyDate: "2026-10-02", OwnerSub: "ada"})
	if diaSinConexion([]Cuaderno{conocido}, "ada", "2026-10-02", "x").ID != "d1" {
		t.Fatal("no abre el día conocido")
	}
	if c := diaSinConexion([]Cuaderno{conocido}, "ada", "2026-10-03", "x"); c.ID != prefijoDia+"2026-10-03" || c.Crudo.DailyDate != "2026-10-03" {
		t.Fatalf("el provisional es %+v", c)
	}
}

// La misma persona en otra organización no ve ni envía lo capturado en la primera: el
// `sub` es el mismo, y sin la organización en la clave la captura de un día saldría con
// el token de la otra y acabaría en su diario.
func TestLaColaEsDeCadaOrganizacion(t *testing.T) {
	p, falso := pantallaConServicio(t, nil)
	p.Almacen, p.Yo = almacenDePrueba(t), func() string { return "ada" }
	org := "a"
	p.Org = func() string { return org }
	p.guardarBorrador("c1", "de a")
	// Sin red al encolar: con red, `encolar` vacía la cola en segundo plano con la
	// organización a, y eso es correcto, pero entonces la prueba no prueba nada y
	// además compite con el cambio de `org` (lo cazó -race en el CI).
	p.red = &offline.Monitor{}
	p.red.Report(offline.Offline)
	if err := p.encolar(api.NoteInput{Title: "día"}.ConClientID("x"), diaSinConexion(nil, "ada", "2026-10-02", "x")); err != nil {
		t.Fatal(err)
	}
	org = "b"
	p.borradores = nil
	p.red.Report(offline.Online)
	p.vaciar()
	esperar(t, func() bool { return len(p.cola) == 0 })
	if len(falso.rutas()) != 0 || p.borradorGuardado("c1") != "" {
		t.Fatalf("en b: salen %v, cola %v", falso.rutas(), idsCola(p.cola))
	}
	org = "a"
	if q := pendientesDe(p.Almacen, p.claveAlmacen()); len(q) != 1 {
		t.Fatalf("en a quedan %v", idsCola(q))
	}
}
