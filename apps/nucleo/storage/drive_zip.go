// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MaxDriveZipFiles bounds an archive's central index on both service and device.
const MaxDriveZipFiles = 5000

// MaxDriveSelectionItems is the suite's limit for an explicit Drive selection.
const MaxDriveSelectionItems = 1000

// DriveZipPath keeps archive entries relative, strips parent navigation and
// neutralizes control characters and Windows drive prefixes in legacy names.
// Domain names stay unchanged.
func DriveZipPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		seg = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f || r == ':' {
				return '_'
			}
			return r
		}, seg)
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

// UniqueDriveZipNames updates normalized paths without changing their order or
// count. Empty paths remain omitted by legacy exports. All other paths must be
// local; suffixes avoid file/directory collisions and case/Unicode aliases.
func UniqueDriveZipNames(names []string) error {
	used, directories := map[string]bool{}, map[string]bool{}
	fold := cases.Fold()
	key := func(name string) string { return fold.String(norm.NFC.String(name)) }
	for _, name := range names {
		if name == "" {
			continue
		}
		if !fs.ValidPath(name) || name == "." || len(name) > 0xffff || strings.ContainsAny(name, ":\\") {
			return errors.New("storage: invalid Drive archive path")
		}
		for folder := path.Dir(name); folder != "." && folder != "/"; folder = path.Dir(folder) {
			directories[key(folder)] = true
		}
	}
	for i := range names {
		entry := &names[i]
		if *entry == "" {
			continue
		}
		original, name := *entry, *entry
		for n := 2; used[key(name)] || directories[key(name)]; n++ {
			ext := path.Ext(original)
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(original, ext), n, ext)
		}
		if len(name) > 0xffff {
			return errors.New("storage: Drive archive path exceeds ZIP name field")
		}
		used[key(name)] = true
		*entry = name
	}
	return nil
}
