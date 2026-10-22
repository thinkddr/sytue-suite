// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Un master como el que escribe ffmpeg con -master_pl_name y var_stream_map.
const masterTres = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-STREAM-INF:BANDWIDTH=5640800,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
1080p/index.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=2890800,RESOLUTION=1280x720,CODECS="avc1.64001f,mp4a.40.2"
720p/index.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=1020800,RESOLUTION=640x360,CODECS="avc1.64001e,mp4a.40.2"
360p/index.m3u8
`

func TestFusionarMaster(t *testing.T) {
	// Quitar: quedan las que quedan, con su línea tal cual.
	got, err := fusionarMaster(masterTres, "", []int{720})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "1080p/") || strings.Contains(got, "360p/") || !strings.Contains(got, "RESOLUTION=1280x720") ||
		!strings.HasPrefix(got, "#EXTM3U\n#EXT-X-VERSION:7\n") {
		t.Fatalf("quitar 1080 y 360:\n%s", got)
	}
	// Añadir: la nueva del master de ffmpeg entra en su sitio por tasa.
	nuevo := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-STREAM-INF:BANDWIDTH=5640800,RESOLUTION=1920x1080,CODECS=\"avc1.640028,mp4a.40.2\"\n1080p/index.m3u8\n"
	got, err = fusionarMaster(got, nuevo, []int{1080, 720})
	if err != nil {
		t.Fatal(err)
	}
	if i, j := strings.Index(got, "1080p/index.m3u8"), strings.Index(got, "720p/index.m3u8"); i < 0 || j < 0 || i > j {
		t.Fatalf("1080 delante de 720:\n%s", got)
	}
	// Una variante que no está en ninguno de los dos es un error, no un master cojo.
	if _, err := fusionarMaster(masterTres, "", []int{480}); err == nil {
		t.Fatal("se esperaba error por la variante que falta")
	}
	if _, err := fusionarMaster("no es un master", "", []int{720}); err == nil {
		t.Fatal("se esperaba error por un master ilegible")
	}
}

func TestPlanDeCalidades(t *testing.T) {
	w, h := int32(1920), int32(1080)
	v := domain.Video{Kind: domain.KindUpload, HlsKey: "x/hls/master.m3u8", WebmKey: "x/webm/720p.webm", Width: &w, Height: &h}
	// Un vídeo de antes: HLS completo, WebM solo 720; la recomendada son las tres.
	p := planDe(v)
	if !slices.Equal(p.webmFaltan, []int{1080, 360}) || len(p.webmSobran) != 0 || len(p.hlsFaltan) != 0 || len(p.hlsSobran) != 0 || !p.webmAntiguo {
		t.Fatalf("vídeo de antes: %+v", p)
	}
	// Elige 720: sobran 1080 y 360 del HLS; del WebM nada falta ni sobra.
	v.Qualities = []int32{720}
	p = planDe(v)
	if !slices.Equal(p.hlsSobran, []int{1080, 360}) || len(p.webmFaltan) != 0 || len(p.webmSobran) != 0 {
		t.Fatalf("elegir 720: %+v", p)
	}
	// Un directo grabado: su HLS no cuenta.
	v.Kind = domain.KindLive
	if p = planDe(v); p.conHLS || len(p.hlsSobran) != 0 {
		t.Fatalf("grabación: %+v", p)
	}
}

// Quitar calidades no necesita ffmpeg: borra sus ficheros, reescribe el master
// y declara lo que queda MEDIDO en el almacén.
func TestTrabajoDeCalidadesQuitaYLibera(t *testing.T) {
	ctx := context.Background()
	cola := nuevaColaFalsa()
	alm := nuevoAlmacenFalso()
	w := trabajadorDePrueba(cola, alm, t.TempDir())
	w.cfg = configDe(t.TempDir())
	acme := tenancy.Tenant{Slug: "acme"}

	ancho, alto := int32(1920), int32(1080)
	v := videoDePrueba(domain.KindUpload)
	v.Status = domain.StatusReady
	v.Width, v.Height = &ancho, &alto
	pref := v.ID.String() + "/"
	v.OriginalKey, v.HlsKey, v.WebmKey = pref+"original", pref+"hls/master.m3u8", pref+"webm/720p.webm"
	v.HlsQualities, v.WebmQualities = []int32{1080, 720, 360}, []int32{1080, 720, 360}
	v.Qualities = []int32{720}
	alm.poner(acme, v.OriginalKey, "video/mp4", make([]byte, 1000))
	alm.poner(acme, v.HlsKey, "application/vnd.apple.mpegurl", []byte(masterTres))
	for _, q := range []string{"1080p", "720p", "360p"} {
		alm.poner(acme, pref+"hls/"+q+"/index.m3u8", "application/vnd.apple.mpegurl", []byte("#EXTM3U\n"))
		alm.poner(acme, pref+"hls/"+q+"/seg_00000.m4s", "video/iso.segment", make([]byte, 300))
		alm.poner(acme, pref+"webm/"+q+".webm", "video/webm", make([]byte, 200))
	}
	v.BytesOriginal, v.BytesTotal = 1000, 99999
	j := cola.encolar(v, domain.JobQualities, "")

	if _, err := w.unaVuelta(ctx); err != nil {
		t.Fatal(err)
	}
	if m, ok := cola.fallos[j.ID]; ok {
		t.Fatalf("el trabajo falló: %s", m)
	}
	est, ok := cola.terminados[j.ID]
	if !ok {
		t.Fatal("el trabajo no terminó")
	}
	for _, k := range []string{"webm/1080p.webm", "webm/360p.webm", "hls/1080p/index.m3u8", "hls/360p/seg_00000.m4s"} {
		if _, _, ok := alm.leer(acme, pref+k); ok {
			t.Errorf("%s tenía que borrarse", k)
		}
	}
	for _, k := range []string{"webm/720p.webm", "hls/720p/seg_00000.m4s", "original"} {
		if _, _, ok := alm.leer(acme, pref+k); !ok {
			t.Errorf("%s tenía que quedarse", k)
		}
	}
	master, _, _ := alm.leer(acme, v.HlsKey)
	if strings.Contains(string(master), "1080p/") || !strings.Contains(string(master), "720p/index.m3u8") {
		t.Errorf("el master sigue anunciando lo borrado:\n%s", master)
	}
	if !slices.Equal(est.WebmQualities, []int32{720}) || !slices.Equal(est.HlsQualities, []int32{720}) {
		t.Errorf("calidades anotadas: webm %v hls %v", est.WebmQualities, est.HlsQualities)
	}
	if est.WebmKey == nil || *est.WebmKey != pref+"webm/720p.webm" {
		t.Errorf("webm_key: %v", est.WebmKey)
	}
	// original 1000 + 720p (index 8 + segmento 300 + webm 200) + master reescrito.
	quiere := int64(1000 + 8 + 300 + 200 + len(master))
	if est.BytesTotal == nil || *est.BytesTotal != quiere {
		t.Errorf("bytes_total %d, se esperaba %d (lo medido, no lo que había)", derefInt64(est.BytesTotal), quiere)
	}
	if !slices.Equal(est.Elegidas, v.Qualities) {
		t.Errorf("la elección vista viaja al Terminar para detectar un cambio a medias: %v", est.Elegidas)
	}
}

// Sin nada que añadir ni quitar no se baja nada: solo se normalizan las columnas.
func TestTrabajoDeCalidadesSinCambiosNoBajaNada(t *testing.T) {
	ctx := context.Background()
	cola := nuevaColaFalsa()
	alm := nuevoAlmacenFalso()
	w := trabajadorDePrueba(cola, alm, t.TempDir())
	w.cfg = configDe(t.TempDir())
	ancho, alto := int32(640), int32(360)
	v := videoDePrueba(domain.KindUpload)
	v.Width, v.Height = &ancho, &alto
	pref := v.ID.String() + "/"
	// Sin original en el almacén: si intentara bajarlo, fallaría.
	v.OriginalKey, v.HlsKey, v.WebmKey = pref+"original", pref+"hls/master.m3u8", pref+"webm/360p.webm"
	v.HlsQualities, v.WebmQualities = []int32{360}, []int32{360}
	j := cola.encolar(v, domain.JobQualities, "")
	if _, err := w.unaVuelta(ctx); err != nil {
		t.Fatal(err)
	}
	if m, ok := cola.fallos[j.ID]; ok {
		t.Fatalf("no había nada que hacer y falló: %s", m)
	}
}
