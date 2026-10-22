// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "image/color"

// ColorearBarraTitulo pinta la barra de título que dibuja el SISTEMA —la de los
// semáforos en macOS, la del aspa en Windows— del color `c`, que es el del cromo de la
// app, y le dice si ese cromo es oscuro para que el texto y los botones contrasten.
//
// `ventana` es el identificador nativo que da `driver.NativeWindow.RunNative` de Fyne:
// la `NSWindow` en macOS, el `HWND` en Windows y la `Window` de X11 en Linux. Cero no
// hace nada. Se llama desde el hilo de la interfaz y es barata de repetir, pero quien
// la llama ya recuerda lo último que puso (`apps/fyne/ui/barratitulo.go`).
//
// Lo que llega en cada sistema:
//
//   - macOS: el color EXACTO. La barra se vuelve transparente, deja ver el fondo de la
//     ventana, que es `c`, y pierde el filete que la separa del contenido.
//   - Windows 11: el color exacto (`DWMWA_CAPTION_COLOR`) y el del texto. En Windows
//     10 solo existe el modo oscuro del marco (`DWMWA_USE_IMMERSIVE_DARK_MODE`): gris
//     del sistema, claro u oscuro según `oscuro`.
//   - Linux con X11: solo la variante, con `_GTK_THEME_VARIANT`, que respetan Mutter
//     (GNOME) y los gestores que pintan con el tema de GTK. KWin no la lee —el suyo es
//     `_KDE_NET_WM_COLOR_SCHEME`, que pide un fichero de esquema— y en Wayland el marco
//     lo pinta la propia GLFW, que no admite color: ahí no hace nada.
func ColorearBarraTitulo(ventana uintptr, c color.NRGBA, oscuro bool) {
	if ventana != 0 {
		colorearBarraTitulo(ventana, c, oscuro)
	}
}

// IdentificarVentana dice al escritorio de qué app (`id`, el identificador de paquete)
// es la ventana nativa `ventana`, para que la una a su .desktop: el lanzador, la barra de
// tareas y el número de avisos van por ahí. Solo hace algo en X11 (también bajo
// XWayland): en Wayland Fyne ya pone el `app_id`, y macOS y Windows lo saben por el
// paquete.
func IdentificarVentana(ventana uintptr, id string) {
	if ventana != 0 && id != "" {
		identificarVentana(ventana, id)
	}
}
