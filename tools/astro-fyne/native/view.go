// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

// Package webui renders the deliberately supported Astro/Preact source subset as
// native Fyne objects. Browser measurements are authoritative at their recorded
// viewport and state; the runtime never stretches a measured page to fit a window.
package webui

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"math"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Node is generated Go, not interpreted markup. ID is stable across reevaluation
// so changing state preserves the object which owns the keyboard focus and cursor.
type Node struct {
	ID, Kind, Text, Value, Placeholder, Href, Variant, Size string
	AccessibleLabel, LabelFor                               string
	Disabled                                                bool
	Style                                                   Style
	Children                                                []Node
	OnTap                                                   func()
	OnChange                                                func(string)
	// OnCommit represents HTML change; OnChange represents immediate input.
	OnCommit      func(string)
	ImageResource fyne.Resource
}

// Style uses CSS pixels, including fractional pixels. Measured positions are
// relative to the parent's border box, not its content box. Browser capture rejects
// unsupported CSS before it supplies a Style: matching boxes alone is not parity.
type Style struct {
	X             float32 `json:"x"`
	Y             float32 `json:"y"`
	Width         float32 `json:"width"`
	Height        float32 `json:"height"`
	PaddingTop    float32 `json:"paddingTop"`
	PaddingRight  float32 `json:"paddingRight"`
	PaddingBottom float32 `json:"paddingBottom"`
	PaddingLeft   float32 `json:"paddingLeft"`
	Gap           float32 `json:"gap"`
	Direction     string  `json:"direction"`
	Background    string  `json:"background"`
	Color         string  `json:"color"`
	BorderColor   string  `json:"borderColor"`
	BorderWidth   float32 `json:"borderWidth"`
	Radius        float32 `json:"radius"`
	FontSize      float32 `json:"fontSize"`
	LineHeight    float32 `json:"lineHeight"`
	FontWeight    float32 `json:"fontWeight"`
	FontFamily    string  `json:"fontFamily"`
	FontStyle     string  `json:"fontStyle"`
	TextAlign     string  `json:"textAlign"`
	WhiteSpace    string  `json:"whiteSpace"`
	Display       string  `json:"display"`
	Opacity       float32 `json:"opacity"`
	Measured      bool    `json:"measured"`
}

// View owns reconciled native objects. All mutation, including Refresh, belongs on
// Fyne's event goroutine; native adapters completing background work use fyne.Do.
type View struct {
	widget.BaseWidget
	backend           Backend
	build             func() []Node
	nodes             []Node
	elements          map[string]*element
	roots             []*element
	measurements      map[string]Style
	measurementState  string
	viewport          fyne.Size
	captureScale      float32
	boundCanvas       fyne.Canvas
	err               error
	canvasErr         error
	navigationErr     error
	refreshing        bool
	bitmaps           map[[32]byte]bitmapAsset
	autoRefreshEvents bool
}

var _ fyne.Widget = (*View)(nil)

// NewView builds the initial tree before its first render, so the first frame is
// complete without relying on Fyne to call a renderer's Refresh for us.
func NewView(build func() []Node, backends ...Backend) *View {
	var backend Backend = FyneBackend{}
	if len(backends) > 0 && backends[0] != nil {
		backend = backends[0]
	}
	v := &View{build: build, backend: backend, elements: make(map[string]*element), bitmaps: make(map[[32]byte]bitmapAsset), autoRefreshEvents: true}
	v.ExtendBaseWidget(v)
	v.reconcile()
	return v
}

// Object retrieves the real native object for accessibility, focus and testing.
func (v *View) Object(id string) fyne.CanvasObject {
	if e := v.elements[id]; e != nil {
		return e.object
	}
	return nil
}

// SetAutoRefreshEvents selects who reevaluates the source tree after callbacks.
// Hand-authored nodes default to automatic refresh. Generated event handlers turn
// it off and refresh only when they queued state, preserving action-only closures.
func (v *View) SetAutoRefreshEvents(enabled bool) { v.autoRefreshEvents = enabled }

func (v *View) refreshAfterEvent() {
	if v.autoRefreshEvents {
		v.Refresh()
	}
}

// Error reports invalid source contracts and stale measurement profiles. Consumers
// must surface this error; a measured page with an error is not parity certified.
func (v *View) Error() error {
	var editingErr error
	if len(v.measurements) != 0 {
		editingErr = v.uncommittedInputError()
	}
	return errors.Join(v.err, v.canvasErr, v.navigationErr, editingErr)
}

func (v *View) uncommittedInputError() error {
	for id, e := range v.elements {
		if e.input != nil && e.input.Text() != e.node.Value {
			return fmt.Errorf("webui: input %q has an uncommitted visual value; commit it before supplying a matching measurement profile", id)
		}
	}
	return nil
}

// SetCaptureScale declares the browser capture's device-pixel ratio. Loading a
// profile before mounting remains possible; certification also requires BindCanvas
// and ValidateCanvas once the native canvas has its final size and scale.
func (v *View) SetCaptureScale(scale float32) error {
	if !finite(scale) || scale <= 0 {
		return fmt.Errorf("webui: capture scale must be positive and finite")
	}
	v.captureScale = scale
	if v.boundCanvas != nil {
		v.Refresh()
		return v.ValidateCanvas()
	}
	return nil
}

// BindCanvas explicitly associates a native canvas with this generated view.
// Auto-discovery is insufficient: named generated widgets embed View, and a
// windowless software canvas is not registered in the application driver's windows.
func (v *View) BindCanvas(target fyne.Canvas) error {
	if target == nil {
		return fmt.Errorf("webui: a native canvas is required")
	}
	v.boundCanvas = target
	v.Refresh()
	return v.ValidateCanvas()
}

// ValidateCanvas proves that a mounted measurement profile uses the captured
// viewport AND device scale. Call it immediately before exporting pixels, even
// if no resize/refresh occurred after a driver changed its scale.
func (v *View) ValidateCanvas() error {
	if len(v.measurements) == 0 {
		return nil
	}
	if v.boundCanvas == nil {
		return fmt.Errorf("webui: bind the native canvas before validating a capture")
	}
	if v.viewport.Width <= 0 || v.viewport.Height <= 0 {
		return fmt.Errorf("webui: measured capture requires an explicit viewport")
	}
	if v.captureScale <= 0 {
		return fmt.Errorf("webui: measured capture requires an explicit device scale")
	}
	if err := v.ValidateViewport(v.boundCanvas.Size()); err != nil {
		return err
	}
	actual := v.boundCanvas.Scale()
	if !finite(actual) || actual != v.captureScale {
		return fmt.Errorf("webui: captured device scale %v cannot render at %v; capture that scale", v.captureScale, actual)
	}
	return nil
}

// SetViewport declares the CSS viewport captured by the browser. Canvas scale is
// independent: a 400px page at device scale 2 still has a 400px native logical width.
func (v *View) SetViewport(width, height float32) {
	if !finite(width) || !finite(height) || width <= 0 || height <= 0 {
		v.err = fmt.Errorf("webui: viewport must have positive finite dimensions")
		return
	}
	v.viewport = fyne.NewSize(width, height)
	v.Resize(v.viewport)
	v.Refresh()
}

// ValidateViewport rejects rescaling a capture. Capture another browser viewport
// instead of claiming pixel parity after stretching the recorded coordinates.
func (v *View) ValidateViewport(size fyne.Size) error {
	if len(v.measurements) != 0 && v.viewport.Width > 0 && size != v.viewport {
		return fmt.Errorf("webui: measured viewport %v cannot render at %v; capture that viewport", v.viewport, size)
	}
	return nil
}

// ApplyMeasurements validates the ENTIRE current tree before replacing a profile.
// Partial captures and extra IDs are errors: no child falls back to guessed layout.
// Profiles describe one visual state and become invalid when that state changes.
func (v *View) ApplyMeasurements(measurements map[string]Style) error {
	if err := v.uncommittedInputError(); err != nil {
		return err
	}
	ids := make(map[string]bool, len(v.elements))
	for id := range v.elements {
		ids[id] = true
		style, ok := measurements[id]
		if !ok {
			return fmt.Errorf("webui: missing browser measurement for %q", id)
		}
		if !style.Measured {
			return fmt.Errorf("webui: measurement for %q is not marked measured", id)
		}
		if err := validateStyle(id, style); err != nil {
			return err
		}
		e := v.elements[id]
		if e.node.Kind == "image" {
			if err := validateImageStyle(id, style); err != nil {
				return err
			}
		}
		if err := v.backend.Validate(style, e.node.Kind != "image" && (e.node.Text != "" || e.node.Kind == "input" || e.node.Kind == "textarea")); err != nil {
			return fmt.Errorf("webui: %q: %w", id, err)
		}
	}
	for id := range measurements {
		if !ids[id] {
			return fmt.Errorf("webui: browser measurement %q has no native node", id)
		}
	}
	if len(measurements) == 0 {
		return fmt.Errorf("webui: a measurement profile must not be empty")
	}
	v.measurements = make(map[string]Style, len(measurements))
	for id, style := range measurements {
		v.measurements[id] = style
	}
	v.measurementState = visualState(v.nodes)
	v.Refresh()
	return errors.Join(v.err, v.canvasErr)
}

// ClearMeasurements returns to source layout; useful before changing state and
// supplying the matching captured profile. It explicitly removes the parity claim.
func (v *View) ClearMeasurements() {
	v.measurements, v.measurementState = nil, ""
	v.Refresh()
}

// Resize keeps the native canvas and the captured CSS viewport accountable.
func (v *View) Resize(size fyne.Size) {
	v.canvasErr = v.ValidateViewport(size)
	v.BaseWidget.Resize(size)
}

func (v *View) Refresh() {
	if v.refreshing {
		return
	}
	v.refreshing = true
	defer func() { v.refreshing = false }()
	v.reconcile()
	v.BaseWidget.Refresh()
}

func (v *View) reconcile() {
	if v.build == nil {
		v.err = fmt.Errorf("webui: a generated view requires a build function")
		return
	}
	nodes, err := buildSafely(v.build)
	if err != nil {
		v.err = err
		return
	}
	ids := make(map[string]bool)
	if err := validateNodes(nodes, ids); err != nil {
		v.err = err
		return // Keep the last valid native tree rather than partly mutating it.
	}
	nodes, err = v.freezeImages(nodes)
	if err != nil {
		v.err = err
		return
	}
	var profileError error
	if len(v.measurements) > 0 {
		if visualState(nodes) != v.measurementState {
			profileError = fmt.Errorf("webui: visual state changed; supply a measurement profile for the new state")
		}
		for id := range ids {
			if _, ok := v.measurements[id]; !ok {
				profileError = fmt.Errorf("webui: new node %q has no browser measurement", id)
			}
		}
		for id := range v.measurements {
			if !ids[id] {
				profileError = fmt.Errorf("webui: measured node %q disappeared from the visual state", id)
			}
		}
	}
	v.err = profileError
	v.nodes = nodes
	current := make(map[string]*element, len(ids))
	var build func([]Node, Style) []*element
	build = func(nodes []Node, inherited Style) []*element {
		out := make([]*element, 0, len(nodes))
		for _, n := range nodes {
			e := v.elements[n.ID]
			if e == nil || e.node.Kind != n.Kind {
				e = newElement(v, n)
			}
			e.node = n
			e.label = n.AccessibleLabel
			style := n.Style
			if measured, ok := v.measurements[n.ID]; ok && profileError == nil {
				style = measured
			}
			e.style = resolveStyle(n, style, inherited, v.backend)
			e.children = build(n.Children, e.style)
			current[n.ID] = e
			e.update()
			out = append(out, e)
		}
		return out
	}
	v.roots = build(nodes, Style{})
	v.elements = current
	for _, e := range current {
		if e.node.LabelFor != "" {
			if target := current[e.node.LabelFor]; target != nil && target.label == "" {
				target.label = e.node.Text
			}
		}
	}
}

// Only generated expression evaluation is a recoverable boundary. Panics in
// renderer/backend code remain visible as implementation failures in the CI.
func buildSafely(build func() []Node) (nodes []Node, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = fmt.Errorf("webui: generated render failed: %w", cause)
			} else {
				err = fmt.Errorf("webui: generated render failed: %v", failure)
			}
		}
	}()
	return build(), nil
}

func validateNodes(nodes []Node, ids map[string]bool) error {
	for _, n := range nodes {
		if n.ID == "" || ids[n.ID] {
			return fmt.Errorf("webui: node ID %q must be nonempty and unique", n.ID)
		}
		ids[n.ID] = true
		switch n.Kind {
		case "container", "text", "button", "input", "textarea", "link", "image":
		default:
			return fmt.Errorf("webui: unsupported native node kind %q at %q", n.Kind, n.ID)
		}
		if n.Kind != "container" && len(n.Children) > 0 {
			return fmt.Errorf("webui: %s %q must carry its text directly, not nested children", n.Kind, n.ID)
		}
		if n.Style.Measured {
			return fmt.Errorf("webui: node %q must supply browser geometry through ApplyMeasurements", n.ID)
		}
		if err := validateStyle(n.ID, n.Style); err != nil {
			return err
		}
		if n.Kind == "image" {
			if err := validateImageStyle(n.ID, n.Style); err != nil {
				return err
			}
		}
		if err := validateNodes(n.Children, ids); err != nil {
			return err
		}
	}
	return nil
}

func validateStyle(id string, s Style) error {
	values := []float32{s.X, s.Y, s.Width, s.Height, s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft,
		s.Gap, s.BorderWidth, s.Radius, s.FontSize, s.LineHeight, s.FontWeight, s.Opacity}
	for _, value := range values {
		if !finite(value) {
			return fmt.Errorf("webui: nonfinite style on %q", id)
		}
	}
	if s.Width < 0 || s.Height < 0 || s.PaddingTop < 0 || s.PaddingRight < 0 || s.PaddingBottom < 0 || s.PaddingLeft < 0 ||
		s.BorderWidth < 0 || s.Radius < 0 || s.FontSize < 0 || s.LineHeight < 0 || s.Gap < 0 {
		return fmt.Errorf("webui: negative dimensions on %q", id)
	}
	if s.Measured && s.Opacity != 1 {
		return fmt.Errorf("webui: group opacity on %q requires an unsupported compositor", id)
	}
	if s.Opacity < 0 || s.Opacity > 1 {
		return fmt.Errorf("webui: opacity outside 0..1 on %q", id)
	}
	for _, c := range []string{s.Background, s.Color, s.BorderColor} {
		if c != "" {
			if _, err := ParseColor(c); err != nil {
				return fmt.Errorf("webui: unsupported CSS color %q on %q", c, id)
			}
		}
	}
	if s.FontWeight < 0 || s.FontWeight > 1000 || s.FontWeight != float32(math.Trunc(float64(s.FontWeight))) {
		return fmt.Errorf("webui: unsupported font weight %v on %q", s.FontWeight, id)
	}
	if s.FontStyle != "" && s.FontStyle != "normal" && s.FontStyle != "italic" {
		return fmt.Errorf("webui: unsupported font style %q on %q", s.FontStyle, id)
	}
	if s.TextAlign != "" && s.TextAlign != "left" && s.TextAlign != "start" && s.TextAlign != "center" && s.TextAlign != "right" && s.TextAlign != "end" {
		return fmt.Errorf("webui: unsupported text alignment %q on %q", s.TextAlign, id)
	}
	if s.WhiteSpace != "" && s.WhiteSpace != "normal" && s.WhiteSpace != "nowrap" {
		return fmt.Errorf("webui: unsupported white-space %q on %q", s.WhiteSpace, id)
	}
	return nil
}

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func visualState(nodes []Node) string {
	type stateNode struct {
		ID, Kind, Text, Value, Placeholder, Href, Variant, Size string
		ImageHash                                               string
		Disabled                                                bool
		Style                                                   Style
		Children                                                []stateNode
	}
	var state func([]Node) []stateNode
	state = func(nodes []Node) []stateNode {
		out := make([]stateNode, len(nodes))
		for i, n := range nodes {
			imageHash := ""
			if n.Kind == "image" && n.ImageResource != nil {
				imageHash = fmt.Sprintf("%x", sha256.Sum256(n.ImageResource.Content()))
			}
			out[i] = stateNode{ID: n.ID, Kind: n.Kind, Text: n.Text, Value: n.Value,
				Placeholder: n.Placeholder, Href: n.Href, Variant: n.Variant, Size: n.Size,
				ImageHash: imageHash, Disabled: n.Disabled, Style: n.Style, Children: state(n.Children)}
		}
		return out
	}
	encoded, _ := json.Marshal(state(nodes)) // All fields are finite validated scalars.
	return string(encoded)
}

func (v *View) CreateRenderer() fyne.WidgetRenderer {
	r := &viewRenderer{view: v}
	r.Refresh()
	return r
}

type viewRenderer struct {
	view    *View
	objects []fyne.CanvasObject
}

func (r *viewRenderer) Destroy()                     {}
func (r *viewRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *viewRenderer) MinSize() fyne.Size {
	if r.view.viewport.Width > 0 {
		return r.view.viewport
	}
	return flowMin(r.objects, false, 0)
}
func (r *viewRenderer) Layout(size fyne.Size) {
	r.view.canvasErr = r.view.ValidateViewport(size)
	if r.view.boundCanvas != nil {
		r.view.canvasErr = errors.Join(r.view.canvasErr, r.view.ValidateCanvas())
	}
	measured := len(r.view.measurements) != 0
	for _, e := range r.view.roots {
		measured = measured && e.style.Measured
	}
	if measured {
		for _, e := range r.view.roots {
			placeMeasured(e)
		}
		return
	}
	flowLayout(r.objects, size, false, 0)
}
func (r *viewRenderer) Refresh() {
	r.objects = r.objects[:0]
	for _, e := range r.view.roots {
		r.objects = append(r.objects, e.object)
	}
	r.Layout(r.view.Size())
	canvas.Refresh(r.view)
}

type element struct {
	view           *View
	node           Node
	label          string
	style          Style
	children       []*element
	object         fyne.CanvasObject
	input          Editor
	renderer       *elementRenderer
	suppressChange bool
	image          *canvas.Image
	imageHash      [32]byte
	imageSize      fyne.Size
}

func newElement(v *View, n Node) *element {
	e := &element{view: v, node: n}
	switch n.Kind {
	case "button", "link":
		w := &actionWidget{element: e}
		w.ExtendBaseWidget(w)
		w.onTap = func() {
			if e.node.Disabled {
				return
			}
			if e.node.OnTap != nil {
				e.node.OnTap()
			} else if e.node.Kind == "link" && e.node.Href != "" {
				if destination, err := url.Parse(e.node.Href); err == nil {
					e.view.navigationErr = fyne.CurrentApp().OpenURL(destination)
				} else {
					e.view.navigationErr = err
				}
			}
			e.view.refreshAfterEvent()
		}
		e.object = w
	case "input", "textarea":
		w := &inputWidget{element: e, committedValue: n.Value}
		w.ExtendBaseWidget(w)
		e.input = v.backend.Editor(n.Kind == "textarea", v.backend.Defaults(n), func(value string) {
			if e.suppressChange || e.node.Disabled {
				return
			}
			w.dirty = true
			if e.node.OnChange != nil {
				e.node.OnChange(value)
				e.view.refreshAfterEvent()
			}
			// Without an immediate callback the editor owns its pending value until
			// commit. Reevaluating Node.Value here would erase every typed character.
		})
		e.object = w
	default:
		w := &plainWidget{element: e}
		w.ExtendBaseWidget(w)
		e.object = w
	}
	return e
}

func (e *element) update() {
	if e.style.Display == "none" {
		e.object.Hide()
	} else {
		e.object.Show()
	}
	if w, ok := e.object.(*actionWidget); ok {
		w.disabled = e.node.Disabled
	}
	if e.node.Kind == "image" {
		hash := sha256.Sum256(e.node.ImageResource.Content())
		if e.image == nil || e.imageHash != hash {
			asset := e.view.bitmaps[hash]
			e.image = canvas.NewImageFromResource(asset.resource)
			e.image.FillMode = canvas.ImageFillStretch
			e.image.ScaleMode = canvas.ImageScaleSmooth
			e.imageHash, e.imageSize = hash, asset.size
		}
	}
	if e.input != nil {
		e.suppressChange = true
		if e.input.Text() != e.node.Value {
			e.input.SetText(e.node.Value)
			if w, ok := e.object.(*inputWidget); ok && !w.dirty {
				w.committedValue = e.node.Value
			}
		}
		e.input.SetPlaceholder(e.node.Placeholder)
		e.input.SetDisabled(e.node.Disabled)
		e.input.SetStyle(e.style)
		e.input.Object().Refresh() // Materialize placeholder/scroller before the first Layout.
		e.suppressChange = false
	}
	if e.renderer != nil {
		e.renderer.Refresh()
	}
}

func resolveStyle(n Node, style, inherited Style, backend Backend) Style {
	if style.Measured {
		return style
	}
	defaults := backend.Defaults(n)
	if style.FontSize == 0 {
		style.FontSize = inherited.FontSize
	}
	if style.FontSize == 0 {
		style.FontSize = defaults.FontSize
	}
	if style.LineHeight == 0 {
		style.LineHeight = inherited.LineHeight
	}
	if style.LineHeight == 0 {
		style.LineHeight = defaults.LineHeight
	}
	if style.FontWeight == 0 {
		style.FontWeight = inherited.FontWeight
	}
	if style.FontWeight == 0 {
		style.FontWeight = defaults.FontWeight
	}
	if style.FontFamily == "" {
		style.FontFamily = inherited.FontFamily
	}
	if style.FontFamily == "" {
		style.FontFamily = defaults.FontFamily
	}
	if style.FontStyle == "" {
		style.FontStyle = inherited.FontStyle
	}
	if style.TextAlign == "" {
		style.TextAlign = inherited.TextAlign
	}
	if style.TextAlign == "" {
		style.TextAlign = defaults.TextAlign
	}
	if style.Color == "" {
		style.Color = inherited.Color
	}
	if style.Color == "" || n.Kind == "button" && n.Style.Color == "" {
		style.Color = defaults.Color
	}
	if style.Background == "" {
		style.Background = defaults.Background
	}
	if style.BorderColor == "" {
		style.BorderColor, style.BorderWidth = defaults.BorderColor, defaults.BorderWidth
	}
	if style.Radius == 0 {
		style.Radius = defaults.Radius
	}
	if style.Height == 0 {
		style.Height = defaults.Height
	}
	if style.Width == 0 {
		style.Width = defaults.Width
	}
	if style.PaddingLeft == 0 && style.PaddingRight == 0 {
		style.PaddingLeft, style.PaddingRight = defaults.PaddingLeft, defaults.PaddingRight
	}
	style.Opacity = 1
	return style
}

func colorCSS(c color.NRGBA) string {
	return fmt.Sprintf("rgba(%d, %d, %d, %g)", c.R, c.G, c.B, float64(c.A)/255)
}
func cssColor(s string) color.NRGBA { c, _ := ParseColor(s); return c }
func (e *element) childObjects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(e.children))
	for _, child := range e.children {
		out = append(out, child.object)
	}
	return out
}
func (e *element) textLines(width float32) []string {
	if e.node.Text == "" || e.node.Kind == "image" {
		return nil
	}
	if e.style.WhiteSpace == "nowrap" {
		return []string{e.node.Text}
	}
	return wrapText(e.node.Text, e.style, width, e.view.backend)
}

func (e *element) minSize() fyne.Size {
	s := e.style
	if s.Display == "none" {
		return fyne.NewSize(0, 0)
	}
	if s.Measured {
		return fyne.NewSize(s.Width, s.Height)
	}
	var size fyne.Size
	switch e.node.Kind {
	case "image":
		size = e.imageSize
		if s.Width > 0 && s.Height == 0 {
			size.Height *= s.Width / size.Width
		} else if s.Height > 0 && s.Width == 0 {
			size.Width *= s.Height / size.Height
		}
	case "container":
		size = flowMin(e.childObjects(), s.Direction == "row", s.Gap)
	case "button", "input", "textarea":
		size = fyne.NewSize(s.Width, s.Height)
		if e.node.Kind == "button" {
			size.Width = e.view.backend.Measure(e.node.Text, s).Width + s.PaddingLeft + s.PaddingRight + 2*s.BorderWidth
		}
	default:
		for _, text := range e.textLines(max(s.Width-s.PaddingLeft-s.PaddingRight-2*s.BorderWidth, 0)) {
			size.Width = max(size.Width, e.view.backend.Measure(text, s).Width)
			size.Height += s.LineHeight
		}
	}
	if e.node.Kind != "button" && e.node.Kind != "input" && e.node.Kind != "textarea" {
		size.Width += s.PaddingLeft + s.PaddingRight + 2*s.BorderWidth
		size.Height += s.PaddingTop + s.PaddingBottom + 2*s.BorderWidth
	}
	if s.Width > 0 {
		size.Width = s.Width
	}
	if s.Height > 0 {
		size.Height = s.Height
	}
	return size
}

type plainWidget struct {
	widget.BaseWidget
	element *element
}

func (w *plainWidget) CreateRenderer() fyne.WidgetRenderer { return newElementRenderer(w.element) }
func (w *plainWidget) AccessibilityLabel() string {
	if w.element.label != "" {
		return w.element.label
	}
	return w.element.node.Text
}
func (w *plainWidget) AccessibilityRole() fyne.AccessibleRole {
	if w.element.node.Kind == "image" {
		// Fyne 2.8 exposes no image role; retain the explicit alternative text.
		return fyne.AccessibleRoleText
	}
	if w.element.node.Kind == "container" {
		return fyne.AccessibleRoleContainer
	}
	return fyne.AccessibleRoleText
}

type actionWidget struct {
	widget.BaseWidget
	element           *element
	disabled, focused bool
	onTap             func()
}

func (w *actionWidget) CreateRenderer() fyne.WidgetRenderer { return newElementRenderer(w.element) }
func (w *actionWidget) AccessibilityRole() fyne.AccessibleRole {
	if w.element.node.Kind == "link" {
		return fyne.AccessibleRoleLink
	}
	return fyne.AccessibleRoleButton
}
func (w *actionWidget) Disabled() bool { return w.element.node.Disabled }
func (w *actionWidget) Enable()        { w.element.node.Disabled = false; w.disabled = false; w.Refresh() }
func (w *actionWidget) Disable()       { w.element.node.Disabled = true; w.disabled = true; w.Refresh() }

type inputWidget struct {
	widget.BaseWidget
	element        *element
	committedValue string
	dirty          bool
}

func (w *inputWidget) CreateRenderer() fyne.WidgetRenderer { return newElementRenderer(w.element) }
func (w *inputWidget) FocusGained() {
	w.committedValue, w.dirty = w.element.input.Text(), false
	w.element.input.FocusGained()
}
func (w *inputWidget) FocusLost() {
	w.element.input.FocusLost()
	w.commit()
}

func (w *inputWidget) commit() {
	if w.element.node.Disabled {
		return
	}
	value := w.element.input.Text()
	changed := w.dirty && value != w.committedValue
	w.committedValue, w.dirty = value, false
	if changed && w.element.node.OnCommit != nil {
		w.element.node.OnCommit(value)
		w.element.view.refreshAfterEvent()
	}
}
func (w *inputWidget) TypedRune(r rune) {
	if !w.element.node.Disabled {
		w.element.input.TypedRune(r)
	}
}
func (w *inputWidget) TypedKey(k *fyne.KeyEvent) {
	if !w.element.node.Disabled {
		w.element.input.TypedKey(k)
		if w.element.node.Kind == "input" && (k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter) {
			w.commit()
		}
	}
}
func (w *inputWidget) TypedShortcut(s fyne.Shortcut) {
	if !w.element.node.Disabled {
		w.element.input.TypedShortcut(s)
	}
}
func (w *inputWidget) Disabled() bool { return w.element.node.Disabled }
func (w *inputWidget) Enable() {
	w.element.node.Disabled = false
	w.element.input.SetDisabled(false)
	w.Refresh()
}
func (w *inputWidget) Disable() {
	w.element.node.Disabled = true
	w.element.input.SetDisabled(true)
	w.Refresh()
}
func (w *inputWidget) AccessibilityLabel() string {
	if w.element.label != "" {
		return w.element.label
	}
	return w.element.node.Placeholder
}
func (w *inputWidget) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }
func (w *inputWidget) Tapped(event *fyne.PointEvent) {
	if w.element.node.Disabled {
		return
	}
	c := w.element.view.boundCanvas
	if c == nil {
		c = fyne.CurrentApp().Driver().CanvasForObject(w)
	}
	if c != nil {
		c.Focus(w)
	}
	if event != nil {
		if pointer, ok := w.element.input.(interface{ Tapped(*fyne.PointEvent) }); ok {
			local := *event
			style := w.element.style
			local.Position = local.Position.Subtract(fyne.NewPos(style.PaddingLeft+style.BorderWidth, style.PaddingTop+style.BorderWidth))
			pointer.Tapped(&local)
		}
	}
}

type elementRenderer struct {
	element *element
	box     Box
	lines   []*canvas.Text
	objects []fyne.CanvasObject
}

func newElementRenderer(e *element) fyne.WidgetRenderer {
	r := &elementRenderer{element: e}
	e.renderer = r
	r.Refresh()
	return r
}
func (r *elementRenderer) MinSize() fyne.Size           { return r.element.minSize() }
func (r *elementRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *elementRenderer) Destroy()                     { r.element.renderer = nil }
func (r *elementRenderer) Refresh() {
	e, s := r.element, r.element.style
	background, border := cssColor(s.Background), cssColor(s.BorderColor)
	if s.BorderWidth == 0 {
		border = color.NRGBA{}
	}
	s.Background, s.BorderColor = colorCSS(background), colorCSS(border)
	r.box = e.view.backend.Box(s)
	r.objects = r.box.Objects()
	r.lines = nil
	if e.input != nil {
		r.objects = append(r.objects, e.input.Object())
	}
	if e.image != nil {
		r.objects = append(r.objects, e.image)
	}
	r.objects = append(r.objects, e.childObjects()...)
	r.Layout(e.object.Size())
	canvas.Refresh(e.object)
}
func (r *elementRenderer) Layout(size fyne.Size) {
	e, s := r.element, r.element.style
	r.box.Resize(size)
	left, top := s.PaddingLeft+s.BorderWidth, s.PaddingTop+s.BorderWidth
	width := max(size.Width-left-s.PaddingRight-s.BorderWidth, 0)
	height := max(size.Height-top-s.PaddingBottom-s.BorderWidth, 0)
	if e.input != nil {
		e.input.Layout(fyne.NewPos(left, top), fyne.NewSize(width, height))
	}
	if e.image != nil {
		e.image.Move(fyne.NewPos(left, top))
		e.image.Resize(fyne.NewSize(width, height))
	}
	if len(e.children) > 0 {
		if s.Measured {
			for _, child := range e.children {
				placeMeasured(child)
			}
		} else {
			objs := e.childObjects()
			flowLayout(objs, fyne.NewSize(width, height), s.Direction == "row", s.Gap)
			for _, child := range objs {
				pos := child.Position()
				child.Move(fyne.NewPos(pos.X+left, pos.Y+top))
			}
		}
	}
	texts := e.textLines(width)
	if len(texts) != len(r.lines) {
		r.lines = make([]*canvas.Text, len(texts))
		r.objects = r.box.Objects()
		if e.input != nil {
			r.objects = append(r.objects, e.input.Object())
		}
		if e.image != nil {
			r.objects = append(r.objects, e.image)
		}
		r.objects = append(r.objects, e.childObjects()...)
		for i, text := range texts {
			r.lines[i] = e.view.backend.Text(text, s)
			r.objects = append(r.objects, r.lines[i])
		}
	}
	textY := top
	if e.node.Kind == "button" {
		textY += (height - float32(len(texts))*s.LineHeight) / 2
	}
	for i, text := range texts {
		line := r.lines[i]
		line.Text, line.TextSize, line.Color = text, s.FontSize, cssColor(s.Color)
		x := left
		lineWidth := e.view.backend.Measure(text, s).Width
		switch s.TextAlign {
		case "center":
			x += (width - lineWidth) / 2
		case "right", "end":
			x += width - lineWidth
		}
		e.view.backend.PlaceText(line, s, x, textY+float32(i)*s.LineHeight)
		line.Refresh()
	}
}

func placeMeasured(e *element) {
	e.object.Move(fyne.NewPos(e.style.X, e.style.Y))
	e.object.Resize(fyne.NewSize(e.style.Width, e.style.Height))
}
