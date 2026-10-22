// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package generated

import "testing"

func TestGeneratedJSXReferencesAndExpressionLiteralsKeepDifferentSemantics(t *testing.T) {
	view, _, _ := conformanceView(t)
	// Markup references decode once; a JS string expression is already literal
	// text and must retain its ampersand sequence across native reevaluation.
	const want = "A & B 🙂 © &amp;"
	requireGeneratedText(t, view, "entity-conformance", want)
	tapGenerated(t, view, "left-increment")
	requireGeneratedText(t, view, "entity-conformance", want)
}
