// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"reflect"
	"strings"

	"fyne.io/fyne/v2"
)

const (
	maxBitmapBytes  = 20 * 1024 * 1024
	maxBitmapSide   = 16384
	maxBitmapPixels = 64 * 1024 * 1024
)

type bitmapAsset struct {
	size     fyne.Size
	resource fyne.Resource
}

// ValidateBitmap accepts fully decodable, bounded PNG/JPEG resources. Browser
// orientation, animation and color-profile metadata require a matching decoder,
// so the initial native subset rejects these instead of ignoring them. Scaling
// parity still requires the browser/native pixel gate at the requested scale.
func ValidateBitmap(resource fyne.Resource) (fyne.Size, error) {
	content, err := bitmapContent(resource)
	if err != nil {
		return fyne.Size{}, err
	}
	return validateBitmapContent(content)
}

func bitmapContent(resource fyne.Resource) ([]byte, error) {
	if resource == nil || reflect.ValueOf(resource).Kind() == reflect.Pointer && reflect.ValueOf(resource).IsNil() {
		return nil, fmt.Errorf("webui: image requires a local PNG/JPEG resource")
	}
	if strings.HasSuffix(strings.ToLower(resource.Name()), ".svg") {
		return nil, fmt.Errorf("webui: SVG resources require an unsupported vector renderer")
	}
	content := resource.Content()
	if len(content) == 0 || len(content) > maxBitmapBytes {
		return nil, fmt.Errorf("webui: image resource must contain 1..%d bytes", maxBitmapBytes)
	}
	return content, nil
}

func validateBitmapContent(content []byte) (fyne.Size, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return fyne.Size{}, fmt.Errorf("webui: cannot decode image header: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return fyne.Size{}, fmt.Errorf("webui: unsupported bitmap format %q; use PNG or JPEG", format)
	}
	if format == "png" && (len(content) < 33 || content[24] != 8 || content[25] != 2 && content[25] != 6) {
		return fyne.Size{}, fmt.Errorf("webui: initial PNG subset requires 8-bit RGB or RGBA pixels")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxBitmapSide || config.Height > maxBitmapSide ||
		int64(config.Width)*int64(config.Height) > maxBitmapPixels {
		return fyne.Size{}, fmt.Errorf("webui: bitmap dimensions %dx%d exceed the supported bounds", config.Width, config.Height)
	}
	if err := validateBitmapMetadata(content, format); err != nil {
		return fyne.Size{}, err
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return fyne.Size{}, fmt.Errorf("webui: cannot decode image pixels: %w", err)
	}
	if decodedFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return fyne.Size{}, fmt.Errorf("webui: decoded bitmap disagrees with its header")
	}
	return fyne.NewSize(float32(config.Width), float32(config.Height)), nil
}

func validateBitmapMetadata(content []byte, format string) error {
	if format == "png" {
		for offset := 8; offset+12 <= len(content); {
			length := uint64(binary.BigEndian.Uint32(content[offset : offset+4]))
			if length > uint64(len(content)-offset-12) {
				return fmt.Errorf("webui: truncated PNG chunk")
			}
			kind := string(content[offset+4 : offset+8])
			switch kind {
			case "iCCP", "gAMA", "cHRM", "eXIf", "acTL":
				return fmt.Errorf("webui: PNG %s metadata requires an unsupported color/orientation/animation decoder", kind)
			}
			offset += int(length) + 12
		}
		return nil
	}
	// Follow every segment, including metadata between progressive scans. Stuffed
	// bytes and restart markers inside entropy-coded data are not segment headers.
	for offset := 2; offset < len(content); {
		if content[offset] != 0xff {
			return fmt.Errorf("webui: invalid JPEG marker")
		}
		for offset < len(content) && content[offset] == 0xff {
			offset++
		}
		if offset == len(content) {
			return fmt.Errorf("webui: truncated JPEG marker")
		}
		marker := content[offset]
		offset++
		if marker == 0xd9 {
			return nil
		}
		if marker == 0xd8 || marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if offset+2 > len(content) {
			return fmt.Errorf("webui: truncated JPEG segment")
		}
		length := int(binary.BigEndian.Uint16(content[offset : offset+2]))
		if length < 2 || length > len(content)-offset {
			return fmt.Errorf("webui: truncated JPEG segment")
		}
		segment := content[offset+2 : offset+length]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) ||
			marker == 0xe2 && bytes.HasPrefix(segment, []byte("ICC_PROFILE\x00")) ||
			marker == 0xee && bytes.HasPrefix(segment, []byte("Adobe")) {
			return fmt.Errorf("webui: JPEG color/orientation metadata requires an unsupported decoder")
		}
		offset += length
		if marker == 0xda {
			for offset < len(content) {
				if content[offset] != 0xff {
					offset++
					continue
				}
				start := offset
				for offset < len(content) && content[offset] == 0xff {
					offset++
				}
				if offset == len(content) {
					return fmt.Errorf("webui: truncated JPEG scan marker")
				}
				if content[offset] == 0 || content[offset] >= 0xd0 && content[offset] <= 0xd7 {
					offset++
					continue
				}
				offset = start
				break
			}
		}
	}
	return nil // The complete decoder subsequently rejects missing scan data.
}

func validateImageStyle(id string, style Style) error {
	if style.Radius != 0 || style.BorderWidth != 0 || style.PaddingTop != 0 || style.PaddingRight != 0 || style.PaddingBottom != 0 || style.PaddingLeft != 0 {
		return fmt.Errorf("webui: image %q requires zero radius, border and padding in the initial bitmap subset", id)
	}
	return nil
}

// Freeze validated content so a host mutating its Resource cannot change pixels
// behind an already recorded visual-state fingerprint. Repeated state renders
// reuse decoded validation results by content hash; each new tree is copied.
func (v *View) freezeImages(nodes []Node) ([]Node, error) {
	out := make([]Node, len(nodes))
	for i, node := range nodes {
		out[i] = node
		if node.Kind == "image" {
			content, err := bitmapContent(node.ImageResource)
			if err != nil {
				return nil, fmt.Errorf("webui: image %q: %w", node.ID, err)
			}
			hash := sha256.Sum256(content)
			asset, exists := v.bitmaps[hash]
			if !exists {
				frozen := bytes.Clone(content)
				size, err := validateBitmapContent(frozen)
				if err != nil {
					return nil, fmt.Errorf("webui: image %q: %w", node.ID, err)
				}
				asset = bitmapAsset{size, fyne.NewStaticResource(node.ImageResource.Name(), frozen)}
				v.bitmaps[hash] = asset
			}
			out[i].ImageResource = asset.resource
		}
		children, err := v.freezeImages(node.Children)
		if err != nil {
			return nil, err
		}
		out[i].Children = children
	}
	return out, nil
}
