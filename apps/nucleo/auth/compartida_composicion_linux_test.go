// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// El CI usa XDG_CONFIG_HOME para aislar el cerrojo real de ajustes. El almacén y
// el IdP son falsos: estas pruebas no leen el llavero ni ninguna sesión del equipo.
type almacenDeComposicion struct {
	mu     sync.Mutex
	tokens Tokens
}

func (a *almacenDeComposicion) cargar() (Tokens, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tokens, nil
}

func (a *almacenDeComposicion) guardar(tokens Tokens) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = tokens
	return nil
}

// detenerTrasCompartida fuerza la pausa después de liberar el cerrojo entre
// procesos, antes de que AccessToken procese el resultado. No depende del azar del
// planificador ni de sleeps: la otra app termina su operación durante esa pausa.
func detenerTrasCompartida(t *testing.T, renovar Renewer) (Renewer, <-chan struct{}, func()) {
	t.Helper()
	pausada, continuar := make(chan struct{}), make(chan struct{})
	reanudar := sync.OnceFunc(func() { close(continuar) })
	t.Cleanup(reanudar)
	return func(ctx context.Context, actuales Tokens) (Tokens, error) {
		nuevos, err := renovar(ctx, actuales)
		close(pausada)
		select {
		case <-continuar:
			return nuevos, err
		case <-ctx.Done():
			return Tokens{}, ctx.Err()
		}
	}, pausada, reanudar
}

func esperarComposicion(ctx context.Context, t *testing.T, lista <-chan struct{}) {
	t.Helper()
	select {
	case <-lista:
	case <-ctx.Done():
		t.Fatal("la operación de la primera app no ha alcanzado la pausa")
	}
}

func resultadoComposicion(ctx context.Context, t *testing.T, resultado <-chan error) error {
	t.Helper()
	select {
	case err := <-resultado:
		return err
	case <-ctx.Done():
		t.Fatal("la primera app no ha terminado")
		return ctx.Err()
	}
}

func TestSessionCompartidaNoSobrescribeUnaRotacionPosterior(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	almacen := &almacenDeComposicion{tokens: Tokens{Refresh: "r0", Subject: "persona"}}
	vigente, rotaciones, revocada := "r0", 0, false
	idp := func(_ context.Context, actuales Tokens) (Tokens, error) {
		if revocada || actuales.Refresh != vigente {
			revocada = true // igual que el IdP al reutilizar un refresh ya gastado
			return Tokens{}, ErrLoginRequired
		}
		rotaciones++
		vigente = fmt.Sprintf("r%d", rotaciones)
		return Tokens{Access: fmt.Sprintf("a%d", rotaciones), Refresh: vigente, Expiry: time.Now().Add(time.Hour)}, nil
	}
	renovarA, pausada, reanudar := detenerTrasCompartida(t, Compartida(idp, almacen.cargar, almacen.guardar))
	appA := NewSession(almacen.tokens, renovarA, almacen.guardar)
	appB := NewSession(almacen.tokens, Compartida(idp, almacen.cargar, almacen.guardar), almacen.guardar)
	resultado := make(chan error, 1)
	go func() { _, err := appA.AccessToken(ctx); resultado <- err }()
	esperarComposicion(ctx, t, pausada)
	if _, err := appB.AccessToken(ctx); err != nil {
		t.Fatalf("la segunda app no ha renovado: %v", err)
	}
	reanudar()
	if err := resultadoComposicion(ctx, t, resultado); err != nil {
		t.Fatalf("la primera app no ha renovado: %v", err)
	}
	if guardados, _ := almacen.cargar(); guardados.Refresh != "r2" {
		t.Error("la escritura tardía de la primera app ha pisado la rotación de la segunda")
	}
	appB.Invalidate("a2")
	if _, err := appB.AccessToken(ctx); err != nil {
		t.Errorf("la siguiente renovación ha perdido la sesión compartida: %v", err)
	}
	if revocada {
		t.Error("se ha reutilizado un refresh gastado y se ha revocado la familia")
	}
}

func TestSessionCompartidaRechazadaNoBorraUnLoginPosterior(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	almacen := &almacenDeComposicion{tokens: Tokens{Refresh: "caducado", Subject: "anterior"}}
	idp := func(context.Context, Tokens) (Tokens, error) {
		// Una respuesta de error nunca debe adoptar estos datos parciales.
		return Tokens{Access: "parcial", Refresh: "parcial", Subject: "parcial"}, ErrLoginRequired
	}
	renovarA, pausada, reanudar := detenerTrasCompartida(t, Compartida(idp, almacen.cargar, almacen.guardar))
	appA := NewSession(almacen.tokens, renovarA, almacen.guardar)
	appB := NewSession(Tokens{}, nil, almacen.guardar)
	resultado := make(chan error, 1)
	go func() { _, err := appA.AccessToken(ctx); resultado <- err }()
	esperarComposicion(ctx, t, pausada)
	appB.Adoptar(Tokens{Access: "nuevo", Refresh: "login-nuevo", Subject: "nueva", Expiry: time.Now().Add(time.Hour)})
	reanudar()
	if err := resultadoComposicion(ctx, t, resultado); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("la primera app debe pedir login: %v", err)
	}
	if guardados, _ := almacen.cargar(); guardados.Refresh != "login-nuevo" || guardados.Subject != "nueva" {
		t.Error("el rechazo antiguo ha borrado el login posterior de otra app")
	}
	if appA.Subject() != "" {
		t.Error("el rechazo ha conservado identidad parcial en la primera app")
	}
	appB.Logout()
	if guardados, _ := almacen.cargar(); guardados != (Tokens{}) {
		t.Error("Logout ha dejado una sesión guardada")
	}
}

func TestSessionCompartidaAvisaUnFalloDeGuardadoSinRepetirlo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, rechazo := range []bool{false, true} {
		t.Run(fmt.Sprintf("rechazo=%t", rechazo), func(t *testing.T) {
			comprobarFalloDeGuardado(t, rechazo)
		})
	}
}

func comprobarFalloDeGuardado(t *testing.T, rechazo bool) {
	t.Helper()
	var avisos bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&avisos, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })
	intentos := 0
	guardar := func(Tokens) error { intentos++; return errors.New("almacén de prueba bloqueado") }
	idp := func(context.Context, Tokens) (Tokens, error) {
		if rechazo {
			return Tokens{Access: "parcial", Refresh: "parcial", Subject: "parcial"}, ErrLoginRequired
		}
		return Tokens{Access: "util-en-memoria", Refresh: "rotado", Expiry: time.Now().Add(time.Hour)}, nil
	}
	sesion := NewSession(Tokens{Refresh: "anterior"}, Compartida(idp,
		func() (Tokens, error) { return Tokens{Refresh: "anterior"}, nil }, guardar), guardar)
	if rechazo {
		if token, err := sesion.AccessToken(context.Background()); token != "" || !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("un rechazo ha dejado un token parcial: %v", err)
		}
		if sesion.tokens != (Tokens{}) {
			t.Error("un rechazo ha dejado datos parciales o la marca en memoria")
		}
	} else {
		for range 2 {
			if token, err := sesion.AccessToken(context.Background()); err != nil || token != "util-en-memoria" {
				t.Fatalf("un fallo de persistencia ha invalidado el token en memoria: %v", err)
			}
		}
	}
	if intentos != 1 {
		t.Errorf("%d intentos de guardar; el fallo bajo el cerrojo no debe reintentarse fuera", intentos)
	}
	if n := strings.Count(avisos.String(), "auth: no se pudo guardar la sesión"); n != 1 {
		t.Errorf("%d avisos de persistencia; quiero uno visible", n)
	}
}

func TestSessionCompartidaSinGuardarInteriorUsaElExterior(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	intentos := 0
	guardar := func(tokens Tokens) error {
		intentos++
		if tokens.Refresh != "rotado" || tokens.persistenciaAtendida {
			t.Error("el guardado exterior no ha recibido la renovación limpia")
		}
		return nil
	}
	idp := func(_ context.Context, actuales Tokens) (Tokens, error) {
		if actuales.persistenciaAtendida {
			t.Error("una marca antigua ha llegado al renovador")
		}
		return Tokens{Access: "acceso", Refresh: "rotado", Expiry: time.Now().Add(time.Hour), persistenciaAtendida: true}, nil
	}
	sesion := NewSession(Tokens{Refresh: "anterior", persistenciaAtendida: true}, Compartida(idp,
		func() (Tokens, error) { return Tokens{Refresh: "anterior"}, nil }, nil), guardar)
	if _, err := sesion.AccessToken(context.Background()); err != nil {
		t.Fatalf("renovar sin guardado interior: %v", err)
	}
	if intentos != 1 || sesion.tokens.persistenciaAtendida {
		t.Error("guardar=nil en Compartida ha suprimido el guardado exterior o ha conservado la marca")
	}
}

func TestSessionConsumeLaMarcaYAdoptarSiempreGuarda(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	intentos := 0
	var almacen Tokens
	guardar := func(tokens Tokens) error {
		intentos++
		if tokens.persistenciaAtendida {
			t.Error("la marca de renovación ha llegado al almacén")
		}
		almacen = tokens
		return nil
	}
	renovar := Compartida(func(context.Context, Tokens) (Tokens, error) {
		return Tokens{Access: "acceso", Refresh: "rotado", Subject: "persona", Expiry: time.Now().Add(time.Hour)}, nil
	}, func() (Tokens, error) { return Tokens{Refresh: "anterior"}, nil }, guardar)
	nuevos, err := renovar(context.Background(), Tokens{})
	if err != nil || !nuevos.persistenciaAtendida || intentos != 1 {
		t.Fatalf("la renovación compartida no ha atendido su persistencia: %v", err)
	}
	crudo, err := serializar(nuevos)
	if err != nil {
		t.Fatal(err)
	}
	recuperados, err := recuperar(crudo)
	if err != nil || recuperados.persistenciaAtendida {
		t.Fatal("la marca se ha serializado")
	}
	sesion := NewSession(nuevos, nil, guardar)
	if sesion.tokens.persistenciaAtendida {
		t.Error("NewSession ha conservado una marca de otra renovación")
	}
	sesion.Adoptar(nuevos)
	if intentos != 2 || sesion.tokens.persistenciaAtendida || almacen.Refresh != "rotado" {
		t.Error("Adoptar no ha guardado los tokens reenviados o ha conservado su marca")
	}
	sesion.Logout()
	if intentos != 3 || sesion.tokens != (Tokens{}) || almacen != (Tokens{}) {
		t.Error("Logout no ha limpiado y guardado la sesión")
	}
}

func TestSessionCompartidaNoVuelveABorrarUnAlmacenVacio(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	intentos := 0
	guardar := func(Tokens) error { intentos++; return nil }
	renovar := Compartida(func(context.Context, Tokens) (Tokens, error) {
		t.Fatal("sin refresh guardado no se debe llamar al IdP")
		return Tokens{}, nil
	}, func() (Tokens, error) { return Tokens{}, nil }, guardar)
	sesion := NewSession(Tokens{Refresh: "viejo", Subject: "persona"}, renovar, guardar)
	if _, err := sesion.AccessToken(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("sin refresh guardado debe pedir login: %v", err)
	}
	if intentos != 0 || sesion.tokens != (Tokens{}) {
		t.Error("el almacén vacío se ha vuelto a borrar o ha quedado estado en memoria")
	}
}

func TestLoginPosteriorEsperaLaRenovacionYNoSePierde(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	almacen := &almacenDeComposicion{tokens: Tokens{Refresh: "r0", Subject: "anterior"}}
	pidiendo, continuar := make(chan struct{}), make(chan struct{})
	reanudar := sync.OnceFunc(func() { close(continuar) })
	defer reanudar()
	idp := func(context.Context, Tokens) (Tokens, error) {
		close(pidiendo)
		select {
		case <-continuar:
			return Tokens{Access: "a1", Refresh: "r1", Subject: "anterior", Expiry: time.Now().Add(time.Hour)}, nil
		case <-ctx.Done():
			return Tokens{}, ctx.Err()
		}
	}
	appA := NewSession(almacen.tokens, Compartida(idp, almacen.cargar, almacen.guardar), almacen.guardar)
	loginGuardado := make(chan struct{})
	appB := NewSession(Tokens{}, nil, func(tokens Tokens) error {
		err := almacen.guardar(tokens)
		close(loginGuardado)
		return err
	})
	resultado := make(chan error, 1)
	go func() { _, err := appA.AccessToken(ctx); resultado <- err }()
	esperarComposicion(ctx, t, pidiendo)
	go appB.Adoptar(Tokens{Refresh: "login-nuevo", Subject: "nueva"})
	select {
	case <-loginGuardado:
		t.Fatal("el login se guardó durante la renovación de la cuenta anterior")
	case <-time.After(100 * time.Millisecond):
	}
	reanudar()
	if err := resultadoComposicion(ctx, t, resultado); err != nil {
		t.Fatal(err)
	}
	esperarComposicion(ctx, t, loginGuardado)
	if tokens, _ := almacen.cargar(); tokens.Refresh != "login-nuevo" || tokens.Subject != "nueva" {
		t.Fatal("la renovación anterior ha sobrescrito el login nuevo")
	}
}
