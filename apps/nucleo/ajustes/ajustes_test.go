// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// aparte manda la carpeta de ajustes a un temporal. `os.UserConfigDir` sale de HOME en
// los dos sistemas donde esto se prueba (en macOS «Library/Application Support», en
// Linux «.config»), así que mover HOME mueve la carpeta entera.
func aparte(t *testing.T) string {
	t.Helper()
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	dir, err := Carpeta()
	if err != nil {
		t.Fatalf("Carpeta: %v", err)
	}
	return dir
}

func TestCambiarYLeerVanYVuelven(t *testing.T) {
	aparte(t)
	if e := Leer(); e != (Estado{Ajustes: Ajustes{Tema: "claro"}}) {
		t.Fatalf("sin nada guardado quiero el tema claro, tengo %+v", e)
	}
	if err := Cambiar(func(a *Ajustes) { a.Tema = "oscuro" }); err != nil {
		t.Fatalf("Cambiar: %v", err)
	}
	// Dos apps distintas tocando cosas distintas: la segunda no pisa a la primera.
	if err := Cambiar(func(a *Ajustes) { a.Idioma = "es-ES" }); err != nil {
		t.Fatalf("Cambiar: %v", err)
	}
	if e := Leer(); e.Tema != "oscuro" || e.Idioma != "es-ES" {
		t.Errorf("Leer = %+v", e)
	}
}

func TestElTemaInicialEsClaroYRespetaElSistemaGuardado(t *testing.T) {
	aparte(t)
	if err := Cambiar(func(a *Ajustes) { a.Idioma = "es-ES" }); err != nil {
		t.Fatal(err)
	}
	if e := Leer(); e.Tema != "claro" {
		t.Fatalf("guardar otro ajuste ha perdido el tema inicial: %+v", e)
	}
	if err := Cambiar(func(a *Ajustes) { a.Tema = "" }); err != nil {
		t.Fatal(err)
	}
	if e := Leer(); e.Tema != "" {
		t.Errorf("la elección guardada del sistema se ha sustituido: %+v", e)
	}
}

// El fichero de ajustes NO es el sitio de la sesión, y esta es la prueba que lo vigila:
// la marca dice que algo cambió, nunca qué. Si alguien mete ahí un token, aquí se ve.
func TestLaMarcaDeSesionNoLlevaLaSesion(t *testing.T) {
	dir := aparte(t)
	antes := Leer().Sesion
	MarcarSesion()
	despues := Leer().Sesion
	if despues == antes || despues == "" {
		t.Fatalf("la marca no ha cambiado: %q → %q", antes, despues)
	}
	if _, err := strconv.ParseInt(despues, 10, 64); err != nil {
		t.Errorf("la marca tiene que ser un número y nada más, es %q", despues)
	}
	// Y nada de lo que hay en la carpeta puede parecerse a una credencial.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		crudo, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, prohibido := range []string{"refresh", "token", "access", "Bearer"} {
			if strings.Contains(strings.ToLower(string(crudo)), strings.ToLower(prohibido)) {
				t.Errorf("%s contiene %q: la sesión no va aquí, va al llavero", e.Name(), prohibido)
			}
		}
	}
}

func TestVigilarAvisaDeLoQueCambiaOtro(t *testing.T) {
	aparte(t)
	ctx, parar := context.WithTimeout(context.Background(), 5*time.Second)
	defer parar()
	visto := make(chan Estado, 4)
	go Vigilar(ctx, 5*time.Millisecond, Leer(), func(e Estado) { visto <- e })

	if err := Cambiar(func(a *Ajustes) { a.Tema = "oscuro" }); err != nil {
		t.Fatalf("Cambiar: %v", err)
	}
	select {
	case e := <-visto:
		if e.Tema != "oscuro" {
			t.Errorf("avisó con %+v", e)
		}
	case <-ctx.Done():
		t.Fatal("nadie avisó del cambio de tema")
	}

	MarcarSesion()
	select {
	case e := <-visto:
		if e.Sesion == "" {
			t.Errorf("avisó sin marca: %+v", e)
		}
	case <-ctx.Done():
		t.Fatal("nadie avisó del cambio de sesión")
	}
}

func TestCerrojoNoDejaEntrarADos(t *testing.T) {
	aparte(t)
	soltar, err := Cerrojo(context.Background())
	if err != nil {
		t.Fatalf("Cerrojo: %v", err)
	}
	ctx, parar := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer parar()
	if _, err := Cerrojo(ctx); err == nil {
		t.Error("el segundo cerrojo ha entrado con el primero puesto")
	}
	soltar()
	segundo, err := Cerrojo(context.Background())
	if err != nil {
		t.Fatalf("tras soltar, Cerrojo: %v", err)
	}
	segundo()
}

// Un cerrojo de un proceso que murió no puede dejar la suite sin poder renovar nunca.
func TestCerrojoSinDuenoNoBloquea(t *testing.T) {
	dir := aparte(t)
	ruta := filepath.Join(dir, ficheroCerrojo)
	if err := os.WriteFile(ruta, []byte("999999"), permisoFichero); err != nil {
		t.Fatal(err)
	}
	viejo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(ruta, viejo, viejo); err != nil {
		t.Fatal(err)
	}
	ctx, parar := context.WithTimeout(context.Background(), time.Second)
	defer parar()
	soltar, err := Cerrojo(ctx)
	if err != nil {
		t.Fatalf("un fichero sin bloqueo del sistema no debe bloquear: %v", err)
	}
	soltar()
}

// El guardián que de verdad importa: TRECE PROCESOS de verdad, no goroutines. Cada uno
// hace leer-sumar-escribir bajo el cerrojo, que es la misma forma que tiene renovar el
// refresh token (auth.Compartida). Sin cerrojo se pierden incrementos y el contador
// final sale por debajo; se ha visto en rojo quitando el Cerrojo del ayudante de abajo:
// «el contador acabó en 1 y tendría que ser 13».
func TestTreceProcesosNoSePisan(t *testing.T) {
	if testing.Short() {
		t.Skip("lanza trece procesos")
	}
	dir := aparte(t)
	cuenta := filepath.Join(dir, "cuenta")
	if err := os.WriteFile(cuenta, []byte("0"), permisoFichero); err != nil {
		t.Fatal(err)
	}

	const apps = 13
	var espera sync.WaitGroup
	fallos := make(chan error, apps)
	for range apps {
		espera.Add(1)
		go func() {
			defer espera.Done()
			// `t.Setenv` ya ha puesto HOME y XDG_CONFIG_HOME en el entorno de ESTE
			// proceso, así que el hijo hereda la misma carpeta temporal.
			cmd := exec.Command(os.Args[0], "-test.run=TestAyudanteSuma", "-test.v=false")
			cmd.Env = append(os.Environ(), "SYTUE_AJUSTES_AYUDANTE=1")
			if salida, err := cmd.CombinedOutput(); err != nil {
				fallos <- err
				t.Logf("ayudante: %s", salida)
			}
		}()
	}
	espera.Wait()
	close(fallos)
	for err := range fallos {
		t.Errorf("un ayudante falló: %v", err)
	}

	crudo, err := os.ReadFile(cuenta)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := strconv.Atoi(string(crudo)); n != apps {
		t.Errorf("el contador acabó en %s y tendría que ser %d: se han pisado", crudo, apps)
	}
}

// TestAyudanteSuma no es una prueba: es el otro proceso de TestTreceProcesosNoSePisan.
// Sin la variable de entorno no hace nada, para que el `go test` normal no lo ejecute.
func TestAyudanteSuma(t *testing.T) {
	if os.Getenv("SYTUE_AJUSTES_AYUDANTE") == "" {
		t.Skip("solo lo lanza TestTreceProcesosNoSePisan")
	}
	dir, err := Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
	defer parar()
	soltar, err := Cerrojo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	ruta := filepath.Join(dir, "cuenta")
	crudo, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(string(crudo))
	// La pausa es el hueco por el que se cuela el otro proceso si no hay cerrojo: sin
	// ella la carrera existe igual pero casi nunca se ve, y un guardián que solo falla
	// a veces no guarda nada.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(ruta, []byte(strconv.Itoa(n+1)), permisoFichero); err != nil {
		t.Fatal(err)
	}
}

// El fichero se lee entero o no se lee: quien vigila no puede pillar un JSON a medias.
func TestEscribirEnteroNoDejaMediasTintas(t *testing.T) {
	dir := aparte(t)
	ruta := filepath.Join(dir, "prueba.json")
	if err := EscribirEntero(ruta, []byte(`{"tema":"oscuro"}`)); err != nil {
		t.Fatal(err)
	}
	if err := EscribirEntero(ruta, []byte(`{"tema":"claro"}`)); err != nil {
		t.Fatal(err)
	}
	crudo, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	var a Ajustes
	if err := json.Unmarshal(crudo, &a); err != nil || a.Tema != "claro" {
		t.Errorf("quedó %q (%v)", crudo, err)
	}
	// Y no deja restos: el temporal se borra pase lo que pase.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		if strings.HasPrefix(e.Name(), "prueba.json.") {
			t.Errorf("ha quedado un temporal: %s", e.Name())
		}
	}
}

func TestCerrojoVivoNoCaducaPorSuEdad(t *testing.T) {
	dir := aparte(t)
	soltar, err := Cerrojo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	viejo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, ficheroCerrojo), viejo, viejo); err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancelar()
	if liberar, err := Cerrojo(ctx); err == nil {
		liberar()
		t.Fatal("un propietario vivo perdió la exclusión por una fecha antigua")
	}
}

func TestCerrojoSeLiberaAlMorirElPropietario(t *testing.T) {
	dir := aparte(t)
	listo := filepath.Join(dir, "propietario-listo")
	cmd := exec.Command(os.Args[0], "-test.run=TestAyudanteMantieneCerrojo", "-test.v=false")
	cmd.Env = append(os.Environ(), "SYTUE_AJUSTES_MANTENER="+listo)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	for {
		if _, err := os.Stat(listo); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("el propietario no cogió el cerrojo")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	soltar, err := Cerrojo(ctx)
	if err != nil {
		t.Fatalf("el sistema no liberó el bloqueo al morir su propietario: %v", err)
	}
	soltar()
}

func TestAyudanteMantieneCerrojo(t *testing.T) {
	listo := os.Getenv("SYTUE_AJUSTES_MANTENER")
	if listo == "" {
		t.Skip("sólo lo lanza TestCerrojoSeLiberaAlMorirElPropietario")
	}
	soltar, err := Cerrojo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	if err := os.WriteFile(listo, []byte("1"), permisoFichero); err != nil {
		t.Fatal(err)
	}
	<-time.After(time.Minute)
}

func TestMigrarPreferenciasConservaLasDelGrupoSinCopiarSesion(t *testing.T) {
	antes := t.TempDir()
	despues := t.TempDir()
	carpeta := filepath.Join(antes, "sytue")
	if err := os.MkdirAll(carpeta, permisoCarpeta); err != nil {
		t.Fatal(err)
	}
	for nombre, datos := range map[string]string{
		ficheroAjustes: `{"tema":"oscuro","bloqueo":true,"secreto_legacy":"no-se-comparte"}`,
		ficheroSesion:  "marca-antigua", ficheroCerrojo: "pid-antiguo",
	} {
		if err := os.WriteFile(filepath.Join(carpeta, nombre), []byte(datos), permisoFichero); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrarPreferencias(despues, antes); err != nil {
		t.Fatal(err)
	}
	crudo, err := os.ReadFile(filepath.Join(despues, "sytue", ficheroAjustes))
	if err != nil || string(crudo) != `{"tema":"oscuro","bloqueo":true}` {
		t.Fatal("la actualización perdió los ajustes anteriores")
	}
	for _, nombre := range []string{ficheroSesion, ficheroCerrojo} {
		if _, err := os.Stat(filepath.Join(despues, "sytue", nombre)); !os.IsNotExist(err) {
			t.Fatal("se copió una marca o un cerrojo anterior")
		}
	}
	if err := os.WriteFile(filepath.Join(despues, "sytue", ficheroAjustes), []byte(`{"tema":"claro"}`), permisoFichero); err != nil {
		t.Fatal(err)
	}
	if err := migrarPreferencias(despues, antes); err != nil {
		t.Fatal(err)
	}
	crudo, _ = os.ReadFile(filepath.Join(despues, "sytue", ficheroAjustes))
	if string(crudo) != `{"tema":"claro"}` {
		t.Fatal("la importación sobrescribió los ajustes actuales")
	}
}
