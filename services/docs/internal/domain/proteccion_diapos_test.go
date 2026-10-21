// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// simD lleva el estado de protección de una presentación como lo haría el almacén.
type simD struct {
	t   *testing.T
	est EstadoProteccion
	lam uint64
}

func nuevaSimD(t *testing.T) *simD {
	return &simD{t: t, est: EstadoProteccion{Claves: map[string]ClaveHash{}}}
}

func (s *simD) clonar(t *testing.T) *simD {
	c := &simD{
		t: t,
		est: EstadoProteccion{
			Modelo:      s.est.Modelo,
			HayRegistro: s.est.HayRegistro,
			Lamport:     s.est.Lamport,
			Dot:         s.est.Dot,
			Claves:      make(map[string]ClaveHash, len(s.est.Claves)),
			Secuencia:   s.est.Secuencia,
		},
		lam: s.lam,
	}
	for k, v := range s.est.Claves {
		c.est.Claves[k] = v
	}
	return c
}

// escribe aplica un set de doc:slides; devuelve el valor que quedó registrado.
func (s *simD) escribe(modelo string, dueno bool, p Proteccion) (string, error) {
	s.t.Helper()
	s.lam++
	v, _ := json.Marshal(modelo)
	res, err := AplicarDiapos(s.est, []Op{{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "c", Counter: s.lam}, Lamport: s.lam}}, dueno, p)
	if err != nil {
		return "", err
	}
	s.est = res.Estado
	var guardado string
	_ = json.Unmarshal(res.Ops[0].Value, &guardado)
	return guardado, nil
}

func (s *simD) niega(modelo string, dueno bool, p Proteccion, quiere error) {
	s.t.Helper()
	antes := s.est.Modelo
	if _, err := s.escribe(modelo, dueno, p); !errors.Is(err, quiere) {
		s.t.Fatalf("esperaba %v, salió %v", quiere, err)
	}
	if s.est.Modelo != antes {
		s.t.Fatal("una escritura denegada cambió el estado")
	}
}

type mod struct {
	s1, s2, s3 string // contenido de las cajas: «a» en s1, «b» en s2, «c» y «d» en s3
	protect    string
	extra      string // campos de nivel superior
	orden      []string
}

// modelo arma una presentación: s1 con la caja b1, s2 con b2 y s3 con b3 y b4.
func (m mod) json() string {
	if m.s1 == "" {
		m.s1 = "a"
	}
	if m.s2 == "" {
		m.s2 = "b"
	}
	if m.s3 == "" {
		m.s3 = "c"
	}
	diapo := func(id string, cajas ...string) string {
		return fmt.Sprintf(`{"id":%q,"boxes":[%s]}`, id, strings.Join(cajas, ","))
	}
	caja := func(id, texto string) string {
		return fmt.Sprintf(`{"id":%q,"x":10,"y":10,"w":50,"h":20,"kind":"text","content":%q}`, id, texto)
	}
	por := map[string]string{
		"s1": diapo("s1", caja("b1", m.s1)),
		"s2": diapo("s2", caja("b2", m.s2)),
		"s3": diapo("s3", caja("b3", m.s3), caja("b4", "d")),
	}
	orden := m.orden
	if orden == nil {
		orden = []string{"s1", "s2", "s3"}
	}
	ds := make([]string, 0, len(orden))
	for _, id := range orden {
		ds = append(ds, por[id])
	}
	out := `{"slides":[` + strings.Join(ds, ",") + `]`
	if m.protect != "" {
		out += `,"protect":` + m.protect
	}
	return out + m.extra + `}`
}

const protegeTodo = `{"slides":["s1"],"boxes":["b3"],"master":true}`

// presentacionProtegida: el propietario protege con contraseña «clave».
func presentacionProtegida(t *testing.T) *simD {
	s := nuevaSimD(t)
	base := mod{extra: `,"theme":"carbon","master":{"boxes":[{"id":"m1","content":"logo"}]}`}
	if _, err := s.escribe(base.json(), false, Proteccion{}); err != nil {
		t.Fatalf("sin protección escribe cualquiera: %v", err)
	}
	m := mod{protect: protegeTodo, extra: base.extra}
	if _, err := s.escribe(m.json(), false, Proteccion{Nuevas: map[string]string{AmbitoDiapos: "clave"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if s.est.Modelo == "" || len(s.est.Claves) != 1 {
		t.Fatalf("estado tras proteger: %+v", s.est)
	}
	return s
}

func TestDiapasBloqueadasCajasYPatron(t *testing.T) {
	const extra = `,"theme":"carbon","master":{"boxes":[{"id":"m1","content":"logo"}]}`
	casos := []struct {
		nombre string
		m      mod
		dueno  bool
		p      Proteccion
		quiere error
	}{
		{"editar una diapositiva sin bloquear", mod{s2: "b nueva", protect: protegeTodo, extra: extra}, false, Proteccion{}, nil},
		{"reordenar, también las bloqueadas", mod{orden: []string{"s3", "s1", "s2"}, protect: protegeTodo, extra: extra}, false, Proteccion{}, nil},
		{"editar una diapositiva bloqueada", mod{s1: "TRUCADA", protect: protegeTodo, extra: extra}, false, Proteccion{}, ErrProtegido},
		{"editar una caja bloqueada", mod{s3: "TRUCADA", protect: protegeTodo, extra: extra}, false, Proteccion{}, ErrProtegido},
		{"cambiar el tema", mod{protect: protegeTodo, extra: strings.Replace(extra, "carbon", "arena", 1)}, false, Proteccion{}, ErrProtegido},
		{"cambiar el patrón", mod{protect: protegeTodo, extra: strings.Replace(extra, "logo", "otro", 1)}, false, Proteccion{}, ErrProtegido},
		{"quitar el patrón", mod{protect: protegeTodo, extra: `,"theme":"carbon"`}, false, Proteccion{}, ErrProtegido},
		{"cambiar el tamaño del lienzo", mod{protect: protegeTodo, extra: extra + `,"size":{"preset":"4:3"}`}, false, Proteccion{}, ErrProtegido},
		{"quitar la protección", mod{extra: extra}, false, Proteccion{}, ErrProtegido},
		{"desbloquear una diapositiva", mod{protect: `{"slides":[],"boxes":["b3"],"master":true}`, extra: extra}, false, Proteccion{}, ErrProtegido},
		{"bloquear una más sin permiso", mod{protect: `{"slides":["s1","s2"],"boxes":["b3"],"master":true}`, extra: extra}, false, Proteccion{}, ErrProtegido},
		{"quitar la contraseña", mod{protect: `{"hasPassword":false,"slides":["s1"],"boxes":["b3"],"master":true}`, extra: extra}, false, Proteccion{}, ErrProtegido},
		{"poner otra contraseña", mod{protect: protegeTodo, extra: extra}, false, Proteccion{Nuevas: map[string]string{AmbitoDiapos: "otra"}}, ErrClaveProteccion},
		{"con la contraseña, editar lo bloqueado", mod{s1: "AUTORIZADA", protect: protegeTodo, extra: extra}, false, Proteccion{Clave: "clave"}, nil},
		{"con la contraseña, cambiar el tema", mod{protect: protegeTodo, extra: strings.Replace(extra, "carbon", "arena", 1)}, false, Proteccion{Clave: "clave"}, nil},
		{"con la contraseña, quitar la protección", mod{extra: extra}, false, Proteccion{Clave: "clave"}, nil},
		{"con la contraseña mala", mod{s1: "TRUCADA", protect: protegeTodo, extra: extra}, false, Proteccion{Clave: "mala"}, ErrClaveProteccion},
		{"el propietario, lo que quiera", mod{s1: "DUEÑO", s3: "DUEÑO", extra: ""}, true, Proteccion{}, nil},
	}
	base := presentacionProtegida(t)
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := base.clonar(t)
			if c.quiere != nil {
				s.niega(c.m.json(), c.dueno, c.p, c.quiere)
				return
			}
			if _, err := s.escribe(c.m.json(), c.dueno, c.p); err != nil {
				t.Fatalf("debía pasar: %v", err)
			}
		})
	}
}

func TestDiapasBloqueadasNoSeBorranNiSeMueven(t *testing.T) {
	s := presentacionProtegida(t)
	const extra = `,"theme":"carbon","master":{"boxes":[{"id":"m1","content":"logo"}]}`
	ds := func(cuerpo string) string {
		return `{"slides":[` + cuerpo + `],"protect":` + protegeTodo + extra + `}`
	}
	s1 := `{"id":"s1","boxes":[{"id":"b1","x":10,"y":10,"w":50,"h":20,"kind":"text","content":"a"}]}`
	s2 := `{"id":"s2","boxes":[{"id":"b2","x":10,"y":10,"w":50,"h":20,"kind":"text","content":"b"}]}`
	caja3 := `{"id":"b3","x":10,"y":10,"w":50,"h":20,"kind":"text","content":"c"}`
	caja4 := `{"id":"b4","x":10,"y":10,"w":50,"h":20,"kind":"text","content":"d"}`
	s3 := `{"id":"s3","boxes":[` + caja3 + `,` + caja4 + `]}`
	if _, err := s.escribe(ds(s1+","+s2+","+s3), false, Proteccion{}); err != nil {
		t.Fatalf("la presentación tal cual: %v", err)
	}
	casos := map[string]string{
		"borrar la diapositiva bloqueada":      ds(s2 + "," + s3),
		"borrar la caja bloqueada":             ds(s1 + "," + s2 + `,{"id":"s3","boxes":[` + caja4 + `]}`),
		"mover la caja bloqueada a otra":       ds(s1 + `,{"id":"s2","boxes":[` + caja3 + `]},{"id":"s3","boxes":[` + caja4 + `]}`),
		"duplicar la diapositiva bloqueada":    ds(`{"id":"s1","boxes":[]},` + s1 + "," + s2 + "," + s3),
		"duplicar la caja bloqueada":           ds(s1 + "," + s2 + `,{"id":"s3","boxes":[` + caja3 + `,` + strings.Replace(caja3, `"c"`, `"X"`, 1) + `,` + caja4 + `]}`),
		"añadir una caja a la bloqueada":       ds(strings.Replace(s1, `"boxes":[`, `"boxes":[{"id":"nueva","kind":"text","content":"x"},`, 1) + "," + s2 + "," + s3),
		"cambiar la nota de la bloqueada":      ds(strings.Replace(s1, `"id":"s1"`, `"id":"s1","notes":"hola"`, 1) + "," + s2 + "," + s3),
		"mover la caja bloqueada (posición)":   ds(s1 + "," + s2 + `,{"id":"s3","boxes":[` + strings.Replace(caja3, `"x":10`, `"x":11`, 1) + `,` + caja4 + `]}`),
		"cambiar un número":                    ds(strings.Replace(s1, `"x":10`, `"x":10.5`, 1) + "," + s2 + "," + s3),
		"sustituir la bloqueada por una vacía": ds(`{"id":"s1","boxes":[]},` + s2 + "," + s3),
	}
	for nombre, modelo := range casos {
		t.Run(nombre, func(t *testing.T) { s.niega(modelo, false, Proteccion{}, ErrProtegido) })
	}
	// Lo que sí: editar la otra caja de la diapositiva de la caja bloqueada, y reordenar claves.
	otra := strings.Replace(caja4, `"content":"d"`, `"content":"D nueva"`, 1)
	if _, err := s.escribe(ds(s1+","+s2+`,{"id":"s3","boxes":[`+caja3+`,`+otra+`]}`), false, Proteccion{}); err != nil {
		t.Fatalf("editar la otra caja: %v", err)
	}
	reordenada := `{"boxes":[{"content":"a","kind":"text","h":20,"w":50,"y":10,"x":10,"id":"b1"}],"id":"s1"}`
	if _, err := s.escribe(ds(reordenada+","+s2+`,{"id":"s3","boxes":[`+caja3+`,`+otra+`]}`), false, Proteccion{}); err != nil {
		t.Fatalf("las mismas claves en otro orden no son un cambio: %v", err)
	}
}

// La contraseña no entra nunca en el modelo: se quita lo que el cliente intente colar, y
// hasPassword lo fija el servidor.
func TestDiapasLaContrasenaNoEntraEnElModelo(t *testing.T) {
	s := nuevaSimD(t)
	guardado, err := s.escribe(mod{protect: `{"password":"s3creto","hasPassword":true,"slides":["s1"]}`}.json(), false, Proteccion{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(guardado, "s3creto") || strings.Contains(guardado, "hasPassword") {
		t.Fatalf("el modelo registrado lleva la contraseña o un hasPassword falso: %s", guardado)
	}
	// Con la contraseña en el sobre, hasPassword la fija el servidor.
	s = nuevaSimD(t)
	guardado, err = s.escribe(mod{protect: `{"slides":["s1"]}`}.json(), false, Proteccion{Nuevas: map[string]string{AmbitoDiapos: "sobre"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(guardado, `"hasPassword":true`) || strings.Contains(guardado, "sobre") {
		t.Fatalf("modelo registrado: %s", guardado)
	}
	h := s.est.Claves[AmbitoDiapos]
	if h.Algoritmo != "SHA-512" || h.Vueltas != claveVueltas || len(h.Hash) == 0 || !h.Coincide("sobre") {
		t.Fatalf("hash de la contraseña: %+v", h)
	}
	if strings.Contains(s.est.Modelo, "sobre") || strings.Contains(s.est.Modelo, "hashValue") {
		t.Fatal("la huella de lo bloqueado lleva la contraseña")
	}
	// Una op que pierde el LWW tampoco deja la contraseña en el registro de operaciones.
	v, _ := json.Marshal(mod{protect: `{"password":"s3creto"}`}.json())
	res, err := AplicarDiapos(s.est, []Op{{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 0}}, false, Proteccion{})
	if err != nil || strings.Contains(string(res.Ops[0].Value), "s3creto") {
		t.Fatalf("op perdedora: err=%v op=%s", err, res.Ops[0].Value)
	}
}

func TestDiapasSinContrasenaSoloElPropietario(t *testing.T) {
	s := nuevaSimD(t)
	if _, err := s.escribe(mod{protect: `{"slides":["s1"]}`}.json(), false, Proteccion{}); err != nil {
		t.Fatal(err)
	}
	s.niega(mod{s1: "X", protect: `{"slides":["s1"]}`}.json(), false, Proteccion{}, ErrProtegido)
	s.niega(mod{s1: "X", protect: `{"slides":["s1"]}`}.json(), false, Proteccion{Clave: "cualquiera"}, ErrProtegido) // no hay contraseña que presentar
	if _, err := s.escribe(mod{s1: "dueño", protect: `{"slides":["s1"]}`}.json(), true, Proteccion{}); err != nil {
		t.Fatalf("el propietario: %v", err)
	}
	if _, err := s.escribe(mod{}.json(), true, Proteccion{}); err != nil || s.est.Modelo != "" {
		t.Fatalf("el propietario quita la protección: %v %q", err, s.est.Modelo)
	}
}

// Quitar la protección borra la contraseña: quien la protege de nuevo parte de cero.
func TestDiapasQuitarLaProteccionBorraLaClave(t *testing.T) {
	s := presentacionProtegida(t)
	s.lam++
	v, _ := json.Marshal(mod{}.json())
	res, err := AplicarDiapos(s.est, []Op{{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "c", Counter: 99}, Lamport: 99}}, true, Proteccion{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Quitar) != 1 || res.Quitar[0] != AmbitoDiapos || res.Estado.Modelo != "" {
		t.Fatalf("quitar: %+v", res)
	}
}

func TestDiapasOpPerdedoraNoCuenta(t *testing.T) {
	s := presentacionProtegida(t)
	antes := s.est.Modelo
	v, _ := json.Marshal(mod{s1: "TRUCADA"}.json())
	// Lamport 0: pierde contra el registro, que el CRDT descarta.
	res, err := AplicarDiapos(s.est, []Op{{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 0}}, false, Proteccion{})
	if err != nil || res.Estado.Modelo != antes {
		t.Fatalf("op perdedora: err=%v", err)
	}
}

func TestDiapasValorIlegibleEquivaleAQuitar(t *testing.T) {
	s := presentacionProtegida(t)
	s.lam += 10
	v, _ := json.Marshal("esto no es json {")
	_, err := AplicarDiapos(s.est, []Op{{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "c", Counter: 50}, Lamport: 50}}, false, Proteccion{})
	if !errors.Is(err, ErrProtegido) {
		t.Fatalf("un editor sustituyó el modelo por basura: %v", err)
	}
}

func TestDiapasSinProteccionNoCuestanNada(t *testing.T) {
	s := nuevaSimD(t)
	if _, err := s.escribe(mod{s1: "x"}.json(), false, Proteccion{}); err != nil {
		t.Fatal(err)
	}
	if s.est.Modelo != "" || len(s.est.Claves) != 0 {
		t.Fatalf("una presentación sin protección guardó estado: %+v", s.est)
	}
}

// Un identificador repetido en el modelo vigente no se puede proteger, y no deja fuera a
// los editores por ello.
func TestDiapasIdentificadorRepetidoNoBloqueaAFuera(t *testing.T) {
	s := nuevaSimD(t)
	dup := `{"slides":[{"id":"s1","boxes":[]},{"id":"s1","boxes":[]},{"id":"s2","boxes":[]}],"protect":{"slides":["s1","s2"]}}`
	if _, err := s.escribe(dup, false, Proteccion{}); err != nil {
		t.Fatal(err)
	}
	q := leerProyeccion(s.est.Modelo)
	if _, hay := q.Slides["s1"]; hay || q.Slides["s2"] == "" {
		t.Fatalf("proyección: %+v", q)
	}
}

func TestEstadoInicialDeDiapos(t *testing.T) {
	st := NewState()
	v, _ := json.Marshal(mod{protect: protegeTodo, extra: `,"theme":"carbon"`}.json())
	if _, err := st.Apply(Op{Kind: OpSet, Key: claveDiapos, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 7}); err != nil {
		t.Fatal(err)
	}
	est, err := EstadoInicialProteccion(KindPresentation, st)
	if err != nil {
		t.Fatal(err)
	}
	q := leerProyeccion(est.Modelo)
	if !est.HayRegistro || est.Lamport != 7 || q == nil || len(q.Slides) != 1 || len(q.Boxes) != 1 || q.Master == "" {
		t.Fatalf("estado inicial: %+v %+v", est, q)
	}
	est, _ = EstadoInicialProteccion(KindPresentation, NewState())
	if est.HayRegistro || est.Modelo != "" {
		t.Fatalf("estado inicial vacío: %+v", est)
	}
}

func TestProtegeEscrituraDeDiapos(t *testing.T) {
	set := []Op{{Kind: OpSet, Key: claveDiapos}}
	if !ProtegeEscritura(KindPresentation, set) || ProtegeEscritura(KindPresentation, []Op{{Kind: OpSet, Key: "otra"}}) {
		t.Fatal("solo el set de doc:slides pasa por la protección de Diapos")
	}
}
