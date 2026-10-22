// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/exportar"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// Errores tipados del servicio de IA.
var (
	ErrAIOcupada           = errors.New("ia: límite de concurrencia alcanzado")
	ErrCupoSuperado        = errors.New("ia: cupo mensual de trabajos agotado")
	ErrClaveInvalida       = errors.New("ia: la clave no es válida según el proveedor")
	ErrAvisoRequerido      = errors.New("ia: se requiere aceptar el aviso de salida de la UE")
	ErrCifradoNoDisponible = errors.New("ia: cifrado KEK no configurado")
	ErrModeloInvalido      = errors.New("ia: modelo no válido")
	ErrTrabajoNoEncontrado = errors.New("ia: trabajo no encontrado")
	ErrTrabajoNoExport     = errors.New("ia: el trabajo no es de exportación o no ha finalizado")
)

// ErrCupoAgotadoDetalle contiene el instante de reseteo del cupo.
type ErrCupoAgotadoDetalle struct {
	ResetAt time.Time
}

func (e ErrCupoAgotadoDetalle) Error() string {
	return fmt.Sprintf("ia: cupo mensual superado, se restablece en %s", e.ResetAt.Format(time.RFC3339))
}

// GuardarClavePeticion son los parámetros para registrar la clave BYOK (§9.2).
type GuardarClavePeticion struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Key            string `json:"key"`
	NoticeAccepted bool   `json:"notice_accepted"`
}

// Servicio gestiona las claves BYOK, cuotas y trabajos asíncronos de IA (§9.2-9.3).
type Servicio struct {
	cfg       config.Config
	store     *store.Store
	asistente *asistente.Cliente
	keyCipher *keycrypto.Cipher
	objetos   *objectstore.Client
	catalogo  *catalogo.Catalogo
	hub       *hubnotify.Client
	logger    *slog.Logger

	mu          sync.Mutex
	trabajosPod int
}

// Nuevo construye el servicio de IA con sus dependencias.
func Nuevo(
	cfg config.Config,
	st *store.Store,
	as *asistente.Cliente,
	kc *keycrypto.Cipher,
	obj *objectstore.Client,
	cat *catalogo.Catalogo,
	hub *hubnotify.Client,
	logger *slog.Logger,
) *Servicio {
	if logger == nil {
		logger = slog.Default()
	}
	return &Servicio{
		cfg:       cfg,
		store:     st,
		asistente: as,
		keyCipher: kc,
		objetos:   obj,
		catalogo:  cat,
		hub:       hub,
		logger:    logger,
	}
}

// ComprobarClave valida la clave con el proveedor a través de la ruta interna de IA (§9.2).
func (s *Servicio) ComprobarClave(ctx context.Context, tenant, sub, provider, key string) ([]string, error) {
	if s.asistente == nil || !s.asistente.Activo() {
		return nil, asistente.ErrSinServicio
	}
	modelos, err := s.asistente.ListarModelos(ctx, s.cfg.AISecret, asistente.Identidad{Tenant: tenant, Sub: sub}, asistente.BYOK{
		Proveedor: provider,
		Clave:     key,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClaveInvalida, err)
	}
	return modelos, nil
}

// GuardarClave prueba, sella y almacena la clave BYOK para el tenant (§9.2).
func (s *Servicio) GuardarClave(ctx context.Context, tenant, sub string, in GuardarClavePeticion) (*domain.AIKeyVista, error) {
	p := strings.ToLower(strings.TrimSpace(in.Provider))
	switch p {
	case domain.AIProviderOpenAI, domain.AIProviderAnthropic, domain.AIProviderZAI:
		if !in.NoticeAccepted {
			return nil, ErrAvisoRequerido
		}
	case domain.AIProviderMistral:
		// Mistral está dentro de la UE y no exige aviso obligatorio
	default:
		return nil, fmt.Errorf("proveedor no admitido: %s", in.Provider)
	}

	if len(in.Model) < 1 || len(in.Model) > 100 {
		return nil, ErrModeloInvalido
	}

	if s.keyCipher == nil {
		return nil, ErrCifradoNoDisponible
	}

	// 1. Probar la clave antes de guardar
	if _, err := s.ComprobarClave(ctx, tenant, sub, p, in.Key); err != nil {
		return nil, err
	}

	// 2. Sellar la clave con keycrypto
	keyEnc, err := s.keyCipher.Encrypt(in.Key)
	if err != nil {
		return nil, fmt.Errorf("ia: sellar clave: %w", err)
	}

	ahora := time.Now()
	var noticeAt *time.Time
	if in.NoticeAccepted || p == domain.AIProviderMistral {
		noticeAt = &ahora
	}

	k := domain.AIKey{
		Tenant:           tenant,
		Provider:         p,
		Model:            strings.TrimSpace(in.Model),
		NoticeAcceptedAt: noticeAt,
		AcceptedBy:       sub,
		CheckedAt:        ahora,
	}

	if err := s.store.GuardarClaveAI(ctx, tenant, k, keyEnc); err != nil {
		return nil, err
	}

	// Notificar a los administradores / Hub del cambio en ajustes
	s.notificarAjustes(ctx, tenant, sub, "Clave de IA configurada")

	return &domain.AIKeyVista{
		Provider:         k.Provider,
		Model:            k.Model,
		HasKey:           true,
		CheckedAt:        &k.CheckedAt,
		NoticeAcceptedAt: k.NoticeAcceptedAt,
	}, nil
}

// ObtenerClave devuelve la vista pública de la clave BYOK activa del tenant (§9.2).
func (s *Servicio) ObtenerClave(ctx context.Context, tenant string) (*domain.AIKeyVista, error) {
	k, _, err := s.store.ObtenerClaveAIVigente(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return &domain.AIKeyVista{HasKey: false}, nil
	}
	return &domain.AIKeyVista{
		Provider:         k.Provider,
		Model:            k.Model,
		HasKey:           true,
		CheckedAt:        &k.CheckedAt,
		NoticeAcceptedAt: k.NoticeAcceptedAt,
	}, nil
}

// BorrarClave elimina la clave BYOK activa del tenant (§9.2).
func (s *Servicio) BorrarClave(ctx context.Context, tenant, sub string) error {
	if err := s.store.BorrarClaveAI(ctx, tenant); err != nil {
		return err
	}
	s.notificarAjustes(ctx, tenant, sub, "Clave de IA eliminada")
	return nil
}

// ObtenerCupo calcula el límite y uso de trabajos estándar en el mes en curso (§9.3).
func (s *Servicio) ObtenerCupo(ctx context.Context, tenant, plan string) (*domain.AIQuota, error) {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		loc = time.FixedZone("CET", 3600)
	}
	ahora := time.Now().In(loc)
	desde := time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, loc)
	resetAt := desde.AddDate(0, 1, 0)
	mes := ahora.Format("2006-01")

	limite := s.cfg.IACupoPro
	if strings.EqualFold(plan, "enterprise") {
		limite = s.cfg.IACupoEnterprise
	}
	if limite <= 0 {
		limite = 100
	}

	usados, err := s.store.ContarTrabajosMes(ctx, tenant, desde)
	if err != nil {
		return nil, err
	}

	return &domain.AIQuota{
		Limit:   limite,
		Used:    usados,
		Month:   mes,
		ResetAt: resetAt,
	}, nil
}

func (s *Servicio) notificarAjustes(ctx context.Context, tenant, sub, accion string) {
	if s.hub == nil {
		return
	}
	notif := hubnotify.Notification{
		Tenant:  tenant,
		Subject: sub,
		Source:  "pages",
		Type:    "pages.settings.changed",
		Level:   "info",
		Title:   accion,
	}
	_ = s.hub.Send(ctx, notif)
}

// adquirirSlot verifica límites de concurrencia y cupo antes de crear el trabajo.
func (s *Servicio) adquirirSlot(ctx context.Context, tenant, plan string) (byok bool, prov, mod, decKey string, err error) {
	s.mu.Lock()
	if s.trabajosPod >= 6 {
		s.mu.Unlock()
		return false, "", "", "", ErrAIOcupada
	}
	s.trabajosPod++
	s.mu.Unlock()

	defer func() {
		if err != nil {
			s.mu.Lock()
			s.trabajosPod--
			s.mu.Unlock()
		}
	}()

	enMarcha, err := s.store.ContarTrabajosEnMarchaOrg(ctx, tenant)
	if err != nil {
		return false, "", "", "", err
	}
	if enMarcha >= 2 {
		return false, "", "", "", ErrAIOcupada
	}

	// Comprobar clave BYOK
	k, keyEnc, err := s.store.ObtenerClaveAIVigente(ctx, tenant)
	if err != nil {
		return false, "", "", "", err
	}
	if k != nil && keyEnc != "" && s.keyCipher != nil {
		dk, err := s.keyCipher.Decrypt(keyEnc)
		if err == nil && dk != "" {
			return true, k.Provider, k.Model, dk, nil
		}
	}

	// Sin BYOK: comprobar cupo mensual
	cupo, err := s.ObtenerCupo(ctx, tenant, plan)
	if err != nil {
		return false, "", "", "", err
	}
	if cupo.Used >= cupo.Limit {
		return false, "", "", "", ErrCupoAgotadoDetalle{ResetAt: cupo.ResetAt}
	}

	return false, "scaleway", "", "", nil
}

func (s *Servicio) liberarSlotPod() {
	s.mu.Lock()
	if s.trabajosPod > 0 {
		s.trabajosPod--
	}
	s.mu.Unlock()
}

// LanzarDesign inicia un trabajo asíncrono de generación de secciones (§9.3).
func (s *Servicio) LanzarDesign(ctx context.Context, tenant, sub string, siteID uuid.UUID, in domain.DesignInput, plan string) (*domain.AIJob, error) {
	byok, prov, mod, decKey, err := s.adquirirSlot(ctx, tenant, plan)
	if err != nil {
		return nil, err
	}

	rawInput, _ := json.Marshal(in)
	job := &domain.AIJob{
		Tenant:    tenant,
		SiteID:    siteID,
		Kind:      domain.AIJobKindDesign,
		Input:     rawInput,
		Provider:  prov,
		BYOK:      byok,
		Model:     mod,
		CreatedBy: sub,
	}

	job, err = s.store.CrearTrabajoAI(ctx, job)
	if err != nil {
		s.liberarSlotPod()
		return nil, err
	}

	bgCtx := context.WithoutCancel(ctx)
	go s.ejecutarDesign(bgCtx, job, in, decKey) //nolint:gosec // trabajo asíncrono sobrevive a la petición HTTP (§9.3)
	return job, nil
}

func (s *Servicio) ejecutarDesign(ctx context.Context, job *domain.AIJob, in domain.DesignInput, decKey string) {
	defer s.liberarSlotPod()

	workerCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Preparar esquema y contexto
	esquema, _ := ir.GenerarEsquemaIA()

	fotosInfo := ""
	if s.catalogo != nil {
		var etiquetas []string
		for i, f := range s.catalogo.Fotos {
			if i >= 30 {
				break
			}
			etiquetas = append(etiquetas, fmt.Sprintf("%s (%s)", f.ID, strings.Join(f.Etiquetas, ",")))
		}
		fotosInfo = "Fotos disponibles en catálogo: " + strings.Join(etiquetas, "; ")
	}

	sistema := fmt.Sprintf(
		"Eres el diseñador de Sytue Web. Genera secciones para una página web moderna, atractiva y accesible en idioma %s.\n"+
			"Tipos de sección válidos: %s.\nIconos válidos: %s.\n%s\n"+
			"Devuelve únicamente un JSON válido que cumpla el esquema estricto proporcionado, con la propiedad 'sections'.",
		in.Language,
		strings.Join(ir.TiposSeccion, ", "),
		strings.Join(ir.Iconos, ", "),
		fotosInfo,
	)

	peticion := asistente.PeticionPages{
		Action:      "design",
		Instruction: in.Prompt,
		Context:     fmt.Sprintf("modo: %s, idioma: %s", in.Mode, in.Language),
		Kind:        "pages",
		Modo:        "calidad",
		Sistema:     sistema,
		Esquema:     esquema,
		MaxTokens:   8192,
	}
	if job.BYOK {
		peticion.BYOK = &asistente.BYOK{
			Proveedor: job.Provider,
			Modelo:    job.Model,
			Clave:     decKey,
		}
	}

	id := asistente.Identidad{Tenant: job.Tenant, Sub: job.CreatedBy}
	resp, err := s.asistente.PedirPages(workerCtx, s.cfg.AISecret, id, peticion)
	var finalErr error
	if err == nil {
		job.TokensIn += resp.TokensEntrada
		job.TokensOut += resp.TokensSalida
		// Validar resultado
		secs, valErr := ir.SeccionesDesdeIA([]byte(resp.Text), in.Language)
		if valErr != nil {
			// Un reintento con los problemas
			peticion.Instruction = fmt.Sprintf("Hubo un error de validación en la estructura generada: %s. Corrige el error y devuelve las secciones válidas.", valErr.Error())
			peticion.Context = resp.Text
			resp2, err2 := s.asistente.PedirPages(workerCtx, s.cfg.AISecret, id, peticion)
			if err2 == nil {
				job.TokensIn += resp2.TokensEntrada
				job.TokensOut += resp2.TokensSalida
				secs, valErr = ir.SeccionesDesdeIA([]byte(resp2.Text), in.Language)
			}
		}

		if valErr == nil && len(secs) > 0 {
			resBytes, _ := json.Marshal(map[string]any{"sections": secs})
			job.Result = resBytes
			job.Status = domain.AIJobStatusDone
		} else {
			finalErr = valErr
		}
	} else {
		finalErr = err
	}

	if finalErr != nil {
		job.Status = domain.AIJobStatusFailed
		job.Error = finalErr.Error()
	}

	ahora := time.Now()
	job.FinishedAt = &ahora
	_ = s.store.ActualizarTrabajoAI(ctx, job)
}

// LanzarTranslate inicia un trabajo asíncrono de traducción (§9.3).
func (s *Servicio) LanzarTranslate(ctx context.Context, tenant, sub string, siteID uuid.UUID, in domain.TranslateInput, plan string) (*domain.AIJob, error) {
	if err := ValidarAlcanceTraduccion(in.Scope); err != nil {
		return nil, err
	}
	if in.Scope == "" {
		in.Scope = "site"
	}
	if err := ValidarIdiomasTraduccion(in); err != nil {
		return nil, err
	}
	byok, prov, mod, decKey, err := s.adquirirSlot(ctx, tenant, plan)
	if err != nil {
		return nil, err
	}

	rawInput, _ := json.Marshal(in)
	job := &domain.AIJob{
		Tenant:    tenant,
		SiteID:    siteID,
		Kind:      domain.AIJobKindTranslate,
		Input:     rawInput,
		Provider:  prov,
		BYOK:      byok,
		Model:     mod,
		CreatedBy: sub,
	}

	job, err = s.store.CrearTrabajoAI(ctx, job)
	if err != nil {
		s.liberarSlotPod()
		return nil, err
	}

	bgCtx := context.WithoutCancel(ctx)
	go s.ejecutarTranslate(bgCtx, job, in, decKey) //nolint:gosec // trabajo asíncrono sobrevive a la petición HTTP (§9.3)
	return job, nil
}

func (s *Servicio) traducirLote(ctx context.Context, job *domain.AIJob, from, targetLang, decKey string, lote map[string]string) (map[string]string, error) {
	if len(lote) == 0 {
		return map[string]string{}, nil
	}
	loteJSON, _ := json.Marshal(lote)
	peticion := asistente.PeticionPages{
		Action: "translate",
		Instruction: fmt.Sprintf(
			"Traduce del idioma %s al idioma %s los textos proporcionados en el objeto JSON.\n"+
				"Preserva exactamente las marcas de formato **negrita**, *cursiva*, y enlaces [texto](enlace:<n>).\n"+
				"Devuelve un objeto JSON con las mismas claves y el texto traducido.",
			from, targetLang,
		),
		Context:   string(loteJSON),
		Kind:      "pages",
		Modo:      "interactivo",
		MaxTokens: 4096,
	}
	if job.BYOK {
		peticion.BYOK = &asistente.BYOK{Proveedor: job.Provider, Modelo: job.Model, Clave: decKey}
	}

	id := asistente.Identidad{Tenant: job.Tenant, Sub: job.CreatedBy}
	resp, err := s.asistente.PedirPages(ctx, s.cfg.AISecret, id, peticion)
	if err != nil {
		return nil, err
	}
	job.TokensIn += resp.TokensEntrada
	job.TokensOut += resp.TokensSalida

	var traducidos map[string]string
	cleanJSON := asistente.JSON(resp.Text)
	if cleanJSON == "" {
		cleanJSON = resp.Text
	}
	if err := json.Unmarshal([]byte(cleanJSON), &traducidos); err != nil {
		return nil, err
	}
	return traducidos, nil
}

func (s *Servicio) traducirIdioma(ctx context.Context, job *domain.AIJob, from, targetLang, decKey string, textos map[string]ir.TextoTraducible, parcheResultado map[string]map[string]any) error {
	loteActual := make(map[string]string)
	tamLote := 2 // llaves del objeto JSON
	const maxLoteBytes = 6000

	aplicarLote := func() error {
		if len(loteActual) == 0 {
			return nil
		}
		traducidos, err := s.traducirLote(ctx, job, from, targetLang, decKey, loteActual)
		if err != nil {
			return fmt.Errorf("ia: translate %s: %w", targetLang, err)
		}
		for key := range loteActual {
			if value, ok := traducidos[key]; !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("ia: incomplete translation batch for %s", targetLang)
			}
		}
		for clave, txt := range traducidos {
			if _, requested := loteActual[clave]; !requested {
				continue
			}
			if _, ok := parcheResultado[clave]; !ok {
				parcheResultado[clave] = make(map[string]any)
			}
			parcheResultado[clave][targetLang] = txt
		}
		return nil
	}

	for _, key := range slices.Sorted(maps.Keys(textos)) {
		text := textos[key]
		claveJSON, _ := json.Marshal(key)
		textoJSON, _ := json.Marshal(text.Texto)
		tamEntrada := len(claveJSON) + 1 + len(textoJSON)
		if tamEntrada+2 > maxLoteBytes {
			return fmt.Errorf("ia: translation text %s exceeds the %d-byte batch limit", key, maxLoteBytes)
		}
		if len(loteActual) > 0 && tamLote+1+tamEntrada > maxLoteBytes {
			if err := aplicarLote(); err != nil {
				return err
			}
			loteActual = make(map[string]string)
			tamLote = 2
		}
		if len(loteActual) > 0 {
			tamLote++
		}
		loteActual[key] = text.Texto
		tamLote += tamEntrada
	}
	return aplicarLote()
}

// traducirBorrador solo publica el parche cuando todos los lotes han terminado.
// El worker persiste este mismo estado; un error no deja un resultado parcial aplicable.
func (s *Servicio) traducirBorrador(ctx context.Context, job *domain.AIJob, sitio *ir.Sitio, in domain.TranslateInput, decKey string) {
	parche, err := s.traduccionesDelBorrador(ctx, job, sitio, in, decKey)
	if err != nil {
		job.Status = domain.AIJobStatusFailed
		job.Error = err.Error()
		job.Result = nil
		return
	}
	job.Result, _ = json.Marshal(parche)
	job.Status = domain.AIJobStatusDone
	job.Error = ""
}

func (s *Servicio) traduccionesDelBorrador(ctx context.Context, job *domain.AIJob, sitio *ir.Sitio, in domain.TranslateInput, decKey string) (map[string]map[string]any, error) {
	if err := ValidarIdiomasTraduccion(in); err != nil {
		return nil, err
	}
	if in.From != sitio.Languages.Default && !slices.Contains(sitio.Languages.Others, in.From) {
		return nil, ErrIdiomasTraduccion
	}
	parche := make(map[string]map[string]any)
	for _, target := range in.To {
		textos, err := textosParaTraducir(sitio, in, target)
		if err != nil {
			return nil, err
		}
		if err := s.traducirIdioma(ctx, job, in.From, target, decKey, textos, parche); err != nil {
			return nil, err
		}
	}
	return parche, nil
}

func (s *Servicio) ejecutarTranslate(ctx context.Context, job *domain.AIJob, in domain.TranslateInput, decKey string) {
	defer s.liberarSlotPod()

	workerCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	sitio, err := s.store.Borrador(workerCtx, job.SiteID)
	if err != nil {
		job.Status = domain.AIJobStatusFailed
		job.Error = err.Error()
		ahora := time.Now()
		job.FinishedAt = &ahora
		_ = s.store.ActualizarTrabajoAI(ctx, job)
		return
	}

	var irSitio ir.Sitio
	if err := json.Unmarshal(sitio.Draft, &irSitio); err != nil {
		job.Status = domain.AIJobStatusFailed
		job.Error = fmt.Sprintf("borrador corrupto: %v", err)
		ahora := time.Now()
		job.FinishedAt = &ahora
		_ = s.store.ActualizarTrabajoAI(ctx, job)
		return
	}

	s.traducirBorrador(workerCtx, job, &irSitio, in, decKey)
	ahora := time.Now()
	job.FinishedAt = &ahora
	_ = s.store.ActualizarTrabajoAI(ctx, job)
}

// LanzarExport inicia un trabajo asíncrono de exportación (§9.3).
func (s *Servicio) LanzarExport(ctx context.Context, tenant, sub string, siteID uuid.UUID, in domain.ExportInput, plan string) (*domain.AIJob, error) {
	byok, prov, mod, _, err := s.adquirirSlot(ctx, tenant, plan)
	if err != nil {
		return nil, err
	}

	rawInput, _ := json.Marshal(in)
	job := &domain.AIJob{
		Tenant:    tenant,
		SiteID:    siteID,
		Kind:      domain.AIJobKindExport,
		Input:     rawInput,
		Provider:  prov,
		BYOK:      byok,
		Model:     mod,
		CreatedBy: sub,
	}

	job, err = s.store.CrearTrabajoAI(ctx, job)
	if err != nil {
		s.liberarSlotPod()
		return nil, err
	}

	bgCtx := context.WithoutCancel(ctx)
	go s.ejecutarExport(bgCtx, job, in) //nolint:gosec // trabajo asíncrono sobrevive a la petición HTTP (§9.3)
	return job, nil
}

func (s *Servicio) ejecutarExport(ctx context.Context, job *domain.AIJob, in domain.ExportInput) {
	defer s.liberarSlotPod()

	workerCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// 1. Obtener IR del sitio
	var irSitio ir.Sitio
	var rawIR []byte = in.IR
	if len(rawIR) == 0 && job.SiteID != uuid.Nil && s.store != nil {
		sitio, err := s.store.Borrador(workerCtx, job.SiteID)
		if err == nil {
			rawIR = sitio.Draft
		}
	}
	if len(rawIR) > 0 {
		_ = json.Unmarshal(rawIR, &irSitio)
	}

	// 2. Obtener productos del catálogo
	var productos []domain.Product
	if len(in.Catalogo) > 0 {
		_ = json.Unmarshal(in.Catalogo, &productos)
	} else if job.SiteID != uuid.Nil && s.store != nil {
		prods, err := s.store.ListarProductos(workerCtx, job.Tenant, job.SiteID)
		if err == nil {
			productos = prods
		}
	}

	// 3. Generar exportación según plataforma destino
	mapaFicheros, err := exportar.Generar(workerCtx, exportar.Destino(in.Destination), &irSitio, productos, nil)
	if err != nil {
		job.Status = domain.AIJobStatusFailed
		job.Error = err.Error()
		ahora := time.Now()
		job.FinishedAt = &ahora
		if s.store != nil {
			_ = s.store.ActualizarTrabajoAI(ctx, job)
		}
		return
	}

	// Almacenar en S3 bajo shared/sitios/<site_id>/exportaciones/<job_id>.json
	resKey := fmt.Sprintf("shared/sitios/%s/exportaciones/%s.json", job.SiteID, job.ID)
	datosJSON, _ := json.Marshal(mapaFicheros)

	if s.objetos != nil {
		_ = s.objetos.Put(workerCtx, tenancy.Tenant{Slug: job.Tenant}, resKey, bytes.NewReader(datosJSON), int64(len(datosJSON)), objectstore.PutOptions{ContentType: "application/json"})
	}

	job.ResultKey = resKey
	job.Result = datosJSON
	job.Status = domain.AIJobStatusDone
	ahora := time.Now()
	job.FinishedAt = &ahora
	if s.store != nil {
		_ = s.store.ActualizarTrabajoAI(ctx, job)
	}

	// Notificar al Hub
	if s.hub != nil {
		enlace := hubnotify.EnlacePages(s.cfg.PublicURL, job.Tenant, fmt.Sprintf("sites/%s?export=%s", job.SiteID, job.ID))
		_ = s.hub.Send(ctx, hubnotify.Notification{
			Tenant:  job.Tenant,
			Subject: job.CreatedBy,
			Source:  "pages",
			Type:    "pages.export.ready",
			Level:   "success",
			Title:   fmt.Sprintf("Exportación a %s lista para descargar", in.Destination),
			URL:     enlace,
		})
	}
}

// ObtenerTrabajo consulta el estado y resultado de un trabajo por ID (§9.3).
func (s *Servicio) ObtenerTrabajo(ctx context.Context, tenant string, id uuid.UUID) (*domain.AIJob, error) {
	return s.store.ObtenerTrabajoAI(ctx, tenant, id)
}

// ObtenerZip emite el streaming zip de un trabajo de exportación (§9.3).
func (s *Servicio) ObtenerZip(ctx context.Context, tenant string, id uuid.UUID, w io.Writer) error {
	job, err := s.store.ObtenerTrabajoAI(ctx, tenant, id)
	if err != nil {
		return err
	}
	if job.Kind != domain.AIJobKindExport || job.Status != domain.AIJobStatusDone {
		return ErrTrabajoNoExport
	}

	var mapaFicheros map[string][]byte
	if len(job.Result) > 0 {
		_ = json.Unmarshal(job.Result, &mapaFicheros)
	}

	if len(mapaFicheros) == 0 && job.ResultKey != "" && s.objetos != nil {
		rc, err := s.objetos.Get(ctx, tenancy.Tenant{Slug: tenant}, job.ResultKey)
		if err == nil {
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			_ = json.Unmarshal(b, &mapaFicheros)
		}
	}

	if len(mapaFicheros) == 0 {
		mapaFicheros = map[string][]byte{
			"README.md": []byte("# Exportación vacía\n"),
		}
	}

	var archivos []publicar.ArchivoZip
	for r, c := range mapaFicheros {
		archivos = append(archivos, publicar.ArchivoZip{
			Ruta:       r,
			Contenido:  c,
			Modificado: time.Now(),
		})
	}
	return publicar.EscribirZip(w, archivos, nil)
}

// BarrerVencidos ejecuta el barrido periódico de trabajos interrumpidos (§9.3).
func (s *Servicio) BarrerVencidos(ctx context.Context) (int64, error) {
	return s.store.BarrerTrabajosVencidos(ctx)
}

// IniciarBarrido arranca una goroutine que cada 5 minutos barre trabajos vencidos (§9.3).
func (s *Servicio) IniciarBarrido(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				afectados, err := s.BarrerVencidos(ctx)
				if err != nil {
					s.logger.Warn("ia: error barriendo trabajos vencidos", "err", err)
				} else if afectados > 0 {
					s.logger.Info("ia: trabajos vencidos cancelados", "afectados", afectados)
				}
			}
		}
	}()
}
