// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && cgo

package portapapeles

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/time.h>

// sytue_esperar espera, como mucho `ms`, un evento de `tipo` dirigido a `w`. Los demás se
// descartan: esta conexión es solo para la pregunta al portapapeles.
static int sytue_esperar(Display *d, Window w, int tipo, XEvent *ev, int ms) {
	struct timeval fin, ahora;
	gettimeofday(&fin, NULL);
	fin.tv_sec += ms / 1000;
	fin.tv_usec += (ms % 1000) * 1000;
	if (fin.tv_usec >= 1000000) {
		fin.tv_sec++;
		fin.tv_usec -= 1000000;
	}
	int fd = ConnectionNumber(d);
	for (;;) {
		while (XPending(d)) {
			XNextEvent(d, ev);
			if (ev->type == tipo && ev->xany.window == w) return 1;
		}
		gettimeofday(&ahora, NULL);
		long resta = (fin.tv_sec - ahora.tv_sec) * 1000000L + (fin.tv_usec - ahora.tv_usec);
		if (resta <= 0) return 0;
		struct timeval t = {resta / 1000000L, resta % 1000000L};
		fd_set fds;
		FD_ZERO(&fds);
		FD_SET(fd, &fds);
		// Las señales con que Go reparte el tiempo entre rutinas cortan `select` (EINTR),
		// y eso no es que se acabe la espera.
		fd_set *sin_escritura = NULL, *sin_excepcion = NULL;
		if (select(fd + 1, &fds, sin_escritura, sin_excepcion, &t) < 0 && errno != EINTR) return 0;
	}
}

// sytue_abrir abre la pantalla. La misma señal corta el `connect` de xcb, que no lo
// reintenta, así que se prueba otra vez: medido, sin esto fallaba 3 de cada 10 con -race.
static Display *sytue_abrir(void) {
	Display *d = NULL;
	for (int i = 0; i < 5 && !d; i++) d = XOpenDisplay(NULL);
	return d;
}

// sytue_leer_propiedad lee y BORRA la propiedad; devuelve los bytes (en *datos) o -1.
static long sytue_leer_propiedad(Display *d, Window w, Atom prop, Atom *tipo, unsigned char **datos) {
	int formato;
	unsigned long n, resto;
	unsigned char *p = NULL;
	*datos = NULL;
	if (XGetWindowProperty(d, w, prop, 0, 0x1fffffff, True, AnyPropertyType, tipo, &formato, &n, &resto, &p) != Success)
		return -1;
	if (*tipo == None) {
		if (p) XFree(p);
		return -1;
	}
	// Formato 8 son bytes y 16 son `short`; 32 son `long` de la máquina, que en un texto
	// no tienen sentido (salvo el tamaño que anuncia INCR, que no se usa).
	long tam = formato == 16 ? (long)n * 2 : formato == 8 ? (long)n : 0;
	if (tam > 0) {
		*datos = malloc(tam);
		memcpy(*datos, p, tam);
	}
	if (p) XFree(p);
	return tam;
}

// sytue_seleccion pide `seleccion` como `tipo` y devuelve cuántos bytes deja en *datos
// (malloc; los libera quien llama). Si no, un código:
//   -1 sin pantalla · -2 sin dueño · -3 el dueño no lo tiene así · -4 no contesta a tiempo
static long sytue_seleccion(const char *seleccion, const char *tipo, int ms, unsigned char **datos) {
	*datos = NULL;
	Display *d = sytue_abrir();
	if (!d) return -1;
	Atom sel = XInternAtom(d, seleccion, False);
	Atom quiero = XInternAtom(d, tipo, False);
	Atom prop = XInternAtom(d, "SYTUE_PORTAPAPELES", False);
	Atom incr = XInternAtom(d, "INCR", False);
	if (XGetSelectionOwner(d, sel) == None) {
		XCloseDisplay(d);
		return -2;
	}
	Window w = XCreateSimpleWindow(d, DefaultRootWindow(d), 0, 0, 1, 1, 0, 0, 0);
	XSelectInput(d, w, PropertyChangeMask);
	XConvertSelection(d, sel, quiero, prop, w, CurrentTime);
	XFlush(d);
	long r;
	XEvent ev;
	Atom real;
	if (!sytue_esperar(d, w, SelectionNotify, &ev, ms)) {
		r = -4;
	} else if (ev.xselection.property == None) {
		r = -3;
	} else if ((r = sytue_leer_propiedad(d, w, prop, &real, datos)) < 0) {
		r = -3;
	} else if (real == incr) {
		// INCR: el dueño manda por trozos. Borrar la propiedad (lo ha hecho la lectura)
		// es la señal para el primero; cada trozo llega como un valor nuevo de la
		// propiedad, y uno vacío es el final.
		free(*datos);
		*datos = NULL;
		unsigned char *todo = NULL, *trozo = NULL;
		long total = 0;
		for (;;) {
			int ok;
			while ((ok = sytue_esperar(d, w, PropertyNotify, &ev, ms)) &&
			       (ev.xproperty.atom != prop || ev.xproperty.state != PropertyNewValue)) {
			}
			long n = ok ? sytue_leer_propiedad(d, w, prop, &real, &trozo) : -1;
			if (n < 0) {
				free(todo);
				todo = NULL;
				total = -4;
				break;
			}
			if (n == 0) break;
			todo = realloc(todo, total + n);
			memcpy(todo + total, trozo, n);
			total += n;
			free(trozo);
			trozo = NULL;
		}
		*datos = todo;
		r = total;
	}
	XDestroyWindow(d, w);
	XCloseDisplay(d);
	return r;
}

// sytue_leido es lo que da sytue_leer: los bytes (malloc) y cuántos, o el código de
// sytue_seleccion. Va POR VALOR para que Go no pase a C un puntero suyo, y en `char *`
// para que Go lo copie con C.GoStringN sin importar `unsafe`.
typedef struct {
	char *datos;
	long n;
} sytue_leido;

static sytue_leido sytue_leer(const char *seleccion, const char *tipo, int ms) {
	unsigned char *datos = NULL;
	long n = sytue_seleccion(seleccion, tipo, ms, &datos);
	sytue_leido r = {(char *)datos, n};
	return r;
}

static void sytue_liberar(char *p) { free(p); }
*/
import "C"

import "os"

// esperaX11 es lo que se espera a cada respuesta del dueño del portapapeles. Se pregunta
// desde otra rutina que la de la interfaz: si el dueño es la propia app, su bucle de
// eventos tiene que estar libre para contestar (que no lo tiene en HTML).
const esperaX11 = 1500

// leer prueba X11 primero —una app de Fyne es un cliente X11, también bajo Wayland por
// XWayland— y, si ahí no hay HTML y hay un compositor Wayland, le pregunta a él: el puente
// de XWayland no siempre pasa todos los tipos.
func leer() ([]byte, error) {
	if os.Getenv("DISPLAY") == "" {
		return leerWayland()
	}
	datos, err := leerX11("CLIPBOARD", "text/html", esperaX11)
	if err != nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		if d, errW := leerWayland(); errW == nil {
			return d, nil
		}
	}
	return datos, err
}

// leerX11 pide `seleccion` como `tipo` en el servidor de $DISPLAY.
func leerX11(seleccion, tipo string, ms int) ([]byte, error) {
	cs, ct := C.CString(seleccion), C.CString(tipo)
	defer C.sytue_liberar(cs)
	defer C.sytue_liberar(ct)
	l := C.sytue_leer(cs, ct, C.int(ms))
	defer C.sytue_liberar(l.datos)
	n := l.n
	switch {
	case n == -1:
		return nil, ErrNoDisponible
	case n <= 0:
		return nil, ErrSinHTML
	case n > MaxHTML*2: // en UTF-16 ocupa el doble
		return nil, ErrSinHTML
	}
	return []byte(C.GoStringN(l.datos, C.int(n))), nil
}
