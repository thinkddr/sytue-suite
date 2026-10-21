<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright 2026 Astro Fyne contributors. -->

# Compatibility contract and roadmap

The target is an automated native implementation of interfaces authored in Astro + Preact,
preserving behavior and producing identical pixels for declared rendering profiles. Stage 01
establishes a compiler, a native runtime and measurable failure gates. Universal compatibility
remains a development objective.

Compatibility has three separate requirements: source syntax must compile, native interaction
must preserve the relevant behavior, and the resulting native image must match the browser.
A parser test alone establishes the first requirement. A generated button responding to a
tap establishes one interaction. A zero-difference geometry capture establishes one visual
profile. These results do not imply coverage of an entire application.

## Stage 01 source contract

The source compiler uses Astro's parser and TypeScript's AST. It lowers a declarative subset
to a portable intermediate representation and then generates Go. It does not evaluate the
original JavaScript or execute a general ECMAScript runtime.

| Area | Initial contract | Outside the initial contract |
| --- | --- | --- |
| Entry files | `.astro`, `.tsx` and `.jsx`; local component imports; default or selected named exports | General package resolution, server execution and arbitrary module side effects |
| Components | Function components with supported props, constants and a declarative JSX return | Class components, arbitrary imperative component bodies and slots/children without a native contract |
| State | Supported `useState` declarations; direct and declarative updater setters in event handlers | Other hooks, arbitrary custom hooks and general effect lifecycles |
| Expressions | Literals, supported property/index access, arrays, objects, templates, conditionals and listed operators | Arbitrary function calls, spread, optional chaining and unsupported JavaScript constructs |
| Branches and lists | Supported JSX conditionals and declarative `map` callbacks | General iteration, arbitrary callback bodies and full keyed Preact reconciliation guarantees |
| Events | Button/link `onClick`; input/textarea `onInput` and `onChange`; Go host callbacks declared explicitly in props and the host translator `t` | Full DOM event propagation, arbitrary event payloads, browser effects and implicit platform adapters |
| HTML nodes | Supported container, plain text, button, link, input, textarea and local bitmap image tags | Rich text nesting, forms, specialized form controls and unsupported tags |
| Application effects | Explicit named native actions supplied by the Go host | Host calls during module initialization; automatic translation of browser APIs, networking, SSR, storage or navigation implementations |

The accepted operators and calls are defined in `src/parser.ts`; their native implementations
live in `native/expressions.go`. This is a portable expression contract rather than proof of
complete ECMAScript equivalence. Expanding it requires browser/native differential tests for
value coercion, Unicode strings, missing values, short-circuit evaluation and event ordering.
Compilation must reject a construction when its semantics cannot be preserved.

Callback names bound to state, constants or shadowing list/event/updater parameters do not
authorize host actions. Calls to shadowed `String`, `Number` or `Boolean` require an explicit
native adapter. A component-local constant may call the host translator `t`; a module-level
constant may not, because its initialization lifecycle is not implemented. HTML `form` nodes
are rejected during Go generation until native submit and implicit submit behavior are defined.
Named handlers used under a `map` that shadows a component or module binding are conservatively
rejected until lexical captures are qualified. Inline handlers retain their list-local bindings;
named handlers outside those shadows remain supported.
Lexical bindings named `undefined` and asynchronous `map` callbacks are rejected explicitly.
Ordinary components cannot receive nested child nodes without a native children/slot contract;
scalar props named `children` retain their ordinary prop value, and explicit adapters keep
their declared native child behavior.

The programmatic compiler API supports explicit package adapters. The initial CLI
configuration only exposes entries and measurement files. Adapters must preserve a component's
behavior and styling contract; recognizing a component name alone does not establish parity.

## Stage 01 visual contract

Source-only layouts are useful for functional prototypes. The built-in utility mapping uses
its own fixed spacing and typography profile, including a 14px `rem`; it is not a general
Tailwind implementation. CSS inside Astro files or imported stylesheets requires browser
measurements. A measured profile uses the browser's actual rectangles and computed styles.

| Feature | Current boundary |
| --- | --- |
| Geometry | CSS pixel dimensions; viewport coordinates for roots; parent border-box coordinates for children |
| Layout | Recorded positions for one viewport and state; source-only row/column layouts within the supported style subset |
| Paint | Supported solid colors, uniform borders and uniform radii; the backend and zero-difference gate decide actual equivalence |
| Opacity | Measured group opacity must be 1; translucent colors require correct composition from the backend |
| Typography | Explicit loaded browser fonts and corresponding native resources; supported text alignment, line height and whitespace |
| Stacking and effects | Capture rejects non-default stacking, transforms, shadows, gradients, filters, masks and pseudo content |
| Overflow | Capture rejects content extending outside its measured box; scrolling and clipping need explicit future support |
| Resources | Embedded local PNG RGB/RGBA8 and supported JPEG, without color/orientation metadata; SVG, media, iframe, canvas, remote images, srcset and Shadow DOM require further native adapters |
| Themes | The capture currently requests light color scheme; dark and system-theme switching need separate scenarios and implementation |
| Scale | Capture records device scale 1 or 2; each native profile must enforce and verify its corresponding scale |
| Platforms | A pass in the pinned test renderer does not certify desktop GPU rendering, other operating systems or mobile devices |

Each capture carries a `sourceHash`, a state label, viewport dimensions, scale, DOM hash,
screenshot hash and root custom-property tokens. The generator checks the source digest and
the supported measurement schema; when the entry declares a `profile`, it also requires the
state label, viewport and scale to match that configured profile. The runtime rejects
incomplete or extra node measurements,
viewport stretching and changes to a measured visual tree. The state label is scenario
metadata; it does not establish that the correct application state was reached. Scenario
assertions and the final image comparison establish that correspondence.

`sourceHash` covers the compiler's recorded source dependency list, including embedded asset
digests. It is not a signature of a running server or proof that an arbitrary URL serves
those sources and asset bytes.
CI must build the selected revision, serve that build, prepare deterministic state and keep
the resulting capture evidence. If a page declares `data-fyne-source-hash` or
`data-fyne-state`, capture also checks those values against its arguments.

## Conformance evidence

A passing profile retains its source digest, environment versions, application data, interaction
scenario, viewport, scale, color scheme and font files. It also retains `measurements.json`,
`web.png`, `native.png`, `diff.png` and the comparison JSON. The default comparison requires
zero differences in all four color channels; no implicit antialiasing tolerance is applied.
The comparison contract uses up to 8-bit channels and rejects 16-bit PNG input, including
changes that would disappear if their channels were reduced to 8 bits.
The PNG CLI also rejects animation, color profiles and unrecognized interpretation chunks;
it validates the entire chunk stream, including CRCs and trailing bytes.

The initial examples exercise state/input behavior and solid geometry at 320 × 240 pixels,
scale 1. A behavioral corpus drives the same click sequence in Preact/Chromium and generated Go.
The differential gate compares texts, mount/unmount results and callback observations;
passing it covers that declared scenario only. Its browser and native JSON traces are CI artifacts.
The tests include a negative visual check that changes one channel of one pixel
and requires the comparator to fail. A release should report which scenarios actually passed
in CI and which remain untested, rather than applying a general fidelity badge to generated
code. Failed or missing comparisons leave a profile uncertified.

## Roadmap toward broad compatibility

The following are engineering stages, not implemented features or delivery dates.

1. **Behavioral semantics.** Extend the differential corpus to nested components, keyed lists,
   batched updates, closures, controlled inputs, refs, context, effects, cleanup and unmounts.
   Separate JSX child rendering from JavaScript conversion rules and preserve ECMAScript
   strings, missing values, coercion and short-circuit evaluation. A Go lowering must implement
   these rules explicitly. For arbitrary JavaScript and third-party Preact libraries, evaluate
   an embedded ECMAScript engine with a Preact host renderer as a separate architecture decision;
   it would add a scripting runtime and platform bridges. A syntax parser alone cannot supply
   general JavaScript execution semantics.
2. **Styles and responsive layout.** Build a versioned CSS cascade and formatting-box model:
   selectors, inherited values, custom properties, units, flexbox, grid, inline formatting,
   intrinsic sizing, wrapping, clipping, scroll containers, positioning and stacking. Continue
   using browser captures as an oracle while adding runtime layout for changing data and window
   sizes. Captured coordinates for a finite collection of states cannot represent arbitrary
   future content or a continuously resized window.
3. **Text and painting.** Define a reproducible font and rendering profile: shaping, fallback,
   ligatures, bidi, grapheme clusters, line breaking, baselines, subpixel origins, alpha
   composition, gradients, shadows and antialiasing. Extend the native backend or maintain
   clearly scoped Fyne forks where its painter or driver prevents the required behavior.
   Matching a font filename, an advance width or a bounding box is insufficient evidence of
   pixel equality.
4. **Interaction and accessibility.** Preserve event phases, focus order, keyboard handling,
   hover/pressed/disabled states, selection, composition, shortcuts, gestures, drag/drop and
   accessibility roles, names and state. Native controls need desktop and mobile IME tests,
   including composition and selection across Unicode text. Compare observable event traces
   as well as images.
5. **Resources and platform APIs.** Add versioned native contracts for routing, history,
   asynchronous effects, timers, fetch, sockets, persistence, clipboard, file access, image
   decoding, SVG, canvas and media. Document permission, cancellation and lifecycle behavior.
   Astro SSR and server-only code remain separate execution concerns: supply their results
   through props or define an explicit server integration.
6. **Release conformance.** Expand reproducible scenarios across light/dark/system themes,
   window sizes, scales, locales, resource loads, desktop backends and real mobile devices.
   Keep failure artifacts, enforce the requested zero-difference policy, and reject unsupported
   features until their behavior and rendering have a passing scenario. Publish a versioned
   capability matrix and extraction/build checks for the independent distribution.

These stages imply a substantial platform compatibility layer. The generated output remains
a native Go interface, with interactive Fyne objects and explicit host boundaries. Each design
decision about a scripting engine, CSS implementation or Fyne fork must state how that native
contract is maintained and how its new capabilities are tested.

## Primary references

The architecture follows the distinction between source parsing, runtime behavior, formatting
boxes and native painting described by their upstream implementations:

- [Astro compiler](https://github.com/withastro/compiler): source AST and server-runtime concerns.
- [Preact hooks](https://preactjs.com/guide/v10/hooks/): state, context, effects and lifecycle behavior.
- [CSS Display specification](https://www.w3.org/TR/css-display-3/): formatting box generation.
- [Fyne custom widgets](https://docs.fyne.io/extend/custom-widget/): stateful widgets and native renderers.
- [QuickJS](https://bellard.org/quickjs/): an example ECMAScript engine to evaluate, not a current dependency.
- [Web Platform Tests](https://web-platform-tests.org/): a model for versioned platform conformance.
