// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// cocoa: el arrastre y el soltar del fork de GLFW en macOS, sin nadie delante.
//
// No hay forma de empezar un arrastre de verdad sin ratón ni permisos de accesibilidad,
// así que se hace lo que haría AppKit: a la vista de una ventana de GLFW (oculta) se le
// mandan `draggingEntered:`, `draggingExited:`, `draggingEnded:` y
// `performDragOperation:` con un `NSDraggingInfo` falso que lleva un tablero de verdad
// (`NSPasteboard`) con un fichero, un texto o un enlace. Lo que se prueba es el código de
// Cocoa del parche, ejecutándose, y lo que llega a las retrollamadas.
//
// Se compila con las fuentes de GLFW del fork, que es lo que se prueba:
//
//   G=$(cd apps/fyne && go list -m -f '{{.Dir}}' github.com/go-gl/glfw/v3.4/glfw)/glfw
//   clang -D_GLFW_COCOA -Wno-deprecated-declarations -I"$G/include" -o /tmp/cocoa \
//     apps/fyne/soltar/testdata/cocoa.m "$G"/src/{context,init,input,monitor,platform,vulkan,window,egl_context,osmesa_context,null_init,null_monitor,null_window,null_joystick,cocoa_time,posix_module,posix_thread}.c \
//     "$G"/src/{cocoa_init,cocoa_joystick,cocoa_monitor,cocoa_window,nsgl_context}.m \
//     -framework Cocoa -framework IOKit -framework CoreFoundation -framework QuartzCore -framework OpenGL
//   /tmp/cocoa
//
// Sale con 0 si todo llega como se espera y con 1 si no, diciendo qué caso y qué llegó.
#define GLFW_INCLUDE_NONE
#define GLFW_EXPOSE_NATIVE_COCOA
#include <GLFW/glfw3.h>
#include <GLFW/glfw3native.h>
#import <Cocoa/Cocoa.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>

// arrastre es el `NSDraggingInfo` falso: solo lo que usa la vista de GLFW.
@interface Arrastre : NSObject
@property (retain) NSPasteboard* tablero;
@property NSPoint punto;
@end

@implementation Arrastre
- (NSPasteboard*)draggingPasteboard { return self.tablero; }
- (NSPoint)draggingLocation { return self.punto; }
@end

static char registro[8192];
static size_t largo;
static int fallos;

static void anotar(const char* formato, ...)
{
    va_list a;
    va_start(a, formato);
    int n = vsnprintf(registro + largo, sizeof(registro) - largo, formato, a);
    va_end(a);
    if (n > 0)
        largo += (size_t) n < sizeof(registro) - largo ? (size_t) n : sizeof(registro) - largo - 1;
}

static void alArrastrar(GLFWwindow* w, int encima) { anotar("arrastre %d\n", encima); }

static void alSoltarRutas(GLFWwindow* w, int n, const char** rutas)
{
    anotar("rutas");
    for (int i = 0; i < n; i++)
        anotar(" %s", rutas[i]);
    anotar("\n");
}

static void alSoltarTexto(GLFWwindow* w, double x, double y, const char* texto, int n, const char** urls)
{
    if (texto)
        anotar("texto %.0f,%.0f %s\n", x, y, texto);
    else
    {
        anotar("urls %.0f,%.0f", x, y);
        for (int i = 0; i < n; i++)
            anotar(" %s", urls[i]);
        anotar("\n");
    }
}

static void comprobar(const char* caso, const char* quiere)
{
    if (strcmp(registro, quiere) == 0)
        printf("ok - %s\n", caso);
    else
    {
        printf("not ok - %s\n  llegó:\n%s  quería:\n%s", caso, registro, quiere);
        fallos++;
    }
    largo = 0;
    registro[0] = '\0';
}

// tablero con esos objetos, en un tablero propio: no toca el portapapeles de nadie.
static Arrastre* arrastre(NSArray* objetos, NSPoint punto)
{
    Arrastre* a = [[Arrastre alloc] init];
    a.tablero = [NSPasteboard pasteboardWithUniqueName];
    [a.tablero clearContents];
    [a.tablero writeObjects:objetos];
    a.punto = punto;
    return a;
}

int main(void)
{
    @autoreleasepool
    {
        glfwInitHint(GLFW_COCOA_MENUBAR, GLFW_FALSE);
        if (!glfwInit())
            return 2;
        glfwWindowHint(GLFW_CLIENT_API, GLFW_NO_API);
        glfwWindowHint(GLFW_VISIBLE, GLFW_FALSE);
        GLFWwindow* w = glfwCreateWindow(400, 300, "prueba-soltar", NULL, NULL);
        if (!w)
            return 3;
        glfwSetDropCallback(w, alSoltarRutas);
        glfwSetDropTextCallback(w, alSoltarTexto);
        glfwSetDragCallback(w, alArrastrar);
        id vista = glfwGetCocoaView(w);
        const CGFloat alto = [vista frame].size.height;
        // (40, alto-40) en coordenadas de AppKit, que suben desde abajo: (40, 40) en GLFW.
        const NSPoint punto = NSMakePoint(40, alto - 40);

        NSURL* fichero = [NSURL fileURLWithPath:@"/tmp/acta final.docx"];
        Arrastre* a = arrastre(@[fichero], punto);
        NSDragOperation op = [vista draggingEntered:(id) a];
        anotar("operación %s\n", op == NSDragOperationNone ? "ninguna" : "aceptada");
        [vista performDragOperation:(id) a];
        [vista draggingEnded:(id) a];
        comprobar("un fichero: se avisa al entrar, se deja de avisar ANTES de entregarlo, y una sola vez",
                  "arrastre 1\noperación aceptada\narrastre 0\nrutas /tmp/acta final.docx\n");

        a = arrastre(@[fichero], punto);
        [vista draggingEntered:(id) a];
        [vista draggingExited:(id) a];
        [vista draggingEnded:(id) a];
        comprobar("un fichero que se va sin soltarse", "arrastre 1\narrastre 0\n");

        a = arrastre(@[@"Hola, cuaderno: ñ"], punto);
        op = [vista draggingEntered:(id) a];
        anotar("operación %s\n", op == NSDragOperationNone ? "ninguna" : "aceptada");
        [vista performDragOperation:(id) a];
        [vista draggingExited:(id) a];
        comprobar("un texto: se suelta en su sitio, y no es un arrastre de ficheros",
                  "operación aceptada\ntexto 40,40 Hola, cuaderno: ñ\n");

        a = arrastre(@[[NSURL URLWithString:@"https://sytue.eu/notes?x=1"]], punto);
        [vista draggingEntered:(id) a];
        [vista performDragOperation:(id) a];
        comprobar("un enlace: llega como URL, y no es un arrastre de ficheros",
                  "urls 40,40 https://sytue.eu/notes?x=1\n");

        // Sin retrollamada de texto, la ventana es la de siempre: un texto se rechaza.
        glfwSetDropTextCallback(w, NULL);
        a = arrastre(@[@"Hola"], punto);
        op = [vista draggingEntered:(id) a];
        anotar("operación %s\n", op == NSDragOperationNone ? "ninguna" : "aceptada");
        comprobar("sin retrollamada de texto, un texto se rechaza como antes del parche",
                  "operación ninguna\n");

        // Y sin retrollamada de arrastre no llega nada, pero el fichero sí.
        glfwSetDragCallback(w, NULL);
        a = arrastre(@[fichero], punto);
        [vista draggingEntered:(id) a];
        [vista performDragOperation:(id) a];
        comprobar("sin retrollamada de arrastre, el fichero llega igual",
                  "rutas /tmp/acta final.docx\n");

        glfwDestroyWindow(w);
        glfwTerminate();
    }
    return fallos ? 1 : 0;
}
