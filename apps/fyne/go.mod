// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// La INTERFAZ de la suite en Fyne: tema y widgets propios (ADR 0048).
//
// Módulo SEPARADO del árbol raíz por lo mismo que apps/nucleo: en
// escritorio arrastra CGO y las bibliotecas gráficas del sistema, y nada de eso debe
// entrar en el go.mod de la plataforma ni en su gate de osv/trivy.
//
// Solo ESCRITORIO Y MÓVIL. La web sigue siendo Astro + Preact + Go, así que aquí no
// hay destino `js/wasm` y no hay que sostener ninguno.
module git.sytue.eu/sytue/one/apps/fyne

go 1.27.3

require (
	fyne.io/fyne/v2 v2.8.2
	git.sytue.eu/sytue/one/apps/nucleo v0.0.0
	git.wow.st/gmp/jni v0.0.0-20210610011705-34026c7e22d0
	github.com/coder/websocket v1.8.15
	github.com/fyne-io/oksvg v0.2.1-0.20260918172519-a9af55fa95d9
	github.com/gen2brain/avif v0.6.0
	github.com/gen2brain/malgo v0.11.26
	github.com/go-gl/glfw/v3.4/glfw v0.1.0-pre.2
	github.com/go-text/typesetting v0.3.5
	github.com/godbus/dbus/v5 v5.2.4
	github.com/google/uuid v1.6.0
	github.com/livekit/protocol v1.53.0
	github.com/livekit/server-sdk-go/v2 v2.19.0
	github.com/pion/rtcp v1.2.20
	github.com/pion/rtp v1.10.6
	github.com/pion/webrtc/v4 v4.2.25
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	github.com/thesyncim/govpx v0.0.0-20260716224042-691cd0512c48
	github.com/thinkddr/astro-fyne/native v0.0.0
	github.com/tphakala/go-opus v1.3.0
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/image v0.47.0
	golang.org/x/net v0.62.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	google.golang.org/protobuf v1.36.12
	maunium.net/go/mautrix v0.32.0
)

// Optional host adapter for the independently licensed Astro/Preact compiler.
replace github.com/thinkddr/astro-fyne/native => ../../tools/astro-fyne/native

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260415201107-50325440f8f2.1 // indirect
	buf.build/go/protovalidate v1.2.0 // indirect
	buf.build/go/protoyaml v0.7.0 // indirect
	cel.dev/expr v0.25.2 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	fyne.io/systray v1.12.3-0.20260810170012-af4e8e793ec4 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/FyshOS/fancyfs v0.0.1 // indirect
	github.com/anthonynsimon/bild v0.14.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dennwc/iters v1.2.2 // indirect
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/frostbyte73/core v0.1.1 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/fyne-io/gl-js v0.2.1-0.20260315212741-029c47fd27e8 // indirect
	github.com/fyne-io/glfw-js v0.4.0 // indirect
	github.com/fyne-io/image v0.1.1 // indirect
	github.com/gammazero/deque v1.2.1 // indirect
	github.com/go-gl/gl v0.0.0-20260331235117-4566fea9a276 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-text/render v0.2.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/cel-go v0.31.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/hack-pad/go-indexeddb v0.3.2 // indirect
	github.com/hack-pad/safejs v0.1.0 // indirect
	github.com/jeandeaual/go-locale v0.0.0-20250612000132-0ef82f21eade // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	github.com/jxskiss/base62 v1.1.0 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/lithammer/shortuuid/v4 v4.3.0 // indirect
	github.com/livekit/mageutil v0.0.0-20250511045019-0f1ff63f7731 // indirect
	github.com/livekit/mediatransportutil v0.0.0-20260821083140-f234b534b095 // indirect
	github.com/livekit/psrpc v0.8.2 // indirect
	github.com/magefile/mage v1.17.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/mattn/go-sqlite3 v1.14.52 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/nicksnyder/go-i18n/v2 v2.5.1 // indirect
	github.com/petermattis/goid v0.0.0-20260918085751-abfca077860b // indirect
	github.com/pion/datachannel v1.6.3 // indirect
	github.com/pion/dtls/v3 v3.1.15 // indirect
	github.com/pion/ice/v4 v4.4.8 // indirect
	github.com/pion/interceptor v0.1.49 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.12.0 // indirect
	github.com/pion/sdp/v3 v3.0.20 // indirect
	github.com/pion/srtp/v3 v3.1.3 // indirect
	github.com/pion/stun/v4 v4.0.1 // indirect
	github.com/pion/transport/v5 v5.1.1 // indirect
	github.com/pion/turn/v5 v5.1.2 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.5.0 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/rymdport/portal v0.4.2 // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/tidwall/gjson v1.20.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tphakala/simd v1.14.0 // indirect
	github.com/twitchtv/twirp v8.1.3+incompatible // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.mau.fi/util v0.11.0 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.uber.org/zap/exp v0.3.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// El núcleo compartido de los clientes vive en el mismo repositorio.
replace git.sytue.eu/sytue/one/apps/nucleo => ../nucleo

// El fork de Fyne v2.8.2 conserva la API pública y añade URL entrantes, gestos
// móviles y geometría de la ventana UIKit al girar o redimensionar. El original
// conserva BSD-3-Clause y las modificaciones Apache-2.0; parches y retirada en
// upstream/README.md.
replace fyne.io/fyne/v2 => github.com/thinkddr/fyne/v2 v2.8.2-sytue.2

// GLFW solo acepta FICHEROS soltados: su XDnD de X11 y su wl_data_device de Wayland
// piden `text/uri-list` y nada más. El fork —el mismo commit que requiere Fyne, con un
// parche en su C y `SetDropTextCallback` en Go— deja que una ventana acepte también
// texto y URL; lo usa `apps/fyne/soltar`. El CI lo descarga directamente con
// `forks.sh` (ver abajo). Parche, versión y cómo quitarlo:
// `apps/fyne/upstream/README.md`.
replace github.com/go-gl/glfw/v3.4/glfw => github.com/thinkddr/glfw/v3.4/glfw v0.1.0-sytue.4

// # El conformador de texto va PARCHEADO. Medido el 2026-09-21
//
// go-text/typesetting escalaba la fuente a `ceil(tamaño)` antes de pedirle los avances
// a harfbuzz (`shaping/shaping.go`), así que un tamaño fraccionario salía con los
// avances del entero siguiente. Con `1 rem = 14 px` casi toda la escala de la suite es
// fraccionaria, y CADA texto salía más ancho: `text-base` (12,25 px) un +6,12 %,
// `text-sm` +5,49 %, `text-xl` +3,90 %. Medido, no estimado.
//
// Y no lo arreglaba una pantalla HiDPI: Fyne conforma en px de CSS y multiplica DESPUÉS
// por la escala del lienzo (`internal/painter/font.go`), así que el error es el mismo a
// ×1 que a ×2.
//
// El fork conserva la base de upstream, el parche fraccionario y su prueba.
// Declara el module canónico para que sus imports internos resuelvan el mismo
// módulo; replace selecciona el repositorio del fork sin renombrar ese contrato.
// La política de actualización y quién la comprueba, en el ADR 0048.
//
// La descarga de nuestros forks sigue siendo directa, sin consultar sus rutas en
// proxy.golang.org ni sum.golang.org: `GOPRIVATE=github.com/thinkddr/*`.
// Esta opción no determina su visibilidad en GitHub. El preflight del CI sigue
// exigiendo acceso mediante SYTUE_GIT_FORKS_TOKEN, sin exponer su valor.
replace github.com/go-text/typesetting => github.com/thinkddr/typesetting v0.3.5-sytue.1

// Verified upstream source plus patches/livekit-server-sdk-go-v2.19.0-vp8.patch.
replace github.com/livekit/server-sdk-go/v2 => ../../.deps/livekit-server-sdk-go
