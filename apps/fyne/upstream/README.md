# El parche de URL entrantes, listo para enviar a Fyne

`apps/fyne/go.mod` reemplaza Fyne por `github.com/thinkddr/fyne/v2 v2.8.2-sytue.5`
(alojado en GitHub desde el 2026-09-27; antes en git.gay, con las mismas ramas y tags)
(ADR «URL entrante» en `docs/decisions.md`). Un fork es deuda: esta carpeta tiene lo que
hace falta para quitarlo, que es que el parche entre aguas arriba.

## `v2.8.2-sytue.4` y `v2.8.2-sytue.5` (2026-11-07): refresco y avisos de fondo en iOS

Las dos tags apuntan a la misma fuente,
[`5438e4c86769e1f5936adacedb29c4436d15cbdc`](https://github.com/thinkddr/fyne/commit/5438e4c86769e1f5936adacedb29c4436d15cbdc):
`.4` es la que trae `0016` funcionando y `.5` añade `0017`. Sus arreglos van ya dentro de
`0016-ios-background-refresh.patch` y `0017-ios-remote-notifications.patch`, regenerados
con `git format-patch` sobre `v2.8.2-sytue.3`. Aplicados con `git am`, dan el árbol de la
fuente etiquetada salvo `.github/ios-geometry/run.sh`, que es CI del fork y no va aguas
arriba. Las matrices de los parches tal y como estaban sacaron dos fallos:

- **La app moría al arrancar en iOS** (`GeometryUITests`: el informe no aparecía; con el
  diagnóstico nuevo de `run.sh`, `SIGSEGV` en `runApp`). cgo compila `darwin_ios.m`
  **sin ARC**. Un arranque normal conecta la escena sin adjuntar y aplaza la conexión, y
  `sceneWillEnterForeground` soltaba las opciones antes de usarlas. Ahora se sueltan
  después, y `sceneDidBecomeActive` conecta también una escena pendiente.
- **Windows**: el `.m` se extrae con CRLF y los contratos de fuente cortaban con -1. Ahora
  leen con LF, fallan nombrando el marcador que falta y `.gitattributes` fija LF en
  `*.m`, `*.h` y `*.c`.

Además, `didReceiveRemoteNotification:fetchCompletionHandler:` solo se añade al delegado
(en `runApp`) si el `Info.plist` declara `remote-notification`: antes UIKit avisaba en
todas las apps. Y el `completionHandler` ya no se pierde (se retenía dos veces).

Pasaron, sobre la fuente etiquetada:

- [Web](https://github.com/thinkddr/fyne/actions/runs/42379644977)
- [Static Analysis](https://github.com/thinkddr/fyne/actions/runs/42379645045)
- [Mobile](https://github.com/thinkddr/fyne/actions/runs/42379645039)
- [Platform](https://github.com/thinkddr/fyne/actions/runs/42379645027), con Windows
- [CodeQL](https://github.com/thinkddr/fyne/actions/runs/42379644935)
- [Fork conformance](https://github.com/thinkddr/fyne/actions/runs/42379645040): el driver
  UIKit compilado y enlazado para dispositivo y simulador, y la app arrancada en el
  simulador de iPhone y de iPad con sus 32 medidas y toques.

No acreditan una tarea de refresco ni un aviso de APNs reales (el simulador no los lanza),
ni un iPhone físico, ni App Review.

## Refresco en segundo plano de iOS (2026-11-07, `v2.8.2-sytue.4`)

`0016-ios-background-refresh.patch` añade la interfaz opcional
`mobile.BackgroundDriver`, que el driver móvil implementa. Sirve al widget nativo con la
app cerrada ([enmienda del ADR 0078](../../../docs/decisions.md#0078)):

- `LaunchedInBackground`: UIKit lanzó el proceso sin escena (tarea de refresco o aviso
  de fondo).
- `OnFirstScene`: lo que necesita interfaz corre al conectar la primera escena, en el
  hilo principal y antes de las URL de esa escena; fuera de iOS, en el acto. Si UIKit
  conecta la escena fuera del primer plano (lo puede hacer durante un trabajo de fondo),
  eso y sus URL esperan a `sceneWillEnterForeground`: sin nadie mirando, el Face ID del
  bloqueo fallaría y la app saldría.
- `ProtectedDataAvailable`: falso con el dispositivo bloqueado. Cada tarea se lo vuelve a
  preguntar a UIKit (`isProtectedDataAvailable`, en el hilo principal) antes de correr el
  manejador: los avisos del delegado pueden llegarle tarde a una app suspendida.
- `OnBackgroundRefresh`: el manejador de la tarea que nombra la primera entrada de
  `BGTaskSchedulerPermittedIdentifiers`. Se registra antes de acabar el lanzamiento, se
  vuelve a pedir al pasar a segundo plano y al terminar, y se completa una sola vez: si
  el sistema la corta, se cancela el contexto y falla en cuanto el manejador vuelve, o a
  los 500 ms si no vuelve; y si la corta antes de que el manejador empiece (el hilo
  principal ocupado), falla en el acto y el manejador no corre: Go la conoce desde que
  iOS la entrega, antes de poner su `expirationHandler`. Ese margen es para que suelte el cerrojo de `ajustes` (un
  `flock` en el App Group): iOS mata con 0xdead10cc a la app que se suspende con un
  cerrojo de fichero tomado en un contenedor compartido.

Una app sin esas claves en el `Info.plist` se comporta como antes. La parte portable
tiene pruebas en todas las plataformas; el puente de Objective-C se fija con contratos
de fuente, porque no compila fuera de Xcode. Se ha comprobado aquí: `go vet` y
`go test` de `driver/mobile`, `internal/driver/mobile` e `internal/driver/mobile/app` en
Linux, y que `apps/fyne/sesion` compila contra el fork parcheado con la interfaz idéntica
a la suya. El `.m` lo compila y arranca en el simulador la matriz de UIKit (arriba); no se
ha corrido en un iPhone.

La suite lo usa por aserción sobre el driver (`sesion/fondo.go`). **Encendido** el
2026-11-07: el `replace` de `apps/fyne/go.mod` usa `v2.8.2-sytue.5`, y en el mismo commit se
subieron las referencias de `ci-desktop.yml`, `ci-mobile.yml`, `empaquetar-test.sh`,
`BACKLOG-FYNE.md` y `docs/CONVENTIONS.md` y se puso `SYTUE_REFRESCO_APPLE = YES` en
`apps/fyne/xcode/Sytue.xcconfig`. Ese es también el camino para un tag futuro.

## Avisos silenciosos de iOS (2026-11-07, `v2.8.2-sytue.5`)

`0017-ios-remote-notifications.patch` va **encima de `0016`** y añade la interfaz
opcional `mobile.RemoteNotificationDriver`. Sirve al despertar del widget con la app
cerrada: el Hub manda un aviso vacío de APNs cuando cambia algo del producto
([enmiendas del ADR 0039 y 0078](../../../docs/decisions.md#0039)):

- `RemoteNotificationsAvailable`: el `Info.plist` declara `remote-notification` en
  `UIBackgroundModes`. Se puede leer desde `main`, antes de que UIKit arranque.
- `RegisterForRemoteNotifications`: pide a UIKit el token del aparato, sin diálogo de
  permiso (un aviso sin alerta no lo necesita), y lo entrega en hexadecimal en minúsculas a
  todos los que lo esperan, o el error de UIKit. Las peticiones a la vez comparten una.
- `OnRemoteNotification`: el manejador de
  `application:didReceiveRemoteNotification:fetchCompletionHandler:`. El contenido no se
  lee nunca. El manejador recibe un contexto que se cancela a los 25 s, y el
  `completionHandler` se llama una sola vez: con datos nuevos, sin datos o fallo según
  lo que devuelva, fallo si se canceló el contexto, o fallo a los 500 ms de gracia si el
  manejador no vuelve (iOS mata la app que no lo llama). Antes de correr el manejador se
  pregunta a UIKit, en el hilo principal, si los datos protegidos están disponibles. Sin
  manejador, el aviso falla en el acto.

Una app sin ese modo de fondo se comporta como antes. Como con `0016`, la máquina de
estados portable tiene pruebas en todas las plataformas y el puente de Objective-C se fija
con contratos de fuente. Se ha comprobado aquí: `go vet` y `go test` de `driver/mobile`,
`internal/driver/mobile` e `internal/driver/mobile/app` en Linux sobre `0016` y que
`apps/fyne/sesion` compila contra el fork con la interfaz idéntica a la suya
(`conductorAvisos`, `sesion/despertar.go`). El `.m` lo compila la matriz de UIKit; no se
ha recibido un aviso en un iPhone.

La suite lo usa por aserción sobre el driver. El fork ya lo trae: `apps/fyne/go.mod` usa
`v2.8.2-sytue.5` desde el 2026-11-07. `SYTUE_DESPERTAR_APPLE = YES` desde el mismo día (decisión del dueño); faltan los pasos
del dueño del [runbook](../../../docs/operations.md#despertar-widgets-ios): la clave, la
capacidad Push en los quince App ID, los perfiles regenerados y el Secret del Hub.
`empaquetar-test.sh` no deja encenderlo con `.4` o anterior.

## Ciclo completo del toque móvil (2026-11-02)

El tag [`v2.8.2-sytue.3`](https://github.com/thinkddr/fyne/tree/v2.8.2-sytue.3)
añade la interfaz opcional `mobile.TouchObserverCanvas`. Su observador recibe
el ciclo físico completo aunque un `TouchFilter` haya devuelto `FilterPass`;
observar no captura ni retrasa los eventos. Esto permite mostrar el estado
pulsado sin impedir que se desplace una lista.

Al cambiar el contenido, las capas, la geometría o el observador, la cancelación
se entrega antes del `DragEnd` del driver. La suite la usa para descartar los
movimientos provisionales, sin persistirlos como si el usuario hubiera soltado.
Las rutas observadas no retienen el árbol anterior de objetos.

La fuente etiquetada es `b3cbd9833ac5018dd93e0789fd4f50d24da9be07`.
Pasaron las matrices de Web, Mobile, Platform, análisis estático y CodeQL,
además de [Fork conformance](https://github.com/thinkddr/fyne/actions/runs/41372869526)
con UIKit en iPhone e iPad. Las pruebas del driver cubren `FilterPass`, replay,
multitáctil y el orden de cancelación; no acreditan el uso en un dispositivo físico.

## Geometría de iOS y actualización a Fyne 2.8.2 (2026-10-25)

La [release estable `v2.8.2-sytue.2`](https://github.com/thinkddr/fyne/releases/tag/v2.8.2-sytue.2)
parte de Fyne 2.8.2 y conserva todos los cambios de `.21`. La ventana sigue el tamaño
y la escala de la vista UIKit a la que está unida, incluidos sus márgenes seguros al
girar y al reducir su viewport. Go conserva los píxeles ya orientados; los toques y el
rectángulo del teclado se convierten al espacio de la vista. Un teclado flotante o
separado del borde inferior no encoge todo el lienzo.

`0015-ios-window-geometry.patch` recoge únicamente el cambio del driver y sus pruebas
portables sobre `v2.8.2-sytue.1`. Los avisos BSD-3-Clause del original y Apache-2.0 de
las modificaciones se conservan. La suite fija también typesetting `v0.3.5-sytue.1`
y GLFW `v0.1.0-sytue.4`, rebases de los originales 0.3.5 y 0.1.0-pre.2, y actualiza el
inventario de originales que consulta OSV.

La fuente final pasó las cinco matrices del fork. Con Xcode 27.0 y Go 1.27.3 se
compiló y enlazó el driver para dispositivo y simulador iOS arm64. Los simuladores de
iPhone 18 Pro e iPad Pro de 13 pulgadas recorrieron cuatro estados de rotación y
cuatro tamaños/posiciones de la vista por estado: 32 medidas y 32 toques UIKit
correctos. El arnés desactiva las animaciones UIKit y lo declara en sus informes;
el bucle de renderizado y el driver de producción no cambian. Se verificaron además
24 casos portables de geometría/teclado. No se automatizan los gestos de Stage Manager
ni la ejecución en un dispositivo físico. La instalación limpia del tag pasó en
[`public-install`](https://github.com/thinkddr/fyne/actions/runs/40198336656/job/121183936450):
Go 1.27.3, descarga del módulo publicado, sumas verificadas y build externo limpio.

## Actualización a Fyne 2.8.2 (2026-10-23)

El fork integra la versión estable de upstream conservando las URL entrantes,
los gestos móviles y los pintores opcionales de la suite. Incluye las correcciones
del teclado y el ciclo de dibujo de iOS, los permisos de lectura del selector de
Android y las coordenadas al soltar archivos en escritorio. La prueba del selector
ejecuta el Java real y su callback JNI, con selección múltiple y proveedores que
rechazan el permiso persistente.

También se actualizan las bases reales de los reemplazos: typesetting
`v0.3.5-sytue.1` conserva las medidas fraccionarias y declara el módulo canónico;
GLFW `v0.1.0-sytue.4` incorpora la base `v0.1.0-pre.2` y mantiene los callbacks de
texto, enlaces y arrastre de archivos. Sus tags anteriores siguen intactos. La
suite descarga estos dos forks directamente; la visibilidad pública de un
repositorio y `GOPRIVATE` son propiedades distintas.

La referencia visual del submenú se actualiza por el ajuste a la altura del
lienzo de upstream. La comparación de píxeles conserva las etiquetas de la raíz;
no motivó ningún cambio adicional de layout ni de conformado de texto. Los
contratos de gestos y renderizado mantienen sus comprobaciones y umbrales.

## Gestos móviles y mantenimiento del fork (2026-10-23)

El tag `v2.8.1-sytue.21` añade `mobile.TouchFilterCanvas`, una interfaz opcional que
permite decidir un gesto antes de entregar el toque a las filas o a los editores.
Reproduce los eventos rechazados con sus tiempos originales, conserva el multitáctil
y cancela al cambiar la raíz, una capa, la geometría o el filtro. iOS entrega
`touchesCancelled` y mantiene el identificador de cada dedo; Android distingue
`ACTION_CANCEL` de una pulsación terminada. `fyne.Canvas` conserva su contrato.

Incluye los cambios anteriores del pintor de imágenes, la sincronización de la
caducidad de caché y los contratos opcionales de texto y rectángulos del pintor de
software. [La comparación con `.16`](https://github.com/thinkddr/fyne/compare/v2.8.1-sytue.16...v2.8.1-sytue.21)
contiene el cambio completo. La versión de rectángulos documentada como `.20` no
se ha vuelto a etiquetar.

La fuente `bf85664117f0a3acc2cf3388e07fd7dee32a3d38` pasó las cinco matrices del
fork: carrera, análisis, plataformas, web y móvil. Su descarga se verificó en
Scaleway mediante `proxy.golang.org` y `sum.golang.org`, sin excluir el módulo de la
verificación pública. La suite fija el tag y las dos sumas junto a sus guardianes
de empaquetado. Las pruebas del driver no acreditan la interacción física con los
gestos reservados por cada sistema operativo.

El código original conserva BSD-3-Clause. Las modificaciones actuales del fork
se publican bajo Apache-2.0; el aviso de licencias del fork conserva la atribución
de upstream. El parche histórico de URL de esta carpeta mantiene su licencia.

## Parche histórico de URL entrantes

- **`0001-receive-incoming-urls.patch`** — el parche, exportado del commit `2430cc176a`
  de la rama `sytue` del fork. 13 ficheros, **235 líneas añadidas y ninguna borrada**:
  no cambia el comportamiento de nadie que no lo pida. Incluye su prueba
  (`internal/urlhandler/urlhandler_test.go`) y está redactado en inglés, como el resto
  del proyecto.
- El código del parche es de Fyne y sale **bajo la BSD-3-Clause de Fyne**, no bajo la
  licencia del resto de este repositorio.

## Qué falta para abrirla, y por qué no está abierta

La PR la tiene que abrir **la cuenta personal del dueño en GitHub**: una PR aguas arriba
va firmada por una persona, no por un agente. Todo lo demás está hecho.

Pasos, cuando se abra:

1. `git clone https://github.com/fyne-io/fyne && cd fyne && git switch develop`
   — Fyne integra por `develop`; el parche se exportó sobre `v2.8.1`, así que puede
   pedir un rebase trivial en `app/app.go`.
2. `git am ../0001-receive-incoming-urls.patch` y `go test ./...`.
3. Enlazar la issue existente **[#1847 «Register URL Protocol
   Handler»](https://github.com/fyne-io/fyne/issues/1847)** (abierta, es justo esto) y la
   cerrada **[#5812 «Write apps that can handle URL
   schemes»](https://github.com/fyne-io/fyne/issues/5812)**. No hay ninguna PR abierta
   que lo resuelva (búsqueda del 2026-09-22 en `fyne-io/fyne`).
4. Cuando se acepte: borrar el `replace` de `apps/fyne/go.mod`, los cuatro guardianes de
   los workflows que lo exigen y esta carpeta. **Antes**, mirar que Fyne lleve también
   los otros dos parches del fork, que no son de URL: la caducidad de la caché de
   renderers (`.3`) y la cola del driver de pruebas (`.4`,
   `TestElFyneDoDeOtraRutinaEsperaAlTest` en `apps/fyne/ui`). Sin ellos vuelven los
   rojos que dependen de la semilla de `-shuffle`.

## Texto de la PR (en inglés, para pegar tal cual)

**Title:** `feat: receive URLs opened by the operating system`

**Body:**

> Fyne applications can open a URL (`App.OpenURL`), but they cannot _receive_ one. There
> is no way to be told that the system opened the app with `myapp://…`, so custom-scheme
> deep links and, more importantly, the OAuth2 redirect required by RFC 8252 §7.1 on
> mobile cannot be completed from a Fyne app. This is issue #1847, and #5812 asked for
> the same thing.
>
> This adds the smallest API that covers it and nothing else:
>
> ```go
> // URLHandler is implemented by applications that can receive URLs opened by
> // the operating system.
> type URLHandler interface {
>     SetOnOpenURL(func(*url.URL))
> }
> ```
>
> It is a separate optional interface rather than a new method on `fyne.App`, so
> third-party `App` implementations keep compiling. Applications that never call
> `SetOnOpenURL` are unaffected: 235 added lines, no lines removed or changed in
> existing behaviour.
>
> Platform bridges:
>
> - **Android** — the launch intent and `onNewIntent` (`GoNativeActivity`). Apps that
>   want this must declare `android:launchMode="singleTask"` and an
>   `intent-filter` with their scheme.
> - **iOS** — the launch URL and `application:openURL:options:`.
> - **macOS** — the `kAEGetURL` Apple Event, registered on start.
> - **Linux and Windows** — the URL arrives as a command-line argument, which is what
>   `x-scheme-handler` and the protocol registry pass; non-URL arguments are ignored.
>
> URLs that arrive before the application has registered its handler are retained and
> delivered afterwards, in the graphical context: on mobile the app is usually being
> _started_ by the URL, so without that the first (and only) delivery is lost.
>
> `internal/urlhandler` holds the queue and has a unit test; it has no dependency on the
> public `fyne` package so the mobile and desktop drivers can use it without an import
> cycle.
>
> This comes out of a thirteen-app Fyne suite where every app has its own scheme and
> the apps open each other's documents; without it, the mobile login cannot complete at
> all. Happy to split the mobile bridges into a second commit, or to rename the
> interface, if that suits the review better.

# El parche de texto soltado, en el fork de GLFW

`apps/fyne/go.mod` reemplaza también `github.com/go-gl/glfw/v3.4/glfw` por
`github.com/thinkddr/glfw/v3.4/glfw`, con la versión indicada arriba. El tag
`v0.1.0-sytue.3` (rama `sytue-3`, commit
`a55430400fa2b9bdefa234806fe15dbd6b87d037`, encima de `2a407d02d01a`, que es justo el
que requería Fyne v2.8.1) es su base histórica. Antes: `v0.1.0-sytue.2` (`3ac371eb6815`) sin el aviso de
arrastre, y `v0.1.0-sytue.1` (`0806cc00ff97`), solo X11 y Wayland. El CI descarga el
repositorio con `.github/actions/toolchain/forks.sh`, como los otros dos.

**Por qué.** GLFW solo acepta FICHEROS soltados, y la web acepta también texto y enlaces
(el cuaderno de Notepads crea con ellos una nota). En X11 y en Wayland el protocolo lo
atiende la conexión de GLFW —el XDnD en `x11_window.c`, el `wl_data_device` en
`wl_window.c`— y desde fuera no hay forma de verlo: los mensajes nunca salen de GLFW.
Así que se parchea su C, y macOS y Windows van en el mismo parche aunque ahí sí se
podría llegar desde fuera (`RunNative`): desde fuera habría que cambiar en tiempo de
ejecución un método de la clase privada `GLFWContentView` de Cocoa, y en Windows un
segundo destino OLE fuera de GLFW se llevaría los ficheros que hoy entrega su
`WM_DROPFILES`, y habría que volver a entregarlos por otro camino. Dentro de GLFW todo
llega por la misma retrollamada y los ficheros no cambian de camino.

**Qué hace** (`glfw-0001-drop-text.patch`, los tres commits, en inglés, como GLFW):

- `glfwSetDropTextCallback`: una ventana con esa retrollamada acepta además texto
  (`text/plain;charset=utf-8`, `UTF8_STRING` o `text/plain`) y URL que no son ficheros.
  Una ventana sin ella se comporta EXACTAMENTE como antes.
- Una lista de URI se reparte: los `file:` siguen a la retrollamada de rutas de siempre
  —Fyne los entrega por `SetOnDropped` sin cambiar nada— y el resto va a la nueva.
- macOS: la vista se registra también para cadenas y, si no hay ficheros, entrega las
  URL (`NSURL`) o el texto (`NSPasteboardTypeString`). Sin retrollamada de texto, un
  arrastre sin URL se rechaza en `draggingEntered:`, como antes.
- Windows: un `IDropTarget` de OLE en cada ventana. Una ventana registrada para OLE deja
  de recibir `WM_DROPFILES` del Explorador, así que el destino entrega también los
  ficheros (`CF_HDROP`) por el mismo camino que `WM_DROPFILES`; los enlaces con
  `UniformResourceLocatorW` y el texto con `CF_UNICODETEXT`. Si el hilo ya estaba en el
  apartamento multihilo, `OleInitialize` falla y la ventana se queda con
  `WM_DROPFILES`, como sin el parche.
- En Go, `glfw.SetDropTextCallback` pone una retrollamada para TODAS las ventanas,
  presentes y futuras, porque Fyne no deja ver su `*glfw.Window`; `apps/fyne/soltar`
  reconoce la ventana por su identificador nativo.
- Dos arreglos en el mismo camino: X11 ya no llama a la retrollamada de rutas con cero
  rutas (salta la aserción de `_glfwInputDrop`), y el `XdndEnter` de Wayland ya no
  desreferencia una ventana nula cuando lo soltado cae en una superficie ajena.
- Desde `.3`, el aviso de arrastre, para el «Suelta para añadir» de Photos:
  `glfwSetDragCallback` dice cuándo un arrastre de FICHEROS entra en la ventana y cuándo
  deja de estar encima (sale, se cancela o se suelta, y en ese caso antes de entregar
  las rutas), una vez por cambio. X11: `XdndEnter` con `text/uri-list`, `XdndLeave` y
  `XdndDrop`; Cocoa: `draggingEntered:` con `NSPasteboardTypeFileURL`,
  `draggingExited:`, `draggingEnded:` y el principio de `performDragOperation:`; Win32:
  `DragEnter` con `CF_HDROP`, `DragLeave` y `Drop`. En Go, `glfw.SetDragCallback`, y en
  la suite `soltar.AlArrastrar`. Límites: en X11 una lista de URI cuenta antes de saber si
  trae ficheros o enlaces (el protocolo no lo dice hasta soltar); Wayland no avisa. El
  primer borrador lo dejó el agente de SyPhotos; se ajustó a lo aprobado (solo ficheros,
  X11 en `XdndEnter`, sin Wayland).

**Probado.** X11, de verdad: `apps/fyne/soltar/soltar_x11_test.go` arranca Xvfb, abre una
ventana de Fyne y le suelta texto, un enlace y un fichero con un origen XDnD escrito para
la prueba (`testdata/origen.c`). Con los backends sin parchear, rojo: el texto se rechaza
en el `XdndStatus` y el enlace llega a `SetOnDropped` como un fichero inventado
(`file:///…/https:/sytue.eu/notes%3Fx=1`). El aviso de arrastre, en la misma prueba: un
fichero da «entra», «sale» y las rutas, en ese orden; uno que se va sin soltar
(`origen --salir`), «entra» y «sale»; un texto, nada. Rojo sin las tres llamadas de
X11: no llega ningún aviso. Wayland compila y está revisado a mano, pero no se ha visto
soltar nada: haría falta un compositor sin pantalla con un puntero virtual que empiece
el arrastre.

Windows: compila con cgo para amd64 (mingw-w64) y arm64 (llvm-mingw), y se ha visto con
Wine 8.0 (arm64) en el contenedor de los agentes: el XDnD de Wine termina en el
`IDropTarget` de la ventana, y el mismo origen de la prueba de X11 suelta texto (llega
entero y en su sitio, pasado a coordenadas de la ventana) y un fichero (llega por la
retrollamada de rutas, `Z:\tmp\acta.docx`). Dos límites del entorno, no del parche: el
arnés es C sin Go, porque Wine 8.0 en arm64 entra en `IDropTarget::Drop` sin restaurar
`x18` (el TEB) y el runtime de Go cae al leerlo —en Windows `x18` es siempre el TEB—; y
el enlace no se puede probar ahí, porque Wine convierte toda `text/uri-list` en
`CF_HDROP` y nunca ofrece `UniformResourceLocatorW`. El aviso de arrastre, con el mismo
arnés: el fichero da «entra», «sale» y las rutas; el que se va, «entra» y «sale»; el
texto, nada. Rojo sin las tres llamadas de Win32. (El arnés imprime con `fputs`: el
`ucrtbase` de Wine 8 en arm64 lee mal los argumentos variables de `printf`.)

macOS: `apps/fyne/soltar/testdata/cocoa.m` se ejecuta en un Mac, sin nadie delante:
crea una ventana de GLFW oculta y le manda a su vista lo que mandaría AppKit
(`draggingEntered:`, `draggingExited:`, `draggingEnded:`, `performDragOperation:`) con un
`NSDraggingInfo` falso sobre un `NSPasteboard` de verdad. Pasa entero: un fichero avisa y
llega por la retrollamada de rutas; el que se va avisa y no entrega; un texto y un
enlace llegan en su sitio (40, 40) sin avisar de arrastre; sin retrollamada de texto, un
texto se rechaza como antes del parche. Rojo con dos sabotajes: sin la condición de
`NSPasteboardTypeFileURL` el texto y el enlace encienden el aviso, y sin el aviso del
principio de `performDragOperation:` el «sale» llega después de las rutas. Lo que no
prueba es el arrastre de un ratón de verdad desde el Finder. `go vet` del fork en un Mac
compila además el Objective-C contra el SDK de Apple.

**Para quitarlo**: que GLFW acepte texto soltado (la PR la abriría la cuenta del dueño
en GitHub, como la de Fyne), que go-gl lo exponga, y borrar el `replace`, `glfw` de
`forks.sh`, este apartado y el parche.

## `v2.8.1-sytue.5` (2026-09-24): capas no modales y foco acotado

Commit `faf352b9c` de la rama `sytue`, encima de `.4` (`e337b0f2b`); el parche exportado es
**`0003-taps-and-wheel-through-overlays-and-focus-scopes.patch`** (7 ficheros, 302 líneas añadidas). Tres
interfaces OPCIONALES, así que no cambia nada para quien no las declare:

- `TapThrough() bool` en una capa: el clic o el toque que la cierra llega también a lo que tapaba (un menú
  que se cierra al apretar, no un diálogo modal). En glfw, `processMouseClicked` vuelve a despachar ese clic;
  en el móvil se reenvía el toque.
- `ScrollThrough() bool` en una capa: la rueda desplaza lo que hay debajo y la capa se refresca después,
  para que un menú siga a lo que se movió.
- `FocusScope() fyne.CanvasObject`: mientras devuelve un subárbol, el Tab no sale de él (el cajón móvil).

Con él, `apps/fyne/ui` retira el clic cruzado y el Tab del cajón que se hacían a mano. Candidato a
proponerse aguas arriba junto con el de URL entrantes.

## `v2.8.1-sytue.6` (2026-09-25): varios ficheros en el móvil, tamaño de un `content://` y recortes anidados

Etiqueta anotada sobre `cbbc17d78` de la rama `sytue-transversal-2`, encima de `.5` (`faf352b9c`); el parche
exportado es **`0004-multiple-file-pick-content-size-and-nested-clips.patch`** (11 ficheros, 192 líneas
añadidas y 10 borradas). Dos commits:

- `6bd31abe9`: `dialog.ShowFileOpenMultiple` elige varios ficheros donde el selector del sistema deja
  (Android, `EXTRA_ALLOW_MULTIPLE` y `ClipData`); en el resto es `ShowFileOpen` con un lector en un slice.
  La URI de Android gana `Size() int64` (`OpenableColumns.SIZE`, -1 si el proveedor no lo sabe), que se
  pide con una aserción a `interface{ Size() int64 }`: no ensancha `fyne.URI`.
- `cbbc17d78`: `walkObjectTree` intersecta el recorte de un objeto con el de su padre en vez de sustituirlo,
  como ya hacía el `ClipStack` de GL. Con el pintor de software, un desplazador dentro de otro (la `Entrada`
  de un diálogo desplazado, la marca de agua de Rooms) pintaba y recibía toques fuera de él.

Con él, Drive sube desde el móvil varios ficheros a la vez y con porcentaje.

## `v2.8.1-sytue.7` (2026-09-25): el interlineado de un `Entry` multilínea

Etiqueta anotada sobre `a353ee549` de la rama `sytue-interlineado`, encima de `.6` (`cbbc17d78`); el parche
exportado es **`0005-multi-line-entry-line-spacing.patch`** (45 ficheros, 256 líneas añadidas y 125 borradas;
casi todo son las referencias de prueba de `Entry`).

`RichText` solo sumaba `SizeNameLineSpacing` entre párrafos, nunca entre las filas de un mismo segmento en
línea, que es todo el texto de un `Entry`: sus filas avanzaban lo que mide la letra, dijera lo que dijera el
tema. Ahora un `Entry` multilínea pide el hueco entre todas sus filas (`RichText.spaceRows`), y el
maquetado, el tamaño mínimo, el cursor, la selección y el toque avanzan letra + interlineado, como un
`line-height` de CSS. Un `Entry` de una línea y el resto de `RichText` no cambian.

Medido contra un `textarea` de Chromium: `text-sm` avanza 15,75 px por línea y `text-base` 19,25; con `.6`
Fyne avanzaba 13,66 y 14,70, y con `.7` lo mismo que la web. Las referencias de prueba de `Entry` con varias
filas bajan 4 px por fila (el interlineado del tema de pruebas).

## `v2.8.1-sytue.8` (2026-09-25): el pintor de software redondea las posiciones

Etiqueta anotada sobre `4d4ebf890` de la rama `sytue-redondeo`, encima de `.7` (`a353ee549`); el parche
exportado es **`0006-software-painter-rounds-positions.patch`** (18 ficheros, 59 líneas añadidas y 21
borradas; 15 son referencias de prueba).

El pintor de software pasaba también las POSICIONES por `scale.ToScreenCoordinate`, que redondea hacia
arriba: todo lo que caía en una coordenada fraccionaria salía un píxel más abajo o más a la derecha que en el
pintor de GL (`roundToPixel`, al más cercano) y que en el navegador. La ventana real ya estaba bien; las
capturas y los tests, no. Ahora las posiciones y el origen del recorte redondean al más cercano y los tamaños
siguen hacia arriba, para que nada pierda su último píxel parcial.

Medido contra Chromium con Figtree en ocho alturas fraccionarias: la línea base del texto coincide en las
ocho (con el techo fallaban tres). Las referencias que cambian mueven un texto o un borde un píxel.

## `v2.8.1-sytue.9` (2026-09-25): los Universal Links de iOS llegan al manejador de URL

Etiqueta anotada sobre `80f85dbc8` de la rama `sytue-universal-links`, encima de `.8` (`4d4ebf890`); el
parche exportado es **`0007-ios-universal-links.patch`** (1 fichero, 12 líneas añadidas).

Un enlace https asociado a la app (el de una reunión de Rooms, `rooms.<dominio>/?rooms=<id>`, con
`apple-app-site-association` y el derecho `associated-domains`) no llega por `openURL`: iOS lo entrega como
`NSUserActivity` de tipo `BrowsingWeb` a `application:continueUserActivity:restorationHandler:`, que el
delegado no implementaba, así que el enlace abría la app y se perdía. Ahora su `webpageURL` va a
`urlOpened`, lo mismo que un esquema propio, en arranque en frío y con la app abierta. Comprobado con
`clang -fsyntax-only` contra UIKit (Mac Catalyst); falta verlo en un iPhone, que pide firma y el derecho.

## `v2.8.1-sytue.10` (2026-09-25): los `fyne.Do` de antes del bucle de eventos, en el hilo principal

Etiqueta anotada sobre `ae1ae7880` de la rama `sytue-cola-temprana`, encima de `.9`; el parche exportado es
**`0008-mobile-queue-early-fyne-do.patch`** (2 ficheros, con su prueba en `driver_test.go`).

En el móvil, una rutina que terminaba ANTES de que `Run` creara la cola ejecutaba su `fyne.Do` en su propia
rutina, a la vez que el hilo principal, y reventaba en el conformador de texto (CertiPDF en el Simulador de iOS:
una petición que falla nada más arrancar). Ahora la cola existe desde `NewGoMobileDriver` y esas llamadas corren
en el hilo principal en cuanto arranca el bucle.

## `v2.8.1-sytue.11` (2026-09-25): iOS adopta el ciclo de vida por escenas

Etiqueta anotada sobre `417dce3e8`, la cabeza de `sytue-cola-temprana`; el parche exportado es
**`0009-ios-scene-lifecycle.patch`** (1 fichero, `darwin_ios.m`). Desde el 2026-09-25 la rama `sytue` del fork
apunta aquí (avance rápido desde `.5`, sin commit de fusión), así que la próxima versión sale de `sytue`.

Compiladas con el SDK de iOS 27, las apps que no adoptan `UIScene` mueren al arrancar en
`_UIApplicationEvaluateRuntimeIssueForNoSceneLifecycleAdoption` (las trece en el Simulador de iOS 27). El
delegado de la aplicación entrega cada sesión a `GoAppSceneDelegate`, dueño de la ventana, que recibe lo que las
escenas quitan al delegado: las URL entrantes (`scene:openURLContexts:` y las de `connectionOptions` en frío), los
Universal Links (`continueUserActivity`) y el paso a primer y segundo plano. Para probarlo sin cuenta de Apple:
`bash apps/fyne/scripts/empaquetar.sh iossimulator <app>`.

## `v2.8.1-sytue.16` (2026-10-04): mantenimiento de Entry y validación del fork

[Release estable](https://github.com/thinkddr/fyne/releases/tag/v2.8.1-sytue.16) y
[comparación completa con `.15`](https://github.com/thinkddr/fyne/compare/v2.8.1-sytue.15...v2.8.1-sytue.16).
El refresco de `Entry` recalcula el desplazamiento cuando Deshacer devuelve texto que cabe en el área visible.
La identidad de la caché de fuentes conserva la dirección de sus bytes sin usar `unsafe`; el tratamiento del
foco y del clic que cierra una capa se extrae en helpers sin cambiar sus condiciones. El resto son pruebas,
referencias visuales revisadas, CI y documentación del fork. No cambian el módulo canónico, sus dependencias,
`LICENSE` ni `AUTHORS`; sigue siendo BSD-3-Clause. No añade soporte de arrastrar y soltar en iOS ni cambia
los bridges nativos. Las pruebas de software no sustituyen una prueba en ventana o dispositivo.

## `v2.8.1-sytue.15` (2026-09-28): una tecla soltada ya no se escribe otra vez en Android

`.14` más `d943ebc78` («fix(mobile/android): map key event actions, not key states, to directions»), en la
rama `sytue-teclado-android`. `processKey` comparaba `AKeyEvent_getAction` —`AKEY_EVENT_ACTION_DOWN` (0) y
`_UP` (1)— con `AKEY_STATE_DOWN` (1) y `_UP` (0), que son estados de tecla y valen al revés: cada tecla
soltada llegaba como pulsada. Con el teclado en pantalla abierto el IME escribe la pulsación por el
`TextWatcher` y la suelta volvía como una segunda pulsación, así que un teclado físico (o `adb shell input
text`) escribía cada letra dos veces («MMuuyy bbuueennoo», medido en el emulador de la API 36). Parche
exportado: **`0013-android-key-direction.patch`**; `TestKeyDirection` falla con la comparación vieja.

## `v2.8.1-sytue.14` (2026-09-28): una fuente del tema se reconoce por sus bytes

`.13` más `bec3cd530` («painter: identify a theme font by its bytes, not by the Resource value»): la caché de
fuentes de `.13` usaba como clave el `fyne.Resource` que da el tema, y dos recursos distintos con los mismos
bytes contaban como dos fuentes. Es la versión de la release. Parche exportado:
**`0012-font-cache-keyed-by-bytes.patch`** (`internal/painter/font.go` y su prueba). Un `Resource` de tipo no
comparable además reventaba como clave de mapa; `TestCachedFontFace_FreshResourceSameBytes` y
`TestCachedFontFace_ResourceNotComparable` fallan con `.13`.

## `v2.8.1-sytue.13` (2026-09-28): la memoria de lo que se tematiza y el alfa del framebuffer

Etiqueta anotada sobre `5a25290db` de la rama `sytue-13` de `github.com/thinkddr/fyne`, encima de `.12`
(`b2a8bdfcb`). Dos parches, exportados juntos en **`0011-theme-override-memory-and-opaque-framebuffer.patch`**:

- **`77c580ac4` — memoria.** Todo objeto puesto bajo un `ThemeOverride` quedaba vivo hasta el final del
  proceso: el tema que se guarda contra cada objeto apuntaba al contenedor (que contiene esos objetos), el
  mapa de ámbitos guardaba los objetos mismos, y la caché de fuentes iba por ámbito, así que cada override
  nuevo volvía a analizar texto, emoji y símbolos y se quedaba la copia. Medido con los tests de Mail: al
  acabar seguían vivas las 78 pantallas (1 GB, +180 MB por cada `-count`, y el OOM con `-race -count=20`);
  con el parche queda una y 44 MB, y no crece. En el recorrido de calendario (55 modales, `-race`), el heap vivo al
  acabar baja de 1553 MB a 168 MB y el RSS máximo de 8,4 GB a 1,1 GB. Guardianes `TestThemeOverride_DoesNotRetainContent` y
  `TestCachedFontFace_SharedAcrossOverrideScopes`, vistos en rojo con cada cambio deshecho.
- **`5a25290db` — alfa opaco.** `BlendFuncSeparate(src, dst, ONE, dst)`: el color con los factores de
  siempre y el alfa acumulando hacia 1. Con `srcAlpha/oneMinusSrcAlpha` también en el alfa, cada borde
  suavizado dejaba alfa < 1 sobre color no premultiplicado y KWin, que compone premultiplicado, pintaba
  flecos de color en todo borde redondeado. Comprobado en un Plasma real; por eso se quita el forzado de
  XWayland en KDE (`apps/fyne/sesion/plataforma_linux.go`).

La suite del fork falla lo mismo que con `.12` (las imágenes de referencia, en ARM64), `go vet` pasa con y sin
`gles`.

## `v2.8.1-sytue.12` (2026-09-26): macOS entrega los documentos abiertos

Etiqueta anotada sobre `b2a8bdfcb` de la rama `sytue-abrir-documentos`, encima de `.11` (`417dce3e8`); el parche
exportado es **`0010-macos-open-documents.patch`** (2 ficheros, `app/incoming_urls_darwin.{go,m}`, 39 líneas
añadidas).

El Finder, `open fichero` y «Abrir con» no pasan argumentos: mandan el Apple Event `kAEOpenDocuments` con una
lista de ficheros, y el fork solo atendía `kAEGetURL`. Ahora cada fichero entra como `file://` por el mismo
`urlhandler`. El manejador se instala en `applicationWillFinishLaunching` (y además al registrar, por si ya
terminó de arrancar): AppKit pone el suyo al terminar de arrancar y pisaría uno instalado antes, y los
documentos del arranque llegan justo después. Es lo que necesitan los `CFBundleDocumentTypes` de #1128.

## `0014-ios-document-picker-import-mode.patch`: selector de documentos en iOS sin iCloud

Exportado en **`0014-ios-document-picker-import-mode.patch`** (2 ficheros, `darwin_ios.m` y `file_ios.m`).

`UIDocumentPickerModeOpen` y `UIDocumentPickerModeMoveToService` exigen el derecho de contenedor de iCloud
(`com.apple.developer.icloud-container-identifiers`). Al invocarse en una app sin ese derecho (todas las
apps que abren ficheros de terceros), UIKit lanza `NSInternalInconsistencyException` y mata el proceso.
Cambiando a `UIDocumentPickerModeImport` y `UIDocumentPickerModeExportToService`, el selector del sistema
permite abrir y guardar ficheros desde Archivos, iCloud Drive, proveedores en la nube y almacenamiento local
sin exigir el derecho de iCloud. Además, se añade `startAccessingSecurityScopedResource` para que la lectura
de URL de sandbox funcione con seguridad y sin violaciones de acceso.
