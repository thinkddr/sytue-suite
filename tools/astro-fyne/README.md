<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright 2026 Astro Fyne contributors. -->

# Astro Fyne

Design and implement an interface in Astro + Preact, then generate Go that builds a native
Fyne interface. The compiler preserves the supported component tree, properties, state and
events. Browser measurements can supply the geometry and computed styles for a declared
visual profile. Native widgets remain interactive; browser screenshots are verification
artifacts.

**Status: stage 01, a strict declarative subset.** The long-term goal is broad Astro + Preact
compatibility with identical appearance and behavior. This version does not yet convert
arbitrary applications or guarantee universal pixel parity. Unsupported constructs produce
errors. The [compatibility contract and roadmap](COMPATIBILITY.md) define the current scope.

The independent project is [thinkddr/astro-fyne](https://github.com/thinkddr/astro-fyne),
licensed under Apache-2.0. The Go module path is `github.com/thinkddr/astro-fyne/native`.
The compiler and default runtime use public dependencies and do not require the Sytue product
or its private forks. CI runs on hosted ARM64 runners and preserves captures, generated code
and comparison results as downloadable evidence.

## Generate an interface

Use Bun and a Go installation compatible with `native/go.mod`; generation invokes `gofmt`.
Native executables also need the platform dependencies listed in the
[Fyne installation documentation](https://docs.fyne.io/started/).

From this directory:

```sh
bun install
bun src/cli.ts generate --config astro-fyne.json
bun src/cli.ts check --config astro-fyne.json
bun src/cli.ts watch --config astro-fyne.json
```

`generate` parses every selected entry and formats its Go before writing artifacts. `check`
fails if generated Go, the shared scope helper or reports differ from the source. `watch`
performs an initial generation and then watches source content; diagnostics remain visible
when an edit is unsupported. These commands compile source syntax without executing the
application's JavaScript.

The configuration paths are relative to the configuration file:

```json
{
  "schema": 1,
  "package": "generated",
  "entries": [
    {
      "name": "Counter",
      "source": "example/src/pages/index.astro",
      "output": "native/generated/counter.gen.go"
    }
  ]
}
```

An entry may also select a named `export`, declare a `measurements` JSON file or bind a
`profile` containing `state`, `width`, `height` and `scale`. A measured entry with a configured
profile must match those values during generation. Use
`--entry Counter` to select one entry in a larger configuration. Generated Go and
`*.report.json` files belong to the generator; edits belong in Astro/TSX or the native host.
Output names must end in `.gen.go`. Before writing any artifact, the compiler checks that
existing Go files carry its generated header and existing reports identify its generator.
It refuses to overwrite hand-written files, including a report at a generated output path.

## Use the generated Go

The constructor returns a generated native widget, such as `*generated.CounterWidget`,
which embeds `*webui.View`, and checks required host actions. After
generating the Counter example, a Go host in the native module can construct it like this:

```go
import (
    webui "github.com/thinkddr/astro-fyne/native"
    "github.com/thinkddr/astro-fyne/native/generated"
)

view, err := generated.NewCounter(webui.Scope{}, webui.Actions{})
if err != nil {
    return err
}
window.SetContent(view)
```

The repository includes generated Go and reports for its five conformance examples in
`native/generated`. They are snapshots produced and tested by remote CI, so the native
module can be used and tested without first rebuilding Astro. CI regenerates them and fails
if committed snapshots or resolved Go dependencies change unexpectedly.

The example contains a stateful counter, a controlled input and conditional content. State
updates reuse objects by identifier so the input can retain focus. Hosts should surface
`view.Error()` when a runtime contract fails and perform UI mutations on Fyne's event
goroutine. Background service work returns to the UI through `fyne.Do`.

Application effects use named `webui.Actions`: the host implements service calls, navigation,
storage and other platform work in Go. A missing required action is an error. The constructor
also accepts an optional `webui.Backend`, which supplies boxes, text measurement and painting,
and editing. The default `webui.FyneBackend` uses public Fyne primitives. Measured text requires
explicit font resources matching the browser's loaded font family, weight and style; supplying
the same font file is necessary but does not by itself prove matching rasterization.

A custom backend can delegate its `Editor` method to `webui.NewEditor(backend, multiline,
style, onChange)`. This shares the native editing engine while using that backend's text
measurement and placement; the generated view owns keyboard focus and commit events.

Generation also emits an entry-specific theme type and constructor, such as
`GeometryTheme` and `NewGeometryTheme(base fyne.Theme)`. The theme receives the CSS custom
properties recorded by capture; the native theme adapter handles the supported token mapping
and uses the supplied base theme for the remaining Fyne theme values. Each generated widget
contains the source tree and layout/style data used by its native renderer. These outputs
automate the current widget and theme construction contract; general CSS theme switching,
arbitrary custom controls and every formatting model remain part of the compatibility roadmap.

Local `<img src="/images/example.png" alt="…" />` nodes generate embedded native resources
and `canvas.Image` renderers. `NewNameResources()` exposes the resources by source path.
URL-root paths resolve from the entry's Astro `public` directory, even for imported shared
components; an entry can set `publicDir` explicitly. Relative image paths are source-relative.
The resource inventory and byte hashes appear in the report and source digest. Dynamic URLs,
remote assets, SVG, srcset, animation and color/orientation metadata require further adapters.
The initial PNG contract is RGB/RGBA with 8-bit channels; supported JPEG files also receive
native decoding, while actual pixel equality remains subject to the image gate.

## Capture and verify a visual profile

A visual profile fixes the source revision, application state, viewport, device scale, theme,
font resources and rendering environment. The initial geometry scenario uses **320 × 240
CSS pixels at scale 1**, with solid rectangular boxes and no text. It exercises the complete
Astro CSS → browser measurement → generated Go → native capture → zero-difference path.

Run browser captures, native tests and builds in a suitable remote environment. In the Sytue
repository, this means CI or the Scaleway development environment, as required by `AGENTS.md`.

Start the example web preview in a separate terminal:

```sh
cd example
bunx astro build
bunx astro preview --host 127.0.0.1 --port 4321
```

From the project directory, while that preview is running:

```sh
bun src/cli.ts analyze --config visual.json --entry Geometry > /tmp/astro-fyne-analysis.json
task_source_hash="$(jq -r '.sourceHash' /tmp/astro-fyne-analysis.json)"
bun capture/astro-fyne-capture.ts \
  --url http://127.0.0.1:4321/geometry \
  --out /tmp/astro-fyne-geometry --width 320 --height 240 --scale 1 \
  --state default --source-hash "$task_source_hash"
bun src/cli.ts generate --config visual.json --entry Geometry \
  --measurements /tmp/astro-fyne-geometry/measurements.json
```

`--selector` defaults to `#fyne-root`. Every element within that root needs a unique explicit
HTML `id`. Use `--ready-selector` for an application readiness signal. `--state` labels the
state already prepared by the URL and scenario; it does not execute interactions. Capture
waits for fonts and images, checks that the DOM and measurements remain stable, and records
root CSS custom properties as `tokens`. It writes `web.png` and `measurements.json`, including
source, DOM and screenshot hashes.

Image captures also require `--analysis PATH` pointing to the compiler's `analyze` result.
Capture checks the bytes of the response Chromium actually used against the compiler's
resource digest, plus its MIME type, literal URL and natural dimensions. The image scenario
in `image-visual.json` uses a local PNG at its natural size; its evidence is retained in
`artifacts/images` separately from the solid-box scenario.

Capture the generated native geometry and compare the actual images:

```sh
cd native
ASTRO_FYNE_ARTIFACTS=/tmp/astro-fyne-geometry go test ./generated -run TestCaptureMeasuredGeometry
go run ./visual/cmd/astro-fyne-compare \
  --reference /tmp/astro-fyne-geometry/web.png \
  --native /tmp/astro-fyne-geometry/native.png \
  --out /tmp/astro-fyne-geometry/diff.png
```

The default gate permits **zero channel difference and zero changed pixels**, including
alpha. It refuses different image dimensions and returns a failing exit code when pixels
differ. It rejects 16-bit PNGs instead of losing their low channel bits during comparison;
the gate compares channels represented at up to 8 bits. Animated PNGs and unsupported
color, precision, background or orientation metadata also fail instead of being ignored.
Its JSON result includes
`changedPixels`, `maxChannelDelta`, `bounds`, `exact` and
`accepted`. Optional tolerances must be selected explicitly; accepted images with differences
still report `exact: false`. Passing the geometry scenario certifies that scenario only.

The generator report always starts with `pixelPerfectVerified: false`. Generation and matching
bounding boxes do not establish visual equality. Keep the comparator result with both captures
and the measurement file as the evidence for a passing profile.

Measured constructors declare the captured device scale. After assigning the widget to its
canvas, hosts must call `view.BindCanvas(window.Canvas())` and check `view.ValidateCanvas()`
before certifying or exporting it. A measured profile requires its exact logical viewport and
device scale; the runtime reports changes to either as errors.

## License

The independent compiler, capture tools and native runtime are licensed under
[Apache License 2.0](LICENSE). See [NOTICE](NOTICE) for attribution. Dependencies and optional
external backend adapters retain their own licenses; this project does not distribute the
proprietary Sytue adapter or its design assets.
