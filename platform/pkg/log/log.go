// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package log construye loggers estructurados con log/slog para todos los
// servicios Sytue One. JSON en prod, texto opcional en dev. Incluye helpers
// para enriquecer y extraer el logger del context.Context.
package log

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

// Options configura la construcción del logger.
type Options struct {
	// Level: debug|info|warn|error.
	Level string
	// Format: json|text.
	Format string
	// Service se añade como atributo persistente "service".
	Service string
	// Version se añade como atributo persistente "version".
	Version string
	// DisableRedaction desactiva la redacción de PII/secretos (NO recomendado;
	// por defecto la redacción está ACTIVADA — seguridad por defecto).
	DisableRedaction bool
	// RedactKeys añade claves sensibles a las redactadas por defecto.
	RedactKeys []string
	// Capture recoge avisos y errores para poder enseñarlos en la consola de
	// plataforma. Nil = no se captura nada, que es lo correcto en tests y en las
	// herramientas de línea de comandos. El sumidero se enchufa más tarde (la
	// base de datos no existe aún cuando arranca el logging), por eso es una
	// referencia y no el sumidero directamente.
	Capture *SinkRef
}

// New construye un *slog.Logger según las opciones dadas.
func New(opts Options) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{
		Level:     parseLevel(opts.Level),
		AddSource: false,
	}

	var h slog.Handler
	if strings.EqualFold(opts.Format, "text") {
		h = slog.NewTextHandler(os.Stdout, handlerOpts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, handlerOpts)
	}

	// Quien no trae sumidero propio (todos menos Sytue ID, que escribe en su
	// base) usa el recolector remoto si el chart lo ha configurado. Va aquí y no
	// en cada `main` porque son quince arranques distintos para la misma línea, y
	// el que se olvide no falla: simplemente no se ve en la consola.
	if opts.Capture == nil {
		if s := collectorDesdeEntorno(); s != nil {
			ref := &SinkRef{}
			ref.Set(s)
			opts.Capture = ref
		}
	}

	// La captura va POR DENTRO de la redacción —el redactor construye un
	// registro limpio y se lo pasa a este handler—, así que lo que se guarda ya
	// viene sin secretos. Al revés, se estarían escribiendo en una tabla los
	// mismos valores que el redactor existe para que no salgan.
	if opts.Capture != nil {
		h, _ = NewCaptureHandler(h, opts.Capture, opts.Service)
	}

	attrs := make([]slog.Attr, 0, 2)
	if opts.Service != "" {
		attrs = append(attrs, slog.String("service", opts.Service))
	}
	if opts.Version != "" {
		attrs = append(attrs, slog.String("version", opts.Version))
	}
	// Envolver con redacción de PII/secretos ANTES de fijar los atributos
	// persistentes, de modo que también se redacten si fueran sensibles.
	if !opts.DisableRedaction {
		h = NewRedactingHandler(h, RedactOptions{ExtraKeys: opts.RedactKeys})
	}

	if len(attrs) > 0 {
		h = h.WithAttrs(attrs)
	}

	l := slog.New(h)
	// La URL solo la pone el chart: con URL y sin token, el Secret sytue-log-ingest
	// falta en el clúster (es `optional`) y el servicio no manda nada a la consola.
	// Se dice al arrancar; sin esta línea se parecía a «no hay errores».
	if opts.Capture == nil && strings.TrimSpace(os.Getenv(CollectorURLEnv)) != "" &&
		strings.TrimSpace(os.Getenv(CollectorTokenEnv)) == "" {
		l.Warn("falta " + CollectorTokenEnv + " (Secret sytue-log-ingest): los avisos de este servicio no llegan a la consola de plataforma")
	}
	return l
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Into devuelve un nuevo context.Context que transporta el logger dado.
func Into(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// From extrae el logger del contexto; si no hay, devuelve slog.Default().
func From(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}

// With deriva del logger del contexto añadiendo atributos y lo reinyecta.
func With(ctx context.Context, args ...any) context.Context {
	return Into(ctx, From(ctx).With(args...))
}
