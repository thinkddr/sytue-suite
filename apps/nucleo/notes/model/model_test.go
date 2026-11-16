// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSharedNoteNormalizationSanitizesEveryContentKindWithoutRoundingData(t *testing.T) {
	for _, kind := range []Kind{KindText, KindTodo, KindImage, KindFile, KindLink, KindColor, KindLauncher, KindCrossref, KindDoc, KindDrive, KindEvent, KindRoom} {
		t.Run(string(kind), func(t *testing.T) {
			data := json.RawMessage(`{"future":{"integer":900719925474099312345,"scientific":1.234567890123456789e+99}}`)
			input := NoteInput{Kind: kind, Title: "  title  ", Content: `<p onclick="steal()">safe<script>steal()</script><a href="javascript:steal()">text</a></p>`, Data: data}
			if err := input.Normalize(false); err != nil {
				t.Fatal(err)
			}
			if input.Title != "title" || strings.Contains(input.Content, "steal") || !strings.Contains(input.Content, "safe") {
				t.Fatal("shared normalization did not sanitize", input.Content)
			}
			if string(input.Data) != string(data) {
				t.Fatal("normalization rounded or rewrote domain JSON")
			}
		})
	}
}

func TestSharedNotebookNormalizationKeepsExistingDefaultsAndValidation(t *testing.T) {
	input := NotebookInput{Title: "  personal  "}
	if err := input.Normalize(); err != nil {
		t.Fatal(err)
	}
	if input.Title != "personal" || input.Layout != LayoutTree || input.Color != "#86198f" || input.Visibility != VisPersonal || input.Indexable == nil || !*input.Indexable {
		t.Fatal("legacy defaults diverged", input)
	}
	bad := NotebookInput{Title: "unsafe", Color: "url(https://example.test)"}
	if err := bad.Normalize(); err == nil {
		t.Fatal("invalid color accepted")
	}
}
