// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// carpetaDePrueba lleva lo visto a una carpeta temporal mientras dura el test.
func carpetaDePrueba(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	antes := carpetaReanudar
	carpetaReanudar = func() (string, error) { return dir, nil }
	t.Cleanup(func() { carpetaReanudar = antes })
	return dir
}

func TestReanudarUmbrales(t *testing.T) {
	const total = 10 * time.Minute
	casos := []struct {
		pos  time.Duration
		vale bool
	}{
		{9900 * time.Millisecond, false},
		{10 * time.Second, true},
		{569 * time.Second, true},
		{570 * time.Second, false}, // el 95 %: ya está visto
	}
	for _, c := range casos {
		if Merece(c.pos, total) != c.vale {
			t.Errorf("Merece(%v, %v) = %v", c.pos, total, !c.vale)
		}
	}
	if Merece(30*time.Second, 0) {
		t.Error("sin duración no hay porcentaje que valga")
	}
}

func TestReanudarAnotaYOlvida(t *testing.T) {
	ahora := time.Unix(1_800_000_000, 0)
	p := Puntos{}.Anotar("a", 2*time.Minute, 10*time.Minute, ahora)
	if d := p.Desde("a", ahora); d != 2*time.Minute {
		t.Fatalf("Desde = %v", d)
	}
	if v := p.Visto("a", ahora); v < 0.199 || v > 0.201 {
		t.Errorf("Visto = %v", v)
	}
	if d := p.Anotar("a", 590*time.Second, 10*time.Minute, ahora).Desde("a", ahora); d != 0 {
		t.Errorf("casi visto tendría que olvidarse; Desde = %v", d)
	}
	if _, ok := p.Anotar("a", 3*time.Second, 10*time.Minute, ahora)["a"]; ok {
		t.Error("volver al principio tendría que olvidarlo")
	}
	if len(p) != 1 {
		t.Error("Anotar no puede tocar los puntos que recibe")
	}
}

func TestReanudarCaduca(t *testing.T) {
	ahora := time.Unix(1_800_000_000, 0)
	p := Puntos{}.Anotar("a", time.Minute, 10*time.Minute, ahora)
	if p.Desde("a", ahora.Add(ReanudarCaducidad-time.Millisecond)) == 0 {
		t.Error("caduca antes de tiempo")
	}
	if p.Desde("a", ahora.Add(ReanudarCaducidad)) != 0 || p.Visto("a", ahora.Add(ReanudarCaducidad)) != 0 {
		t.Error("no caduca a los 90 días")
	}
	if _, ok := p.Anotar("b", time.Minute, 10*time.Minute, ahora.Add(ReanudarCaducidad))["a"]; ok {
		t.Error("al anotar otro, lo caducado se poda")
	}
}

func TestReanudarTopeDeEntradas(t *testing.T) {
	ahora := time.Unix(1_800_000_000, 0)
	p := Puntos{}
	for i := 0; i <= ReanudarMax; i++ {
		p = p.Anotar(fmt.Sprintf("v%d", i), time.Minute, 10*time.Minute, ahora.Add(time.Duration(i)*time.Second))
	}
	if len(p) != ReanudarMax {
		t.Fatalf("hay %d puntos", len(p))
	}
	fin := ahora.Add(ReanudarMax * time.Second)
	if p.Desde("v0", fin) != 0 || p.Desde(fmt.Sprintf("v%d", ReanudarMax), fin) == 0 {
		t.Error("se tienen que quedar los más recientes")
	}
}

// TestReanudarPorPersona: cada persona lo suyo, en el mismo formato que la web, y un
// fichero roto no rompe nada.
func TestReanudarPorPersona(t *testing.T) {
	dir := carpetaDePrueba(t)
	ahora := time.Now()
	GuardarPuntos("ana", Puntos{}.Anotar("v", 2*time.Minute, 10*time.Minute, ahora))
	GuardarPuntos("luis", Puntos{}.Anotar("w", time.Minute, 10*time.Minute, ahora))
	if d := LeerPuntos("ana").Desde("v", ahora); d != 2*time.Minute {
		t.Errorf("Ana sigue en %v", d)
	}
	if LeerPuntos("luis").Desde("v", ahora) != 0 || len(LeerPuntos("")) != 0 {
		t.Error("lo de Ana es solo de Ana")
	}
	datos, err := os.ReadFile(filepath.Join(dir, ficheroReanudar))
	if err != nil || !strings.Contains(string(datos), `"v":{"s":120,"t":600,"en":`) {
		t.Errorf("formato inesperado: %s (%v)", datos, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ficheroReanudar), []byte("{roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(LeerPuntos("ana")) != 0 {
		t.Error("un fichero roto se lee vacío")
	}
}

// TestReanudarTarjeta: la barra de lo visto va al pie del póster y el nombre lo dice;
// un directo no la lleva.
func TestReanudarTarjeta(t *testing.T) {
	v := vistaDePrueba(t, tema.Claro)
	v.Visto = func(string) float64 { return 0.4 }
	vid := api.CastVideo{ID: "v", Title: "Formación", Status: api.CastStatusReady, DurationMS: 600_000}
	tj := NuevaTarjeta(v, vid, false, nil)
	r := test.TempWidgetRenderer(t, tj).(*tarjetaRenderer)
	r.Layout(fyne.NewSize(320, AltoTarjeta(320)))
	if r.visto == nil {
		t.Fatal("sin barra de lo visto")
	}
	if !strings.Contains(tj.AccessibilityLabel(), "Visto el 40 %") {
		t.Errorf("el nombre no lo dice: %q", tj.AccessibilityLabel())
	}
	vivo := vid
	vivo.Status, vivo.Kind = api.CastStatusLive, "live"
	if d := NuevaTarjeta(v, vivo, false, nil); d.Visto != 0 {
		t.Error("un directo no lleva barra")
	}
	v.Visto = nil
	if NuevaTarjeta(v, vid, false, nil).Visto != 0 {
		t.Error("sin nada visto no hay barra")
	}
}

// TestReanudarVisor: «Empezar desde el principio» se pone y se quita, y no en directo.
func TestReanudarVisor(t *testing.T) {
	x := NuevoVisor(vistaDePrueba(t, tema.Claro))
	x.Mando = &mandoFalso{pos: int64(2 * time.Minute), dur: int64(10 * time.Minute)}
	pulsado := false
	x.AlDesdeInicio = func() { pulsado = true }
	x.Rehacer()
	if !x.TieneInicio() {
		t.Fatal("no se ofrece volver al principio")
	}
	x.DesdeInicio()
	if !pulsado {
		t.Error("el botón no hace nada")
	}
	x.AlDesdeInicio = nil
	x.Rehacer()
	if x.TieneInicio() {
		t.Error("quitarlo no lo quita")
	}
	x.Directo, x.AlDesdeInicio = true, func() {}
	x.Rehacer()
	if x.TieneInicio() {
		t.Error("en directo no hay principio al que volver")
	}
}

// TestReanudarEnLaPaginaDelVideo: con el reproductor de verdad (el WebM de 2 s), la
// página sigue donde se dejó, ofrece volver al principio y, al volver, lo olvida.
func TestReanudarEnLaPaginaDelVideo(t *testing.T) {
	carpetaDePrueba(t)
	cola := make(chan func(), 4096)
	vaciar := func(que string, hasta func() bool) {
		t.Helper()
		limite := time.Now().Add(15 * time.Second)
		for !hasta() {
			if time.Now().After(limite) {
				t.Fatalf("no llega %s", que)
			}
			select {
			case f := <-cola:
				f()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Yo = func() string { return "u-ana" }
	a.enLaInterfaz = func(f func()) { cola <- f }
	a.AbrirMedia = nil
	// El punto vale aunque el vídeo de prueba dure 2 s: el reproductor lo recorta.
	GuardarPuntos("u-ana", Puntos{}.Anotar("v01", time.Minute, 10*time.Minute, time.Now()))
	a.AbrirVideo("v01")
	vaciar("el reproductor", func() bool { return a.reproductor != nil && a.visor.Mando != nil })
	if p := a.reproductor.Posicion(); p < time.Second {
		t.Errorf("no sigue donde se dejó: está en %v", p)
	}
	if !a.visor.TieneInicio() {
		t.Fatal("no ofrece empezar desde el principio")
	}
	a.visor.DesdeInicio()
	if a.visor.TieneInicio() {
		t.Error("el ofrecimiento sigue tras usarlo")
	}
	if LeerPuntos("u-ana").Desde("v01", time.Now()) != 0 {
		t.Error("volver al principio tendría que olvidar el punto")
	}
	a.Ir(RutaInicio)
	if a.reanudo != "" {
		t.Error("salir de la página tendría que dejar de recordar")
	}
}
