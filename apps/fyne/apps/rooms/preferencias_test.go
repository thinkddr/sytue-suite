// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"
	"time"
)

// almacenFalso es el almacén por persona en memoria.
type almacenFalso map[string]string

func (a almacenFalso) Leer(k string) string { return a[k] }
func (a almacenFalso) Poner(k, v string) {
	if v == "" {
		delete(a, k)
		return
	}
	a[k] = v
}

// Lo plegado, la barra de formato y la caducidad del estado sobreviven a cerrar la app:
// otra Sala con el mismo almacén los encuentra, y un estado ya vencido se borra al
// arrancar.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `sa.guardarPlegadas()` en
// PlegarSeccion, la segunda Sala abre con «Mensajes directos» desplegada.
func TestLasPreferenciasSobrevivenAlCierre(t *testing.T) {
	almacen := almacenFalso{}
	ahora := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Almacen, sa.Ahora = almacen, func() time.Time { return ahora }
	sa.Arrancar(context.Background())
	sa.PlegarSeccion(Seccion{Clave: ClaveDirectos})
	sa.P.Canal.Compositor.AlternarFormato()
	sa.CambiarPresencia(presenciaAPI(sa.P.Lateral.Presencia), "Comiendo", true, ahora.Add(30*time.Minute))
	if almacen[ClavePlegadas] != `["dms"]` || almacen[ClaveFormato] != "1" || almacen[ClaveEstado] == "" {
		t.Fatalf("se guarda %v", almacen)
	}

	s2 := servicioConDos()
	sa2, relojes := salaDePrueba(t, s2)
	sa2.Almacen, sa2.Ahora = almacen, func() time.Time { return ahora.Add(10 * time.Minute) }
	sa2.P.Lateral.EstadoPersonal = "Comiendo"
	sa2.Arrancar(context.Background())
	if !sa2.P.Lateral.Plegadas[ClaveDirectos] || !sa2.P.Canal.Compositor.FormatoVisible {
		t.Errorf("la segunda Sala abre con plegadas %v y formato %v", sa2.P.Lateral.Plegadas, sa2.P.Canal.Compositor.FormatoVisible)
	}
	if relojes[20*time.Minute] == nil || sa2.P.Lateral.EstadoPersonal != "Comiendo" {
		t.Errorf("el estado vigente no se reprograma: %v, %q", relojes, sa2.P.Lateral.EstadoPersonal)
	}

	s3 := servicioConDos()
	sa3, _ := salaDePrueba(t, s3)
	sa3.Almacen, sa3.Ahora = almacen, func() time.Time { return ahora.Add(time.Hour) }
	sa3.P.Lateral.EstadoPersonal = "Comiendo"
	sa3.Arrancar(context.Background())
	if sa3.P.Lateral.EstadoPersonal != "" || almacen[ClaveEstado] != "" {
		t.Errorf("el estado vencido sigue: %q, %q", sa3.P.Lateral.EstadoPersonal, almacen[ClaveEstado])
	}
}
