// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// montajeDrive es lo que hace «Conectar Drive en este equipo». En Windows, la carpeta
// de archivos a petición (`nube`, Cloud Files), que guarda lo que se fija para usarlo sin
// conexión; la atiende el Hub, que es la app que se queda en la bandeja. Es una por
// cuenta y organización (FYS-04): la de quien está dentro, y al cambiar de organización o
// de cuenta, o al salir, la de antes se para; al volver, se reanuda. En macOS y Linux, el
// montaje WebDAV del explorador de ficheros.
func montajeDrive(s *sesion.Suite) func(direccion string) error {
	if runtime.GOOS != "windows" {
		return escritorio.MontarWebDAV
	}
	c := &conexionNube{
		cuenta:    func() nube.Cuenta { return nube.Cuenta{Sub: s.Sesion.Subject(), Org: s.Sesion.Organizacion()} },
		fuente:    func(cu nube.Cuenta) nube.Fuente { return api.NewDrive(s.URL("drive"), nube.Atada(s.Sesion, cu), nil) },
		conectar:  nube.Conectar,
		conectada: nube.Conectada,
		carpeta:   nube.Carpeta,
	}
	// Fuera del hilo de la interfaz: parar la de antes espera a que Windows la suelte.
	s.AlCambiar(func(sesion.Estado) { go c.seguir() })
	go c.seguir()
	return func(string) error {
		dir, err := c.abrir()
		if err != nil {
			return err
		}
		return exec.CommandContext(context.Background(), "explorer.exe", dir).Start() //nolint:gosec // G204: la carpeta es la de nube.Carpeta
	}
}

// conexionNube atiende la carpeta de la cuenta de la sesión, y solo esa: `nube` admite
// una conexión por proceso. Si se cae sola, el siguiente clic o cambio de sesión la
// vuelve a arrancar.
//
// ponytail: si OTRA app cambia de organización, el Hub sigue con su token hasta renovarlo
// y la carpeta nueva arranca en el siguiente aviso de la sesión o al pulsar «Conectar»;
// desde que el token es de la otra, la vieja ya no puede hacer nada (`nube.Atada`).
type conexionNube struct {
	cuenta func() nube.Cuenta
	fuente func(nube.Cuenta) nube.Fuente
	// nube.Conectar, nube.Conectada y nube.Carpeta; las pruebas ponen las suyas.
	conectar  func(context.Context, string, nube.Fuente) error
	conectada func(nube.Cuenta) (string, bool)
	carpeta   func(nube.Cuenta) (string, error)

	mu     sync.Mutex
	viva   nube.Cuenta // la de la conexión que corre
	parar  context.CancelFunc
	parada chan struct{} // se cierra cuando esa conexión ha terminado
}

// seguir deja la conexión en la cuenta de la sesión: para la de otra y arranca la de
// esta si ya estaba conectada en este equipo.
func (c *conexionNube) seguir() {
	cu := c.cuenta()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.parar != nil && c.viva == cu {
		return
	}
	c.pararLa()
	if dir, ok := c.conectada(cu); ok {
		c.arrancar(cu, dir)
	}
}

// abrir es el clic: la carpeta de quien está dentro, apuntada y en marcha.
func (c *conexionNube) abrir() (string, error) {
	cu := c.cuenta()
	dir, err := c.carpeta(cu)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.parar == nil || c.viva != cu {
		c.pararLa()
		c.arrancar(cu, dir)
	}
	return dir, nil
}

// pararLa corta la conexión viva y espera a que termine. Con el cerrojo cogido.
func (c *conexionNube) pararLa() {
	if c.parar == nil {
		return
	}
	c.parar()
	<-c.parada
	c.parar, c.parada, c.viva = nil, nil, nube.Cuenta{}
}

// arrancar conecta `dir` para `cu`, con una fuente que solo habla con Drive mientras la
// sesión sea de `cu`. Con el cerrojo cogido.
func (c *conexionNube) arrancar(cu nube.Cuenta, dir string) {
	ctx, parar := context.WithCancel(context.Background())
	parada := make(chan struct{})
	c.viva, c.parar, c.parada = cu, parar, parada
	go func() {
		defer parar()
		err := c.conectar(ctx, dir, c.fuente(cu))
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("hub: la carpeta de Drive de Windows no se ha podido conectar", "error", err)
		}
		close(parada) // antes del cerrojo: quien la para lo tiene cogido mientras espera
		c.mu.Lock()
		if c.parada == parada {
			c.parar, c.parada, c.viva = nil, nil, nube.Cuenta{}
		}
		c.mu.Unlock()
	}()
}
