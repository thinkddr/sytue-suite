// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ocr habla con Sytue OCR, el servicio que reconoce el texto de una
// página escaneada (ADR-0034).
//
// LA DIVISIÓN DEL TRABAJO, que es lo que hace que esto sea corto: el servicio de
// OCR solo sabe de imágenes y devuelve píxeles; **quien sabe de PDF es Docs**, y
// por eso la conversión de coordenadas y la escritura de la capa se quedan aquí.
// Al servicio de OCR se le puede cambiar el motor sin tocar una línea de PDF.
package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Palabra reconocida, en píxeles de la imagen y con origen arriba-izquierda.
type Palabra struct {
	Texto     string  `json:"texto"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Ancho     int     `json:"ancho"`
	Alto      int     `json:"alto"`
	Confianza float64 `json:"confianza"`
	Linea     int     `json:"linea"`
}

// Resultado de reconocer una imagen.
type Resultado struct {
	Palabras []Palabra `json:"palabras"`
	Idiomas  string    `json:"idiomas"`
}

var (
	// ErrNoConfigurado es «no hay reconocimiento detrás»: ni URL puesta, ni nadie
	// contestando en la que hay. Las dos cosas se responden IGUAL —la herramienta
	// no se ofrece— y distinguirlas solo daría dos mensajes que llevan al mismo
	// sitio. En el perfil piloto pasa la segunda: el chart de docs pasa
	// `SYTUE_DOCS_OCR_URL` sin condición, pero Sytue OCR no se despliega.
	ErrNoConfigurado = errors.New("ocr: el servicio de reconocimiento no está disponible")
	// ErrIdioma es un idioma que el servidor no tiene instalado.
	ErrIdioma = errors.New("ocr: idioma no disponible en el servidor")
	// ErrOcupado es la cola llena.
	ErrOcupado = errors.New("ocr: el servicio está ocupado")
)

// Cliente llama a Sytue OCR.
type Cliente struct {
	baseURL string
	http    *http.Client
}

// Nuevo construye el cliente. Con `baseURL` vacía devuelve nil, y quien lo use
// tiene que comprobarlo: es la forma de que la interfaz NO ofrezca reconocer
// texto cuando no hay servicio, en vez de ofrecerlo y fallar.
func Nuevo(baseURL string, timeout time.Duration) *Cliente {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Cliente{baseURL: baseURL, http: &http.Client{Timeout: timeout}}
}

// maxRespuesta acota lo que se lee de vuelta. Una página densa son unas pocas
// miles de palabras; diez megas es un servidor que se ha vuelto loco.
const maxRespuesta = 10 << 20

// Reconocer manda una imagen y devuelve sus palabras.
//
// `autorizacion` es la cabecera Authorization del usuario, que se reenvía tal
// cual: el servicio de OCR valida el MISMO token, así que no hay secreto
// compartido entre servicios y cada reconocimiento queda atribuido a quien lo
// pidió.
func (c *Cliente) Reconocer(ctx context.Context, imagen []byte, idiomas, autorizacion string) (Resultado, error) {
	var out Resultado
	if c == nil {
		return out, ErrNoConfigurado
	}

	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	parte, err := w.CreateFormFile("file", "pagina.png")
	if err != nil {
		return out, err
	}
	if _, err := parte.Write(imagen); err != nil {
		return out, err
	}
	if idiomas != "" {
		if err := w.WriteField("idiomas", idiomas); err != nil {
			return out, err
		}
	}
	if err := w.Close(); err != nil {
		return out, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/recognize", &cuerpo)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if autorizacion != "" {
		req.Header.Set("Authorization", autorizacion)
	}

	res, err := c.http.Do(req)
	if err != nil {
		// UN FALLO DE TRANSPORTE ES «NO HAY SERVICIO», no un error a enseñar: para
		// quien llama no hay diferencia entre una URL vacía y una URL que no
		// contesta, y su texto lleva dentro la dirección interna del clúster. Sin
		// esta traducción salía un 502 con `dial tcp 10.x.x.x:8080: connection
		// refused` de mensaje, que el BFF reenvía tal cual al navegador.
		return out, fmt.Errorf("%w: %w", ErrNoConfigurado, err)
	}
	defer func() { _ = res.Body.Close() }()

	datos, err := io.ReadAll(io.LimitReader(res.Body, maxRespuesta))
	if err != nil {
		return out, fmt.Errorf("ocr: respuesta ilegible: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return out, errorDeRespuesta(res.StatusCode, datos)
	}
	if err := json.Unmarshal(datos, &out); err != nil {
		return out, fmt.Errorf("ocr: respuesta inesperada: %w", err)
	}
	return out, nil
}

// Idiomas pregunta qué modelos hay instalados.
func (c *Cliente) Idiomas(ctx context.Context, autorizacion string) ([]string, error) {
	if c == nil {
		return nil, ErrNoConfigurado
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/languages", http.NoBody)
	if err != nil {
		return nil, err
	}
	if autorizacion != "" {
		req.Header.Set("Authorization", autorizacion)
	}
	res, err := c.http.Do(req)
	if err != nil {
		// Mismo criterio que en Reconocer: un servicio que no contesta cuenta como
		// ausente, y es lo que `ocrVivo` traduce en «no enseñes el botón».
		return nil, fmt.Errorf("%w: %w", ErrNoConfigurado, err)
	}
	defer func() { _ = res.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, errorDeRespuesta(res.StatusCode, datos)
	}
	var cuerpo struct {
		Idiomas []string `json:"idiomas"`
	}
	if err := json.Unmarshal(datos, &cuerpo); err != nil {
		return nil, fmt.Errorf("ocr: respuesta inesperada: %w", err)
	}
	return cuerpo.Idiomas, nil
}

// errorDeRespuesta traduce el error del servicio, conservando su código. Sin
// esto, «ese idioma no está instalado» —que el usuario puede arreglar— llegaría
// como un 500 genérico.
func errorDeRespuesta(estado int, cuerpo []byte) error {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(cuerpo, &e)
	switch e.Code {
	case "ocr_language_unavailable":
		return fmt.Errorf("%w: %s", ErrIdioma, e.Message)
	case "ocr_busy":
		return ErrOcupado
	case "ocr_engine_missing":
		return ErrNoConfigurado
	}
	if e.Message != "" {
		return fmt.Errorf("ocr: %s", e.Message)
	}
	return fmt.Errorf("ocr: el servicio respondió %d", estado)
}
