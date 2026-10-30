// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
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
)

type objetoS3 struct {
	datos       []byte
	contentType string
	meta        map[string]string
	modTime     time.Time
}

// servidorS3Real es un servidor HTTP S3 compatible en memoria que verifica
// firmas AWS Signature Version 4 en URLs prefirmadas (incluyendo cabeceras
// firmadas como content-length) y sirve las operaciones reales de minio-go
// (PUT, GET, HEAD, DELETE, CopyObject).
type servidorS3Real struct {
	mu        sync.RWMutex
	bucket    string
	region    string
	accessKey string
	secretKey string
	objetos   map[string]objetoS3
	srv       *httptest.Server
}

func nuevoServidorS3Real(t *testing.T, bucket, region, accessKey, secretKey string) *servidorS3Real {
	t.Helper()
	s := &servidorS3Real{
		bucket:    bucket,
		region:    region,
		accessKey: accessKey,
		secretKey: secretKey,
		objetos:   make(map[string]objetoS3),
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.servirHTTP))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *servidorS3Real) existe(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.objetos[key]
	return ok
}

func (s *servidorS3Real) poner(key string, datos []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := append([]byte(nil), datos...)
	s.objetos[key] = objetoS3{
		datos:       cp,
		contentType: "application/octet-stream",
		modTime:     time.Now().UTC(),
	}
}

func (s *servidorS3Real) servirHTTP(w http.ResponseWriter, r *http.Request) {
	// Si es una petición con URL prefirmada (X-Amz-Signature en query),
	// verificamos la firma AWS SigV4 antes de aceptar nada.
	if r.URL.Query().Get("X-Amz-Signature") != "" {
		if err := s.verificarPresignSigV4(r); err != nil {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>SignatureDoesNotMatch</Code><Message>%s</Message></Error>`, err.Error())
			return
		}
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	partes := strings.SplitN(path, "/", 2)
	bucket := partes[0]
	if bucket != s.bucket {
		http.Error(w, "NoSuchBucket", http.StatusNotFound)
		return
	}
	// Operaciones a nivel de bucket (BucketExists, getBucketLocation).
	if len(partes) == 1 || partes[1] == "" {
		if _, ok := r.URL.Query()["location"]; ok {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">%s</LocationConstraint>`, s.region)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	key := partes[1]

	switch r.Method {
	case http.MethodPut:
		if copySrc := r.Header.Get("X-Amz-Copy-Source"); copySrc != "" {
			s.manejarCopy(w, r, copySrc, key)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		meta := make(map[string]string)
		for k, vv := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") && len(vv) > 0 {
				meta[k] = vv[0]
			}
		}
		ct := r.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}
		sum := sha256.Sum256(body)
		etag := fmt.Sprintf(`"%x"`, sum[:16])
		s.mu.Lock()
		s.objetos[key] = objetoS3{
			datos:       body,
			contentType: ct,
			meta:        meta,
			modTime:     time.Now().UTC(),
		}
		s.mu.Unlock()
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)

	case http.MethodHead:
		s.mu.RLock()
		obj, ok := s.objetos[key]
		s.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		escribirCabecerasObjeto(w, obj)
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		s.mu.RLock()
		obj, ok := s.objetos[key]
		s.mu.RUnlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
			return
		}
		escribirCabecerasObjeto(w, obj)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.datos)

	case http.MethodDelete:
		s.mu.Lock()
		delete(s.objetos, key)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func escribirCabecerasObjeto(w http.ResponseWriter, obj objetoS3) {
	for k, v := range obj.meta {
		w.Header().Set(k, v)
	}
	sum := sha256.Sum256(obj.datos)
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:16]))
	w.Header().Set("Content-Type", obj.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(obj.datos)))
	w.Header().Set("Last-Modified", obj.modTime.UTC().Format(http.TimeFormat))
}

func (s *servidorS3Real) manejarCopy(w http.ResponseWriter, _ *http.Request, copySrc, dstKey string) {
	decoded, err := url.PathUnescape(copySrc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	decoded = strings.TrimPrefix(decoded, "/")
	partes := strings.SplitN(decoded, "/", 2)
	if len(partes) != 2 || partes[0] != s.bucket {
		http.Error(w, "InvalidCopySource", http.StatusBadRequest)
		return
	}
	srcKey := partes[1]

	s.mu.Lock()
	src, ok := s.objetos[srcKey]
	if !ok {
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code></Error>`)
		return
	}
	cpMeta := make(map[string]string, len(src.meta))
	for k, v := range src.meta {
		cpMeta[k] = v
	}
	now := time.Now().UTC()
	s.objetos[dstKey] = objetoS3{
		datos:       append([]byte(nil), src.datos...),
		contentType: src.contentType,
		meta:        cpMeta,
		modTime:     now,
	}
	s.mu.Unlock()

	sum := sha256.Sum256(src.datos)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CopyObjectResult><LastModified>%s</LastModified><ETag>"%x"</ETag></CopyObjectResult>`,
		now.Format(time.RFC3339), sum[:16])
}

// verificarPresignSigV4 implementa la verificación canónica AWS SigV4 para URLs
// prefirmadas, comprobando fecha, expiración, credenciales, cabeceras firmadas
// (incluyendo content-length si figura en X-Amz-SignedHeaders) y HMAC-SHA256.
func (s *servidorS3Real) verificarPresignSigV4(r *http.Request) error {
	q := r.URL.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		return errors.New("algoritmo no soportado")
	}
	cred := q.Get("X-Amz-Credential")
	amzDate := q.Get("X-Amz-Date")
	signedHeadersStr := q.Get("X-Amz-SignedHeaders")
	gotSig := q.Get("X-Amz-Signature")
	if cred == "" || amzDate == "" || signedHeadersStr == "" || gotSig == "" {
		return errors.New("faltan parámetros SigV4")
	}
	t, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return fmt.Errorf("fecha inválida: %w", err)
	}
	scope := strings.Join([]string{t.Format("20060102"), s.region, "s3", "aws4_request"}, "/")
	if cred != s.accessKey+"/"+scope {
		return fmt.Errorf("credencial inesperada: %s", cred)
	}

	signedHeaders := strings.Split(signedHeadersStr, ";")
	sort.Strings(signedHeaders)
	var canonHeaders strings.Builder
	for _, h := range signedHeaders {
		h = strings.ToLower(strings.TrimSpace(h))
		var val string
		switch h {
		case "host":
			val = r.Host
		case "content-length":
			if r.Header.Get("Content-Length") != "" {
				val = r.Header.Get("Content-Length")
			} else if r.ContentLength >= 0 {
				val = strconv.FormatInt(r.ContentLength, 10)
			}
		default:
			val = strings.TrimSpace(r.Header.Get(h))
		}
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(val)
		canonHeaders.WriteByte('\n')
	}

	qSinFirma := make(url.Values, len(q))
	for k, vv := range q {
		if k == "X-Amz-Signature" {
			continue
		}
		qSinFirma[k] = vv
	}
	canonQuery := strings.ReplaceAll(qSinFirma.Encode(), "+", "%20")

	canonReq := strings.Join([]string{
		r.Method,
		r.URL.EscapedPath(),
		canonQuery,
		canonHeaders.String(),
		strings.Join(signedHeaders, ";"),
		"UNSIGNED-PAYLOAD",
	}, "\n")

	hashReq := sha256.Sum256([]byte(canonReq))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(hashReq[:]),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(t.Format("20060102")))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	wantSig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	if !hmac.Equal([]byte(gotSig), []byte(wantSig)) {
		return fmt.Errorf("firma no coincide (signedHeaders=%s)", signedHeadersStr)
	}
	return nil
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func nuevoServicioConS3Real(t *testing.T) (*service.Service, *store.Store, *objectstore.Client, *servidorS3Real, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := service.NewStore(h.Pool)

	const (
		bucket    = "drive-blobs"
		region    = "fr-par"
		accessKey = "SCWTESTACCESSKEY0001"
		secretKey = "scw-test-secret-key-0000000000000001"
	)
	s3Srv := nuevoServidorS3Real(t, bucket, region, accessKey, secretKey)
	u, err := url.Parse(s3Srv.srv.URL)
	if err != nil {
		t.Fatalf("parse S3 URL: %v", err)
	}
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint:  u.Host,
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,
		UseSSL:    false,
		Bucket:    bucket,
	})
	if err != nil {
		t.Fatalf("objectstore.New: %v", err)
	}

	cfg := config.Config{
		DefaultQuotaBytes: 10 << 20,
		MaxUploadBytes:    1 << 20,
		PresignTTL:        15 * time.Minute,
		S3SecretKey:       secretKey,
		TrashRetention:    30 * 24 * time.Hour,
	}
	svc := service.New(cfg, st, blobs, nil, nil)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	return svc, st, blobs, s3Srv, ctx
}

// TestS3RealPresignFirmaContentLengthYFlujoCompleto (FILE-08) verifica contra un
// servidor HTTP S3 real con validación SigV4 y PostgreSQL real:
//  1. PresignUpload incluye content-length en X-Amz-SignedHeaders, de modo que
//     un PUT directo al S3 con tamaño mayor o menor que el autorizado recibe
//     403 Forbidden SignatureDoesNotMatch en el propio S3 antes de tocar el backend.
//  2. Un PUT con el tamaño exacto autorizado sube el objeto, y FinalizeUpload
//     lo pone a salvo en .firme, verifica su tamaño y SHA-256, lo deduplica en
//     shared/drive/<tenant>/<sha256>, borra las claves temporales y cierra la reserva.
//  3. Reutilizar la misma URL prefirmada y el mismo handle tras FinalizeUpload
//     es rechazado con ErrPresignHandleInvalid y limpia el objeto resubido de S3.
func TestS3RealPresignFirmaContentLengthYFlujoCompleto(t *testing.T) {
	svc, _, _, s3Srv, baseCtx := nuevoServicioConS3Real(t)
	ctx := store.WithOwner(baseCtx, "user-1")

	contenido := bytes.Repeat([]byte("SytueDriveS3Real"), 4) // 64 bytes
	tamano := int64(len(contenido))

	res, err := svc.PresignUpload(ctx, "/informe-s3.bin", tamano, "application/octet-stream", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	parsedURL, err := url.Parse(res.URL)
	if err != nil {
		t.Fatalf("URL prefirmada inválida: %v", err)
	}
	signedHeaders := parsedURL.Query().Get("X-Amz-SignedHeaders")
	if !strings.Contains(signedHeaders, "content-length") {
		t.Fatalf("X-Amz-SignedHeaders=%q debe incluir content-length para que S3 acote el tamaño en origen", signedHeaders)
	}

	// 1. Intento de subir un cuerpo mayor que el autorizado a la URL prefirmada:
	// S3 debe rechazarlo con 403 Forbidden porque la firma ata content-length=64.
	exceso := bytes.Repeat([]byte("X"), int(tamano)+10)
	reqExceso, err := http.NewRequestWithContext(ctx, http.MethodPut, res.URL, bytes.NewReader(exceso))
	if err != nil {
		t.Fatal(err)
	}
	respExceso, err := http.DefaultClient.Do(reqExceso)
	if err != nil {
		t.Fatalf("PUT con exceso: %v", err)
	}
	_ = respExceso.Body.Close()
	if respExceso.StatusCode != http.StatusForbidden {
		t.Fatalf("PUT con tamaño distinto al firmado devolvió %d, esperaba 403 Forbidden de S3", respExceso.StatusCode)
	}

	// 2. PUT con el tamaño exacto autorizado (64 bytes) → 200 OK en S3.
	reqOK, err := http.NewRequestWithContext(ctx, http.MethodPut, res.URL, bytes.NewReader(contenido))
	if err != nil {
		t.Fatal(err)
	}
	respOK, err := http.DefaultClient.Do(reqOK)
	if err != nil {
		t.Fatalf("PUT exacto: %v", err)
	}
	_ = respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		t.Fatalf("PUT con tamaño exacto devolvió %d, esperaba 200 OK", respOK.StatusCode)
	}

	// FinalizeUpload verifica, copia a .firme, deduplica a shared/drive/acme/<sha256>
	// y limpia tanto la clave temporal como .firme.
	fichero, err := svc.FinalizeUpload(ctx, res.Handle)
	if err != nil {
		t.Fatalf("FinalizeUpload: %v", err)
	}
	suma := sha256.Sum256(contenido)
	wantSharedKey := "shared/drive/acme/" + hex.EncodeToString(suma[:])
	if fichero.StorageKey != wantSharedKey {
		t.Fatalf("StorageKey=%q, esperaba %q", fichero.StorageKey, wantSharedKey)
	}
	if !s3Srv.existe(wantSharedKey) {
		t.Fatalf("el blob deduplicado %q no existe en S3", wantSharedKey)
	}

	// La clave temporal donde subió el cliente y su .firme deben haberse borrado de S3.
	claveObjetoS3 := strings.TrimPrefix(parsedURL.Path, "/drive-blobs/")
	if s3Srv.existe(claveObjetoS3) {
		t.Fatalf("la clave temporal %q sigue en S3 tras FinalizeUpload", claveObjetoS3)
	}
	if s3Srv.existe(claveObjetoS3 + ".firme") {
		t.Fatalf("la copia temporal %q sigue en S3 tras FinalizeUpload", claveObjetoS3+".firme")
	}

	// 3. Reuso de la reserva: el cliente vuelve a hacer PUT de 64 bytes a la misma
	// URL prefirmada (aún dentro de PresignTTL) e invoca FinalizeUpload con el mismo handle.
	otroContenido := bytes.Repeat([]byte("ReusoDeReserva!!"), 4) // 64 bytes
	reqReuso, err := http.NewRequestWithContext(ctx, http.MethodPut, res.URL, bytes.NewReader(otroContenido))
	if err != nil {
		t.Fatal(err)
	}
	respReuso, err := http.DefaultClient.Do(reqReuso)
	if err != nil {
		t.Fatalf("PUT reuso: %v", err)
	}
	_ = respReuso.Body.Close()
	if respReuso.StatusCode != http.StatusOK {
		t.Fatalf("PUT reuso devolvió %d", respReuso.StatusCode)
	}
	if _, err := svc.FinalizeUpload(ctx, res.Handle); !errors.Is(err, service.ErrPresignHandleInvalid) {
		t.Fatalf("segundo FinalizeUpload con reserva ya consumida = %v, esperaba ErrPresignHandleInvalid", err)
	}
	if s3Srv.existe(claveObjetoS3) {
		t.Fatalf("el objeto resubido %q debía borrarse de S3 al rechazar el reuso", claveObjetoS3)
	}
}

// TestS3RealBarridoFirmeHuerfanoYConservacionDeFirmeVivo (FILE-08) comprueba contra
// PostgreSQL real y S3 real que PurgeExpiredTrash:
//  1. Borra de S3 tanto <storage_key> como <storage_key>.firme cuando una reserva
//     de subida caduca sin haberse registrado ningún fichero.
//  2. NO borra de S3 un objeto <storage_key>.firme que esté referenciado por un
//     fichero vivo de una persona en «Mi unidad» (owner_sub = "user-1"), aunque
//     el barrido corra en el contexto de mantenimiento sin owner_sub.
func TestS3RealBarridoFirmeHuerfanoYConservacionDeFirmeVivo(t *testing.T) {
	svc, st, _, s3Srv, baseCtx := nuevoServicioConS3Real(t)
	ctxUser := store.WithOwner(baseCtx, "user-1")

	root, err := st.EnsureRoot(ctxUser)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	if _, err := st.EnsureQuota(ctxUser, 10<<20); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}

	// Caso 1: reserva caducada huérfana con objeto y .firme abandonados en S3.
	claveHuerfana := "files/" + root.ID.String() + "/huerfana-1"
	if err := st.ReservarSubida(ctxUser, claveHuerfana, 128, time.Now().Add(-time.Minute), true); err != nil {
		t.Fatalf("ReservarSubida huérfana: %v", err)
	}
	s3Srv.poner("tenants/acme/"+claveHuerfana, []byte("temporal-abandonado"))
	s3Srv.poner("tenants/acme/"+claveHuerfana+".firme", []byte("firme-abandonado"))

	// Caso 2: reserva caducada cuyo .firme quedó como StorageKey de un fichero VIVO
	// de user-1 en «Mi unidad» (p. ej. fallo al deduplicar en shared/ o cierre de
	// reserva no completado antes de vencer el TTL).
	claveViva := "files/" + root.ID.String() + "/viva-1"
	claveVivaFirme := claveViva + ".firme"
	if err := st.ReservarSubida(ctxUser, claveViva, 64, time.Now().Add(-time.Minute), true); err != nil {
		t.Fatalf("ReservarSubida viva: %v", err)
	}
	datosVivos := []byte("documento-vivo-en-firme")
	s3Srv.poner("tenants/acme/"+claveVivaFirme, datosVivos)
	if _, err := st.CreateFile(ctxUser, store.NewFileRecord{
		FolderID:    root.ID,
		Name:        "vivo-firme.txt",
		SizeBytes:   int64(len(datosVivos)),
		ContentType: "text/plain",
		StorageKey:  claveVivaFirme,
		Checksum:    "abc123",
		OwnerSub:    "user-1",
	}); err != nil {
		t.Fatalf("CreateFile vivo: %v", err)
	}

	// El barrido de GC corre en background sin owner_sub en el contexto.
	if _, err := svc.PurgeExpiredTrash(context.Background()); err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}

	if s3Srv.existe("tenants/acme/" + claveHuerfana) {
		t.Fatalf("el objeto huérfano %q debía haberse borrado de S3", claveHuerfana)
	}
	if s3Srv.existe("tenants/acme/" + claveHuerfana + ".firme") {
		t.Fatalf("el objeto .firme huérfano %q debía haberse borrado de S3", claveHuerfana+".firme")
	}
	if !s3Srv.existe("tenants/acme/" + claveVivaFirme) {
		t.Fatalf("el objeto .firme VIVO %q de user-1 fue borrado por el GC!", claveVivaFirme)
	}
}
