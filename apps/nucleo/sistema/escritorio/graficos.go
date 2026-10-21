// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrSinGraficos es que el equipo no ofrece el OpenGL 2.1 con el que pinta Fyne.
var ErrSinGraficos = errors.New("escritorio: el controlador gráfico no ofrece OpenGL 2.1")

// SinGraficos es ErrSinGraficos con lo que el sistema dice que tiene («OpenGL 1.1.0,
// GDI Generic»), que es lo que se enseña: no se traduce.
type SinGraficos struct{ Detalle string }

func (e *SinGraficos) Error() string { return ErrSinGraficos.Error() + ": " + e.Detalle }

// Unwrap deja comparar con errors.Is(err, ErrSinGraficos).
func (e *SinGraficos) Unwrap() error { return ErrSinGraficos }

// Graficos comprueba, ANTES de abrir ninguna ventana, que el equipo sabe pintarla. En
// Windows sin controlador de la tarjeta gráfica —una máquina virtual, un escritorio
// remoto, un equipo recién instalado— solo hay el OpenGL 1.1 de «GDI Generic», y Fyne
// no crea la ventana: la app se queda viva y sin forma de verla, con su icono en la
// bandeja (medido en Windows Server 2022). Devuelve ErrSinGraficos, con lo que el
// sistema dice que tiene, para que quien llama lo cuente. En los demás sistemas, nil.
//
// En el proceso que RelanzarConReserva abre, antes de preguntar pone la carpeta `mesa`
// del instalador como directorio de DLL: el opengl32.dll que se cargue desde ahí es el
// de Mesa, y lo que se comprueba es lo que va a pintar.
func Graficos() error {
	if os.Getenv(envReserva) != "" {
		if err := ponerDirectorioDLL(dirMesa()); err != nil {
			return &SinGraficos{"SetDllDirectory: " + err.Error()}
		}
	}
	return graficos()
}

// envReserva marca el proceso que se abrió con el OpenGL de reserva.
const envReserva = "SYTUE_GL_RESERVA"

// ejecutable es os.Executable; una variable para las pruebas.
var ejecutable = os.Executable

// dirMesa es donde el instalador de Windows deja Mesa: en una carpeta APARTE, nunca al
// lado del .exe. Ahí Windows lo cargaría antes que el del sistema y el controlador de
// la tarjeta no se usaría nunca.
func dirMesa() string {
	exe, err := ejecutable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "mesa")
}

// RelanzarConReserva es lo que se hace cuando Graficos falla: si el instalador dejó
// Mesa (llvmpipe, que pinta con la CPU) abre otra vez esta app, con los mismos
// argumentos, para que lo use; y dice true, y quien llama sale. Solo así Mesa va DETRÁS
// del controlador de verdad: este proceso ya ha cargado el opengl32.dll del sistema y
// Windows no carga otro con el mismo nombre. El relanzado no vuelve a relanzar.
func RelanzarConReserva() bool {
	if os.Getenv(envReserva) != "" {
		return false
	}
	exe, err := ejecutable()
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dirMesa(), "opengl32.dll")); err != nil {
		return false
	}
	c := exec.CommandContext(context.Background(), exe, os.Args[1:]...) //nolint:gosec // G204: es esta misma app
	c.Env = append(os.Environ(), envReserva+"=1")
	if os.Getenv("GALLIUM_DRIVER") == "" {
		c.Env = append(c.Env, "GALLIUM_DRIVER=llvmpipe")
	}
	return c.Start() == nil
}

// versionSuficiente dice si un GL_VERSION («1.1.0», «4.6.0 NVIDIA 555.85», «2.1 Mesa…»)
// llega a 2.1.
func versionSuficiente(v string) bool {
	palabras := strings.Fields(v)
	if len(palabras) == 0 {
		return false
	}
	campos := strings.SplitN(palabras[0], ".", 3)
	if len(campos) < 2 {
		return false
	}
	mayor, err1 := strconv.Atoi(campos[0])
	menor, err2 := strconv.Atoi(campos[1])
	return err1 == nil && err2 == nil && (mayor > 2 || mayor == 2 && menor >= 1)
}
