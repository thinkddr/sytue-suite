// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func esperarFototeca(t *testing.T, f *Fototeca) {
	t.Helper()
	listo := make(chan struct{})
	go func() { f.Esperar(); close(listo) }()
	select {
	case <-listo:
	case <-time.After(5 * time.Second):
		t.Fatal("el trabajo retirado no terminó")
	}
}

func TestFototecaLimitaDescargasYCancelaLaCola(t *testing.T) {
	f := NuevaFototeca()
	datos := pngDe(bandas(16, 16))
	f.Poner("lista", datos)
	preparada := f.TeselaYa("lista", 16, 16, 0)
	iniciadas := make(chan string, 100)
	var llamadas, canceladas atomic.Int64
	f.Pedir = func(ctx context.Context, id, _ string) ([]byte, error) {
		llamadas.Add(1)
		iniciadas <- id
		<-ctx.Done()
		canceladas.Add(1)
		return nil, ctx.Err()
	}
	defer f.Cancelar()
	for i := range 100 {
		f.Precargar(fmt.Sprint(i))
	}
	// Presupuesto del contrato, independiente de la constante de implementación.
	const maxPedidos = 6
	for range maxPedidos {
		select {
		case <-iniciadas:
		case <-time.After(time.Second):
			t.Fatal("no arrancaron las descargas")
		}
	}
	select {
	case <-iniciadas:
		t.Fatal("se abrió más de una pantalla de descargas a la vez")
	case <-time.After(30 * time.Millisecond):
	}
	// La foto abierta tiene su propia fototeca: una galería en espera no la bloquea.
	visor := NuevaFototeca()
	visor.Pedir = func(context.Context, string, string) ([]byte, error) { return datos, nil }
	visor.Precargar("abierta")
	esperarFototeca(t, visor)
	if visor.TeselaYa("abierta", 16, 16, 0) == nil {
		t.Fatal("la foto abierta esperó a la galería")
	}
	f.Cancelar()
	esperarFototeca(t, f)
	if llamadas.Load() != maxPedidos || canceladas.Load() != maxPedidos {
		t.Fatalf("la cola retirada abrió peticiones: llamadas=%d canceladas=%d", llamadas.Load(), canceladas.Load())
	}
	if f.Tesela("lista", 16, 16, 0) != preparada {
		t.Fatal("cambiar de pantalla vació una foto ya lista")
	}
	f.Pedir = func(context.Context, string, string) ([]byte, error) { return datos, nil }
	f.Precargar("0")
	esperarFototeca(t, f)
	if f.TeselaYa("0", 16, 16, 0) == nil {
		t.Fatal("cancelar marcó la foto como fallida y ya no se puede reintentar")
	}
}

func TestFototecaDescartaRespuestaDeLaPantallaAnterior(t *testing.T) {
	f := NuevaFototeca()
	inicio, soltar := make(chan struct{}), make(chan struct{})
	vieja := pngDe(bandas(16, 16))
	nueva := pngDe(bandas(32, 32))
	var llamadas atomic.Int64
	f.Pedir = func(context.Context, string, string) ([]byte, error) {
		if llamadas.Add(1) == 1 {
			close(inicio)
			<-soltar
			return vieja, nil
		}
		return nueva, nil
	}
	f.Precargar("misma")
	select {
	case <-inicio:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la descarga anterior")
	}
	f.Cancelar()
	f.Precargar("misma")
	close(soltar)
	esperarFototeca(t, f)
	f.mu.Lock()
	crudas := f.crudas["misma"]
	f.mu.Unlock()
	if !bytes.Equal(crudas, nueva) {
		t.Fatal("una respuesta cancelada sustituyó la foto de la pantalla nueva")
	}
}

func TestFototecaTirarNoRestauraUnaPreparacionVieja(t *testing.T) {
	f := NuevaFototeca()
	f.Poner("rotada", pngDe(bandas(16, 16)))
	f.mu.Lock()
	vieja := preparacionFoto{clave: clavePintada{"rotada", 16, 16}, trabajoFoto: f.trabajo("rotada")}
	f.mu.Unlock()
	f.Tirar("rotada")
	f.Poner("rotada", pngDe(bandas(32, 32)))
	f.prepararTrabajo(vieja)
	if f.Pintada("rotada") != nil {
		t.Fatal("una preparación anterior a la rotación volvió a la caché")
	}
	if f.TeselaYa("rotada", 16, 16, 0) == nil {
		t.Fatal("no se puede preparar la foto rotada")
	}
}

func TestPhotosRetiraFototecasAlNavegarYAlSalir(t *testing.T) {
	f := NuevaFototeca()
	v := vistaDePrueba(t, f)
	v.TecaGrande = f
	a := Nueva(nil, v, nil)
	inicio, cancelada := make(chan struct{}, 2), make(chan struct{}, 2)
	f.Pedir = func(ctx context.Context, _, _ string) ([]byte, error) {
		inicio <- struct{}{}
		<-ctx.Done()
		cancelada <- struct{}{}
		return nil, ctx.Err()
	}
	comprobar := func(retirar func()) {
		t.Helper()
		f.Precargar("pendiente")
		select {
		case <-inicio:
		case <-time.After(time.Second):
			t.Fatal("no comenzó la foto")
		}
		retirar()
		select {
		case <-cancelada:
		case <-time.After(time.Second):
			t.Fatal("salir dejó una petición de la pantalla anterior")
		}
		esperarFototeca(t, f)
	}
	comprobar(func() { a.Ir(RutaFotos) })
	comprobar(a.AlFondo)
}
