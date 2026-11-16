// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// lienzoGrabador es un [Lienzo] que apunta lo que le mandan.
type lienzoGrabador struct{ vistos []string }

func (l *lienzoGrabador) Objeto() fyne.CanvasObject     { return ui.NuevoVacio("", "") }
func (l *lienzoGrabador) Cargar(string) bool            { return true }
func (l *lienzoGrabador) Serializar() string            { return "" }
func (l *lienzoGrabador) Estadisticas() (int, int, int) { return 1, 1, 0 }
func (l *lienzoGrabador) Formato() Formato              { return Formato{} }
func (l *lienzoGrabador) Mandar(c Comando, v string)    { l.vistos = append(l.vistos, string(c)+"="+v) }

// nuevaGrabadora aísla cada test: el grabador es común a todo el proceso.
func nuevaGrabadora(t *testing.T) {
	t.Helper()
	antes, personal := grabadora, Personal
	grabadora, Personal = &macros{}, nil
	t.Cleanup(func() { grabadora, Personal = antes, personal })
}

// almacenDePrueba es un `sesion.PorPersona` en memoria: la persona va en la clave.
type almacenDePrueba struct {
	persona string
	datos   map[string]string
}

func (a *almacenDePrueba) Persona() string { return a.persona }

func (a *almacenDePrueba) Leer(k string) string {
	if a.persona == "" {
		return ""
	}
	return a.datos[a.persona+"\x00"+k]
}

func (a *almacenDePrueba) Poner(k, v string) {
	switch {
	case a.persona == "":
	case v == "":
		delete(a.datos, a.persona+"\x00"+k)
	default:
		a.datos[a.persona+"\x00"+k] = v
	}
}

func grabar(c *Contexto, cmds ...[2]string) {
	grabadora.empezar()
	for _, x := range cmds {
		c.Mandar(Comando(x[0]), x[1])
	}
}

func TestGrabarYReproducir(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz}
	grabar(c, [2]string{"bold", ""}, [2]string{"color", "#DC2626"}, [2]string{"undo", ""}, [2]string{"heading", "h2"})
	pasos, descartados := grabadora.terminar()
	// «undo» no se graba: deshacer corrompería la reproducción.
	if len(pasos) != 3 || descartados != 1 {
		t.Fatalf("pasos=%d descartados=%d; quería 3 y 1", len(pasos), descartados)
	}
	if !grabadora.guardar("Mía", office.Documento, pasos) {
		t.Fatal("no se pudo guardar")
	}
	lz.vistos = nil
	if r, _ := c.reproducirMacro(grabadora.lista()[0]); r != office.Valido {
		t.Fatalf("reproducir: %q", r)
	}
	if want := []string{"bold=", "color=#DC2626", "heading=h2"}; !slices.Equal(lz.vistos, want) {
		t.Errorf("llegaron %v y quería %v, en orden", lz.vistos, want)
	}
	// Y reproducir no se graba a sí misma.
	grabadora.empezar()
	_, _ = c.reproducirMacro(grabadora.lista()[0])
	if p, _ := grabadora.terminar(); len(p) != 0 {
		t.Errorf("reproducir se grabó a sí misma: %v", p)
	}
}

func TestReproducirNoDespachaNadaSiUnPasoFalla(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{
		{Editor: office.Documento, Cmd: "bold"},
		{Editor: office.Documento, Cmd: "font", Valor: "Papyrus", ConValor: true},
	}}
	if r, i := c.reproducirMacro(m); r != office.ValorMalo || i != 1 {
		t.Fatalf("veredicto %q en %d; quería valor en 1", r, i)
	}
	if len(lz.vistos) != 0 {
		t.Errorf("se despachó %v antes de saber que fallaba: una macro a medias deja el documento en un estado que nadie pidió", lz.vistos)
	}
}

func TestReproducirNoEditaEnSoloLectura(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz, SoloLectura: true}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{{Editor: office.Documento, Cmd: "bold"}}}
	if r, _ := c.reproducirMacro(m); r != office.SoloLectura {
		t.Fatalf("veredicto %q; quería soloLectura", r)
	}
	if len(lz.vistos) != 0 {
		t.Errorf("en solo lectura llegó %v al lienzo", lz.vistos)
	}
}

func TestUnaMacroDeOtroTipoNoSeReproduce(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Hoja, Lienzo: &lienzoGrabador{}}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{{Editor: office.Documento, Cmd: "bold"}}}
	if r, _ := c.reproducirMacro(m); r != office.OtroTipo {
		t.Fatalf("veredicto %q; quería tipo", r)
	}
}

func TestLosTechosDeLaGrabadora(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Documento, Lienzo: &lienzoGrabador{}}
	grabadora.empezar()
	for i := 0; i < office.MaxPasos+7; i++ {
		c.Mandar(Negrita, "")
	}
	pasos, descartados := grabadora.terminar()
	if len(pasos) != office.MaxPasos || descartados != 7 {
		t.Errorf("pasos=%d descartados=%d; quería %d y 7", len(pasos), descartados, office.MaxPasos)
	}
	for i := 0; i < office.MaxMacros; i++ {
		if !grabadora.guardar("m", office.Documento, pasos[:1]) {
			t.Fatalf("la macro %d debería caber", i)
		}
	}
	if grabadora.guardar("una más", office.Documento, pasos[:1]) {
		t.Errorf("la macro %d pasa el techo de %d", office.MaxMacros+1, office.MaxMacros)
	}
}

// TestSinGrabarNoSeApuntaNada: el gancho de Mandar fuera de grabación es un `if` que vuelve.
func TestSinGrabarNoSeApuntaNada(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Documento, Lienzo: &lienzoGrabador{}}
	c.Mandar(Negrita, "")
	if len(grabadora.pasos) != 0 {
		t.Errorf("sin grabar se apuntó %v", grabadora.pasos)
	}
}

// TestLasMacrosSonDeLaPersonaYSobrevivenAlProceso: guardadas con alguien dentro van a su
// almacén, con el JSON de la web, y un proceso nuevo (otra grabadora) las encuentra; otra
// cuenta no las ve, y al volver la primera siguen ahí. Borrar también se guarda. Se vio en
// ROJO cuando vivían solo en la memoria del proceso.
func TestLasMacrosSonDeLaPersonaYSobrevivenAlProceso(t *testing.T) {
	nuevaGrabadora(t)
	alm := &almacenDePrueba{persona: "sub-ana", datos: map[string]string{}}
	Personal = alm
	pasos := []office.Paso{{Editor: office.Documento, Cmd: "bold"}, {Editor: office.Documento, Cmd: "color", Valor: "#DC2626", ConValor: true}}
	if !grabadora.guardar("Resaltar", office.Documento, pasos) {
		t.Fatal("no se pudo guardar")
	}
	crudo := alm.datos["sub-ana\x00"+ClaveMacros]
	if !strings.Contains(crudo, `"name":"Resaltar"`) || !strings.Contains(crudo, `{"editor":"document","cmd":"bold"}`) ||
		!strings.Contains(crudo, `{"editor":"document","cmd":"color","value":"#DC2626"}`) {
		t.Errorf("lo guardado no tiene la forma de la web: %s", crudo)
	}
	grabadora = &macros{} // otro proceso
	got := grabadora.lista()
	if len(got) != 1 || got[0].nombre != "Resaltar" || got[0].tipo != office.Documento || !slices.Equal(got[0].pasos, pasos) {
		t.Fatalf("al volver a abrir: %+v", got)
	}
	alm.persona = "sub-luis"
	if n := len(grabadora.lista()); n != 0 {
		t.Errorf("otra cuenta ve %d macros que no son suyas", n)
	}
	alm.persona = ""
	if n := len(grabadora.lista()); n != 0 {
		t.Errorf("sin nadie dentro se ven %d macros de alguien", n)
	}
	alm.persona = "sub-ana"
	grabadora.borrar(grabadora.lista()[0].id)
	if _, hay := alm.datos["sub-ana\x00"+ClaveMacros]; hay {
		t.Error("borrar la última se guarda")
	}
}

// TestSinNadieDentroLasMacrosVivenEnMemoria: como en la web sin `sub`, se pueden grabar y
// reproducir, pero no se escriben en ningún sitio.
func TestSinNadieDentroLasMacrosVivenEnMemoria(t *testing.T) {
	nuevaGrabadora(t)
	alm := &almacenDePrueba{datos: map[string]string{}}
	Personal = alm
	if !grabadora.guardar("Mía", office.Documento, []office.Paso{{Editor: office.Documento, Cmd: "bold"}}) {
		t.Fatal("no se pudo guardar")
	}
	if len(grabadora.lista()) != 1 || len(alm.datos) != 0 {
		t.Errorf("sin nadie: %d en memoria y %v escrito", len(grabadora.lista()), alm.datos)
	}
}

// TestLoGuardadoSeDepuraAlLeer: lo leído no se da por bueno —lo pudo escribir otra
// versión—: fuera los pasos que el registro no conoce o no se reproducen, los de un editor
// que no existe y los de valor desmesurado; una macro sin pasos válidos no se carga; y un
// blob que no es JSON no es ninguna. Los mismos criterios que `parseMacros`.
func TestLoGuardadoSeDepuraAlLeer(t *testing.T) {
	largo := strings.Repeat("x", office.MaxValor+1)
	crudo := `[` +
		`{"id":"a","name":"Buena","steps":[{"editor":"document","cmd":"bold"},{"editor":"document","cmd":"noExiste"},` +
		`{"editor":"document","cmd":"undo"},{"editor":"marciano","cmd":"bold"},{"editor":"document","cmd":"color","value":"` + largo + `"}]},` +
		`{"id":"c","name":"Tolerante","steps":[{"editor":5,"cmd":"bold"},{"editor":"document","cmd":"italic","value":7}]},` +
		`{"id":"b","name":"Vacía","steps":[{"editor":"document","cmd":"noExiste"}]},` +
		`{"name":"Sin id","steps":[{"editor":"document","cmd":"bold"}]},` +
		`"no es una macro"]`
	got := leerMacros(crudo)
	if len(got) != 2 || got[0].id != "a" || len(got[0].pasos) != 1 || got[0].pasos[0].Cmd != "bold" {
		t.Fatalf("depurado: %+v", got)
	}
	// Un paso mal formado tira solo ese paso, y un `value` que no es texto se ignora, como
	// `coerceStep`: antes se perdía la macro entera.
	if got[1].id != "c" || len(got[1].pasos) != 1 || got[1].pasos[0].Cmd != "italic" || got[1].pasos[0].ConValor {
		t.Errorf("tolerante: %+v", got[1])
	}
	if leerMacros("{roto") != nil || leerMacros("") != nil {
		t.Error("un blob roto no es ninguna macro")
	}
}
