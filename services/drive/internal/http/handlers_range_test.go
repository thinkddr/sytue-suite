package drivehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// TestDownloadFileRange guarda lo que necesita un <video>/<audio> para buscar: un
// Range de un tramo contesta 206 con el trozo justo, y lo que no se sabe servir
// cae al 200 entero (nunca a un error).
func TestDownloadFileRange(t *testing.T) {
	const body = "0123456789"
	for _, tc := range []struct {
		name, rng, ifRange string
		code               int
		want, crange       string
	}{
		{"sin Range", "", "", 200, body, ""},
		{"tramo cerrado", "bytes=2-5", "", 206, "2345", "bytes 2-5/10"},
		{"hasta el final", "bytes=7-", "", 206, "789", "bytes 7-9/10"},
		{"sufijo", "bytes=-3", "", 206, "789", "bytes 7-9/10"},
		{"fin recortado", "bytes=8-99", "", 206, "89", "bytes 8-9/10"},
		{"fuera del fichero", "bytes=10-", "", 416, "", "bytes */10"},
		{"varios tramos", "bytes=0-1,4-5", "", 200, body, ""},
		{"sintaxis rara", "items=0-1", "", 200, body, ""},
		{"If-Range coincide", "bytes=0-0", `"abc"`, 206, "0", "bytes 0-0/10"},
		{"If-Range caducado", "bytes=0-0", `"otro"`, 200, body, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.New()
			h := newHandlers(&fakeSvc{
				openFile: domain.File{ID: id, Name: "v.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "abc"},
				openBody: body,
			})
			req := mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()})
			if tc.rng != "" {
				req.Header.Set("Range", tc.rng)
			}
			if tc.ifRange != "" {
				req.Header.Set("If-Range", tc.ifRange)
			}
			rec := httptest.NewRecorder()
			h.DownloadFile(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("código = %d, quiero %d", rec.Code, tc.code)
			}
			if rec.Body.String() != tc.want {
				t.Fatalf("cuerpo = %q, quiero %q", rec.Body.String(), tc.want)
			}
			if got := rec.Header().Get("Content-Range"); got != tc.crange {
				t.Fatalf("Content-Range = %q, quiero %q", got, tc.crange)
			}
			if rec.Header().Get("Accept-Ranges") != "bytes" {
				t.Fatal("falta Accept-Ranges: bytes")
			}
		})
	}
}
