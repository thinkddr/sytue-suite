// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"image/color"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// The small source-layout mode is deterministic. It intentionally does not attempt
// CSS flex/grid inference: capture provides every child's actual border-box bounds.
func flowMin(objects []fyne.CanvasObject, row bool, gap float32) fyne.Size {
	var primary, cross float32
	count := 0
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		size := object.MinSize()
		if count > 0 {
			primary += gap
		}
		count++
		if row {
			primary += size.Width
			cross = max(cross, size.Height)
		} else {
			primary += size.Height
			cross = max(cross, size.Width)
		}
	}
	if row {
		return fyne.NewSize(primary, cross)
	}
	return fyne.NewSize(cross, primary)
}
func flowLayout(objects []fyne.CanvasObject, size fyne.Size, row bool, gap float32) {
	var offset float32
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		minimum := object.MinSize()
		if row {
			object.Move(fyne.NewPos(offset, 0))
			object.Resize(minimum)
			offset += minimum.Width + gap
		} else {
			object.Move(fyne.NewPos(0, offset))
			object.Resize(fyne.NewSize(size.Width, minimum.Height))
			offset += minimum.Height + gap
		}
	}
}
func wrapText(text string, style Style, width float32, backend Backend) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	if width <= 0 {
		return []string{strings.Join(words, " ")}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		i := len(lines) - 1
		candidate := lines[i] + " " + word
		if backend.Measure(candidate, style).Width <= width {
			lines[i] = candidate
		} else {
			lines = append(lines, word)
		}
	}
	return lines
}

func (w *actionWidget) Tapped(*fyne.PointEvent) {
	if !w.disabled && w.onTap != nil {
		w.onTap()
	}
}
func (w *actionWidget) FocusGained() { w.focused = true; w.Refresh() }
func (w *actionWidget) FocusLost()   { w.focused = false; w.Refresh() }
func (w *actionWidget) TypedRune(r rune) {
	if r == ' ' {
		w.Tapped(nil)
	}
}
func (w *actionWidget) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
		w.Tapped(nil)
	}
}
func (w *actionWidget) AccessibilityLabel() string {
	if w.element.label != "" {
		return w.element.label
	}
	return w.element.node.Text
}

// primitiveEditor is an independently authored native editing engine for the
// initial compatibility subset. It handles Unicode runes, cursor motion, selection
// and clipboard shortcuts. IME composition, rich text, undo and platform spellcheck
// need a host editor adapter and are not claimed by this initial backend.
type primitiveEditor struct {
	canvas                      *editorCanvas
	backend                     Backend
	style                       Style
	text, placeholder           string
	cursor, anchor              int
	active, disabled, multiline bool
	onChange                    func(string)
}

func (b FyneBackend) Editor(multiline bool, style Style, onChange func(string)) Editor {
	return NewEditor(b, multiline, style, onChange)
}

// NewEditor reuses the public editing engine with the host's text measurement and
// placement. Its canvas owns drawing only: View owns focus, keyboard events and
// HTML commit callbacks. This factory never calls Backend.Editor, so a backend's
// Editor implementation may safely delegate here without recursive construction.
func NewEditor(backend Backend, multiline bool, style Style, onChange func(string)) Editor {
	if backend == nil {
		backend = FyneBackend{}
	}
	e := &primitiveEditor{backend: backend, style: style, multiline: multiline, onChange: onChange}
	e.canvas = &editorCanvas{editor: e}
	e.canvas.ExtendBaseWidget(e.canvas)
	return e
}
func (e *primitiveEditor) Object() fyne.CanvasObject { return e.canvas }
func (e *primitiveEditor) Refresh()                  { e.canvas.Refresh() }
func (e *primitiveEditor) Size() fyne.Size           { return e.canvas.Size() }
func (e *primitiveEditor) Text() string              { return e.text }
func (e *primitiveEditor) SetText(text string) {
	if text == e.text {
		return
	}
	e.text = text
	e.cursor = len([]rune(text))
	e.anchor = e.cursor
	e.Refresh()
}
func (e *primitiveEditor) SetPlaceholder(text string) { e.placeholder = text; e.Refresh() }
func (e *primitiveEditor) SetDisabled(disabled bool)  { e.disabled = disabled; e.Refresh() }
func (e *primitiveEditor) SetStyle(style Style)       { e.style = style; e.Refresh() }
func (e *primitiveEditor) Layout(position fyne.Position, size fyne.Size) {
	e.canvas.Move(position)
	e.canvas.Resize(size)
}
func (e *primitiveEditor) FocusGained()   { e.active = true; e.Refresh() }
func (e *primitiveEditor) FocusLost()     { e.active = false; e.Refresh() }
func (e *primitiveEditor) Disabled() bool { return e.disabled }
func (e *primitiveEditor) selected() (int, int) {
	return min(e.cursor, e.anchor), max(e.cursor, e.anchor)
}
func (e *primitiveEditor) replace(value string) {
	if e.disabled {
		return
	}
	runes := []rune(e.text)
	lo, hi := e.selected()
	insert := []rune(value)
	result := append([]rune{}, runes[:lo]...)
	result = append(result, insert...)
	result = append(result, runes[hi:]...)
	e.text = string(result)
	e.cursor = lo + len(insert)
	e.anchor = e.cursor
	e.Refresh()
	if e.onChange != nil {
		e.onChange(e.text)
	}
}
func (e *primitiveEditor) TypedRune(r rune) {
	if !unicode.IsControl(r) {
		e.replace(string(r))
	}
}
func (e *primitiveEditor) TypedKey(event *fyne.KeyEvent) {
	if e.disabled {
		return
	}
	runes := []rune(e.text)
	lo, hi := e.selected()
	switch event.Name {
	case fyne.KeyLeft:
		e.cursor = max(e.cursor-1, 0)
		e.anchor = e.cursor
	case fyne.KeyRight:
		e.cursor = min(e.cursor+1, len(runes))
		e.anchor = e.cursor
	case fyne.KeyHome:
		e.cursor = 0
		e.anchor = 0
	case fyne.KeyEnd:
		e.cursor = len(runes)
		e.anchor = e.cursor
	case fyne.KeyBackspace:
		if lo != hi {
			e.replace("")
			return
		}
		if e.cursor > 0 {
			e.anchor = e.cursor - 1
			e.replace("")
			return
		}
	case fyne.KeyDelete:
		if lo != hi {
			e.replace("")
			return
		}
		if e.cursor < len(runes) {
			e.anchor = e.cursor + 1
			e.replace("")
			return
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.multiline {
			e.replace("\n")
			return
		}
	}
	e.Refresh()
}
func (e *primitiveEditor) TypedShortcut(shortcut fyne.Shortcut) {
	lo, hi := e.selected()
	runes := []rune(e.text)
	switch s := shortcut.(type) {
	case *fyne.ShortcutSelectAll:
		e.anchor = 0
		e.cursor = len(runes)
		e.Refresh()
	case *fyne.ShortcutCopy:
		if s.Clipboard != nil {
			s.Clipboard.SetContent(string(runes[lo:hi]))
		}
	case *fyne.ShortcutCut:
		if !e.disabled && s.Clipboard != nil {
			s.Clipboard.SetContent(string(runes[lo:hi]))
			e.replace("")
		}
	case *fyne.ShortcutPaste:
		if !e.disabled && s.Clipboard != nil {
			value := s.Clipboard.Content()
			if !e.multiline {
				value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
			}
			e.replace(value)
		}
	}
}
func (e *primitiveEditor) Tapped(event *fyne.PointEvent) {
	if e.disabled {
		return
	}
	if event == nil {
		return
	}
	lines := strings.Split(e.text, "\n")
	row := 0
	if e.multiline && e.style.LineHeight > 0 {
		row = min(max(int(event.Position.Y/e.style.LineHeight), 0), len(lines)-1)
	}
	runes := []rune(lines[row])
	x := event.Position.X
	width := e.backend.Measure(lines[row], e.style).Width
	switch e.style.TextAlign {
	case "center":
		x -= (e.Size().Width - width) / 2
	case "right", "end":
		x -= e.Size().Width - width
	}
	cursor := len(runes)
	previous := float32(0)
	for i := range runes {
		advance := e.backend.Measure(string(runes[:i+1]), e.style).Width
		if x < (previous+advance)/2 {
			cursor = i
			break
		}
		previous = advance
	}
	for _, line := range lines[:row] {
		cursor += len([]rune(line)) + 1
	}
	e.cursor, e.anchor = cursor, cursor
	e.Refresh()
}

// editorCanvas owns only drawing. Keeping keyboard focus on inputWidget avoids
// exposing two tab stops and two accessibility controls for one HTML input.
type editorCanvas struct {
	widget.BaseWidget
	editor *primitiveEditor
}

func (c *editorCanvas) CreateRenderer() fyne.WidgetRenderer { return c.editor.CreateRenderer() }

func (e *primitiveEditor) CreateRenderer() fyne.WidgetRenderer {
	r := &editorRenderer{editor: e, cursor: canvas.NewRectangle(parseColor(e.style.Color)), selection: canvas.NewRectangle(color.NRGBA{59, 130, 246, 60})}
	r.Refresh()
	return r
}

type editorRenderer struct {
	editor            *primitiveEditor
	lines             []*canvas.Text
	cursor, selection *canvas.Rectangle
	objects           []fyne.CanvasObject
}

func (r *editorRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.editor.style.LineHeight) }
func (r *editorRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *editorRenderer) Destroy()                     {}
func (r *editorRenderer) IsClip()                      {}
func (r *editorRenderer) Refresh() {
	e := r.editor
	s := e.style
	text := e.text
	if text == "" {
		text = e.placeholder
		c := parseColor(s.Color)
		c.A = uint8(float64(c.A) * 0.5)
		s.Color = colorCSS(c)
	}
	lines := strings.Split(text, "\n")
	if !e.multiline {
		lines = []string{text}
	}
	r.lines = make([]*canvas.Text, len(lines))
	r.objects = []fyne.CanvasObject{r.selection}
	for i, line := range lines {
		r.lines[i] = e.backend.Text(line, s)
		r.objects = append(r.objects, r.lines[i])
	}
	r.objects = append(r.objects, r.cursor)
	r.Layout(e.Size())
	canvas.Refresh(e.canvas)
}
func (r *editorRenderer) Layout(size fyne.Size) {
	e := r.editor
	s := e.style
	y := float32(0)
	if !e.multiline {
		y = (size.Height - s.LineHeight) / 2
	}
	for i, line := range r.lines {
		x := float32(0)
		width := e.backend.Measure(line.Text, s).Width
		switch s.TextAlign {
		case "center":
			x = (size.Width - width) / 2
		case "right", "end":
			x = size.Width - width
		}
		e.backend.PlaceText(line, s, x, y+float32(i)*s.LineHeight)
	}
	r.cursor.Hide()
	r.selection.Hide()
	if !e.active || e.disabled {
		return
	}
	runes := []rune(e.text)
	prefix := string(runes[:e.cursor])
	row := strings.Count(prefix, "\n")
	if i := strings.LastIndexByte(prefix, '\n'); i >= 0 {
		prefix = prefix[i+1:]
	}
	x := e.backend.Measure(prefix, s).Width
	r.cursor.Move(fyne.NewPos(x, y+float32(row)*s.LineHeight))
	r.cursor.Resize(fyne.NewSize(1, s.LineHeight))
	r.cursor.FillColor = parseColor(s.Color)
	r.cursor.Show()
	lo, hi := e.selected()
	if lo != hi && !strings.ContainsRune(string(runes[lo:hi]), '\n') {
		start := e.backend.Measure(string(runes[:lo]), s).Width
		end := e.backend.Measure(string(runes[:hi]), s).Width
		r.selection.Move(fyne.NewPos(start, y))
		r.selection.Resize(fyne.NewSize(end-start, s.LineHeight))
		r.selection.Show()
	}
}
