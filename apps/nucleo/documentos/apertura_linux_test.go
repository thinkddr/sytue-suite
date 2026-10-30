// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type datosAperturaPrueba struct {
	mu       sync.Mutex
	cred     Credencial
	acceso   ajustes.AccesoDrive
	abiertas []string
}

func prepararDatosApertura(t *testing.T) (*datosAperturaPrueba, *auth.Session, context.Context, Identidad) {
	t.Helper()
	s := auth.NewSession(auth.Tokens{Issuer: "https://id.a.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-a", Refresh: "synthetic-refresh", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	i := s.SnapshotIdentity()
	dueno := Identidad{Emisor: i.Issuer, Cliente: i.Client, Sujeto: i.Subject, Organizacion: i.Organization, CuentaSesion: "synthetic-store-a"}
	d := &datosAperturaPrueba{
		cred:   Credencial{ID: "fixture", URL: "https://drive.a.example/dav/", Usuario: "ana@a.example", Clave: "synthetic-secret", Dueno: dueno},
		acceso: ajustes.AccesoDrive{Version: 1, Identidad: i.IdentidadDrive(), Revision: strings.Repeat("a", 32), Activo: true},
	}
	l, x, a, o := leer, excluir, leerAccesoApertura, abrirUbicacion
	t.Cleanup(func() { leer, excluir, leerAccesoApertura, abrirUbicacion = l, x, a, o })
	semaforo := make(chan struct{}, 1)
	semaforo <- struct{}{}
	excluir = func(ctx context.Context) (func(), error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-semaforo:
		}
		var once sync.Once
		return func() { once.Do(func() { semaforo <- struct{}{} }) }, nil
	}
	leer = func(string) ([]byte, error) { d.mu.Lock(); defer d.mu.Unlock(); return json.Marshal(d.cred) }
	leerAccesoApertura = func() (ajustes.AccesoDrive, error) { d.mu.Lock(); defer d.mu.Unlock(); return d.acceso, nil }
	abrirUbicacion = func(_ context.Context, uri string) error {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.abiertas = append(d.abiertas, uri)
		return nil
	}
	ctx := s.WithAuthorizationIntent(context.Background())
	return d, s, ctx, dueno
}

func TestAperturaRechazaCambioDuranteLecturaRealDeCredencial(t *testing.T) {
	for _, cambio := range []string{"sesion", "sesion-aba", "proveedor", "organizacion-misma-uri"} {
		t.Run(cambio, func(t *testing.T) {
			d, s, ctx, dueno := prepararDatosApertura(t)
			entrando, liberar := make(chan struct{}), make(chan struct{})
			var once sync.Once
			original := leer
			leer = func(k string) ([]byte, error) { close(entrando); <-liberar; return original(k) }
			done := make(chan error, 1)
			terminada := make(chan struct{})
			t.Cleanup(func() { once.Do(func() { close(liberar) }); <-terminada })
			go func() {
				defer close(terminada)
				a, err := PrepararApertura(ctx, dueno, "https://drive.a.example/dav/")
				if err == nil {
					defer a.Close()
					err = a.Abrir(ctx)
				}
				done <- err
			}()
			select {
			case <-entrando:
			case <-time.After(time.Second):
				t.Fatal("no se bloqueó la lectura del almacén de credenciales")
			}
			if cambio == "sesion" || cambio == "sesion-aba" {
				s.Sincronizar(auth.Tokens{Issuer: dueno.Emisor, Cliente: dueno.Cliente, Subject: "bea", Organizacion: "otra", Access: "synthetic-b", Refresh: "synthetic-b", Expiry: time.Now().Add(time.Hour)})
				if cambio == "sesion-aba" {
					s.Sincronizar(auth.Tokens{Issuer: dueno.Emisor, Cliente: dueno.Cliente, Subject: dueno.Sujeto, Organizacion: dueno.Organizacion, Access: "synthetic-a2", Refresh: "synthetic-a2", Expiry: time.Now().Add(time.Hour)})
				}
			} else {
				d.mu.Lock()
				d.cred.Dueno.Organizacion = "otra"
				if cambio == "proveedor" {
					d.cred.Dueno.Sujeto, d.cred.Usuario, d.cred.URL = "bea", "bea@b.example", "https://drive.b.example/dav/"
				}
				// La marca y la Session siguen en A: el propietario real ya discrepa.
				d.mu.Unlock()
			}
			once.Do(func() { close(liberar) })
			select {
			case err := <-done:
				d.mu.Lock()
				n := len(d.abiertas)
				d.mu.Unlock()
				if err == nil || n != 0 {
					t.Fatal("se abrió una ubicación de otra cuenta después de resolver la credencial")
				}
			case <-time.After(time.Second):
				t.Fatal("no terminó la resolución descartada")
			}
		})
	}
}

func TestCerrarDestinoDescartadoLiberaElProveedorSinAbrir(t *testing.T) {
	d, s, ctx, dueno := prepararDatosApertura(t)
	a, err := PrepararApertura(ctx, dueno, "https://drive.a.example/dav/")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	s.Sincronizar(auth.Tokens{Issuer: dueno.Emisor, Cliente: dueno.Cliente, Subject: "bea", Organizacion: "otra", Access: "synthetic-b", Refresh: "synthetic-b", Expiry: time.Now().Add(time.Hour)})
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	plazo, cancelar := context.WithTimeout(context.Background(), time.Second)
	defer cancelar()
	soltar, err := excluir(plazo)
	if err != nil {
		t.Fatal("descartar un destino sin aplicarlo retuvo el cerrojo del proveedor")
	}
	soltar()
	if err := a.Abrir(ctx); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("un destino descartado se pudo abrir")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.abiertas) != 0 {
		t.Fatal("descartar abrió una ubicación")
	}
}

func TestAperturaDescartaRevisionCambiadaOCancelacionYLiberaCerrojo(t *testing.T) {
	for _, cambio := range []string{"revision-aba", "cancelacion"} {
		t.Run(cambio, func(t *testing.T) {
			d, _, padre, dueno := prepararDatosApertura(t)
			ctx, cancelar := context.WithCancel(padre)
			defer cancelar()
			a, err := PrepararApertura(ctx, dueno, "https://drive.a.example/dav/")
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if cambio == "revision-aba" {
				d.mu.Lock()
				d.acceso.Revision = strings.Repeat("b", 32)
				d.mu.Unlock()
			} else {
				cancelar()
			}
			if err := a.Abrir(ctx); err == nil {
				t.Fatal("una autorización retirada pudo abrir su destino anterior")
			}
			plazo, parar := context.WithTimeout(context.Background(), time.Second)
			defer parar()
			soltar, err := excluir(plazo)
			if err != nil {
				t.Fatal("el fallo de la guarda retuvo el cerrojo del proveedor")
			}
			soltar()
			d.mu.Lock()
			defer d.mu.Unlock()
			if len(d.abiertas) != 0 {
				t.Fatal("se abrió una ubicación después de cancelarla")
			}
		})
	}
}

func TestAperturaConsumeSoloLaURIPreparadaYNoUnaCredencialGlobalNueva(t *testing.T) {
	d, _, ctx, dueno := prepararDatosApertura(t)
	var lecturas atomic.Int32
	original := leer
	leer = func(k string) ([]byte, error) { lecturas.Add(1); return original(k) }
	a, err := PrepararApertura(ctx, dueno, "https://drive.a.example/dav/")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	d.mu.Lock()
	d.cred.Usuario, d.cred.URL = "bea@b.example", "https://drive.b.example/dav/"
	d.mu.Unlock() // Si aplicar volviese a resolver la URL global, obtendría B.
	if err := a.Abrir(ctx); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if lecturas.Load() != 1 || len(d.abiertas) != 1 || d.abiertas[0] != "davs://ana%40a.example@drive.a.example/dav/" {
		t.Fatal("aplicar no consumió exclusivamente el destino capturado de A")
	}
	if err := a.Abrir(ctx); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("se pudo reutilizar una apertura cuyo cerrojo ya se había liberado")
	}
}

func TestAperturaNoPermiteSustituirElMontajeMientrasSeAbreLaMismaURI(t *testing.T) {
	d, _, ctx, dueno := prepararDatosApertura(t)
	iniciada, liberar := make(chan struct{}), make(chan struct{})
	var once sync.Once
	abrirUbicacion = func(context.Context, string) error { close(iniciada); <-liberar; return nil }
	a, err := PrepararApertura(ctx, dueno, "https://drive.a.example/dav/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { once.Do(func() { close(liberar) }); a.Close() }()
	openDone := make(chan error, 1)
	go func() { openDone <- a.Abrir(ctx) }()
	select {
	case <-iniciada:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la apertura")
	}
	plazo, cancelar := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelar()
	soltar, err := excluir(plazo)
	if err == nil {
		soltar()
		t.Fatal("otra organización pudo tomar el cerrojo y sustituir el montaje durante la apertura")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	once.Do(func() { close(liberar) })
	select {
	case err := <-openDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("no terminó la apertura")
	}
	plazoLibre, cancelarLibre := context.WithTimeout(context.Background(), time.Second)
	defer cancelarLibre()
	soltar, err = excluir(plazoLibre)
	if err != nil {
		t.Fatal("el cerrojo no se liberó al acabar")
	}
	d.mu.Lock()
	d.cred.Dueno.Organizacion = "otra"
	d.mu.Unlock()
	soltar()
}
