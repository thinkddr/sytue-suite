// SPDX-License-Identifier: Apache-2.0
package model

import (
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	MaxPlaylistBytes = 1 << 20
	MaxPlaylistLines = 20000
)

func playlistTarget(key, reference string) (string, error) {
	parsed, err := url.Parse(reference)
	if err != nil || reference == "" || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" || strings.HasPrefix(parsed.Path, "/") || strings.ContainsAny(parsed.Path, "\\\x00\r\n") || strings.Contains(parsed.Path, "{$") {
		return "", ErrSnapshot
	}
	return path.Join(path.Dir(key), parsed.Path), nil
}

func attributeURI(attributes string) (string, error) {
	var result string
	for attributes != "" {
		key, value, ok := strings.Cut(attributes, "=")
		if !ok {
			return result, nil
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		var rest string
		if strings.HasPrefix(value, `"`) {
			value, rest, ok = strings.Cut(value[1:], `"`)
			if !ok || rest != "" && !strings.HasPrefix(rest, ",") {
				return "", ErrSnapshot
			}
			rest = strings.TrimPrefix(rest, ",")
		} else {
			value, rest, _ = strings.Cut(value, ",")
		}
		if key == "URI" {
			if value == "" || result != "" {
				return "", ErrSnapshot
			}
			result = value
		} else if strings.HasSuffix(key, "URI") {
			return "", ErrSnapshot
		}
		attributes = rest
	}
	return result, nil
}

// PlaylistReferences reads only bounded retained bytes. Every reference remains
// a local object relationship; this function never makes a network request.
func PlaylistReferences(key string, raw []byte) ([]string, error) {
	if len(raw) == 0 || len(raw) > MaxPlaylistBytes || !utf8.Valid(raw) {
		return nil, ErrSnapshot
	}
	text := strings.TrimPrefix(string(raw), "\uFEFF")
	lines := strings.Split(text, "\n")
	if len(lines) > MaxPlaylistLines || strings.TrimSpace(lines[0]) != "#EXTM3U" {
		return nil, ErrSnapshot
	}
	references := []string{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		reference := line
		if strings.HasPrefix(line, "#") {
			_, attributes, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			var err error
			reference, err = attributeURI(attributes)
			if err != nil {
				return nil, err
			}
			if reference == "" {
				continue
			}
		}
		target, err := playlistTarget(key, reference)
		if err != nil {
			return nil, err
		}
		references = append(references, target)
	}
	return references, nil
}

// ValidatePlaylists requires all playlists, their init/key objects, variants and
// segments to be retained in the same video's authenticated media inventory.
func (s *Snapshot) ValidatePlaylists(playlists map[string][]byte) error {
	media := s.Media()
	byVideo := map[string]map[string]string{}
	for _, m := range media {
		if byVideo[m.VideoID] == nil {
			byVideo[m.VideoID] = map[string]string{}
		}
		byVideo[m.VideoID][m.Key] = m.ID
	}
	edges := map[string][]string{}
	for _, m := range media {
		if path.Ext(m.Key) != ".m3u8" {
			continue
		}
		raw, exists := playlists[m.ID]
		if !exists {
			return ErrSnapshot
		}
		references, err := PlaylistReferences(m.Key, raw)
		if err != nil {
			return err
		}
		for _, key := range references {
			target, exists := byVideo[m.VideoID][key]
			if !exists {
				return ErrSnapshot
			}
			if path.Ext(key) == ".m3u8" {
				edges[m.ID] = append(edges[m.ID], target)
			}
		}
		if edges[m.ID] == nil {
			edges[m.ID] = []string{}
		}
	}
	if len(edges) != len(playlists) {
		return ErrSnapshot
	}
	return validatePlaylistCycles(edges)
}

func validatePlaylistCycles(edges map[string][]string) error {
	states := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		if states[id] == 1 {
			return ErrSnapshot
		}
		if states[id] == 2 {
			return nil
		}
		states[id] = 1
		for _, target := range edges[id] {
			if err := visit(target); err != nil {
				return err
			}
		}
		states[id] = 2
		return nil
	}
	for id := range edges {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
