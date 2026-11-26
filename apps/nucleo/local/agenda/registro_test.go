// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// El alta de una cuenta DAV (la función que usa Ajustes → Cuentas): prueba cada
// servicio con la contraseña, guarda la cuenta sin el secreto en el JSON y el secreto
// aparte; luego Calendar y Contacts sincronizan con ella.
func TestAltaDAVYSincronizacionDeLasDosApps(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	srv.Crear(davprueba.CasaCal+"personal/", "Personal", false)
	srv.Crear(davprueba.CasaCard+"libreta/", "Libreta", false)
	srv.Poner(davprueba.CasaCal+"personal/a.ics", []byte(ics("a", "Cena", time.Now().Add(48*time.Hour), "")))
	srv.Poner(davprueba.CasaCard+"libreta/ana.vcf", []byte(vcf("ana", "Ana")))

	ajustes := t.TempDir()
	sec := secretos.SoloFichero(filepath.Join(ajustes, "secretos"))
	r := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), sec, nil)
	r.HTTP = func(red.Proposito) *http.Client { return srv.Client() }

	// Contraseña mala: no se guarda nada.
	if _, err := r.AnadirDAV(t.Context(), agenda.Alta{Usuario: srv.Usuario, Clave: "mala", Calendario: srv.URL + "/cal/", Contactos: srv.URL + "/card/"}); !errors.Is(err, agenda.ErrAcceso) && err == nil {
		t.Fatal("se dio de alta con una contraseña mala")
	}
	if len(r.Cuentas()) != 0 {
		t.Fatal("quedó una cuenta a medias")
	}
	c, err := r.AnadirDAV(t.Context(), agenda.Alta{Rotulo: "Nextcloud", Usuario: srv.Usuario, Clave: srv.Clave, Calendario: srv.URL + "/cal/", Contactos: srv.URL + "/card/"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Da(agenda.Calendario) || !c.Da(agenda.Contactos) || len(c.Servidores) < 2 {
		t.Fatalf("cuenta: %+v", c)
	}
	crudo, _ := os.ReadFile(filepath.Join(ajustes, "agenda.json"))
	if strings.Contains(string(crudo), srv.Clave) {
		t.Fatal("la contraseña está en el JSON")
	}

	// Otra app abre el mismo fichero y ve la cuenta.
	r2 := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), sec, nil)
	r2.HTTP = r.HTTP
	if len(r2.Cuentas()) != 1 {
		t.Fatal("la otra app no ve la cuenta")
	}

	cal := abrirCalendario(t, t.TempDir())
	lc := agenda.LocalCalendario{Repo: cal}
	res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, filepath.Join(t.TempDir(), "agenda"), "(copia)")
	if len(errs) > 0 || len(res) != 1 || res[0].Bajados != 1 {
		t.Fatalf("calendar: %+v %v", res, errs)
	}
	con := abrirContactos(t, t.TempDir())
	dirC := filepath.Join(t.TempDir(), "agenda")
	res, errs = r2.SincronizarTodo(t.Context(), agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC, "(copia)")
	if len(errs) > 0 || len(res) != 1 || res[0].Bajados != 1 {
		t.Fatalf("contacts: %+v %v", res, errs)
	}
	gs := r2.Grupos(agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC)
	if len(gs) != 1 || gs[0].Cuenta.Rotulo != "Nextcloud" || len(gs[0].Colecciones) != 1 {
		t.Fatalf("grupos: %+v", gs)
	}

	// La contraseña cambia en el servidor: la cuenta pide volver a entrar.
	srv.Clave = "otra"
	_, errs = r2.SincronizarTodo(t.Context(), agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC, "(copia)")
	if len(errs) != 1 || r2.Cuentas()[0].Estado != agenda.EstadoReconectar {
		t.Fatalf("reconectar: %v %+v", errs, r2.Cuentas())
	}
	// Volver a entrar con la contraseña nueva: la misma cuenta (mismo id), ya sin aviso,
	// y la siguiente sincronización va bien. Con una mala, sigue pidiéndolo.
	if err := r2.Reconectar(t.Context(), c.ID, "mala", nil); err == nil {
		t.Fatal("volvió a entrar con una contraseña mala")
	}
	if err := r2.Reconectar(t.Context(), c.ID, "otra", nil); err != nil {
		t.Fatal(err)
	}
	if cs := r.Cuentas(); len(cs) != 1 || cs[0].ID != c.ID || cs[0].Estado != agenda.EstadoOK {
		t.Fatalf("tras volver a entrar: %+v", cs)
	}
	if _, errs = r2.SincronizarTodo(t.Context(), agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC, "(copia)"); len(errs) != 0 {
		t.Fatalf("tras volver a entrar: %v", errs)
	}

	if err := r.Quitar(c.ID); err != nil || len(r.Cuentas()) != 0 {
		t.Fatalf("quitar: %v", err)
	}
	if v, _ := sec.Leer("cuenta-" + c.ID + "-contrasena"); v != "" {
		t.Fatal("el secreto se quedó")
	}
}

func TestPreajusteDesconocidoYSinServicio(t *testing.T) {
	r := agenda.AbrirEn(filepath.Join(t.TempDir(), "agenda.json"), secretos.SoloFichero(t.TempDir()), nil)
	if _, err := r.Probar(context.Background(), agenda.Alta{Proveedor: "nadie"}); err == nil {
		t.Fatal("aceptó un proveedor desconocido")
	}
}

// El descubrimiento de CalDAV y CardDAV pregunta los SRV y TXT del dominio al DNS del
// portero, no al del sistema: con «Sin conexión» no sale ni una consulta (ni la petición
// a `.well-known`), y el portero las ve todas.
func TestElDescubrimientoPreguntaAlDNSPorElPortero(t *testing.T) {
	ajustes := t.TempDir()
	p := red.Abrir("")
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	var intentos []red.Intento
	t.Cleanup(red.VigilarDePrueba(func(i red.Intento) { intentos = append(intentos, i) }))
	r := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), p)
	if _, err := r.Descubrir(t.Context(), "ana@casa.eu"); err == nil {
		t.Fatal("ha descubierto algo con «Sin conexión»")
	}
	dns := 0
	for _, i := range intentos {
		if i.Proposito != red.Descubrimiento {
			t.Errorf("intento con otro propósito: %+v", i)
		}
		if i.Red == "dns" {
			dns++
		}
	}
	if dns == 0 {
		t.Errorf("las consultas DNS no han pasado por el portero: %+v", intentos)
	}
}

func TestMotorNoSeSolapa(t *testing.T) {
	var corriendo, maximo, vueltas atomic.Int32
	m := &agenda.Motor{Cada: time.Hour, Hacer: func(context.Context) {
		n := corriendo.Add(1)
		if n > maximo.Load() {
			maximo.Store(n)
		}
		time.Sleep(20 * time.Millisecond)
		corriendo.Add(-1)
		vueltas.Add(1)
	}}
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	m.Arrancar(ctx)
	for range 5 {
		m.Ya()
	}
	time.Sleep(150 * time.Millisecond)
	if maximo.Load() != 1 || vueltas.Load() < 2 {
		t.Fatalf("máximo a la vez %d, vueltas %d", maximo.Load(), vueltas.Load())
	}
}
