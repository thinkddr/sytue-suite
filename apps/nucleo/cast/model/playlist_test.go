// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func mediaSnapshot(t *testing.T, objects map[string][]byte) (*Snapshot, []storage.StreamSourceRow, map[string][]byte) {
	t.Helper()
	rows := make([]Row, 0, 1+len(objects))
	rows = append(rows, retainedRow(t, "video", `{"id":"`+retainedVideo+`","tenant":"acme","owner_sub":"alice","kind":"upload","status":"ready","visibility":"private","title":"private media","original_key":"v/original.mp4","hls_key":"v/master.m3u8","webm_key":"v/video.webm","poster_key":"v/poster.png"}`))
	descriptors := make([]storage.StreamSourceRow, 0, len(objects))
	playlists := map[string][]byte{}
	for key, body := range objects {
		id, err := MediaID(retainedVideo, key)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		m := MediaRow{ID: id, Tenant: "acme", Owner: "alice", VideoID: retainedVideo, Key: key, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ContentType: "application/octet-stream"}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		row, err := RetainedRow("media", raw)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
		descriptors = append(descriptors, m.Descriptor())
		if strings.HasSuffix(key, ".m3u8") {
			playlists[id] = body
		}
	}
	snapshot, err := NewSnapshot(castScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, descriptors, playlists
}

func TestCastPrivateMediaRequiresOriginalRootsAndEveryPlaylistReference(t *testing.T) {
	for _, scenario := range []string{"complete", "missing original", "missing segment", "external URL", "cycle"} {
		t.Run(scenario, func(t *testing.T) {
			objects := map[string][]byte{"v/original.mp4": []byte("exact original"), "v/master.m3u8": []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100000\nvariants/v0.m3u8\n"), "v/variants/v0.m3u8": []byte("#EXTM3U\n#EXT-X-MAP:URI=\"../init.mp4\"\n#EXTINF:1,\n../segments/0.ts\n#EXT-X-ENDLIST\n"), "v/init.mp4": []byte("exact init"), "v/segments/0.ts": []byte("exact segment"), "v/video.webm": []byte("exact webm"), "v/poster.png": []byte("exact poster")}
			switch scenario {
			case "missing original":
				delete(objects, "v/original.mp4")
			case "missing segment":
				delete(objects, "v/segments/0.ts")
			case "external URL":
				objects["v/master.m3u8"] = []byte("#EXTM3U\nhttps://external.invalid/video.m3u8\n")
			case "cycle":
				objects["v/master.m3u8"] = []byte("#EXTM3U\nmaster.m3u8\n")
			}
			snapshot, descriptors, playlists := mediaSnapshot(t, objects)
			err := snapshot.ValidateMedia(descriptors)
			if err == nil {
				err = snapshot.ValidatePlaylists(playlists)
			}
			if scenario == "complete" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrSnapshot) {
				t.Fatal("incomplete private media accepted", err)
			}
		})
	}
}

func TestCastPrivateMediaRequiresEveryAdvertisedWebMQuality(t *testing.T) {
	for _, tc := range []struct {
		name      string
		qualities []int
		missing   string
		wantError bool
	}{
		{name: "complete", qualities: []int{720, 360}},
		{name: "missing alternative", qualities: []int{720, 360}, missing: "v/360p.webm", wantError: true},
		{name: "duplicate", qualities: []int{720, 720}, wantError: true},
		{name: "invalid", qualities: []int{0}, wantError: true},
		{name: "legacy single file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := map[string][]byte{"v/original.mp4": []byte("original"), "v/master.m3u8": []byte("#EXTM3U\n"), "v/video.webm": []byte("default"), "v/poster.png": []byte("poster"), "v/720p.webm": []byte("720"), "v/360p.webm": []byte("360")}
			delete(objects, tc.missing)
			snapshot, descriptors, _ := mediaSnapshot(t, objects)
			key := "video\x00" + retainedVideo
			row := snapshot.rows[key]
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(row.JSON, &fields); err != nil {
				t.Fatal(err)
			}
			fields["webm_qualities"], _ = json.Marshal(tc.qualities)
			row.JSON, _ = json.Marshal(fields)
			snapshot.rows[key] = row
			err := snapshot.ValidateMedia(descriptors)
			if tc.wantError != errors.Is(err, ErrSnapshot) {
				t.Fatalf("ValidateMedia: %v", err)
			}
		})
	}
}

func TestCastVideoRejectsMalformedPlaybackProjection(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		wantError   bool
	}{
		{name: "legacy absent"},
		{name: "legacy null", field: `,"webm_qualities":null`},
		{name: "valid qualities", field: `,"webm_qualities":[720,360]`},
		{name: "string qualities", field: `,"webm_qualities":"720"`, wantError: true},
		{name: "partially valid qualities", field: `,"webm_qualities":[720,"360"]`, wantError: true},
		{name: "invalid media key", field: `,"webm_key":42`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"id":"` + retainedVideo + `","tenant":"acme","owner_sub":"alice","kind":"upload","status":"ready","visibility":"private","title":"video"` + tc.field + `}`
			row := retainedRow(t, "video", raw)
			err := ValidateRow(castScope(), row)
			if tc.wantError != errors.Is(err, ErrSnapshot) {
				t.Fatalf("ValidateRow: %v", err)
			}
		})
	}
}

func TestCastPlaylistParserBoundsInputAndHandlesQuotedURIs(t *testing.T) {
	references, err := PlaylistReferences("v/master.m3u8", []byte("#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",URI=\"audio,main.m3u8\"\n"))
	if err != nil || len(references) != 1 || references[0] != "v/audio,main.m3u8" {
		t.Fatal("quoted URI changed", references, err)
	}
	for _, raw := range [][]byte{[]byte(strings.Repeat("x", MaxPlaylistBytes+1)), []byte("#EXTM3U\n" + strings.Repeat("\n", MaxPlaylistLines)), []byte("not a playlist"), []byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"https://external.invalid/key\"\n"), []byte("#EXTM3U\nfile:///etc/passwd\n"), []byte("#EXTM3U\n#EXT-X-CONTENT-STEERING:server-uri=\"https://external.invalid/control\"\n"), []byte("#EXTM3U\n#EXT-X-CONTENT-STEERING:SERVER-URI=\"https://external.invalid/control\"\n"), []byte("#EXTM3U\n#EXT-X-MEDIA:URI=\"local.m3u8\",URI=\"https://external.invalid/media\"\n"), []byte("#EXTM3U\n//external.invalid/media.ts\n")} {
		if _, err := PlaylistReferences("v/master.m3u8", raw); !errors.Is(err, ErrSnapshot) {
			t.Fatal("unbounded or external reference accepted", err)
		}
	}
}
