// SPDX-License-Identifier: Apache-2.0
package recorrido

import "fyne.io/fyne/v2"

// Buscar returns visible objects matching a semantic predicate, in tree order.
// It shares the same maintained widget traversal as the interaction sweep.
func Buscar(root fyne.CanvasObject, coincide func(fyne.CanvasObject) bool) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	var visit func(fyne.CanvasObject)
	visit = func(object fyne.CanvasObject) {
		if object == nil || !object.Visible() {
			return
		}
		if coincide(object) {
			out = append(out, object)
		}
		for _, child := range hijos(object) {
			visit(child)
		}
	}
	if coincide != nil {
		visit(root)
	}
	return out
}
