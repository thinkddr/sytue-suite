// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import "testing"

func TestTipoDe(t *testing.T) {
	casos := map[string]string{
		"v/hls/master.m3u8":        "application/vnd.apple.mpegurl",
		"v/hls/720p/seg_00001.m4s": "video/iso.segment",
		"v/hls/720p/init.mp4":      "video/mp4",
		"v/live/seg_0.ts":          "video/mp2t",
		"v/webm/720p.webm":         "video/webm",
		"v/poster.jpg":             "image/jpeg",
		"v/poster.JPEG":            "image/jpeg",
		"v/poster.webp":            "image/webp",
		"v/x.png":                  "image/png",
		"v/original":               "application/octet-stream",
	}
	for ruta, quiere := range casos {
		if got := TipoDe(ruta); got != quiere {
			t.Errorf("%s → %s, se esperaba %s", ruta, got, quiere)
		}
	}
}
