// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// VECTORIZACIÓN SERVERLESS — sustituye a la GPU de lote (deploy/fotos-gpu,
// retirada 2026-09-19).
//
// ANTES: una GPU L4 efímera corría SigLIP 2 sobre las miniaturas porque el
// catálogo serverless no traía ningún codificador de imágenes. Ahora el
// camino es todo de Generative APIs y en dos pasos:
//
//  1. DESCRIBIR: la visión de gemma-4-26b (/assist, action describir-foto)
//     escribe lo que se ve en la foto — descripción que además ya es el
//     texto alternativo de la imagen.
//  2. EMBEBER: qwen3-embedding-8b (/embeddings) convierte esa descripción en
//     el vector que vive en la base de datos.
//
// La búsqueda por parecido pasa a ser semántica sobre descripciones: lo que
// la IA «vio» en la foto. Es un espacio de vectores distinto al de SigLIP, así
// que al cambiar de modelo lo vectorizado vuelve a estar pendiente — igual
// que cualquier cambio de modelo de embeddings.
//
// CREDENCIALES: este comando SÍ habla con el servicio de IA, así que necesita
// SYTUE_AI_URL y SYTUE_AI_TOKEN (un token con acceso a /api/v1/*). Ya no vale
// el truco de la máquina sin credenciales, porque ya no hay máquina: lo que
// viaja son descripciones y vectores, no píxeles.

// idiomasIA nombra en inglés cada idioma de producto, porque la instrucción está
// en inglés. Es la misma tabla que `apps/nucleo/api/photos_ia.go` y que la ruta
// `describir.ts` de la web.
var idiomasIA = map[string]string{
	"en": "English", "de": "German", "es": "Spanish", "fr": "French", "it": "Italian",
}

// instruccionDescribir es la del botón «Describir» de la web y la nativa, con el
// idioma de la dueña NOMBRADO: el lote va con token de servicio y sin él la
// descripción y las etiquetas salían en inglés. Sin idioma conocido no se fija
// ninguno, en vez de inventar uno.
func instruccionDescribir(idioma string) string {
	partes := []string{
		"Describe what is visible in this photo for use as alt text.",
		`Answer ONLY with a JSON object: {"descripcion": string, "etiquetas": string[]}.`,
	}
	if nombre, ok := idiomasIA[idioma]; ok {
		partes = append(partes, "Write the description and every tag in "+nombre+"; keep the JSON keys as they are.")
	}
	return strings.Join(append(partes,
		"The description must be one sentence, at most 200 characters.",
		"Use at most 8 short lowercase tags naming objects, places or scenes that appear.",
		"Do not name or identify people. Do not guess who someone is.",
		"Describe only what you can see; do not speculate about the context.",
	), " ")
}

// leerDescripcion saca descripción y etiquetas de lo que contestó el modelo, que
// pone el vallado ```json aunque se le diga que no. Sin descripción no hay nada
// que guardar ni que vectorizar: ok=false.
func leerDescripcion(texto string) (string, []string, bool) {
	i, j := strings.Index(texto, "{"), strings.LastIndex(texto, "}")
	if i < 0 || j <= i {
		return "", nil, false
	}
	var leido struct {
		Descripcion string   `json:"descripcion"`
		Etiquetas   []string `json:"etiquetas"`
	}
	if err := json.Unmarshal([]byte(texto[i:j+1]), &leido); err != nil {
		return "", nil, false
	}
	leido.Descripcion = strings.TrimSpace(leido.Descripcion)
	return leido.Descripcion, leido.Etiquetas, leido.Descripcion != ""
}

type clienteIA struct {
	base  string
	token string
	hc    *http.Client
}

func nuevoClienteIA(base, token string) (*clienteIA, error) {
	if base == "" || token == "" {
		return nil, fmt.Errorf("hacen falta SYTUE_AI_URL y SYTUE_AI_TOKEN: el lote serverless habla con el servicio de IA")
	}
	return &clienteIA{
		base:  base,
		token: token,
		hc:    &http.Client{Timeout: 3 * time.Minute},
	}, nil
}

// describir pide a la visión de gemma-4 lo que se ve en la miniatura.
//
// El idioma va en la instrucción y en `Accept-Language`: el token de servicio no
// trae `locale`, y esa cabecera es lo que Sytue AI usa entonces para fijarlo.
func (c *clienteIA) describir(ctx context.Context, miniaturaJPEG []byte, idioma string) (string, error) {
	cuerpo := map[string]any{
		"action":      "describir-foto",
		"kind":        "photo",
		"instruction": instruccionDescribir(idioma),
		"context":     "",
		"esfuerzo":    "medium",
		"imagen":      base64.StdEncoding.EncodeToString(miniaturaJPEG),
		"imagen_tipo": "image/jpeg",
	}
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return "", err
	}
	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/assist", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	peticion.Header.Set("Authorization", "Bearer "+c.token)
	peticion.Header.Set("Content-Type", "application/json")
	if _, ok := idiomasIA[idioma]; ok {
		peticion.Header.Set("Accept-Language", idioma)
	}
	respuesta, err := c.hc.Do(peticion)
	if err != nil {
		return "", err
	}
	defer respuesta.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(respuesta.Body, 256<<10))
	if err != nil {
		return "", err
	}
	if respuesta.StatusCode != http.StatusOK {
		return "", fmt.Errorf("assist HTTP %d", respuesta.StatusCode)
	}
	var leido struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(datos, &leido); err != nil {
		return "", err
	}
	return leido.Text, nil
}

// embeber convierte una descripción en vector con qwen3-embedding-8b.
func (c *clienteIA) embeber(ctx context.Context, texto string) ([]float32, error) {
	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/embeddings",
		bytes.NewBufferString(`{"inputs":[`+jsonTexto(texto)+`]}`))
	if err != nil {
		return nil, err
	}
	peticion.Header.Set("Authorization", "Bearer "+c.token)
	peticion.Header.Set("Content-Type", "application/json")
	respuesta, err := c.hc.Do(peticion)
	if err != nil {
		return nil, err
	}
	defer respuesta.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(respuesta.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if respuesta.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings HTTP %d", respuesta.StatusCode)
	}
	var leido struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(datos, &leido); err != nil || len(leido.Embeddings) != 1 {
		return nil, fmt.Errorf("respuesta de embeddings ilegible")
	}
	return leido.Embeddings[0], nil
}

// jsonTexto serializa un string como literal JSON.
func jsonTexto(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// runVectoresServerless vectoriza las fotos pendientes SIN GPU: describir con
// la visión serverless, embeber la descripción, guardar. Idempotente: una foto
// que ya tiene vector del modelo no vuelve a salir.
func runVectoresServerless(args []string) error {
	fs := flag.NewFlagSet("vectores-serverless", flag.ExitOnError)
	modelo := fs.String("modelo", "qwen3-embedding-8b", "modelo de embeddings con el que se guardan los vectores")
	limite := fs.Int("limite", 500, "cuántas fotos como mucho por ejecución")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, objetos, cerrar, err := abrirDependencias()
	if err != nil {
		return err
	}
	defer cerrar()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ia, err := nuevoClienteIA(cfg.SytueAIURL, cfg.SytueAIToken)
	if err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()

	pendientes, err := st.PendientesDeVector(ctx, *modelo, *limite)
	if err != nil {
		return fmt.Errorf("pendientes: %w", err)
	}
	if len(pendientes) == 0 {
		fmt.Println("nada que vectorizar")
		return nil
	}

	bien, mal := 0, 0
	for _, p := range pendientes {
		if ctx.Err() != nil {
			break
		}
		if err := vectorizarUna(ctx, st, objetos, ia, p, *modelo); err != nil {
			mal++
			continue
		}
		bien++
	}
	fmt.Printf("serverless: %d vectores guardados, %d descartados (modelo %s)\n", bien, mal, *modelo)
	if bien == 0 && mal > 0 {
		return fmt.Errorf("no se vectorizó ni una foto: revisa SYTUE_AI_URL/SYTUE_AI_TOKEN y los permisos")
	}
	return nil
}

// vectorizarUna lleva UNA foto de pendiente a vector: miniatura del almacén,
// descripción por la visión serverless y vector por el embebedor.
func vectorizarUna(ctx context.Context, st *store.Store, objetos *objectstore.Client, ia *clienteIA, p store.Pendiente, modelo string) error {
	if excluida, err := st.TenantExcluidoDelLoteIA(ctx, p.Tenant); err != nil {
		return err
	} else if excluida {
		return nil
	}
	// Ya descrita —por su dueña o por el botón «Describir»—: se vectoriza lo que
	// hay. Ni se paga otra llamada de visión ni se pisa lo escrito.
	texto := p.Descripcion
	if texto == "" {
		var err error
		if texto, err = describirYGuardar(ctx, st, objetos, ia, p); err != nil {
			return err
		}
	}
	ctxVector, cancelar := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelar()
	vector, err := ia.embeber(ctxVector, texto)
	if err != nil || len(vector) == 0 {
		return fmt.Errorf("embeber: %w", err)
	}
	return st.GuardarVector(ctx, p.ID, modelo, vector)
}

// describirYGuardar pide a la visión lo que sale en la miniatura, en el idioma de
// su dueña, y lo guarda si la foto sigue sin descripción. Devuelve el texto que
// se vectoriza: la descripción y sus etiquetas, que también son lo que se busca.
//
// ANTES se guardaba la respuesta EN BRUTO —el JSON entero, vallado incluido— y
// sin la organización en el contexto, así que la escritura fallaba siempre y en
// silencio: ninguna foto quedó descrita por el lote, y el vector se sacaba del
// JSON en vez de la frase.
func describirYGuardar(ctx context.Context, st *store.Store, objetos *objectstore.Client, ia *clienteIA, p store.Pendiente) (string, error) {
	huella := p.Huella
	if huella == "" {
		huella = service.HuellaDelOriginal()
	}
	clave, err := (domain.Derivada{
		ActivoID: p.ID, Huella: huella,
		Variante: domain.VarianteMiniatura, Formato: "jpg",
	}).Clave()
	if err != nil {
		return "", err
	}
	org := tenancy.Tenant{Slug: p.Tenant}
	rc, err := objetos.Get(ctx, org, clave)
	if err != nil {
		return "", err
	}
	var miniatura bytes.Buffer
	_, copyErr := io.Copy(&miniatura, rc)
	_ = rc.Close()
	if copyErr != nil {
		return "", copyErr
	}

	ctxFoto, cancelarFoto := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelarFoto()
	respuesta, err := ia.describir(ctxFoto, miniatura.Bytes(), p.Idioma)
	if err != nil {
		return "", fmt.Errorf("describir: %w", err)
	}
	descripcion, etiquetas, ok := leerDescripcion(respuesta)
	if !ok {
		return "", fmt.Errorf("describir: respuesta ilegible")
	}
	// El vector no depende de que la descripción quede escrita, pero un fallo al
	// guardar no se calla: es justo lo que pasó durante meses sin que nadie lo viera.
	if _, err := st.GuardarDescripcionDelLote(tenancy.Into(ctx, org), p.ID, p.Duenyo, descripcion, etiquetas); err != nil {
		fmt.Printf("serverless: no se guardó la descripción de %s: %v\n", p.ID, err)
	}
	if len(etiquetas) > 0 {
		return descripcion + " " + strings.Join(etiquetas, ", "), nil
	}
	return descripcion, nil
}
