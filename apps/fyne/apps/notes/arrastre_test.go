// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func notasOrdenadasParaArrastre() []api.Note {
	return []api.Note{
		{ID: "a", NotebookID: "c1", Kind: "text", Title: "Primera", Position: 0},
		{ID: "b", NotebookID: "c1", Kind: "todo", Title: "Segunda", Position: 1},
		{ID: "c", NotebookID: "c1", Kind: "text", Title: "Tercera", Position: 2},
	}
}

func TestElArrastreReordenaConUnaAccionDeDeshacer(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, notasOrdenadasParaArrastre()...)
	cambios := planOrdenNotas(p.notas, "c", "a", false, -1)
	destinos := make([]posicionNota, len(cambios))
	for i, c := range cambios {
		destinos[i] = posicionNota{c.id, c.despues}
	}
	// El cuerpo descifrado no se reconstruye desde los bytes cifrados al mover.
	p.notas[0].Contenido = "<p>Contenido abierto</p>"
	p.notas[0].Crudo.Content = "enc:v1:contenido-cifrado"
	if err := p.colocarVarias(destinos, true); err != nil {
		t.Fatal(err)
	}
	if got := ids(Ordenar(p.notas, OrdenManual, "es")); got != "c,a,b" {
		t.Fatalf("el movimiento no se ve antes de recargar: %s", got)
	}
	for id, posicion := range map[string]float64{"c": 0, "a": 1, "b": 2} {
		if cuerpo := falso.ultima(t, "POST /notes/"+id+"/move"); cuerpo["position"] != posicion {
			t.Errorf("posición de %s no persistida: %v", id, cuerpo)
		}
	}
	if n, _ := p.nota("a"); n.Contenido != "<p>Contenido abierto</p>" {
		t.Fatal("mover ha sustituido el contenido abierto por el cifrado")
	}
	if _, err := p.hist.Deshacer(); err != nil {
		t.Fatal(err)
	}
	if got := ids(Ordenar(p.notas, OrdenManual, "es")); got != "a,b,c" || p.hist.SiguienteDeshacer() != "" {
		t.Fatalf("el gesto no se deshace de una vez: %s", got)
	}
}

func TestElDestinoDelArrastreNoCambiaGruposNiFijadas(t *testing.T) {
	crudas := notasOrdenadasParaArrastre()
	crudas[1].GroupID = ptr("a")
	crudas[2].Pinned = true
	notas := DeNotas(crudas)
	for _, destino := range []string{"b", "c", "a", "inexistente"} {
		if plan := planOrdenNotas(notas, "a", destino, true, -1); len(plan) != 0 {
			t.Errorf("acepta un destino incompatible %q: %+v", destino, plan)
		}
	}
	plan := planOrdenNotas(notas, "b", "", false, 2)
	if len(plan) != 1 || plan[0].despues.Col != 2 || plan[0].despues.GroupID == nil || *plan[0].despues.GroupID != "a" {
		t.Fatalf("la columna vacía pierde grupo o destino: %+v", plan)
	}
}

func TestUnFalloParcialDelOrdenRestauraServicioYPantalla(t *testing.T) {
	var mu sync.Mutex
	posiciones := map[string]int{"a": 0, "b": 1, "c": 2}
	fallo := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var pos api.NotePosition
		if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
			t.Errorf("petición de orden inválida: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/notes/"), "/move")
		mu.Lock()
		defer mu.Unlock()
		if id == "b" && !fallo {
			fallo = true
			w.WriteHeader(http.StatusConflict)
			return
		}
		posiciones[id] = pos.Position
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	p, _ := pantallaConServicio(t, nil, notasOrdenadasParaArrastre()...)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	destinos := []posicionNota{
		{"c", api.NotePosition{Position: 0}},
		{"a", api.NotePosition{Position: 1}},
		{"b", api.NotePosition{Position: 2}},
	}
	if err := p.colocarVarias(destinos, true); err == nil {
		t.Fatal("el fallo de una posición ha dado el gesto por guardado")
	}
	if got := ids(Ordenar(p.notas, OrdenManual, "es")); got != "a,b,c" || p.hist.SiguienteDeshacer() != "" {
		t.Fatalf("la pantalla o el historial conservan el gesto fallido: %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	for id, original := range map[string]int{"a": 0, "b": 1, "c": 2} {
		if posiciones[id] != original {
			t.Errorf("%s queda en %d en el servicio, quería %d", id, posiciones[id], original)
		}
	}
}

func TestElAsaTactilNoMueveElTextoNiTapaLaCasilla(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	p, w := montarCaptura(t, 390, 844, "list")
	var recibida fyne.Position
	p.lienzo.AlSoltarNota = func(_ Nota, en fyne.Position) { recibida = en }
	p.Repintar()
	var asa *asaArrastre
	recorrerInterno(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if a, ok := o.(*asaArrastre); ok && a.t.nota.ID == "n3" {
			asa = a
		}
	})
	if asa == nil {
		t.Fatal("la lista no ofrece asa de arrastre")
	}
	if asa.Size().Width < ui.SueloTactilBarra || asa.Size().Height < ui.SueloTactilBarra {
		t.Fatalf("el asa táctil no mide 44px: %v", asa.Size())
	}
	pos := asa.t.Position()
	en := fyne.NewPos(200, 500)
	asa.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: en}, Dragged: fyne.NewDelta(30, 60)})
	if asa.t.Position() != pos {
		t.Fatal("el texto sigue al dedo y tapa el destino de inserción")
	}
	asa.DragEnd()
	if recibida != en {
		t.Fatalf("se pierde la posición real del dedo al soltar: %v", recibida)
	}
	for _, cancelar := range []func(){
		func() { asa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) },
		asa.FocusLost,
		asa.CancelarArrastre,
	} {
		recibida = fyne.Position{}
		asa.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: en}, Dragged: fyne.NewDelta(0, 30)})
		cancelar()
		// El dedo sigue abajo después de Escape o de perder el foco.
		asa.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: en}, Dragged: fyne.NewDelta(0, 30)})
		asa.DragEnd()
		if recibida != (fyne.Position{}) || asa.t.arrastrando {
			t.Fatal("el gesto cancelado se reinicia y guarda antes de levantar el dedo")
		}
	}
	// El gesto de la casilla permanece separado del asa.
	var casilla *ui.Eleccion
	recorrerInterno(asa.t, func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok {
			casilla = c
		}
	})
	if casilla == nil {
		t.Fatal("ha desaparecido la casilla de la tarea")
	}
	drv := fyne.CurrentApp().Driver()
	a, c := drv.AbsolutePositionForObject(asa), drv.AbsolutePositionForObject(casilla)
	if a.X < c.X+casilla.Size().Width && c.X < a.X+asa.Size().Width {
		t.Fatal("el asa se superpone a la casilla")
	}
}

func TestFalloDeOrdenConservaLaCapturaYEdicionQueLleganMientrasEspera(t *testing.T) {
	bloqueada, salida := make(chan struct{}), make(chan struct{})
	liberar := sync.OnceFunc(func() { close(salida) })
	var primera sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallar := false
		primera.Do(func() {
			fallar = true
			close(bloqueada)
			<-salida
		})
		if fallar {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(liberar)
	p, _ := pantallaConServicio(t, nil, notasOrdenadasParaArrastre()...)
	p.svc = api.NewNotes(srv.URL, nil, nil)
	listo := make(chan error, 1)
	go func() { listo <- p.colocarVarias([]posicionNota{{"a", api.NotePosition{X: 80, Position: 4}}}, true) }()
	// La escritura empieza en la goroutine de UI. El driver de pruebas solo
	// despacha su cola al sincronizar desde el test, igual que el bucle de la app.
	esperar(t, func() bool {
		select {
		case <-bloqueada:
			return true
		default:
			return false
		}
	})
	fyne.DoAndWait(func() {
		n, _ := p.nota("a")
		n.Contenido = "<p>Edición posterior</p>"
		n.Crudo.ContentCipher = "enc:v1:edicion-posterior"
		p.reemplazar(n)
		p.notas = append(p.notas, DeNota(api.Note{ID: "nueva", NotebookID: "c1", Kind: "text", Content: "Captura nueva"}))
	})
	liberar()
	select {
	case err := <-listo:
		if err == nil {
			t.Fatal("el servicio ha rechazado el movimiento")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el movimiento fallido no termina")
	}
	var actual Nota
	var capturaPresente bool
	fyne.DoAndWait(func() {
		actual, _ = p.nota("a")
		_, capturaPresente = p.nota("nueva")
	})
	if actual.X != 0 || actual.Posicion != 0 || actual.Contenido != "<p>Edición posterior</p>" || actual.Crudo.ContentCipher != "enc:v1:edicion-posterior" {
		t.Fatalf("el rollback no se limita a la posición: %+v", actual)
	}
	if !capturaPresente {
		t.Fatal("el rollback elimina la captura que ya llegó")
	}
}

func TestLaCapturaPermaneceFueraDelDesplazador(t *testing.T) {
	p, w := montarCaptura(t, 390, 844, "list")
	p.Repintar()
	campo := p.campoCaptura
	if campo == nil {
		t.Fatal("la lista no ofrece captura rápida")
	}
	seDesplaza := false
	recorrerInterno(p.desplazador.Content, func(o fyne.CanvasObject) {
		if o == campo {
			seDesplaza = true
		}
	})
	if seDesplaza {
		t.Fatal("la captura desaparece al desplazar las notas")
	}
	campo.PonerTexto("Llamar mañana")
	p.Repintar()
	if p.campoCaptura != campo || campo.Texto() != "Llamar mañana" {
		t.Fatal("repintar el pie pierde el campo o su borrador")
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(campo)
	if pos.Y < p.desplazador.Size().Height || pos.Y+campo.Size().Height > w.Canvas().Size().Height {
		t.Fatalf("el campo no permanece visible al pie: %v %v", pos, campo.Size())
	}
}
