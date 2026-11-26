// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package avisos

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type sistemaDePrueba struct {
	programados map[string]Aviso
	cancelados  []string
	n           int
}

func (s *sistemaDePrueba) Programar(_ string, a Aviso) (string, error) {
	s.n++
	id := fmt.Sprintf("s%d", s.n)
	s.programados[id] = a
	return id, nil
}

func (s *sistemaDePrueba) Cancelar(id string) error {
	s.cancelados = append(s.cancelados, id)
	delete(s.programados, id)
	return nil
}

// Se programa solo la ventana de 30 días; lo que no cambia no se toca; lo que cambia o
// desaparece se quita; y al volver a abrir se acuerda de lo que dejó al sistema.
func TestReprogramar(t *testing.T) {
	ahora := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	ruta := filepath.Join(t.TempDir(), "avisos.json")
	s := &sistemaDePrueba{programados: map[string]Aviso{}}
	a := Abrir(ruta, s)
	lista := []Aviso{
		{Clave: "e1", Titulo: "Dentista", Cuando: ahora.Add(time.Hour)},
		{Clave: "e2", Titulo: "Pasado", Cuando: ahora.Add(-time.Hour)},
		{Clave: "e3", Titulo: "Lejos", Cuando: ahora.Add(40 * 24 * time.Hour)},
		{Clave: "e4", Titulo: "Comida", Cuando: ahora.Add(48 * time.Hour)},
	}
	if err := a.Reprogramar("calendar", lista, ahora); err != nil {
		t.Fatal(err)
	}
	if len(s.programados) != 2 {
		t.Fatalf("programados: %+v", s.programados)
	}
	// Otra apertura: el mismo estado; cambia el título de e4 y desaparece e1.
	b := Abrir(ruta, s)
	if got := b.Programados("calendar"); len(got) != 2 || got[0].Clave != "e1" {
		t.Fatalf("recordados: %+v", got)
	}
	n := s.n
	lista = []Aviso{{Clave: "e4", Titulo: "Comida con Ana", Cuando: ahora.Add(48 * time.Hour)}}
	if err := b.Reprogramar("calendar", lista, ahora); err != nil {
		t.Fatal(err)
	}
	if len(s.cancelados) != 2 || s.n != n+1 || len(s.programados) != 1 {
		t.Fatalf("cancelados %v, programados %+v", s.cancelados, s.programados)
	}
	// Sin cambios, nada se toca.
	n, c := s.n, len(s.cancelados)
	if err := b.Reprogramar("calendar", lista, ahora); err != nil || s.n != n || len(s.cancelados) != c {
		t.Fatalf("sin cambios se reprogramó: %v", err)
	}
	// Otro origen no pisa al primero.
	if err := b.Reprogramar("notes", []Aviso{{Clave: "n1", Titulo: "Nota", Cuando: ahora.Add(time.Minute)}}, ahora); err != nil {
		t.Fatal(err)
	}
	if len(b.Programados("calendar")) != 1 || len(b.Programados("notes")) != 1 {
		t.Fatal("un origen pisó a otro")
	}
}

// Nunca más de Maximo: iOS no guarda más.
func TestMaximo(t *testing.T) {
	ahora := time.Now()
	s := &sistemaDePrueba{programados: map[string]Aviso{}}
	lista := make([]Aviso, 0, Maximo+10)
	for i := range Maximo + 10 {
		lista = append(lista, Aviso{Clave: fmt.Sprint(i), Titulo: "x", Cuando: ahora.Add(time.Duration(i+1) * time.Minute)})
	}
	if err := Abrir("", s).Reprogramar("calendar", lista, ahora); err != nil {
		t.Fatal(err)
	}
	if len(s.programados) != Maximo {
		t.Fatalf("programados: %d", len(s.programados))
	}
}

// El reloj del proceso avisa a su hora, y lo cancelado no suena.
func TestTemporizador(t *testing.T) {
	var mu sync.Mutex
	var sonaron []string
	listo := make(chan struct{}, 2)
	tm := &Temporizador{Avisar: func(a Aviso) {
		mu.Lock()
		sonaron = append(sonaron, a.Clave)
		mu.Unlock()
		listo <- struct{}{}
	}}
	ruta := filepath.Join(t.TempDir(), "avisos.json")
	a := Abrir(ruta, tm)
	ahora := time.Now()
	if err := a.Reprogramar("calendar", []Aviso{
		{Clave: "ya", Titulo: "ya", Cuando: ahora.Add(20 * time.Millisecond)},
		{Clave: "no", Titulo: "no", Cuando: ahora.Add(time.Hour)},
	}, ahora); err != nil {
		t.Fatal(err)
	}
	select {
	case <-listo:
	case <-time.After(5 * time.Second):
		t.Fatal("el aviso no sonó")
	}
	_ = a.Reprogramar("calendar", nil, time.Now())
	mu.Lock()
	defer mu.Unlock()
	if len(sonaron) != 1 || sonaron[0] != "ya" {
		t.Fatalf("sonaron: %v", sonaron)
	}
	// Un reloj del proceso no se fía de lo apuntado: al reabrir lo programa todo.
	if got := Abrir(ruta, &Temporizador{}).Programados("calendar"); len(got) != 0 {
		t.Fatalf("reabierto con reloj del proceso: %+v", got)
	}
}
