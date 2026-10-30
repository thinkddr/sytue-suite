// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package webdav_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/webdav"
)

type objS3Mem struct {
	datos []byte
	meta  map[string]string
}

type s3EnMemoria struct {
	mu      sync.RWMutex
	bucket  string
	objetos map[string]objS3Mem
	srv     *httptest.Server
}

func nuevoS3EnMemoria(t *testing.T, bucket string) *s3EnMemoria {
	t.Helper()
	s := &s3EnMemoria{
		bucket:  bucket,
		objetos: make(map[string]objS3Mem),
	}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		partes := strings.SplitN(path, "/", 2)
		if partes[0] != s.bucket {
			http.Error(w, "NoSuchBucket", http.StatusNotFound)
			return
		}
		if len(partes) == 1 || partes[1] == "" {
			if _, ok := r.URL.Query()["location"]; ok {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">fr-par</LocationConstraint>`)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		key := partes[1]
		switch r.Method {
		case http.MethodPut:
			if copySrc := r.Header.Get("X-Amz-Copy-Source"); copySrc != "" {
				dec, _ := url.PathUnescape(copySrc)
				dec = strings.TrimPrefix(dec, "/")
				srcParts := strings.SplitN(dec, "/", 2)
				s.mu.Lock()
				obj, ok := s.objetos[srcParts[1]]
				if !ok {
					s.mu.Unlock()
					w.WriteHeader(http.StatusNotFound)
					return
				}
				cpMeta := make(map[string]string, len(obj.meta))
				for k, v := range obj.meta {
					cpMeta[k] = v
				}
				s.objetos[key] = objS3Mem{datos: append([]byte(nil), obj.datos...), meta: cpMeta}
				s.mu.Unlock()
				sum := sha256.Sum256(obj.datos)
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CopyObjectResult><LastModified>%s</LastModified><ETag>"%x"</ETag></CopyObjectResult>`,
					time.Now().UTC().Format(time.RFC3339), sum[:16])
				return
			}
			body, _ := io.ReadAll(r.Body)
			meta := make(map[string]string)
			for k, vv := range r.Header {
				if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") && len(vv) > 0 {
					meta[k] = vv[0]
				}
			}
			s.mu.Lock()
			if prev, ok := s.objetos[key]; ok {
				if len(meta) == 0 && len(prev.meta) > 0 {
					meta = prev.meta
				}
				if pn := r.URL.Query().Get("partNumber"); pn != "" && pn != "1" {
					body = append(append([]byte(nil), prev.datos...), body...)
				}
			}
			s.objetos[key] = objS3Mem{datos: body, meta: meta}
			s.mu.Unlock()
			sum := sha256.Sum256(body)
			w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:16]) )
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			s.mu.RLock()
			obj, ok := s.objetos[key]
			s.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			for k, v := range obj.meta {
				w.Header().Set(k, v)
			}
			sum := sha256.Sum256(obj.datos)
			w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:16]))
			w.Header().Set("Content-Length", strconv.Itoa(len(obj.datos)))
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			s.mu.RLock()
			obj, ok := s.objetos[key]
			s.mu.RUnlock()
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code></Error>`)
				return
			}
			for k, v := range obj.meta {
				w.Header().Set(k, v)
			}
			sum := sha256.Sum256(obj.datos)
			w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:16]))
			w.Header().Set("Content-Length", strconv.Itoa(len(obj.datos)))
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(obj.datos)
		case http.MethodPost:
			if _, ok := r.URL.Query()["uploads"]; ok {
				meta := make(map[string]string)
				for k, vv := range r.Header {
					if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") && len(vv) > 0 {
						meta[k] = vv[0]
					}
				}
				s.mu.Lock()
				s.objetos[key] = objS3Mem{meta: meta}
				s.mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>up-1</UploadId></InitiateMultipartUploadResult>`, s.bucket, key)
				return
			}
			if r.URL.Query().Get("uploadId") != "" {
				s.mu.RLock()
				obj := s.objetos[key]
				s.mu.RUnlock()
				sum := sha256.Sum256(obj.datos)
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>"%x"</ETag></CompleteMultipartUploadResult>`, s.bucket, key, sum[:16])
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodDelete:
			s.mu.Lock()
			delete(s.objetos, key)
			s.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func nuevoServidorWebDAVReal(t *testing.T) http.Handler {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := service.NewStore(h.Pool)

	s3Srv := nuevoS3EnMemoria(t, "drive-blobs")
	u, err := url.Parse(s3Srv.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint:  u.Host,
		Region:    "fr-par",
		AccessKey: "ak",
		SecretKey: "sk",
		UseSSL:    true,
		Bucket:    "drive-blobs",
		Transport: s3Srv.srv.Client().Transport,
	})
	if err != nil {
		t.Fatalf("objectstore.New: %v", err)
	}
	svc := service.New(config.Config{
		DefaultQuotaBytes: 50 << 20,
		MaxUploadBytes:    10 << 20,
		PresignTTL:        15 * time.Minute,
		S3SecretKey:       "sk",
	}, st, blobs, nil, nil)

	dav := webdav.Handler("/dav", svc)
	// Middleware de prueba que inyecta el tenant y el owner_sub según cabeceras HTTP.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := r.Header.Get("X-Test-Tenant")
		if slug == "" {
			slug = "acme"
		}
		owner := r.Header.Get("X-Test-Owner")
		if owner == "" {
			owner = "alice"
		}
		ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: slug})
		ctx = store.WithOwner(ctx, owner)
		dav.ServeHTTP(w, r.WithContext(ctx))
	})
}

func peticionDAV(t *testing.T, h http.Handler, owner, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rBody io.Reader = http.NoBody
	if body != "" {
		rBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://drive.example.test"+target, rBody)
	req.Header.Set("X-Test-Owner", owner)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestWebDAVClienteRealFinderMacOS reproduce la secuencia real de macOS Finder
// contra el servidor WebDAV de Drive con PostgreSQL y S3 reales:
//   - OPTIONS y PROPFIND Depth 0/1
//   - MKCOL, subida de fichero + AppleDouble (._*) y .DS_Store (ocultos al listar)
//   - LOCK -> PUT con cabecera If sin etiqueta y con ruta relativa etiquetada
//     `If: </dav/...> (<opaquelocktoken:...>)` -> UNLOCK
//   - Renombrado de solo mayúsculas/minúsculas (`notas.txt` -> `Notas.txt`) con `Overwrite: F`
//   - Guardado atómico (`PUT` de temporal + `MOVE` sobre destino existente sin cabecera `Overwrite`)
//   - Mover + renombrar entre carpetas cuando el nombre antiguo ya existe en la carpeta destino
func TestWebDAVClienteRealFinderMacOS(t *testing.T) {
	h := nuevoServidorWebDAVReal(t)

	// 1. OPTIONS /dav/
	opt := peticionDAV(t, h, "alice", "OPTIONS", "/dav/", "", nil)
	if opt.Code != http.StatusOK {
		t.Fatalf("OPTIONS status=%d, quiero 200", opt.Code)
	}
	if !strings.Contains(opt.Header().Get("Dav"), "2") {
		t.Fatalf("cabecera DAV=%q debe anunciar clase 2", opt.Header().Get("Dav"))
	}

	// 2. MKCOL /dav/Proyectos y /dav/Archivo
	for _, dir := range []string{"/dav/Proyectos", "/dav/Archivo"} {
		mk := peticionDAV(t, h, "alice", "MKCOL", dir, "", nil)
		if mk.Code != http.StatusCreated {
			t.Fatalf("MKCOL %s status=%d, quiero 201", dir, mk.Code)
		}
	}

	// 3. Finder sube el fichero real + sus metadatos AppleDouble y .DS_Store.
	put1 := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Proyectos/notas.txt", "borrador inicial", nil)
	if put1.Code != http.StatusCreated {
		t.Fatalf("PUT notas.txt status=%d", put1.Code)
	}
	putAD := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Proyectos/._notas.txt", "appledouble", nil)
	if putAD.Code != http.StatusCreated {
		t.Fatalf("PUT ._notas.txt status=%d", putAD.Code)
	}
	putDS := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Proyectos/.DS_Store", "dsstore", nil)
	if putDS.Code != http.StatusCreated {
		t.Fatalf("PUT .DS_Store status=%d", putDS.Code)
	}

	pf := peticionDAV(t, h, "alice", "PROPFIND", "/dav/Proyectos", "", map[string]string{"Depth": "1"})
	if pf.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND Depth 1 status=%d", pf.Code)
	}
	xmlPF := pf.Body.String()
	if !strings.Contains(xmlPF, "notas.txt") || !strings.Contains(xmlPF, "._notas.txt") || !strings.Contains(xmlPF, ".DS_Store") {
		t.Fatalf("PROPFIND debe incluir notas.txt, ._notas.txt y .DS_Store para que Finder no falle con -36: %s", xmlPF)
	}
	delAD := peticionDAV(t, h, "alice", http.MethodDelete, "/dav/Proyectos/._notas.txt", "", nil)
	if delAD.Code != http.StatusNoContent {
		t.Fatalf("DELETE ._notas.txt status=%d", delAD.Code)
	}

	// 4. LOCK -> PUT con If etiquetado con ruta relativa `</dav/...> (<opaquelocktoken:...>)` -> UNLOCK
	lockBody := `<?xml version="1.0" encoding="utf-8" ?><D:lockinfo xmlns:D='DAV:'><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner><D:href>finder</D:href></D:owner></D:lockinfo>`
	lk := peticionDAV(t, h, "alice", "LOCK", "/dav/Proyectos/notas.txt", lockBody, map[string]string{"Timeout": "Second-600"})
	if lk.Code != http.StatusOK {
		t.Fatalf("LOCK status=%d body=%s", lk.Code, lk.Body.String())
	}
	lockToken := lk.Header().Get("Lock-Token")
	if lockToken == "" {
		t.Fatal("LOCK no devolvió cabecera Lock-Token")
	}

	// Finder y clientes RFC 4918 §10.4.2 envían tagged-list con ruta absoluta `</dav/...>`.
	putLocked := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Proyectos/notas.txt", "editado con lock", map[string]string{
		"If": "</dav/Proyectos/notas.txt> (" + lockToken + ")",
	})
	if putLocked.Code != http.StatusCreated {
		t.Fatalf("PUT con If etiquetado por ruta devolvió %d (esperaba 201): %s", putLocked.Code, putLocked.Body.String())
	}

	unlk := peticionDAV(t, h, "alice", "UNLOCK", "/dav/Proyectos/notas.txt", "", map[string]string{
		"Lock-Token": lockToken,
	})
	if unlk.Code != http.StatusNoContent {
		t.Fatalf("UNLOCK status=%d, quiero 204", unlk.Code)
	}

	// 5. Renombrado de solo mayúsculas/minúsculas (`notas.txt` -> `Notas.txt`) con Overwrite: F.
	mvCase := peticionDAV(t, h, "alice", "MOVE", "/dav/Proyectos/notas.txt", "", map[string]string{
		"Destination": "http://drive.example.test/dav/Proyectos/Notas.txt",
		"Overwrite":   "F",
	})
	if mvCase.Code != http.StatusCreated && mvCase.Code != http.StatusNoContent {
		t.Fatalf("MOVE solo cambio de caja devolvió %d, esperaba 201/204", mvCase.Code)
	}
	pfCase := peticionDAV(t, h, "alice", "PROPFIND", "/dav/Proyectos", "", map[string]string{"Depth": "1"})
	if !strings.Contains(pfCase.Body.String(), "Notas.txt") {
		t.Fatalf("tras renombrar a Notas.txt el listado sigue sin mostrar Notas.txt: %s", pfCase.Body.String())
	}

	// 6. Guardado atómico de Finder/Office: sube un temporal y hace MOVE sobre el
	// fichero existente SIN enviar cabecera Overwrite (por defecto es T según RFC 4918 §9.9.3).
	putTmp := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Proyectos/notas-tmp.txt", "versión atómica final", nil)
	if putTmp.Code != http.StatusCreated {
		t.Fatalf("PUT temporal status=%d", putTmp.Code)
	}
	mvAtom := peticionDAV(t, h, "alice", "MOVE", "/dav/Proyectos/notas-tmp.txt", "", map[string]string{
		"Destination": "http://drive.example.test/dav/Proyectos/Notas.txt",
	})
	if mvAtom.Code != http.StatusNoContent && mvAtom.Code != http.StatusCreated {
		t.Fatalf("MOVE sin cabecera Overwrite sobre destino existente devolvió %d, esperaba 204/201", mvAtom.Code)
	}
	getAtom := peticionDAV(t, h, "alice", http.MethodGet, "/dav/Proyectos/Notas.txt", "", nil)
	if getAtom.Code != http.StatusOK || getAtom.Body.String() != "versión atómica final" {
		t.Fatalf("GET tras guardado atómico: status=%d body=%q", getAtom.Code, getAtom.Body.String())
	}

	// 7. Mover + renombrar entre carpetas (`/dav/Proyectos/Notas.txt` -> `/dav/Archivo/final.txt`)
	// cuando en `/dav/Archivo` ya existe otro fichero llamado `Notas.txt`.
	putEnArchivo := peticionDAV(t, h, "alice", http.MethodPut, "/dav/Archivo/Notas.txt", "otro fichero en archivo", nil)
	if putEnArchivo.Code != http.StatusCreated {
		t.Fatalf("PUT en Archivo status=%d", putEnArchivo.Code)
	}
	mvCross := peticionDAV(t, h, "alice", "MOVE", "/dav/Proyectos/Notas.txt", "", map[string]string{
		"Destination": "http://drive.example.test/dav/Archivo/final.txt",
		"Overwrite":   "F",
	})
	if mvCross.Code != http.StatusCreated && mvCross.Code != http.StatusNoContent {
		t.Fatalf("MOVE+rename entre carpetas devolvió %d, esperaba 201/204", mvCross.Code)
	}
	getFinal := peticionDAV(t, h, "alice", http.MethodGet, "/dav/Archivo/final.txt", "", nil)
	if getFinal.Code != http.StatusOK || getFinal.Body.String() != "versión atómica final" {
		t.Fatalf("GET /dav/Archivo/final.txt: status=%d body=%q", getFinal.Code, getFinal.Body.String())
	}
}

// TestWebDAVClienteRealWindowsWebClient reproduce el patrón real del redirector
// Mini-Redirector (WebClient) de Windows Explorer:
//  1. PROPFIND con cuerpo vacío (Content-Length: 0).
//  2. Subida en dos pasos: PUT de 0 bytes para crear el recurso, LOCK, PUT con los
//     bytes reales.
//  3. PROPPATCH de Win32LastModifiedTime / Win32FileAttributes sobre el fichero
//     recién subido: debe responder 207 Multi-Status con 200 OK y NO truncar ni
//     sobreescribir con 0 bytes el contenido del fichero.
func TestWebDAVClienteRealWindowsWebClient(t *testing.T) {
	h := nuevoServidorWebDAVReal(t)

	// 1. PROPFIND con cuerpo vacío.
	pf := peticionDAV(t, h, "alice", "PROPFIND", "/dav/", "", map[string]string{"Depth": "1"})
	if pf.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND vacío status=%d, quiero 207", pf.Code)
	}

	// 2. PUT en 2 pasos (0 bytes y luego el contenido real bajo LOCK).
	put0 := peticionDAV(t, h, "alice", http.MethodPut, "/dav/presupuesto.xlsx", "", nil)
	if put0.Code != http.StatusCreated {
		t.Fatalf("PUT 0 bytes status=%d", put0.Code)
	}
	lockBody := `<?xml version="1.0" encoding="utf-8" ?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner><D:href>S-1-5-21-windows</D:href></D:owner></D:lockinfo>`
	lk := peticionDAV(t, h, "alice", "LOCK", "/dav/presupuesto.xlsx", lockBody, map[string]string{"Timeout": "Second-3600"})
	if lk.Code != http.StatusOK {
		t.Fatalf("LOCK status=%d", lk.Code)
	}
	token := lk.Header().Get("Lock-Token")

	contenido := strings.Repeat("HojaDeCalculoWindows2026!", 8) // 200 bytes
	putReal := peticionDAV(t, h, "alice", http.MethodPut, "/dav/presupuesto.xlsx", contenido, map[string]string{
		"If": "(" + token + ")",
	})
	if putReal.Code != http.StatusCreated {
		t.Fatalf("PUT contenido real status=%d", putReal.Code)
	}

	// 3. Windows WebClient envía PROPPATCH con Win32LastModifiedTime antes de UNLOCK.
	proppatchBody := `<?xml version="1.0" encoding="utf-8" ?>
<D:propertyupdate xmlns:D="DAV:" xmlns:Z="urn:schemas-microsoft-com:">
  <D:set>
    <D:prop>
      <Z:Win32LastModifiedTime>Thu, 29 Oct 2026 21:00:00 GMT</Z:Win32LastModifiedTime>
      <Z:Win32FileAttributes>00000020</Z:Win32FileAttributes>
    </D:prop>
  </D:set>
</D:propertyupdate>`
	pp := peticionDAV(t, h, "alice", "PROPPATCH", "/dav/presupuesto.xlsx", proppatchBody, map[string]string{
		"If": "(" + token + ")",
	})
	if pp.Code != http.StatusMultiStatus {
		t.Fatalf("PROPPATCH status=%d, quiero 207", pp.Code)
	}
	if strings.Contains(pp.Body.String(), "403 Forbidden") || !strings.Contains(pp.Body.String(), "200 OK") {
		t.Fatalf("PROPPATCH debe aceptar Win32LastModifiedTime con 200 OK, devolvió:\n%s", pp.Body.String())
	}

	unlk := peticionDAV(t, h, "alice", "UNLOCK", "/dav/presupuesto.xlsx", "", map[string]string{
		"Lock-Token": token,
	})
	if unlk.Code != http.StatusNoContent {
		t.Fatalf("UNLOCK status=%d", unlk.Code)
	}

	// Verificación crítica: el fichero NO debe haberse truncado a 0 bytes tras PROPPATCH.
	get := peticionDAV(t, h, "alice", http.MethodGet, "/dav/presupuesto.xlsx", "", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET tras PROPPATCH status=%d", get.Code)
	}
	if got := get.Body.String(); got != contenido {
		t.Fatalf("PROPPATCH corrompió o truncó el fichero: len=%d (esperaba %d)", len(got), len(contenido))
	}
}

// TestWebDAVClienteRealMicrosoftOfficeYMiUnidadAislada verifica el flujo completo
// de Microsoft Office (OPTIONS con MS-Author-Via: DAV, HEAD, LOCK, GET, PUT con
// token, UNLOCK) y el aislamiento de «Mi unidad» (owner_sub) entre dos personas
// de la misma organización tanto en ficheros como en locks.
func TestWebDAVClienteRealMicrosoftOfficeYMiUnidadAislada(t *testing.T) {
	h := nuevoServidorWebDAVReal(t)

	opt := peticionDAV(t, h, "alice", "OPTIONS", "/dav/", "", nil)
	if got := opt.Header().Get("MS-Author-Via"); got != "DAV" {
		t.Fatalf("MS-Author-Via=%q, Office requiere DAV", got)
	}

	v1 := bytes.Repeat([]byte("ContratoV1-"), 20)
	put1 := peticionDAV(t, h, "alice", http.MethodPut, "/dav/contrato.docx", string(v1), nil)
	if put1.Code != http.StatusCreated {
		t.Fatalf("PUT inicial status=%d", put1.Code)
	}

	head := peticionDAV(t, h, "alice", http.MethodHead, "/dav/contrato.docx", "", nil)
	if head.Code != http.StatusOK || head.Header().Get("Content-Length") != strconv.Itoa(len(v1)) {
		t.Fatalf("HEAD status=%d Content-Length=%q, quiero %d", head.Code, head.Header().Get("Content-Length"), len(v1))
	}

	lockBody := `<?xml version="1.0" encoding="utf-8" ?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner><D:href>OfficeUser</D:href></D:owner></D:lockinfo>`
	lk := peticionDAV(t, h, "alice", "LOCK", "/dav/contrato.docx", lockBody, map[string]string{"Timeout": "Second-1800"})
	if lk.Code != http.StatusOK {
		t.Fatalf("LOCK Office status=%d", lk.Code)
	}
	token := lk.Header().Get("Lock-Token")

	// Otra persona (bob) de la misma organización tiene su propia «Mi unidad»:
	// no ve el fichero de alice ni choca con el lock de alice al subir un fichero homónimo.
	pfBob := peticionDAV(t, h, "bob", "PROPFIND", "/dav/", "", map[string]string{"Depth": "1"})
	if strings.Contains(pfBob.Body.String(), "contrato.docx") {
		t.Fatalf("bob no debe ver contrato.docx de alice en su PROPFIND:\n%s", pfBob.Body.String())
	}
	getBob := peticionDAV(t, h, "bob", http.MethodGet, "/dav/contrato.docx", "", nil)
	if getBob.Code != http.StatusNotFound {
		t.Fatalf("GET de bob al fichero de alice devolvió %d, quiero 404", getBob.Code)
	}
	putBob := peticionDAV(t, h, "bob", http.MethodPut, "/dav/contrato.docx", "contrato privado de bob", nil)
	if putBob.Code != http.StatusCreated {
		t.Fatalf("bob debe poder subir su propio /dav/contrato.docx aunque alice tenga el suyo bloqueado: status=%d", putBob.Code)
	}

	// Alice actualiza su documento con su lock token y desbloquea.
	v2 := bytes.Repeat([]byte("ContratoV2Actualizado-"), 15)
	put2 := peticionDAV(t, h, "alice", http.MethodPut, "/dav/contrato.docx", string(v2), map[string]string{
		"If": "(" + token + ")",
	})
	if put2.Code != http.StatusCreated {
		t.Fatalf("PUT Office con lock status=%d", put2.Code)
	}
	unlk := peticionDAV(t, h, "alice", "UNLOCK", "/dav/contrato.docx", "", map[string]string{
		"Lock-Token": token,
	})
	if unlk.Code != http.StatusNoContent {
		t.Fatalf("UNLOCK Office status=%d", unlk.Code)
	}

	getAlice := peticionDAV(t, h, "alice", http.MethodGet, "/dav/contrato.docx", "", nil)
	if getAlice.Body.String() != string(v2) {
		t.Fatalf("el documento de alice no tiene la versión 2")
	}
	getBob2 := peticionDAV(t, h, "bob", http.MethodGet, "/dav/contrato.docx", "", nil)
	if getBob2.Body.String() != "contrato privado de bob" {
		t.Fatalf("el documento de bob se mezcló con el de alice: %q", getBob2.Body.String())
	}
}
