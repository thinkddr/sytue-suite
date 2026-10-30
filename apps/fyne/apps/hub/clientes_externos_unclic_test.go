// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TestUnClicCreaCopiaYDespuesAbre: el programa se abre DESPUÉS de tener la contraseña en
// el portapapeles, que es lo que va a pedir nada más abrirse; y si Sytue ID no la crea,
// no se copia ni se abre nada. VISTO EN ROJO abriendo antes de copiar.
func TestUnClicCreaCopiaYDespuesAbre(t *testing.T) {
	var orden []string
	crear := func(_ context.Context, p Plataforma) (string, error) {
		orden = append(orden, "crear "+string(p))
		return "abcd-efgh", nil
	}
	copiar := func(clave string) { orden = append(orden, "copiar "+clave) }
	abrir := func(p Plataforma) error {
		orden = append(orden, "abrir "+string(p))
		return nil
	}
	c := configurarCliente(context.Background(), Thunderbird, crear, copiar, abrir)
	if quiero := []string{"crear thunderbird", "copiar abcd-efgh", "abrir thunderbird"}; !slices.Equal(orden, quiero) {
		t.Fatalf("orden = %q, quiero %q", orden, quiero)
	}
	if c != (Cliente{Plataforma: Thunderbird, Clave: "abcd-efgh", Abierto: true}) {
		t.Errorf("cliente = %+v", c)
	}

	// Sin programa en el equipo: la contraseña está, y la pantalla dirá «ábrelo tú».
	c = configurarCliente(context.Background(), Outlook, crear, copiar, func(Plataforma) error { return errors.New("no está") })
	if c.Clave == "" || c.Abierto {
		t.Errorf("sin Outlook = %+v", c)
	}

	orden = nil
	fallo := func(context.Context, Plataforma) (string, error) { return "", errors.New("429") }
	c = configurarCliente(context.Background(), Android, fallo, copiar, abrir)
	if len(orden) != 0 || !c.Fallo || c.Clave != "" {
		t.Errorf("con fallo: orden %q, cliente %+v", orden, c)
	}
}
