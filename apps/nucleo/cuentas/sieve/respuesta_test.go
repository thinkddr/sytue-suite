// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sieve

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/sieve/sieveprueba"
)

const propio = "sytue-office"

func dia(a int, m time.Month, d int) time.Time { return time.Date(a, m, d, 0, 0, 0, 0, time.Local) }

func respuestaDePrueba() Respuesta {
	return Respuesta{
		Asunto:      `Fuera "de la oficina" \ vuelvo pronto`,
		Mensaje:     "Hola:\n\n.Estoy fuera hasta el lunes.\n..y con dos puntos\n.\nUn saludo",
		Dias:        3,
		De:          usuario,
		Direcciones: []string{usuario, "ana@alias.eu", "ANA@ejemplo.eu"},
	}
}

func TestComponerYLeerVuelvenAlMismoSitio(t *testing.T) {
	r := respuestaDePrueba()
	r.Desde, r.Hasta = dia(2026, 12, 1), dia(2026, 12, 15)
	texto, err := Componer(r, `mis "reglas"`, true)
	if err != nil {
		t.Fatal(err)
	}
	if !EsPropio(texto) || !strings.Contains(texto, "\r\n") || strings.Contains(strings.ReplaceAll(texto, "\r\n", ""), "\n") {
		t.Fatalf("cabecera o fin de línea CRLF:\n%s", texto)
	}
	for _, quiero := range []string{
		`require ["vacation", "date", "relational", "include"];`,
		`:subject "Fuera \"de la oficina\" \\ vuelvo pronto"`,
		`:addresses ["ana@ejemplo.eu", "ana@alias.eu"]`,
		"\r\n..Estoy fuera hasta el lunes.\r\n",
		"\r\n...y con dos puntos\r\n",
		"\r\n..\r\nUn saludo\r\n.\r\n",
		`include :personal :optional "mis \"reglas\"";`,
		`currentdate :zone "` + dia(2026, 12, 1).Format("-0700") + `" :value "ge" "date" "2026-12-01"`,
		`:value "le" "date" "2026-12-15"`,
		`# sytue-office: anterior "mis \"reglas\""`,
	} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, texto)
		}
	}
	// La inclusión va DETRÁS de la respuesta.
	if strings.Index(texto, "include :personal") < strings.Index(texto, "vacation :days") {
		t.Fatal("el include tiene que ir detrás de la respuesta")
	}
	got, anterior, ok := LeerRespuesta(texto)
	if !ok || anterior != `mis "reglas"` {
		t.Fatalf("ok=%v anterior=%q", ok, anterior)
	}
	quiero := Normalizar(r)
	if got.Asunto != quiero.Asunto || got.Mensaje != quiero.Mensaje || got.Dias != 3 || got.De != usuario ||
		!slices.Equal(got.Direcciones, quiero.Direcciones) || !got.Desde.Equal(r.Desde) || !got.Hasta.Equal(r.Hasta) {
		t.Fatalf("leído:\n%+v\nquiero:\n%+v", got, quiero)
	}
}

func TestComponerSinFechasNiAnterior(t *testing.T) {
	r := Respuesta{Asunto: "Vacaciones", Mensaje: "", Dias: 0}
	texto, err := Componer(r, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(texto, "include") || strings.Contains(texto, "currentdate") || strings.Contains(texto, "anterior") ||
		!strings.Contains(texto, `require ["vacation"];`) || !strings.Contains(texto, "vacation :days 7 :subject \"Vacaciones\" text:\r\n.\r\n;") {
		t.Fatalf("sin fechas ni anterior:\n%s", texto)
	}
	got, anterior, ok := LeerRespuesta(texto)
	if !ok || anterior != "" || got.Mensaje != "" || got.Dias != DiasDeSerie {
		t.Fatalf("%+v %q %v", got, anterior, ok)
	}
	if _, err := Componer(Respuesta{Desde: dia(2026, 1, 1)}, "", false); !errors.Is(err, ErrSinFechas) {
		t.Fatalf("fechas sin date: %v", err)
	}
	if _, err := Componer(Respuesta{}, "mal\nnombre", false); !errors.Is(err, ErrNombre) {
		t.Fatalf("anterior con salto de línea: %v", err)
	}
	// Solo una de las fechas: un `if` sin `allof`.
	texto, _ = Componer(Respuesta{Hasta: dia(2026, 3, 9)}, "", true)
	if strings.Contains(texto, "allof") || !strings.Contains(texto, `:value "le" "date" "2026-03-09"`) {
		t.Fatalf("solo hasta:\n%s", texto)
	}
	got, _, _ = LeerRespuesta(texto)
	if !got.Desde.IsZero() || !got.Hasta.Equal(dia(2026, 3, 9)) {
		t.Fatalf("solo hasta: %+v", got)
	}
}

// El asunto y las direcciones no pueden partir una línea, ni nada colar caracteres de
// control en el script.
func TestNormalizarLimpia(t *testing.T) {
	r := Normalizar(Respuesta{Asunto: "a\r\nb\x00c\u2028", De: " x@y \n", Mensaje: "uno\r\ndos\rtres\x07\tcuatro", Dias: 99})
	if r.Asunto != "a  bc" || r.De != "x@y" || r.Mensaje != "uno\ndos\ntres\tcuatro" || r.Dias != DiasMaximo {
		t.Fatalf("%+v", r)
	}
	texto, err := Componer(Respuesta{Asunto: "a\nb", Mensaje: "línea\n.\n"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trocear(texto); err != nil {
		t.Fatalf("lo compuesto se tiene que poder leer: %v", err)
	}
	got, _, _ := LeerRespuesta(texto)
	if got.Mensaje != "línea\n.\n" || got.Asunto != "a b" {
		t.Fatalf("%+v", got)
	}
}

func TestTrocearSieve(t *testing.T) {
	ps, err := trocear("require [\"a\", \"b\"]; # comentario \"no\"\r\n/* bloque\nvacation */ if size :over 10K { reject text: # x\r\nno\r\n..dos\r\n.\r\n; }")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range ps {
		if p.tipo == 'i' {
			ids = append(ids, p.texto)
		}
		if p.tipo == 's' && strings.Contains(p.texto, "dos") && p.texto != "no\n.dos\n" {
			t.Fatalf("multilínea: %q", p.texto)
		}
	}
	if !slices.Equal(ids, []string{"require", "if", "size", "reject"}) {
		t.Fatalf("identificadores: %v (el comentario de bloque no cuenta)", ids)
	}
	for _, malo := range []string{"\"sin cerrar", "/* sin cerrar", "text:\r\nsin punto", "@", "text: x\r\n.\r\n"} {
		if _, err := trocear(malo); err == nil {
			t.Errorf("%q no es Sieve", malo)
		}
	}
}

// ---- La política con el servidor ----

func estado(t *testing.T, s *sieveprueba.Servidor) Estado {
	t.Helper()
	st, err := LeerEstado(conectado(t, s), propio)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func guardar(t *testing.T, s *sieveprueba.Servidor, r Respuesta, activar bool) error {
	t.Helper()
	return Guardar(conectado(t, s), propio, r, activar)
}

func TestSinScriptSeSubeYSeActiva(t *testing.T) {
	s := servidor(t, nil)
	if st := estado(t, s); st.Existe || st.Activa || st.Otro != "" || !st.ConFechas || !st.ConInclude || st.Dias != DiasDeSerie {
		t.Fatalf("vacío: %+v", st)
	}
	r := respuestaDePrueba()
	r.Desde = dia(2026, 12, 1)
	if err := guardar(t, s, r, true); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != propio {
		t.Fatalf("activo: %q", s.Activo())
	}
	texto, _ := s.Script(propio)
	if strings.Contains(texto, "include") {
		t.Fatalf("sin script de la persona no se incluye nada:\n%s", texto)
	}
	if !slices.Contains(s.Ordenes(), "CHECKSCRIPT") {
		t.Fatal("se valida antes de subir")
	}
	st := estado(t, s)
	if !st.Activa || st.Asunto != Normalizar(r).Asunto || st.Mensaje != Normalizar(r).Mensaje || !st.Desde.Equal(r.Desde) || st.Anterior != "" {
		t.Fatalf("releído: %+v", st)
	}
	// Apagar deja lo de antes: nada activo; el texto se queda para la próxima vez.
	if err := guardar(t, s, r, false); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != "" {
		t.Fatalf("apagada, activo %q", s.Activo())
	}
	if st := estado(t, s); st.Activa || !st.Existe || st.Asunto != Normalizar(r).Asunto {
		t.Fatalf("apagada: %+v", st)
	}
}

const reglas = "require [\"fileinto\"];\r\nif header :contains \"subject\" \"factura\" { fileinto \"Facturas\"; }\r\n"

// Con otro script activo y `include`, las reglas de la persona siguen funcionando detrás
// de la respuesta y vuelven a ser lo activo al apagarla. Su script no se toca.
func TestOtroActivoConIncludeSeCombinaYSeRestaura(t *testing.T) {
	s := servidor(t, nil)
	s.Poner("reglas", reglas, true)
	st := estado(t, s)
	if st.Otro != "reglas" || st.Bloqueada != "" {
		t.Fatalf("%+v", st)
	}
	r := respuestaDePrueba()
	if err := guardar(t, s, r, true); err != nil {
		t.Fatal(err)
	}
	texto, _ := s.Script(propio)
	if s.Activo() != propio || !strings.Contains(texto, `include :personal :optional "reglas";`) || !strings.Contains(texto, `# sytue-office: anterior "reglas"`) {
		t.Fatalf("activo %q:\n%s", s.Activo(), texto)
	}
	if st := estado(t, s); !st.Activa || st.Anterior != "reglas" || st.Otro != "" {
		t.Fatalf("combinada: %+v", st)
	}
	// Cambiar el texto con la respuesta ya activa sigue incluyendo lo mismo.
	r.Asunto = "Otro asunto"
	if err := guardar(t, s, r, true); err != nil {
		t.Fatal(err)
	}
	if texto, _ := s.Script(propio); !strings.Contains(texto, `include :personal :optional "reglas";`) || !strings.Contains(texto, "Otro asunto") {
		t.Fatalf("sustituida:\n%s", texto)
	}
	if err := guardar(t, s, r, false); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != "reglas" {
		t.Fatalf("al apagar vuelve a estar activo «reglas», no %q", s.Activo())
	}
	if got, _ := s.Script("reglas"); got != reglas {
		t.Fatalf("el script de la persona cambió:\n%s", got)
	}
	if texto, _ := s.Script(propio); strings.Contains(texto, "include") {
		t.Fatalf("el propio apagado ya no incluye nada:\n%s", texto)
	}
	for _, o := range s.Ordenes() {
		if o == "DELETESCRIPT" {
			t.Fatal("nunca se borra nada")
		}
	}
}

// Sin `include` en el servidor, con otro script activo no se toca NADA.
func TestOtroActivoSinIncludeNoSeToca(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) {
		s.Extensiones = []string{"fileinto", "vacation", "date", "relational"}
	})
	s.Poner("reglas", reglas, true)
	st := estado(t, s)
	if st.Bloqueada != "reglas" || st.ConInclude {
		t.Fatalf("%+v", st)
	}
	err := guardar(t, s, respuestaDePrueba(), true)
	var eb *ErrBloqueada
	if !errors.As(err, &eb) || eb.Script != "reglas" {
		t.Fatalf("quiero ErrBloqueada: %v", err)
	}
	for _, o := range s.Ordenes() {
		if o == "PUTSCRIPT" || o == "SETACTIVE" || o == "DELETESCRIPT" {
			t.Fatalf("no se toca nada y se mandó %s", o)
		}
	}
	if s.Activo() != "reglas" || len(s.Scripts()) != 1 {
		t.Fatalf("activo %q, scripts %v", s.Activo(), s.Scripts())
	}
	// Apagar (lo que ya está apagado) tampoco toca lo activo.
	if err := guardar(t, s, respuestaDePrueba(), false); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != "reglas" {
		t.Fatalf("activo %q", s.Activo())
	}
}

// Un script que ya responde o rechaza correo, o que incluye scripts globales, no se
// combina: daría una segunda respuesta o un rechazo que no casa con ella.
func TestOtroActivoQueNoCasaNoSeCombina(t *testing.T) {
	for nombre, texto := range map[string]string{
		"responde": "require [\"vacation\"];\r\nvacation \"fuera\";\r\n",
		"rechaza":  "require [\"reject\"];\r\nif size :over 1M { reject \"grande\"; }\r\n",
		"global":   "require [\"include\"];\r\ninclude :global \"comun\";\r\n",
		"anidado":  "require [\"include\"];\r\ninclude :personal \"dentro\";\r\n",
		"roto":     "require [\"fileinto\"]; \"sin cerrar",
	} {
		s := servidor(t, nil)
		s.Poner("dentro", "require [\"vacation\"];\r\nvacation \"otra\";\r\n", false)
		s.Poner(nombre, texto, true)
		if st := estado(t, s); st.Bloqueada != nombre {
			t.Errorf("%s: %+v", nombre, st)
		}
		var eb *ErrBloqueada
		if err := guardar(t, s, respuestaDePrueba(), true); !errors.As(err, &eb) {
			t.Errorf("%s: %v", nombre, err)
		}
		if s.Activo() != nombre {
			t.Errorf("%s: activo %q", nombre, s.Activo())
		}
	}
	// Un include de algo que no existe no impide nada, y uno de algo inocente tampoco.
	s := servidor(t, nil)
	s.Poner("sano", reglas, false)
	s.Poner("reglas", "require [\"include\"];\r\ninclude :personal \"sano\";\r\ninclude :personal :optional \"no-esta\";\r\n", true)
	if st := estado(t, s); st.Bloqueada != "" {
		t.Fatalf("%+v", st)
	}
}

// Un script de la persona que se llama como el de la app no se sobrescribe nunca.
func TestScriptAjenoConElNombrePropio(t *testing.T) {
	s := servidor(t, nil)
	s.Poner(propio, reglas, true)
	st := estado(t, s)
	if st.Bloqueada != propio || st.Existe || st.Activa {
		t.Fatalf("%+v", st)
	}
	for _, activar := range []bool{true, false} {
		var eb *ErrBloqueada
		if err := guardar(t, s, respuestaDePrueba(), activar); !errors.As(err, &eb) {
			t.Fatalf("activar=%v: %v", activar, err)
		}
	}
	if got, _ := s.Script(propio); got != reglas || s.Activo() != propio {
		t.Fatal("se tocó el script de la persona")
	}
}

// Si el servidor rechaza el script, no se sube ni se activa nada.
func TestScriptRechazadoNoCambiaNada(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) {
		s.Fallos = map[string]string{"CHECKSCRIPT": `NO "line 5: error: unknown command"`}
	})
	s.Poner("reglas", reglas, true)
	err := guardar(t, s, respuestaDePrueba(), true)
	var es *ErrorServidor
	if !errors.As(err, &es) || es.Orden != "CHECKSCRIPT" {
		t.Fatalf("%v", err)
	}
	if s.Activo() != "reglas" || len(s.Scripts()) != 1 {
		t.Fatalf("activo %q, scripts %v", s.Activo(), s.Scripts())
	}
}

// Sin VERSION no hay CHECKSCRIPT: valida PUTSCRIPT.
func TestSinCheckscriptValidaPutscript(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.SinVersion = true })
	if err := guardar(t, s, respuestaDePrueba(), true); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(s.Ordenes(), "CHECKSCRIPT") || s.Activo() != propio {
		t.Fatalf("órdenes %v, activo %q", s.Ordenes(), s.Activo())
	}
}

func TestSinVacationNiFechas(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.Extensiones = []string{"fileinto"} })
	if _, err := LeerEstado(conectado(t, s), propio); !errors.Is(err, ErrSinVacaciones) {
		t.Fatalf("sin vacation: %v", err)
	}
	s2 := servidor(t, func(s *sieveprueba.Servidor) { s.Extensiones = []string{"vacation", "date"} })
	if st := estado(t, s2); st.ConFechas {
		t.Fatal("date sin relational no da fechas")
	}
	r := respuestaDePrueba()
	r.Hasta = dia(2026, 12, 24)
	if err := guardar(t, s2, r, true); !errors.Is(err, ErrSinFechas) {
		t.Fatalf("%v", err)
	}
	if s2.Activo() != "" || len(s2.Scripts()) != 0 {
		t.Fatal("no se subió nada")
	}
}

// Si el script de la persona que se incluía ya no está, apagar deja todo apagado (no hay
// nada que devolver) en vez de fallar.
func TestAnteriorBorradoAlApagar(t *testing.T) {
	s := servidor(t, nil)
	s.Poner("reglas", reglas, true)
	if err := guardar(t, s, respuestaDePrueba(), true); err != nil {
		t.Fatal(err)
	}
	c := conectado(t, s)
	if _, err := c.orden("DELETESCRIPT", "reglas"); err != nil {
		t.Fatal(err)
	}
	if st := estado(t, s); st.Anterior != "" || !st.Activa {
		t.Fatalf("%+v", st)
	}
	if err := guardar(t, s, respuestaDePrueba(), false); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != "" {
		t.Fatalf("activo %q", s.Activo())
	}
}

// Apagar sin haberla encendido nunca y sin texto no deja un script vacío en el servidor.
func TestApagarSinNadaNoEscribe(t *testing.T) {
	s := servidor(t, nil)
	if err := guardar(t, s, Respuesta{}, false); err != nil {
		t.Fatal(err)
	}
	if len(s.Scripts()) != 0 || slices.Contains(s.Ordenes(), "PUTSCRIPT") {
		t.Fatalf("scripts %v", s.Scripts())
	}
}
