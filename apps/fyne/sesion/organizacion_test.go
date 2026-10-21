// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// organizadorDoble apunta lo que se le pide.
type organizadorDoble struct {
	pasos []string
	fallo error
}

func (o *organizadorDoble) Organizaciones(context.Context) ([]api.Organizacion, error) {
	return []api.Organizacion{{ID: "o1", Nombre: "Acme", Activa: true}, {ID: "o2", Nombre: "Globex"}}, nil
}

func (o *organizadorDoble) CambiarOrganizacion(_ context.Context, id string) error {
	o.pasos = append(o.pasos, "cambiar "+id)
	return o.fallo
}

// TestCambiarDeOrganizacionEsVolverAEntrar: primero el cambio en Sytue ID y DESPUÉS la
// vuelta a entrar, que es lo que hace que el token lleve la organización nueva. Si
// Sytue ID dice que no, no se abre el navegador y el error llega a quien lo pidió.
//
// Visto en ROJO a mano (en el frontal): sin la vuelta a entrar —solo el PUT—, el test
// dice que falta el paso «entrar», que es el fallo que tendría la sesión: seguir con
// el tenant de antes.
func TestCambiarDeOrganizacionEsVolverAEntrar(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	doble := &organizadorDoble{}
	s.orgs = doble
	s.reentrar = func(context.Context) error { doble.pasos = append(doble.pasos, "entrar"); return nil }

	if orgs, err := s.Organizaciones(context.Background()); err != nil || len(orgs) != 2 || !orgs[0].Activa {
		t.Fatalf("organizaciones: %+v, %v", orgs, err)
	}
	if err := s.CambiarOrganizacion(context.Background(), "o2"); err != nil {
		t.Fatal(err)
	}
	if quiero := []string{"cambiar o2", "entrar"}; !slices.Equal(doble.pasos, quiero) {
		t.Errorf("pasos %v; quiero %v", doble.pasos, quiero)
	}

	doble.pasos, doble.fallo = nil, errors.New("403: esa organización está suspendida")
	if err := s.CambiarOrganizacion(context.Background(), "o3"); err == nil {
		t.Error("un cambio que Sytue ID rechaza no puede salir bien")
	}
	if quiero := []string{"cambiar o3"}; !slices.Equal(doble.pasos, quiero) {
		t.Errorf("con el cambio rechazado se ha seguido: %v", doble.pasos)
	}
}
