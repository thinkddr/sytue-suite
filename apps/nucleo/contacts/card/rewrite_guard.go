// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-vcard"
)

// errUnsafeVCardRewrite means that this codec cannot safely rewrite this input
// under the checks below. It does not mean that the input is invalid vCard.
var errUnsafeVCardRewrite = errors.New("contacts: vCard no apta para reescritura")

// guardVCardRewrite is an unconnected, conservative pre-decode guard. It must
// only inspect the original bytes, never a card already parsed by go-vcard. It
// catches the specific ambiguities below and lines that go-vcard discards; it
// is not a vCard validator or a proof of semantic preservation. It is NOT wired
// into mutations: recovery of previously prepared WAL entries must first be
// coordinated. For example, managed fields may lose parameters or groups,
// URI values containing commas may be escaped incorrectly, and newly supplied
// structured fields can be misencoded. maxBytes is a positive, caller-supplied
// input bound. Some valid cards are intentionally rejected, including version 3
// cards with VERSION in another position and some safe escaped or list values.
//
// Each byte is scanned a constant number of times; the auxiliary memory is
// bounded by the longest unfolded line, not by the number of properties.
func guardVCardRewrite(data string, maxBytes int) error {
	if maxBytes <= 0 || data == "" || len(data) > maxBytes {
		return errUnsafeVCardRewrite
	}
	var line strings.Builder
	haveLine := false
	state := 0 // begin, version, properties, end
	consume := func() error {
		if !haveLine {
			return nil
		}
		value := line.String()
		line.Reset()
		haveLine = false
		var err error
		state, err = advanceVCardRewriteState(state, value)
		return err
	}

	for data != "" {
		physical, rest, hasNewline, err := nextPhysicalVCardLine(data)
		if err != nil {
			return err
		}
		data = rest
		if physical[0] == ' ' || physical[0] == '\t' {
			if !haveLine || !hasNewline {
				return errUnsafeVCardRewrite
			}
			line.WriteString(physical[1:])
			continue
		}
		if err := consume(); err != nil {
			return err
		}
		line.WriteString(physical)
		haveLine = true
	}
	if err := consume(); err != nil {
		return err
	}
	if state != 3 {
		return errUnsafeVCardRewrite
	}
	return nil
}

func advanceVCardRewriteState(state int, value string) (int, error) {
	if !utf8.ValidString(value) {
		return state, errUnsafeVCardRewrite
	}
	switch state {
	case 0:
		if !strings.EqualFold(value, "BEGIN:VCARD") {
			return state, errUnsafeVCardRewrite
		}
		return 1, nil
	case 1:
		if !strings.EqualFold(value, "VERSION:3.0") && !strings.EqualFold(value, "VERSION:4.0") {
			return state, errUnsafeVCardRewrite
		}
		return 2, nil
	case 2:
		if strings.EqualFold(value, "END:VCARD") {
			return 3, nil
		}
		return state, guardVCardContentLine(value)
	default:
		return state, errUnsafeVCardRewrite
	}
}

func nextPhysicalVCardLine(data string) (string, string, bool, error) {
	var physical string
	hasNewline := false
	if i := strings.IndexByte(data, '\n'); i >= 0 {
		physical, data = data[:i], data[i+1:]
		physical = strings.TrimSuffix(physical, "\r")
		hasNewline = true
	} else {
		physical, data = data, ""
	}
	if strings.ContainsRune(physical, '\r') || physical == "" {
		return "", "", false, errUnsafeVCardRewrite
	}
	for i := 0; i < len(physical); i++ {
		if physical[i] < 0x20 && physical[i] != '\t' || physical[i] == 0x7f {
			return "", "", false, errUnsafeVCardRewrite
		}
	}
	return physical, data, hasNewline, nil
}

// guardVCardContentLine checks only framing and ambiguity-prone syntax. It
// deliberately leaves property interpretation to the codec.
func guardVCardContentLine(line string) error {
	name, i, err := parseVCardContentName(line)
	if err != nil {
		return err
	}
	i, err = skipVCardParams(line, i)
	if err != nil {
		return err
	}
	if i == len(line) || line[i] != ':' {
		return errUnsafeVCardRewrite
	}
	value := line[i+1:]
	if strings.Contains(value, `\;`) {
		return errUnsafeVCardRewrite
	}
	switch name {
	case "N", "ADR", "ORG":
		if strings.ContainsAny(value, `\,`) {
			return errUnsafeVCardRewrite
		}
	case "CATEGORIES", "NICKNAME":
		if strings.ContainsRune(value, ',') {
			return errUnsafeVCardRewrite
		}
	}

	// Decode one isolated content line to detect the decoder's silent skip on
	// parse errors. This only detects a skipped line, not a wrong interpretation.
	reader := io.MultiReader(strings.NewReader("BEGIN:VCARD\r\n"), strings.NewReader(line), strings.NewReader("\r\nEND:VCARD\r\n"))
	card, err := vcard.NewDecoder(reader).Decode()
	if err != nil {
		return errUnsafeVCardRewrite
	}
	fields := 0
	for _, values := range card {
		fields += len(values)
	}
	if fields != 1 {
		return errUnsafeVCardRewrite
	}
	return nil
}

func parseVCardContentName(line string) (string, int, error) {
	i := 0
	for i < len(line) && line[i] != ';' && line[i] != ':' {
		if !vcardNameByte(line[i]) && line[i] != '.' {
			return "", 0, errUnsafeVCardRewrite
		}
		i++
	}
	if i == 0 || i == len(line) {
		return "", 0, errUnsafeVCardRewrite
	}
	name := line[:i]
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		if dot == 0 || dot == len(name)-1 || strings.ContainsRune(name[:dot], '.') {
			return "", 0, errUnsafeVCardRewrite
		}
		name = name[dot+1:]
	}
	name = strings.ToUpper(name)
	if name == "BEGIN" || name == "END" || name == "VERSION" {
		return "", 0, errUnsafeVCardRewrite
	}
	return name, i, nil
}

func skipVCardParams(line string, i int) (int, error) {
	for i < len(line) && line[i] == ';' {
		i++
		start := i
		for i < len(line) && vcardNameByte(line[i]) {
			i++
		}
		if i == start || i == len(line) || line[i] != '=' {
			return 0, errUnsafeVCardRewrite
		}
		var err error
		i, err = skipVCardParamValue(line, i+1)
		if err != nil {
			return 0, err
		}
	}
	return i, nil
}

func skipVCardParamValue(line string, i int) (int, error) {
	if i == len(line) {
		return 0, errUnsafeVCardRewrite
	}
	if line[i] == '"' {
		i++
		for i < len(line) && line[i] != '"' {
			if strings.ContainsRune(`:;,\`, rune(line[i])) {
				return 0, errUnsafeVCardRewrite
			}
			i++
		}
		if i == len(line) {
			return 0, errUnsafeVCardRewrite
		}
		return i + 1, nil
	}
	start := i
	for i < len(line) && line[i] != ';' && line[i] != ':' {
		if strings.ContainsRune(`,"\`, rune(line[i])) {
			return 0, errUnsafeVCardRewrite
		}
		i++
	}
	if i == start {
		return 0, errUnsafeVCardRewrite
	}
	return i, nil
}

func vcardNameByte(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-'
}
