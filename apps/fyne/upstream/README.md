<a id="el-parche-de-url-entrantes-listo-para-enviar-a-fyne"></a>
# The incoming URL patch, ready to send to Fyne

`apps/fyne/go.mod` replaces Fyne with `github.com/thinkddr/fyne/v2 v2.8.2-sytue.8`
(hosted on GitHub since 2026-09-27; previously on git.gay, with the same branches and tags)
(the «URL entrante» (incoming URL) ADR in `docs/decisions.md`). A fork is debt: this folder holds what
is needed to remove it, which is getting the patch accepted upstream.

<a id="bucle-por-eventos-en-escritorio-2026-11-17-v282-sytue7"></a>
## Event-driven loop on desktop (2026-11-17, `v2.8.2-sytue.7`)

`0018-desktop-event-driven-loop.patch`: commit
[`caa879f`](https://github.com/thinkddr/fyne/commit/caa879f9051c9c0ba6b82662ce50c64a00b921d0),
tagged `v2.8.2-sytue.7` on the fork's `main` on top of `v2.8.2-sytue.6`, which in turn brings
`RenderOffscreen` from the software driver. `apps/fyne/go.mod` has used it since the same day, and
with it `ci-desktop.yml`, `ci-mobile.yml`, `.github/tests/fyne-cli-test.sh` and
`scripts/empaquetar-test.sh`.

`v2.8.2-sytue.7` was tagged before Static Analysis finished, and it came out red because of
a single style warning (staticcheck ST1022: the comment on `WakeHook` must start with
its name). It does not change behavior: the other four workflows are green. The
fix, [`715ab78`](https://github.com/thinkddr/fyne/commit/715ab788ac20410c29cfe25547d13be9c0052a64),
is `v2.8.2-sytue.8`, the version `apps/fyne/go.mod` uses.

**What it fixes.** The GLFW driver handled input and painted on a fixed 60 Hz clock.
Every key, click or network result (`fyne.Do`) waited for the next tick: 8 ms on average
and up to 17 ms before being painted. In addition, an idle app woke up 60 times per
second. Now the loop sleeps in `glfw.WaitEventsTimeout` and is woken by input,
`fyne.Do` (`glfw.PostEmptyEvent`), a `Refresh` (`Canvas.SetDirty`) or an animation.
It paints immediately if the last frame is at least one interval old, and during
continuous interaction never faster than the main display (60 to 240 Hz). On
mobile the frame is requested right after the touch or key, without waiting for the clock.

**Measured** with Xvfb and Mesa, from a synthetic key press (XTest) to reading the changed
pixel on the X server, with 150 samples:

| | Before | After |
| --- | --- | --- |
| Key → pixel (mean / p90 / max.) | 10.5 / 16.7 / 18.8 ms | 1.8 / 2.0 / 4–7 ms |
| `fyne.Do` → pixel (mean) | 10.7 ms | 1.6 ms |
| CPU of an idle window | 35 ms every 2 s | ~1 ms every 2 s |

The fork's `go test -race ./...` passes in full (desktop and `mobile` tags). The new
tests cover the wake-up and the wait calculation: `TestRunOnMainWakesIdleLoop` fails
(~950 ms per call) if the wake-up is removed. The apps in `apps/fyne` build against it
(`go vet -tags goolm ./...`) and their `ui`, `calcus`, `notes` and `photos` tests pass.

<a id="v282-sytue4-y-v282-sytue5-2026-11-07-refresco-y-avisos-de-fondo-en-ios"></a>
## `v2.8.2-sytue.4` and `v2.8.2-sytue.5` (2026-11-07): background refresh and notifications on iOS

Both tags point to the same source,
[`5438e4c86769e1f5936adacedb29c4436d15cbdc`](https://github.com/thinkddr/fyne/commit/5438e4c86769e1f5936adacedb29c4436d15cbdc):
`.4` is the one that brings `0016` working and `.5` adds `0017`. Their fixes are already included in
`0016-ios-background-refresh.patch` and `0017-ios-remote-notifications.patch`, regenerated
with `git format-patch` on top of `v2.8.2-sytue.3`. Applied with `git am`, they give the tree of the
tagged source except `.github/ios-geometry/run.sh`, which is the fork's CI and does not go
upstream. The matrices on the patches as they were turned up two failures:

- **The app died on startup on iOS** (`GeometryUITests`: the report did not appear; with the
  new diagnostics in `run.sh`, `SIGSEGV` in `runApp`). cgo compiles `darwin_ios.m`
  **without ARC**. A normal startup connects the scene without attaching and defers the connection, and
  `sceneWillEnterForeground` released the options before using them. Now they are released
  afterwards, and `sceneDidBecomeActive` also connects a pending scene.
- **Windows**: the `.m` is extracted with CRLF and the source contracts cut at -1. Now
  they read with LF, fail naming the missing marker, and `.gitattributes` forces LF on
  `*.m`, `*.h` and `*.c`.

In addition, `didReceiveRemoteNotification:fetchCompletionHandler:` is only added to the delegate
(in `runApp`) if the `Info.plist` declares `remote-notification`: previously UIKit warned in
every app. And the `completionHandler` is no longer leaked (it was retained twice).

Passed, on the tagged source:

- [Web](https://github.com/thinkddr/fyne/actions/runs/42379644977)
- [Static Analysis](https://github.com/thinkddr/fyne/actions/runs/42379645045)
- [Mobile](https://github.com/thinkddr/fyne/actions/runs/42379645039)
- [Platform](https://github.com/thinkddr/fyne/actions/runs/42379645027), with Windows
- [CodeQL](https://github.com/thinkddr/fyne/actions/runs/42379644935)
- [Fork conformance](https://github.com/thinkddr/fyne/actions/runs/42379645040): the
  UIKit driver compiled and linked for device and simulator, and the app started in the
  iPhone and iPad simulators with its 32 measurements and taps.

They do not prove a real refresh task or a real APNs notification (the simulator does not trigger them),
nor a physical iPhone, nor App Review.

<a id="refresco-en-segundo-plano-de-ios-2026-11-07-v282-sytue4"></a>
## iOS background refresh (2026-11-07, `v2.8.2-sytue.4`)

`0016-ios-background-refresh.patch` adds the optional interface
`mobile.BackgroundDriver`, which the mobile driver implements. It serves the native widget with the
app closed ([amendment to ADR 0078](../../../docs/decisions.md#0078)):

- `LaunchedInBackground`: UIKit launched the process without a scene (refresh task or background
  notification).
- `OnFirstScene`: whatever needs UI runs when the first scene connects, on the
  main thread and before that scene's URLs; outside iOS, immediately. If UIKit
  connects the scene outside the foreground (it can do so during background work),
  that and its URLs wait for `sceneWillEnterForeground`: with nobody looking, the lock's Face ID
  would fail and the app would exit.
- `ProtectedDataAvailable`: false with the device locked. Each task asks UIKit again
  (`isProtectedDataAvailable`, on the main thread) before running the
  handler: the delegate's notifications can reach a suspended app late.
- `OnBackgroundRefresh`: the handler of the task named by the first entry of
  `BGTaskSchedulerPermittedIdentifiers`. It is registered before launch finishes, it is
  requested again when moving to the background and on completion, and it completes only once: if
  the system cuts it short, the context is cancelled and it fails as soon as the handler returns, or after
  500 ms if it does not return; and if it is cut short before the handler starts (the main
  thread busy), it fails immediately and the handler does not run: Go knows about it from the moment
  iOS delivers it, before setting its `expirationHandler`. That margin is so that it releases the `ajustes` lock (a
  `flock` in the App Group): iOS kills with 0xdead10cc any app that suspends while holding a
  file lock in a shared container.

An app without those keys in its `Info.plist` behaves as before. The portable part
has tests on every platform; the Objective-C bridge is pinned with source
contracts, because it does not compile outside Xcode. Checked here: `go vet` and
`go test` of `driver/mobile`, `internal/driver/mobile` and `internal/driver/mobile/app` on
Linux, and that `apps/fyne/sesion` compiles against the patched fork with an interface identical
to its own. The UIKit matrix (above) compiles the `.m` and starts it in the simulator; it has not
been run on an iPhone.

The suite uses it through an assertion on the driver (`sesion/fondo.go`). **Switched on**
on 2026-11-07: the `replace` in `apps/fyne/go.mod` uses `v2.8.2-sytue.5`, and in the same commit
the references in `ci-desktop.yml`, `ci-mobile.yml`, `empaquetar-test.sh`,
`BACKLOG-FYNE.md` and `docs/CONVENTIONS.md` were bumped and `SYTUE_REFRESCO_APPLE = YES` was set in
`apps/fyne/xcode/Sytue.xcconfig`. That is also the path for a future tag.

<a id="avisos-silenciosos-de-ios-2026-11-07-v282-sytue5"></a>
## iOS silent notifications (2026-11-07, `v2.8.2-sytue.5`)

`0017-ios-remote-notifications.patch` goes **on top of `0016`** and adds the optional
interface `mobile.RemoteNotificationDriver`. It serves to wake the widget with the app
closed: the Hub sends an empty APNs notification when something in the product changes
([amendments to ADR 0039 and 0078](../../../docs/decisions.md#0039)):

- `RemoteNotificationsAvailable`: the `Info.plist` declares `remote-notification` in
  `UIBackgroundModes`. It can be read from `main`, before UIKit starts.
- `RegisterForRemoteNotifications`: asks UIKit for the device token, without a permission
  dialog (a notification without an alert does not need one), and delivers it in lowercase hexadecimal to
  everyone waiting for it, or UIKit's error. Concurrent requests share a single one.
- `OnRemoteNotification`: the handler of
  `application:didReceiveRemoteNotification:fetchCompletionHandler:`. The payload is never
  read. The handler receives a context that is cancelled after 25 s, and the
  `completionHandler` is called only once: with new data, no data or failure depending on
  what it returns, failure if the context was cancelled, or failure after a 500 ms grace period if the
  handler does not return (iOS kills the app that does not call it). Before running the handler,
  UIKit is asked, on the main thread, whether protected data is available. Without a
  handler, the notification fails immediately.

An app without that background mode behaves as before. As with `0016`, the portable state
machine has tests on every platform and the Objective-C bridge is pinned
with source contracts. Checked here: `go vet` and `go test` of `driver/mobile`,
`internal/driver/mobile` and `internal/driver/mobile/app` on Linux on top of `0016`, and that
`apps/fyne/sesion` compiles against the fork with an interface identical to its own
(`conductorAvisos`, `sesion/despertar.go`). The UIKit matrix compiles the `.m`; no
notification has been received on an iPhone.

The suite uses it through an assertion on the driver. The fork already includes it: `apps/fyne/go.mod` has used
`v2.8.2-sytue.5` since 2026-11-07. `SYTUE_DESPERTAR_APPLE = YES` since the same day (the owner's decision); the owner's
steps from the [runbook](../../../docs/operations.md#despertar-widgets-ios) are still missing: the key, the
Push capability on the fifteen App IDs, the regenerated profiles and the Hub's Secret.
`empaquetar-test.sh` does not allow switching it on with `.4` or earlier.

<a id="ciclo-completo-del-toque-móvil-2026-11-02"></a>
## Full mobile touch cycle (2026-11-02)

The tag [`v2.8.2-sytue.3`](https://github.com/thinkddr/fyne/tree/v2.8.2-sytue.3)
adds the optional interface `mobile.TouchObserverCanvas`. Its observer receives
the complete physical cycle even if a `TouchFilter` has returned `FilterPass`;
observing neither captures nor delays the events. This makes it possible to show the pressed
state without preventing a list from scrolling.

When the content, the layers, the geometry or the observer change, the cancellation
is delivered before the driver's `DragEnd`. The suite uses it to discard
provisional movements, without persisting them as if the user had released.
The observed paths do not retain the previous object tree.

The tagged source is `b3cbd9833ac5018dd93e0789fd4f50d24da9be07`.
The Web, Mobile, Platform, static analysis and CodeQL matrices passed,
as well as [Fork conformance](https://github.com/thinkddr/fyne/actions/runs/41372869526)
with UIKit on iPhone and iPad. The driver tests cover `FilterPass`, replay,
multi-touch and the cancellation order; they do not prove use on a physical device.

<a id="geometría-de-ios-y-actualización-a-fyne-282-2026-10-25"></a>
## iOS geometry and update to Fyne 2.8.2 (2026-10-25)

The [stable release `v2.8.2-sytue.2`](https://github.com/thinkddr/fyne/releases/tag/v2.8.2-sytue.2)
starts from Fyne 2.8.2 and keeps all the changes from `.21`. The window follows the size
and scale of the UIKit view it is attached to, including its safe area insets when
rotating and when its viewport shrinks. Go keeps the already-oriented pixels; touches and the
keyboard rectangle are converted to the view's space. A floating keyboard or one
detached from the bottom edge does not shrink the whole canvas.

`0015-ios-window-geometry.patch` contains only the driver change and its portable
tests on top of `v2.8.2-sytue.1`. The BSD-3-Clause notices of the original and the Apache-2.0 notices of
the modifications are kept. The suite also pins typesetting `v0.3.5-sytue.1`
and GLFW `v0.1.0-sytue.4`, rebases of the originals 0.3.5 and 0.1.0-pre.2, and updates the
inventory of originals that OSV queries.

The final source passed the fork's five matrices. With Xcode 27.0 and Go 1.27.3 the
driver was compiled and linked for iOS arm64 device and simulator. The
iPhone 18 Pro and 13-inch iPad Pro simulators went through four rotation states and
four view sizes/positions per state: 32 measurements and 32 correct UIKit
taps. The harness disables UIKit animations and states so in its reports;
the render loop and the production driver do not change. In addition,
24 portable geometry/keyboard cases were verified. Stage Manager gestures
and running on a physical device are not automated. The clean install of the tag passed in
[`public-install`](https://github.com/thinkddr/fyne/actions/runs/40198336656/job/121183936450):
Go 1.27.3, download of the published module, verified sums and a clean external build.

<a id="actualización-a-fyne-282-2026-10-23"></a>
## Update to Fyne 2.8.2 (2026-10-23)

The fork integrates the stable upstream version while keeping the suite's incoming URLs,
mobile gestures and optional painters. It includes the fixes
for the iOS keyboard and draw cycle, the read permissions of the Android
picker and the coordinates when dropping files on desktop. The picker test
runs the real Java and its JNI callback, with multiple selection and providers that
reject the persistable permission.

The real bases of the replacements are also updated: typesetting
`v0.3.5-sytue.1` keeps fractional measurements and declares the canonical module;
GLFW `v0.1.0-sytue.4` incorporates the `v0.1.0-pre.2` base and keeps the text,
link and file drag callbacks. Their previous tags remain intact. The
suite downloads these two forks directly; a repository's public visibility
and `GOPRIVATE` are different properties.

The visual reference of the submenu is updated because of upstream's adjustment to the canvas
height. The pixel comparison keeps the root labels;
it did not lead to any additional layout or text shaping change. The
gesture and rendering contracts keep their checks and thresholds.

<a id="gestos-móviles-y-mantenimiento-del-fork-2026-10-23"></a>
## Mobile gestures and fork maintenance (2026-10-23)

The tag `v2.8.1-sytue.21` adds `mobile.TouchFilterCanvas`, an optional interface that
makes it possible to decide on a gesture before delivering the touch to rows or editors.
It replays the rejected events with their original timings, keeps multi-touch
and cancels when the root, a layer, the geometry or the filter changes. iOS delivers
`touchesCancelled` and keeps each finger's identifier; Android distinguishes
`ACTION_CANCEL` from a completed press. `fyne.Canvas` keeps its contract.

It includes the earlier changes to the image painter, the synchronization of the
cache expiry and the optional text and rectangle contracts of the software
painter. [The comparison with `.16`](https://github.com/thinkddr/fyne/compare/v2.8.1-sytue.16...v2.8.1-sytue.21)
contains the full change. The rectangle version documented as `.20` has not
been re-tagged.

The source `bf85664117f0a3acc2cf3388e07fd7dee32a3d38` passed the fork's five
matrices: race, analysis, platforms, web and mobile. Its download was verified on
Scaleway through `proxy.golang.org` and `sum.golang.org`, without excluding the module from
public verification. The suite pins the tag and the two sums next to their packaging
guards. The driver tests do not prove physical interaction with the
gestures reserved by each operating system.

The original code keeps BSD-3-Clause. The fork's current modifications
are published under Apache-2.0; the fork's license notice keeps the attribution
to upstream. The historical URL patch in this folder keeps its license.

<a id="parche-histórico-de-url-entrantes"></a>
## Historical incoming URL patch

- **`0001-receive-incoming-urls.patch`** — the patch, exported from commit `2430cc176a`
  on the fork's `sytue` branch. 13 files, **235 lines added and none deleted**:
  it does not change the behavior of anyone who does not ask for it. It includes its test
  (`internal/urlhandler/urlhandler_test.go`) and is written in English, like the rest
  of the project.
- The patch code belongs to Fyne and is released **under Fyne's BSD-3-Clause**, not under the
  license of the rest of this repository.

<a id="qué-falta-para-abrirla-y-por-qué-no-está-abierta"></a>
## What is missing to open it, and why it is not open

The PR has to be opened by **the owner's personal GitHub account**: an upstream PR
is signed by a person, not by an agent. Everything else is done.

Steps, when it is opened:

1. `git clone https://github.com/fyne-io/fyne && cd fyne && git switch develop`
   — Fyne integrates through `develop`; the patch was exported on top of `v2.8.1`, so it may
   need a trivial rebase in `app/app.go`.
2. `git am ../0001-receive-incoming-urls.patch` and `go test ./...`.
3. Link the existing issue **[#1847 “Register URL Protocol
   Handler”](https://github.com/fyne-io/fyne/issues/1847)** (open, it is exactly this) and the
   closed **[#5812 “Write apps that can handle URL
   schemes”](https://github.com/fyne-io/fyne/issues/5812)**. There is no open PR
   that solves it (search of 2026-09-22 in `fyne-io/fyne`).
4. When it is accepted: delete the `replace` in `apps/fyne/go.mod`, the four workflow
   guards that require it and this folder. **Before that**, check that Fyne also includes
   the fork's other two patches, which are not about URLs: the renderer cache
   expiry (`.3`) and the test driver queue (`.4`,
   `TestElFyneDoDeOtraRutinaEsperaAlTest` in `apps/fyne/ui`). Without them, the
   reds that depend on the `-shuffle` seed come back.

<a id="texto-de-la-pr-en-inglés-para-pegar-tal-cual"></a>
## PR text (in English, to paste as is)

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

<a id="el-parche-de-texto-soltado-en-el-fork-de-glfw"></a>
# The dropped-text patch, in the GLFW fork

`apps/fyne/go.mod` also replaces `github.com/go-gl/glfw/v3.4/glfw` with
`github.com/thinkddr/glfw/v3.4/glfw`, at the version given above. The tag
`v0.1.0-sytue.3` (branch `sytue-3`, commit
`a55430400fa2b9bdefa234806fe15dbd6b87d037`, on top of `2a407d02d01a`, which is exactly the
one Fyne v2.8.1 required) is its historical base. Before: `v0.1.0-sytue.2` (`3ac371eb6815`) without the
drag notification, and `v0.1.0-sytue.1` (`0806cc00ff97`), X11 and Wayland only. The CI downloads the
repository with `.github/actions/toolchain/forks.sh`, like the other two.

**Why.** GLFW only accepts dropped FILES, and the web also accepts text and links
(the Notepads notebook creates a note from them). On X11 and on Wayland the protocol is
handled by GLFW's connection —XDnD in `x11_window.c`, `wl_data_device` in
`wl_window.c`— and there is no way to see it from outside: the messages never leave GLFW.
So its C is patched, and macOS and Windows go in the same patch even though there it
would be possible to reach it from outside (`RunNative`): from outside you would have to change at
runtime a method of Cocoa's private `GLFWContentView` class, and on Windows a
second OLE target outside GLFW would take the files that its
`WM_DROPFILES` delivers today, and they would have to be delivered again by another path. Inside GLFW everything
arrives through the same callback and the files do not change path.

**What it does** (`glfw-0001-drop-text.patch`, the three commits, in English, like GLFW):

- `glfwSetDropTextCallback`: a window with that callback also accepts text
  (`text/plain;charset=utf-8`, `UTF8_STRING` or `text/plain`) and URLs that are not files.
  A window without it behaves EXACTLY as before.
- A URI list is split: the `file:` ones go on to the usual paths callback
  —Fyne delivers them through `SetOnDropped` without changing anything— and the rest go to the new one.
- macOS: the view also registers for strings and, if there are no files, delivers the
  URLs (`NSURL`) or the text (`NSPasteboardTypeString`). Without a text callback, a
  drag without URLs is rejected in `draggingEntered:`, as before.
- Windows: an OLE `IDropTarget` on each window. A window registered for OLE stops
  receiving `WM_DROPFILES` from Explorer, so the target also delivers the
  files (`CF_HDROP`) through the same path as `WM_DROPFILES`; links with
  `UniformResourceLocatorW` and text with `CF_UNICODETEXT`. If the thread was already in the
  multithreaded apartment, `OleInitialize` fails and the window keeps
  `WM_DROPFILES`, as without the patch.
- In Go, `glfw.SetDropTextCallback` sets a callback for ALL windows,
  present and future, because Fyne does not expose its `*glfw.Window`; `apps/fyne/soltar`
  recognizes the window by its native identifier.
- Two fixes on the same path: X11 no longer calls the paths callback with zero
  paths (it trips the `_glfwInputDrop` assertion), and Wayland's `XdndEnter` no longer
  dereferences a null window when the drop lands on a foreign surface.
- Since `.3`, the drag notification, for Photos' «Suelta para añadir» (Drop to add):
  `glfwSetDragCallback` says when a drag of FILES enters the window and when
  it stops being over it (it leaves, is cancelled or is dropped, and in that case before delivering
  the paths), once per change. X11: `XdndEnter` with `text/uri-list`, `XdndLeave` and
  `XdndDrop`; Cocoa: `draggingEntered:` with `NSPasteboardTypeFileURL`,
  `draggingExited:`, `draggingEnded:` and the start of `performDragOperation:`; Win32:
  `DragEnter` with `CF_HDROP`, `DragLeave` and `Drop`. In Go, `glfw.SetDragCallback`, and in
  the suite `soltar.AlArrastrar`. Limits: on X11 a URI list counts before knowing whether
  it carries files or links (the protocol does not say until the drop); Wayland does not notify. The
  first draft was left by the SyPhotos agent; it was adjusted to what was approved (files only,
  X11 in `XdndEnter`, no Wayland).

**Tested.** X11, for real: `apps/fyne/soltar/soltar_x11_test.go` starts Xvfb, opens a
Fyne window and drops text, a link and a file onto it with an XDnD source written for
the test (`testdata/origen.c`). With the unpatched backends, red: the text is rejected
in the `XdndStatus` and the link reaches `SetOnDropped` as a made-up file
(`file:///…/https:/sytue.eu/notes%3Fx=1`). The drag notification, in the same test: a
file gives “enter”, “leave” and the paths, in that order; one that goes away without being dropped
(`origen --salir`), “enter” and “leave”; a text, nothing. Red without the three
X11 calls: no notification arrives. Wayland compiles and has been reviewed by hand, but nothing has been seen
being dropped: it would need a headless compositor with a virtual pointer that starts
the drag.

Windows: it compiles with cgo for amd64 (mingw-w64) and arm64 (llvm-mingw), and it has been seen with
Wine 8.0 (arm64) in the agents' container: Wine's XDnD ends up in the
window's `IDropTarget`, and the same source from the X11 test drops text (it arrives
whole and in place, converted to window coordinates) and a file (it arrives through the
paths callback, `Z:\tmp\acta.docx`). Two limits of the environment, not of the patch: the
harness is C without Go, because Wine 8.0 on arm64 enters `IDropTarget::Drop` without restoring
`x18` (the TEB) and the Go runtime crashes when reading it —on Windows `x18` is always the TEB—; and
the link cannot be tested there, because Wine turns every `text/uri-list` into
`CF_HDROP` and never offers `UniformResourceLocatorW`. The drag notification, with the same
harness: the file gives “enter”, “leave” and the paths; the one that goes away, “enter” and “leave”; the
text, nothing. Red without the three Win32 calls. (The harness prints with `fputs`: Wine 8's
`ucrtbase` on arm64 misreads `printf`'s variadic arguments.)

macOS: `apps/fyne/soltar/testdata/cocoa.m` runs on a Mac, with nobody in front of it:
it creates a hidden GLFW window and sends its view what AppKit would send
(`draggingEntered:`, `draggingExited:`, `draggingEnded:`, `performDragOperation:`) with a
fake `NSDraggingInfo` over a real `NSPasteboard`. It passes in full: a file notifies and
arrives through the paths callback; the one that goes away notifies and is not delivered; a text and a
link arrive in place (40, 40) without a drag notification; without a text callback, a
text is rejected as before the patch. Red with two sabotages: without the
`NSPasteboardTypeFileURL` condition the text and the link trigger the notification, and without the notification at the
start of `performDragOperation:` the “leave” arrives after the paths. What it does not
test is dragging with a real mouse from the Finder. The fork's `go vet` on a Mac
also compiles the Objective-C against Apple's SDK.

**To remove it**: GLFW must accept dropped text (the PR would be opened by the owner's account
on GitHub, like the Fyne one), go-gl must expose it, and then delete the `replace`, `glfw` from
`forks.sh`, this section and the patch.

<a id="v281-sytue5-2026-09-24-capas-no-modales-y-foco-acotado"></a>
## `v2.8.1-sytue.5` (2026-09-24): non-modal layers and scoped focus

Commit `faf352b9c` on the `sytue` branch, on top of `.4` (`e337b0f2b`); the exported patch is
**`0003-taps-and-wheel-through-overlays-and-focus-scopes.patch`** (7 files, 302 lines added). Three
OPTIONAL interfaces, so nothing changes for anyone who does not declare them:

- `TapThrough() bool` on a layer: the click or tap that closes it also reaches what it covered (a menu
  that closes when pressed, not a modal dialog). In glfw, `processMouseClicked` dispatches that click again;
  on mobile the tap is forwarded.
- `ScrollThrough() bool` on a layer: the wheel scrolls what is underneath and the layer is refreshed afterwards,
  so that a menu follows what moved.
- `FocusScope() fyne.CanvasObject`: while it returns a subtree, Tab does not leave it (the mobile drawer).

With it, `apps/fyne/ui` drops the pass-through click and the drawer Tab handling that were done by hand. A candidate to
be proposed upstream together with the incoming URL one.

<a id="v281-sytue6-2026-09-25-varios-ficheros-en-el-móvil-tamaño-de-un-content-y-recortes-anidados"></a>
## `v2.8.1-sytue.6` (2026-09-25): multiple files on mobile, size of a `content://` and nested clips

Annotated tag on `cbbc17d78` of the `sytue-transversal-2` branch, on top of `.5` (`faf352b9c`); the exported
patch is **`0004-multiple-file-pick-content-size-and-nested-clips.patch`** (11 files, 192 lines
added and 10 deleted). Two commits:

- `6bd31abe9`: `dialog.ShowFileOpenMultiple` picks several files where the system picker allows it
  (Android, `EXTRA_ALLOW_MULTIPLE` and `ClipData`); elsewhere it is `ShowFileOpen` with one reader in a slice.
  The Android URI gains `Size() int64` (`OpenableColumns.SIZE`, -1 if the provider does not know it), which is
  requested with an assertion to `interface{ Size() int64 }`: it does not widen `fyne.URI`.
- `cbbc17d78`: `walkObjectTree` intersects an object's clip with its parent's instead of replacing it,
  as GL's `ClipStack` already did. With the software painter, a scroller inside another one (the `Entrada`
  of a scrolled dialog, Rooms' watermark) painted and received taps outside it.

With it, Drive uploads several files at once from mobile, with a percentage.

<a id="v281-sytue7-2026-09-25-el-interlineado-de-un-entry-multilínea"></a>
## `v2.8.1-sytue.7` (2026-09-25): the line spacing of a multi-line `Entry`

Annotated tag on `a353ee549` of the `sytue-interlineado` branch, on top of `.6` (`cbbc17d78`); the exported
patch is **`0005-multi-line-entry-line-spacing.patch`** (45 files, 256 lines added and 125 deleted;
almost all of them are `Entry` test references).

`RichText` only added `SizeNameLineSpacing` between paragraphs, never between the rows of a single inline
segment, which is all the text of an `Entry`: its rows advanced by the height of the font, whatever the
theme said. Now a multi-line `Entry` asks for the gap between all its rows (`RichText.spaceRows`), and the
layout, the minimum size, the cursor, the selection and the tap advance font + line spacing, like a CSS
`line-height`. A single-line `Entry` and the rest of `RichText` do not change.

Measured against a Chromium `textarea`: `text-sm` advances 15.75 px per line and `text-base` 19.25; with `.6`
Fyne advanced 13.66 and 14.70, and with `.7` the same as the web. The `Entry` test references with several
rows move down 4 px per row (the line spacing of the test theme).

<a id="v281-sytue8-2026-09-25-el-pintor-de-software-redondea-las-posiciones"></a>
## `v2.8.1-sytue.8` (2026-09-25): the software painter rounds positions

Annotated tag on `4d4ebf890` of the `sytue-redondeo` branch, on top of `.7` (`a353ee549`); the exported
patch is **`0006-software-painter-rounds-positions.patch`** (18 files, 59 lines added and 21
deleted; 15 are test references).

The software painter also passed POSITIONS through `scale.ToScreenCoordinate`, which rounds
up: everything that fell on a fractional coordinate came out one pixel lower or further right than in the
GL painter (`roundToPixel`, to the nearest) and than in the browser. The real window was already fine; the
screenshots and the tests were not. Now positions and the clip origin round to the nearest and sizes
keep rounding up, so that nothing loses its last partial pixel.

Measured against Chromium with Figtree at eight fractional heights: the text baseline matches at all
eight (with ceiling, three failed). The references that change move a text or a border by one pixel.

<a id="v281-sytue9-2026-09-25-los-universal-links-de-ios-llegan-al-manejador-de-url"></a>
## `v2.8.1-sytue.9` (2026-09-25): iOS Universal Links reach the URL handler

Annotated tag on `80f85dbc8` of the `sytue-universal-links` branch, on top of `.8` (`4d4ebf890`); the
exported patch is **`0007-ios-universal-links.patch`** (1 file, 12 lines added).

An https link associated with the app (that of a Rooms meeting, `rooms.<dominio>/?rooms=<id>`, with
`apple-app-site-association` and the `associated-domains` entitlement) does not arrive through `openURL`: iOS delivers it as an
`NSUserActivity` of type `BrowsingWeb` to `application:continueUserActivity:restorationHandler:`, which the
delegate did not implement, so the link opened the app and was lost. Now its `webpageURL` goes to
`urlOpened`, the same as a custom scheme, on cold start and with the app open. Checked with
`clang -fsyntax-only` against UIKit (Mac Catalyst); it still has to be seen on an iPhone, which requires signing and the entitlement.

<a id="v281-sytue10-2026-09-25-los-fynedo-de-antes-del-bucle-de-eventos-en-el-hilo-principal"></a>
## `v2.8.1-sytue.10` (2026-09-25): `fyne.Do` calls from before the event loop, on the main thread

Annotated tag on `ae1ae7880` of the `sytue-cola-temprana` branch, on top of `.9`; the exported patch is
**`0008-mobile-queue-early-fyne-do.patch`** (2 files, with its test in `driver_test.go`).

On mobile, a goroutine that finished BEFORE `Run` created the queue ran its `fyne.Do` in its own
goroutine, at the same time as the main thread, and crashed in the text shaper (CertiPDF in the iOS Simulator:
a request that fails right at startup). Now the queue exists from `NewGoMobileDriver` and those calls run
on the main thread as soon as the loop starts.

<a id="v281-sytue11-2026-09-25-ios-adopta-el-ciclo-de-vida-por-escenas"></a>
## `v2.8.1-sytue.11` (2026-09-25): iOS adopts the scene-based lifecycle

Annotated tag on `417dce3e8`, the head of `sytue-cola-temprana`; the exported patch is
**`0009-ios-scene-lifecycle.patch`** (1 file, `darwin_ios.m`). Since 2026-09-25 the fork's `sytue` branch
points here (fast-forward from `.5`, without a merge commit), so the next version comes from `sytue`.

Built with the iOS 27 SDK, apps that do not adopt `UIScene` die on startup in
`_UIApplicationEvaluateRuntimeIssueForNoSceneLifecycleAdoption` (all thirteen in the iOS 27 Simulator). The
application delegate hands each session to `GoAppSceneDelegate`, the owner of the window, which receives what the
scenes take away from the delegate: incoming URLs (`scene:openURLContexts:` and those in `connectionOptions` on cold start), the
Universal Links (`continueUserActivity`) and the transitions to foreground and background. To test it without an Apple account:
`bash apps/fyne/scripts/empaquetar.sh iossimulator <app>`.

<a id="v281-sytue16-2026-10-04-mantenimiento-de-entry-y-validación-del-fork"></a>
## `v2.8.1-sytue.16` (2026-10-04): Entry maintenance and fork validation

[Stable release](https://github.com/thinkddr/fyne/releases/tag/v2.8.1-sytue.16) and
[full comparison with `.15`](https://github.com/thinkddr/fyne/compare/v2.8.1-sytue.15...v2.8.1-sytue.16).
The `Entry` refresh recalculates the scroll offset when Undo restores text that fits in the visible area.
The identity of the font cache keeps the address of its bytes without using `unsafe`; the handling of
focus and of the click that closes a layer is extracted into helpers without changing their conditions. The rest are tests,
reviewed visual references, CI and fork documentation. The canonical module, its dependencies,
`LICENSE` and `AUTHORS` do not change; it is still BSD-3-Clause. It does not add drag-and-drop support on iOS or change
the native bridges. Software tests are no substitute for a test in a window or on a device.

<a id="v281-sytue15-2026-09-28-una-tecla-soltada-ya-no-se-escribe-otra-vez-en-android"></a>
## `v2.8.1-sytue.15` (2026-09-28): a released key is no longer typed again on Android

`.14` plus `d943ebc78` (“fix(mobile/android): map key event actions, not key states, to directions”), on the
`sytue-teclado-android` branch. `processKey` compared `AKeyEvent_getAction` —`AKEY_EVENT_ACTION_DOWN` (0) and
`_UP` (1)— with `AKEY_STATE_DOWN` (1) and `_UP` (0), which are key states and have the opposite values: every
released key arrived as pressed. With the on-screen keyboard open the IME types the press through the
`TextWatcher` and the release came back as a second press, so a physical keyboard (or `adb shell input
text`) typed every letter twice («MMuuyy bbuueennoo», measured on the API 36 emulator). Exported
patch: **`0013-android-key-direction.patch`**; `TestKeyDirection` fails with the old comparison.

<a id="v281-sytue14-2026-09-28-una-fuente-del-tema-se-reconoce-por-sus-bytes"></a>
## `v2.8.1-sytue.14` (2026-09-28): a theme font is identified by its bytes

`.13` plus `bec3cd530` (“painter: identify a theme font by its bytes, not by the Resource value”): the font
cache in `.13` used the `fyne.Resource` given by the theme as its key, and two different resources with the same
bytes counted as two fonts. It is the release version. Exported patch:
**`0012-font-cache-keyed-by-bytes.patch`** (`internal/painter/font.go` and its test). A `Resource` of a non-comparable
type also crashed as a map key; `TestCachedFontFace_FreshResourceSameBytes` and
`TestCachedFontFace_ResourceNotComparable` fail with `.13`.

<a id="v281-sytue13-2026-09-28-la-memoria-de-lo-que-se-tematiza-y-el-alfa-del-framebuffer"></a>
## `v2.8.1-sytue.13` (2026-09-28): the memory of what is themed and the framebuffer alpha

Annotated tag on `5a25290db` of the `sytue-13` branch of `github.com/thinkddr/fyne`, on top of `.12`
(`b2a8bdfcb`). Two patches, exported together in **`0011-theme-override-memory-and-opaque-framebuffer.patch`**:

- **`77c580ac4` — memory.** Every object placed under a `ThemeOverride` stayed alive until the end of the
  process: the theme stored against each object pointed to the container (which contains those objects), the
  scope map stored the objects themselves, and the font cache was per scope, so every new override
  re-analyzed text, emoji and symbols and kept the copy. Measured with the Mail tests: at the
  end all 78 screens were still alive (1 GB, +180 MB per `-count`, and OOM with `-race -count=20`);
  with the patch one remains and 44 MB, and it does not grow. In the calendar walkthrough (55 modals, `-race`), the live heap at the
  end drops from 1553 MB to 168 MB and the maximum RSS from 8.4 GB to 1.1 GB. Guards `TestThemeOverride_DoesNotRetainContent` and
  `TestCachedFontFace_SharedAcrossOverrideScopes`, seen red with each change undone.
- **`5a25290db` — opaque alpha.** `BlendFuncSeparate(src, dst, ONE, dst)`: color with the usual
  factors and alpha accumulating towards 1. With `srcAlpha/oneMinusSrcAlpha` on alpha as well, every
  anti-aliased edge left alpha < 1 over non-premultiplied color and KWin, which composites premultiplied, painted
  colored fringes on every rounded edge. Checked on a real Plasma; that is why forcing
  XWayland on KDE is removed (`apps/fyne/sesion/plataforma_linux.go`).

The fork's suite fails the same as with `.12` (the reference images, on ARM64); `go vet` passes with and without
`gles`.

<a id="v281-sytue12-2026-09-26-macos-entrega-los-documentos-abiertos"></a>
## `v2.8.1-sytue.12` (2026-09-26): macOS delivers opened documents

Annotated tag on `b2a8bdfcb` of the `sytue-abrir-documentos` branch, on top of `.11` (`417dce3e8`); the exported
patch is **`0010-macos-open-documents.patch`** (2 files, `app/incoming_urls_darwin.{go,m}`, 39 lines
added).

The Finder, `open fichero` and “Open With” do not pass arguments: they send the `kAEOpenDocuments` Apple Event with a
list of files, and the fork only handled `kAEGetURL`. Now each file comes in as `file://` through the same
`urlhandler`. The handler is installed in `applicationWillFinishLaunching` (and also on registration, in case it has already
finished launching): AppKit installs its own when it finishes launching and would overwrite one installed earlier, and the
startup documents arrive right after. It is what the `CFBundleDocumentTypes` of #1128 need.

<a id="0014-ios-document-picker-import-modepatch-selector-de-documentos-en-ios-sin-icloud"></a>
## `0014-ios-document-picker-import-mode.patch`: document picker on iOS without iCloud

Exported in **`0014-ios-document-picker-import-mode.patch`** (2 files, `darwin_ios.m` and `file_ios.m`).

`UIDocumentPickerModeOpen` and `UIDocumentPickerModeMoveToService` require the iCloud container entitlement
(`com.apple.developer.icloud-container-identifiers`). When invoked in an app without that entitlement (all the
apps that open third-party files), UIKit throws `NSInternalInconsistencyException` and kills the process.
By switching to `UIDocumentPickerModeImport` and `UIDocumentPickerModeExportToService`, the system picker
allows opening and saving files from Files, iCloud Drive, cloud providers and local storage
without requiring the iCloud entitlement. In addition, `startAccessingSecurityScopedResource` is added so that reading
sandboxed URLs works securely and without access violations.
