// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type delServidor struct {
	id     string
	tomada time.Time
}

func dia(a, m, d int) time.Time { return time.Date(a, time.Month(m), d, 12, 0, 0, 0, time.UTC) }

func cifradaDe(id string, t time.Time) Foto {
	return Foto{Fichero: api.File{ID: id, Name: id + ".jpg"}, Entrada: Entrada{Foto: id, Nombre: id + ".jpg", Tomada: t}}
}

func ids(l []ElementoLinea[delServidor]) string {
	var out []string
	for _, e := range l {
		if e.EsCifrada {
			out = append(out, "c:"+e.Cifrada.Fichero.ID)
		} else {
			out = append(out, e.Servidor.id)
		}
	}
	return strings.Join(out, " ")
}

// La regla de la web (`lineaMezclada.test.ts`): por fecha, y una cifrada solo entra
// cuando la línea del servidor ya ha llegado hasta su fecha o ha terminado.
func TestMezclarLineaPorFecha(t *testing.T) {
	tomada := func(s delServidor) time.Time { return s.tomada }
	servidor := []delServidor{{"s1", dia(2026, 5, 1)}, {"s2", dia(2025, 3, 1)}, {"s3", dia(2024, 8, 1)}}
	cifradas := []Foto{cifradaDe("nueva", dia(2026, 9, 1)), cifradaDe("empate", dia(2025, 3, 1)), cifradaDe("media", dia(2024, 12, 1)), cifradaDe("vieja", dia(2019, 1, 1))}

	casos := []struct {
		nombre   string
		servidor []delServidor
		o        OpcionesLinea
		quiero   string
	}{
		{"la vieja espera a que llegue su fecha; a igual fecha, primero la del servidor", servidor, OpcionesLinea{}, "c:nueva s1 s2 c:empate c:media s3"},
		{"con la línea terminada entran todas", servidor, OpcionesLinea{Fin: true}, "c:nueva s1 s2 c:empate c:media s3 c:vieja"},
		{"sin nada del servidor todavía, ninguna", nil, OpcionesLinea{}, ""},
		{"sin fotos en el servidor y terminado, todas", nil, OpcionesLinea{Fin: true}, "c:nueva c:empate c:media c:vieja"},
		{"con un año elegido, nada posterior", servidor[1:], OpcionesLinea{Anyo: 2025}, "s2 c:empate c:media s3"},
		{"con una búsqueda, las cifradas delante", servidor, OpcionesLinea{Busqueda: true, Anyo: 2025}, "c:empate c:media c:vieja s1 s2 s3"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := ids(MezclarLinea(c.servidor, tomada, cifradas, c.o)); got != c.quiero {
				t.Fatalf("línea = %q, quería %q", got, c.quiero)
			}
		})
	}
}

func TestBuscarCifradas(t *testing.T) {
	fotos := []Foto{cifradaDe("Playa", dia(2025, 7, 1)), cifradaDe("monte", dia(2024, 1, 1))}
	mes := func(t time.Time) string { return t.Format("January 2006") }
	if got := Buscar(fotos, " PLAYA ", mes); len(got) != 1 || got[0].Fichero.ID != "Playa" {
		t.Fatalf("por nombre = %+v", got)
	}
	if got := Buscar(fotos, "january 2024", mes); len(got) != 1 || got[0].Fichero.ID != "monte" {
		t.Fatalf("por fecha = %+v", got)
	}
	if got := Buscar(fotos, "", mes); len(got) != 2 {
		t.Fatal("sin texto, todas")
	}
}
