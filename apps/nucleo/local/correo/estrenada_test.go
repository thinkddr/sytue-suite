// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// avisos recoge lo que el buzón avisa (AlLlegar) y lo devuelve, vaciándolo, al pedirlo.
func avisos(b *Buzon) func() []Llegada {
	var mu sync.Mutex
	var ls []Llegada
	b.AlLlegar = func(x []Llegada) {
		mu.Lock()
		defer mu.Unlock()
		ls = append(ls, x...)
	}
	return func() []Llegada {
		mu.Lock()
		defer mu.Unlock()
		out := ls
		ls = nil
		return out
	}
}

// TestLaEntradaVaciaEnLaPrimeraVueltaRecibeLoNuevo: una cuenta cuya entrada estaba VACÍA
// en su primera vuelta no tiene nada «de antes», así que el primer correo que llega
// después es nuevo: avisa y, si es la respuesta de un invitado, va a la cola de Calendar.
// Una entrada estrenada vacía a la que de golpe llega más que la ventana (un archivo
// traído de otro sitio) no avisa de todo eso: se baja como la primera vez, y lo que llega
// después sí es nuevo. Y la primera vuelta de una entrada con historia sigue sin avisar
// de lo que ya estaba.
func TestLaEntradaVaciaEnLaPrimeraVueltaRecibeLoNuevo(t *testing.T) {
	ctx := context.Background()

	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	b := abrir(t, si, ss)
	tomar := avisos(b)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar(); len(ls) != 0 {
		t.Fatalf("la primera vuelta vacía no avisa de nada: %+v", ls)
	}
	si.entregar(t, "INBOX", replyDe("primero", "Luis <luis@ejemplo.eu>", "luis@ejemplo.eu", usuario, "ACCEPTED"))
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar(); len(ls) != 1 || ls[0].Mensaje.MessageID != "primero@ejemplo.eu" {
		t.Fatalf("el primer correo de una entrada que estaba vacía es nuevo: %+v", ls)
	}
	if rs, err := RespuestasPorAplicar(b.raiz); err != nil || len(rs) != 1 {
		t.Fatalf("y la respuesta del invitado va a la cola de Calendar: %d %v", len(rs), err)
	}
	si.entregar(t, "INBOX", segundo)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar(); len(ls) != 1 || ls[0].Mensaje.Subject != "Presupuesto" {
		t.Fatalf("y lo que sigue llegando también: %+v", ls)
	}

	// Estrenada vacía y, de golpe, más que la ventana: historia, no llegadas.
	si2, ss2 := nuevoIMAP(t), nuevoSMTP(t)
	b2 := abrir(t, si2, ss2)
	tomar2 := avisos(b2)
	if err := b2.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	for i := range Ventana + 1 {
		si2.entregar(t, "INBOX", strings.Replace(primero, "<uno@", "<archivo-"+strconv.Itoa(i)+"@", 1))
	}
	if err := b2.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar2(); len(ls) != 0 {
		t.Fatalf("un archivo entero no avisa: %d llegadas", len(ls))
	}
	if ms, _, _ := b2.Messages(ctx, "rol:inbox", "", Ventana+10); len(ms) != Ventana {
		t.Fatalf("se baja la ventana, como la primera vez: %d", len(ms))
	}
	si2.entregar(t, "INBOX", segundo)
	if err := b2.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar2(); len(ls) != 1 || ls[0].Mensaje.Subject != "Presupuesto" {
		t.Fatalf("lo que llega después sí es nuevo: %+v", ls)
	}

	// Con historia: lo que ya estaba en la primera vuelta no avisa.
	si3, ss3 := nuevoIMAP(t), nuevoSMTP(t)
	si3.entregar(t, "INBOX", primero)
	b3 := abrir(t, si3, ss3)
	tomar3 := avisos(b3)
	if err := b3.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if ls := tomar3(); len(ls) != 0 {
		t.Fatalf("lo de antes de la primera vuelta no avisa: %+v", ls)
	}
}
