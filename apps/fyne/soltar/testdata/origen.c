// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// origen: un origen XDnD mínimo para soltar_x11_test.go.
//
//   origen [--salir] <ventana> <x> <y> <tipo> <datos> [<tipo> <datos>…]
//
// <ventana> es el identificador X11 de la ventana destino, o su título (para una ventana
// de Wine, que no dice su identificador X11), y <x> <y> el punto donde se suelta,
// relativo a ella. Hace el protocolo XDnD 5 entero como un navegador: ofrece
// los tipos en XdndEnter, se mueve con XdndPosition, espera el XdndStatus, suelta con
// XdndDrop, sirve la conversión de XdndSelection que le pidan y espera el XdndFinished.
// Si la ventana rechaza lo ofrecido, manda XdndLeave, que es lo que haría el navegador.
//
// Con --salir, en vez de soltar se va (XdndLeave) en cuanto la ventana contesta: es
// quien arrastra y se lo piensa, o pulsa Escape.
//
// Sale con 0 si la ventana aceptó y terminó (o, con --salir, si contestó), 2 si rechazó y
// 3 si no contestó a tiempo.
#include <X11/Xatom.h>
#include <X11/Xlib.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

static Display* d;

static Atom atomo(const char* nombre) { return XInternAtom(d, nombre, False); }

// buscar es la primera ventana, bajo `w`, con ese título.
static Window buscar(Window w, const char* titulo)
{
    char* nombre = NULL;
    if (XFetchName(d, w, &nombre) && nombre)
    {
        const int igual = strcmp(nombre, titulo) == 0;
        XFree(nombre);
        if (igual)
            return w;
    }
    Window raiz, padre, *hijos = NULL;
    unsigned int n = 0;
    Window hallada = None;
    if (XQueryTree(d, w, &raiz, &padre, &hijos, &n))
    {
        for (unsigned int i = 0; i < n && hallada == None; i++)
            hallada = buscar(hijos[i], titulo);
        if (hijos)
            XFree(hijos);
    }
    return hallada;
}

static void enviar(Window destino, Atom tipo, long l0, long l1, long l2, long l3, long l4)
{
    XEvent e;
    memset(&e, 0, sizeof(e));
    e.xclient.type = ClientMessage;
    e.xclient.window = destino;
    e.xclient.message_type = tipo;
    e.xclient.format = 32;
    e.xclient.data.l[0] = l0;
    e.xclient.data.l[1] = l1;
    e.xclient.data.l[2] = l2;
    e.xclient.data.l[3] = l3;
    e.xclient.data.l[4] = l4;
    XSendEvent(d, destino, False, NoEventMask, &e);
    XFlush(d);
}

int main(int argc, char** argv)
{
    const int salir = argc > 1 && strcmp(argv[1], "--salir") == 0;
    if (salir)
    {
        argv++;
        argc--;
    }
    if (argc < 6 || (argc - 4) % 2 != 0 || (argc - 4) / 2 > 16)
    {
        fprintf(stderr, "uso: origen <ventana> <x> <y> <tipo> <datos> [<tipo> <datos>…]\n");
        return 64;
    }
    d = XOpenDisplay(NULL);
    if (!d)
    {
        fprintf(stderr, "origen: no hay servidor X\n");
        return 65;
    }

    char* fin;
    Window destino = strtoul(argv[1], &fin, 0);
    if (*fin != '\0')
        destino = None; // un título que empieza por cifras
    for (int i = 0; *fin != '\0' && destino == None && i < 100; i++)
    {
        destino = buscar(DefaultRootWindow(d), argv[1]);
        if (destino == None)
            usleep(100000);
    }
    if (destino == None)
    {
        fprintf(stderr, "origen: no hay ninguna ventana «%s»\n", argv[1]);
        return 66;
    }
    const int x = atoi(argv[2]), y = atoi(argv[3]);
    const int n = (argc - 4) / 2;
    Atom tipos[16];
    const char* datos[16];
    for (int i = 0; i < n; i++)
    {
        tipos[i] = atomo(argv[4 + 2 * i]);
        datos[i] = argv[5 + 2 * i];
    }

    const Atom enter = atomo("XdndEnter"), position = atomo("XdndPosition"),
               status = atomo("XdndStatus"), leave = atomo("XdndLeave"),
               drop = atomo("XdndDrop"), finished = atomo("XdndFinished"),
               seleccion = atomo("XdndSelection"), copiar = atomo("XdndActionCopy");

    const Window raiz = DefaultRootWindow(d);
    const Window yo = XCreateSimpleWindow(d, raiz, 0, 0, 1, 1, 0, 0, 0);
    XSetSelectionOwner(d, seleccion, yo, CurrentTime);
    if (n > 3)
        XChangeProperty(d, yo, atomo("XdndTypeList"), XA_ATOM, 32, PropModeReplace,
                        (unsigned char*) tipos, n);

    int rx, ry;
    Window hijo;
    XTranslateCoordinates(d, destino, raiz, x, y, &rx, &ry, &hijo);

    enviar(destino, enter, yo, (5L << 24) | (n > 3 ? 1 : 0),
           tipos[0], n > 1 ? tipos[1] : None, n > 2 ? tipos[2] : None);
    enviar(destino, position, yo, 0, ((long) rx << 16) | ry, CurrentTime, copiar);

    const time_t limite = time(NULL) + 10;
    int soltado = 0;
    while (time(NULL) < limite)
    {
        while (XPending(d))
        {
            XEvent e;
            XNextEvent(d, &e);
            if (e.type == ClientMessage && e.xclient.message_type == status && !soltado)
            {
                if (!(e.xclient.data.l[1] & 1))
                {
                    enviar(destino, leave, yo, 0, 0, 0, 0);
                    XSync(d, False);
                    XCloseDisplay(d);
                    printf("rechazado\n");
                    return 2;
                }
                if (salir)
                {
                    enviar(destino, leave, yo, 0, 0, 0, 0);
                    // XSync y no solo XFlush: sin la ida y vuelta, el proceso salía con
                    // respuestas sin leer en el socket, el cierre iba como RST y el
                    // servidor tiraba el XdndLeave que aún no había procesado. La ventana
                    // se quedaba «encima» para siempre y el test fallaba de vez en cuando
                    // (1 de cada ~5 con varias pruebas en paralelo; medido 2026-09-25).
                    XSync(d, False);
                    XCloseDisplay(d);
                    printf("salido\n");
                    return 0;
                }
                enviar(destino, drop, yo, 0, CurrentTime, 0, 0);
                soltado = 1;
            }
            else if (e.type == SelectionRequest)
            {
                const XSelectionRequestEvent* r = &e.xselectionrequest;
                XEvent respuesta;
                memset(&respuesta, 0, sizeof(respuesta));
                respuesta.xselection.type = SelectionNotify;
                respuesta.xselection.requestor = r->requestor;
                respuesta.xselection.selection = r->selection;
                respuesta.xselection.target = r->target;
                respuesta.xselection.time = r->time;
                respuesta.xselection.property = None;
                for (int i = 0; i < n; i++)
                {
                    if (r->target == tipos[i])
                    {
                        XChangeProperty(d, r->requestor, r->property, r->target, 8,
                                        PropModeReplace, (const unsigned char*) datos[i],
                                        (int) strlen(datos[i]));
                        respuesta.xselection.property = r->property;
                        break;
                    }
                }
                XSendEvent(d, r->requestor, False, NoEventMask, &respuesta);
                XFlush(d);
            }
            else if (e.type == ClientMessage && e.xclient.message_type == finished)
            {
                printf("terminado\n");
                return 0;
            }
        }
        usleep(10000);
    }
    fprintf(stderr, "origen: la ventana no terminó a tiempo (soltado=%d)\n", soltado);
    return 3;
}
