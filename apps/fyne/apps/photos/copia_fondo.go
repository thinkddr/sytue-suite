// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
)

// # La copia de seguridad con la app cerrada
//
// La misma ronda que con la app abierta (copia.go), lanzada por el sistema: en Android,
// cuando hay fotos o vídeos nuevos en la galería y cada pocas horas (TareaFondo, con la
// red que pidan los ajustes); en iOS, en la tarea de refresco que el sistema da a la app.
// La registra este `init`: en Android ese trabajo carga la biblioteca de la app sin
// correr su `main`. Lo que la app abierta y esta comparten está en el registro de la
// copia (CopiaDe), así que una de las dos espera a la otra.
//
// Solo con el dispositivo desbloqueado: la sesión de la suite no se abre con la pantalla
// bloqueada (sesion, tareas_fondo.go).

func init() { sesion.RegistrarTareaEnFondo("photos", copiaEnFondo) }

// galeriaFondo y rutaFondo son las de verdad; las pruebas las cambian.
var (
	galeriaFondo GaleriaSistema = CarreteDelSistema{}
	rutaFondo                   = RutaRegistroCopia
)

// copiaEnFondo es la TareaEnFondo de Photos.
func copiaEnFondo(ctx context.Context, f sesion.EnFondo) sesion.Resultado {
	ruta, err := rutaFondo()
	if err != nil {
		return sesion.Fallo
	}
	c := CopiaDe(galeriaFondo, ruta)
	if !c.Ajustes().Activa {
		return sesion.SinCambios
	}
	d := nuevoDestinoDrive(api.NewDrive(f.URL("drive"), f.Sesion, nil), api.NewPhotos(f.URL("photos"), f.Sesion, nil))
	p, err := c.Ronda(ctx, f.Cuenta, d, nil)
	return resultadoDeCopia(p, err)
}

// resultadoDeCopia traduce cómo acabó una ronda a lo que se le dice al sistema: lo que hay
// que reintentar pronto (sin Wi-Fi, sin tiempo) es Aplazado; lo que no se arregla
// reintentando (sin permiso, sin galería, apagada) es SinCambios.
func resultadoDeCopia(p ProgresoCopia, err error) sesion.Resultado {
	switch {
	case err == nil && p.Subidas > 0:
		return sesion.Nuevos
	case err == nil, errors.Is(err, ErrCopiaEnMarcha),
		errors.Is(err, carrete.ErrSinPermiso), errors.Is(err, carrete.ErrNoDisponible):
		return sesion.SinCambios
	case errors.Is(err, ErrCopiaEsperaWifi), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return sesion.Aplazado
	default:
		return sesion.Fallo
	}
}
