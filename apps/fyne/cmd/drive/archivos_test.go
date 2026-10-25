// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// doble es el sistema y Sytue ID de mentira, con lo que se ha hecho en orden.
type doble struct {
	hechos     []string
	credencial *sistema.CredencialArchivos
	id, de     string // la apuntada
	sujeto     string
	fallaCrear bool
	creadas    int
}

func (d *doble) archivos() *archivos {
	return &archivos{
		hay: func() (bool, error) { return d.credencial != nil, nil },
		guardar: func(c sistema.CredencialArchivos) error {
			d.hechos = append(d.hechos, "guardar "+c.Usuario+" "+c.URL)
			d.credencial = &c
			return nil
		},
		registrar: func(n string) error { d.hechos = append(d.hechos, "registrar "+n); return nil },
		olvidar: func() error {
			d.hechos = append(d.hechos, "olvidar")
			d.credencial = nil
			return nil
		},
		sujeto:   func() string { return d.sujeto },
		apuntada: func() (string, string, error) { return d.id, d.de, nil },
		apuntar: func(id, de string) error {
			d.hechos = append(d.hechos, "apuntar "+id+" "+de)
			d.id, d.de = id, de
			return nil
		},
		revocar: func(context.Context) error {
			if d.id != "" {
				d.hechos = append(d.hechos, "revocar "+d.id)
				d.id, d.de = "", ""
			}
			return nil
		},
		crear: func(_ context.Context, etiqueta string) (api.AppPasswordNueva, error) {
			if d.fallaCrear {
				return api.AppPasswordNueva{}, errors.New("sin red")
			}
			if !strings.HasPrefix(etiqueta, "Archivos · ") {
				d.hechos = append(d.hechos, "etiqueta rara "+etiqueta)
			}
			d.creadas++
			return api.AppPasswordNueva{AppPassword: api.AppPassword{ID: "p" + string(rune('0'+d.creadas))}, Password: "clave"}, nil
		},
		webdav: func(context.Context) (string, error) { return "https://drive.sytue.eu/dav", nil },
		correo: func(context.Context) (string, error) { return "ana@sytue.eu", nil },
	}
}

func TestArchivosSigueLaSesion(t *testing.T) {
	d := &doble{sujeto: "ana"}
	a := d.archivos()

	// Al entrar: la contraseña, apuntada ANTES de guardarla, y el dominio.
	a.seguir(true)
	quiero := []string{"apuntar p1 ana", "guardar ana@sytue.eu https://drive.sytue.eu/dav", "registrar Sytue Drive"}
	if !slices.Equal(d.hechos, quiero) {
		t.Fatalf("al entrar: %q, quiero %q", d.hechos, quiero)
	}
	// Otro aviso de la misma sesión (cambiar el tema) no hace nada.
	d.hechos = nil
	a.seguir(true)
	if len(d.hechos) != 0 {
		t.Fatalf("un aviso repetido ha hecho %q", d.hechos)
	}
	// Al salir: fuera el dominio y la credencial (la revocación es de sesion.Salir).
	a.seguir(false)
	if !slices.Equal(d.hechos, []string{"olvidar"}) || d.credencial != nil {
		t.Fatalf("al salir: %q", d.hechos)
	}
}

// Ana salió desde otra app —que no ve el llavero de Drive— y entra Berta: la credencial
// de Ana se tira y la apuntada se revoca antes de crear la de Berta.
func TestArchivosNoHeredaLaDeOtraPersona(t *testing.T) {
	d := &doble{sujeto: "berta", credencial: &sistema.CredencialArchivos{Usuario: "ana@sytue.eu"}, id: "p9", de: "ana"}
	d.archivos().seguir(true)
	quiero := []string{"olvidar", "revocar p9", "apuntar p1 berta", "guardar ana@sytue.eu https://drive.sytue.eu/dav", "registrar Sytue Drive"}
	if !slices.Equal(d.hechos, quiero) {
		t.Fatalf("%q, quiero %q", d.hechos, quiero)
	}
}

// Con la credencial ya puesta y de quien entra, solo se vuelve a dar de alta el dominio
// (tras reinstalar la app no está); y un fallo no se da por hecho: se reintenta.
func TestArchivosReintenta(t *testing.T) {
	d := &doble{sujeto: "ana", fallaCrear: true}
	a := d.archivos()
	a.seguir(true)
	if d.credencial != nil || slices.Contains(d.hechos, "registrar Sytue Drive") {
		t.Fatalf("sin red ha seguido: %q", d.hechos)
	}
	d.fallaCrear, d.hechos = false, nil
	a.seguir(true)
	if d.credencial == nil || d.creadas != 1 {
		t.Fatalf("no ha reintentado: %q", d.hechos)
	}
	d.hechos = nil
	b := d.archivos() // otro arranque de la app
	b.seguir(true)
	if !slices.Equal(d.hechos, []string{"registrar Sytue Drive"}) {
		t.Fatalf("con la credencial puesta: %q", d.hechos)
	}
}

// Desactivar retira el dominio y revoca la credencial; no la recrea al cambiar tema.
func TestArchivosDesactivadosNoSeRecrean(t *testing.T) {
	d := &doble{sujeto: "ana"}
	a := d.archivos()
	habilitado := true
	a.habilitado = func() bool { return habilitado }
	a.seguir(true)
	d.hechos = nil
	habilitado = false
	a.seguir(true)
	if !slices.Equal(d.hechos, []string{"olvidar", "revocar p1"}) || d.credencial != nil {
		t.Fatalf("desactivar: %q", d.hechos)
	}
	d.hechos = nil
	a.seguir(true)
	if len(d.hechos) != 0 {
		t.Fatalf("recreó estando desactivado: %q", d.hechos)
	}
	habilitado = true
	a.seguir(true)
	if d.creadas != 2 || d.credencial == nil {
		t.Fatalf("reactivar: %q", d.hechos)
	}
}

func TestArchivosAvisoAntiguoNoReabreSesionCerrada(t *testing.T) {
	d := &doble{sujeto: "ana"}
	a := d.archivos()
	a.dentro = func() bool { return false }
	a.seguir(true)
	if d.creadas != 0 || d.credencial != nil {
		t.Fatalf("reabrió: %q", d.hechos)
	}
}

func TestArchivosNoPublicaClaveSinOrganizacionVerificada(t *testing.T) {
	for _, tenant := range []string{"", "otra"} {
		d := &doble{sujeto: "ana"}
		a := d.archivos()
		a.organizacion = func() string { return "acme" }
		crear := a.crear
		a.crear = func(ctx context.Context, etiqueta string) (api.AppPasswordNueva, error) {
			clave, err := crear(ctx, etiqueta)
			clave.OrgID, clave.Tenant = "org-1", tenant
			return clave, err
		}
		a.seguir(true)
		if d.credencial != nil || slices.Contains(d.hechos, "registrar Sytue Drive") || d.id != "" {
			t.Fatalf("tenant %q publicó credencial: %q", tenant, d.hechos)
		}
	}
}

func TestAbrirArchivosEsperaElAltaEnVuelo(t *testing.T) {
	d := &doble{sujeto: "ana"}
	a := d.archivos()
	registrado, continuar, abierto := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	a.registrar = func(string) error { close(registrado); <-continuar; return nil }
	alta := make(chan error, 1)
	go func() { alta <- a.seguir(true) }()
	<-registrado
	resultado := make(chan error, 1)
	go func() { resultado <- a.abrir(func() error { abierto <- struct{}{}; return nil }) }()
	select {
	case <-abierto:
		t.Fatal("se abrió Finder/Archivos antes de terminar el alta")
	case <-time.After(30 * time.Millisecond):
	}
	close(continuar)
	if err := <-alta; err != nil {
		t.Fatal(err)
	}
	if err := <-resultado; err != nil {
		t.Fatal(err)
	}
	if d.creadas != 1 {
		t.Fatalf("el enlace duplicó credenciales: %d", d.creadas)
	}
	select {
	case <-abierto:
	default:
		t.Fatal("no se abrió tras el alta")
	}
}

func TestAbrirArchivosNoOcultaUnFalloDelAlta(t *testing.T) {
	d := &doble{sujeto: "ana", fallaCrear: true}
	abierto := false
	err := d.archivos().abrir(func() error { abierto = true; return nil })
	if err == nil || abierto {
		t.Fatal("un alta fallida abrió una ubicación que no existe")
	}
}
