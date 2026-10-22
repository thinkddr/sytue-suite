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

const instruccionDescribir = "Describe what is visible in this photo for use as alt text. " +
	"Answer ONLY with a JSON object: {\"descripcion\": string, \"etiquetas\": string[]}. " +
	"The description must be one sentence, at most 200 characters. " +
	"Use at most 8 short lowercase tags naming objects, places or scenes that appear. " +
	"Do not name or identify people. Describe only what you can see; do not speculate."

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
func (c *clienteIA) describir(ctx context.Context, miniaturaJPEG []byte) (string, error) {
	cuerpo := map[string]any{
		"action":      "describir-foto",
		"instruction": instruccionDescribir,
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
	huella := p.Huella
	if huella == "" {
		huella = service.HuellaDelOriginal()
	}
	clave, err := (domain.Derivada{
		ActivoID: p.ID, Huella: huella,
		Variante: domain.VarianteMiniatura, Formato: "jpg",
	}).Clave()
	if err != nil {
		return err
	}
	rc, err := objetos.Get(ctx, tenancy.Tenant{Slug: p.Tenant}, clave)
	if err != nil {
		return err
	}
	var miniatura bytes.Buffer
	_, copyErr := io.Copy(&miniatura, rc)
	_ = rc.Close()
	if copyErr != nil {
		return copyErr
	}

	ctxFoto, cancelarFoto := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelarFoto()
	descripcion, err := ia.describir(ctxFoto, miniatura.Bytes())
	if err != nil || descripcion == "" {
		return fmt.Errorf("describir: %w", err)
	}
	// La descripción se guarda igual de best effort: el vector no depende de
	// que el alt text haya quedado escrito.
	_ = st.GuardarDescripcion(ctxFoto, p.ID, p.Duenyo, descripcion, nil)
	vector, err := ia.embeber(ctxFoto, descripcion)
	if err != nil || len(vector) == 0 {
		return fmt.Errorf("embeber: %w", err)
	}
	return st.GuardarVector(ctx, p.ID, modelo, vector)
}
