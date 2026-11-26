// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// # Unidades de la nube (DR-13)
//
// Las cuentas de `l.Nube` (las de Ajustes → Nube) son unidades de Drive. Lo que se abre
// de una se baja a `<caché>/drive/nube`; cuando su app lo guarda ahí, esto lo ve
// (`fsnotify` sobre la caché, y también al volver a la ventana) y lo sube a la nube con
// `archivos.Remotas.Sincronizar`, que no pisa una versión que otra persona haya cambiado
// mientras tanto. La red es la del portero (`l.Nube.HTTP`): sin cuentas, ni una conexión.

// esperaSubida es lo que se espera tras el último cambio en la caché antes de subir: una
// app guarda a ráfagas (temporal, renombrado, metadatos).
const esperaSubida = 2 * time.Second

// ficheroMarcas es donde se guardan, en los ajustes de la edición, la estrella y las
// etiquetas de lo que está en las nubes (en este equipo, no en la cuenta).
const ficheroMarcas = "drive-marcas-nube.json"

// abrirRemotas envuelve la carpeta del disco con las unidades de la nube.
func abrirRemotas(l *local.App, disco *archivos.Archivos) *archivos.Remotas {
	r := archivos.ConRemotas(disco, l.Nube.Cuentas, l.Nube.Proveedor, filepath.Join(l.Espacio.Rutas.Cache, "drive", "nube"))
	if err := r.GuardarMarcasEn(l.Espacio.Ajuste(ficheroMarcas)); err != nil {
		slog.Warn("drive: no se pudieron leer los favoritos y etiquetas de la nube", "error", err)
	}
	return r
}

// motivos son los textos de por qué algo no se movió entre unidades.
var motivos = map[archivos.MotivoTraslado]string{
	archivos.FalloCopia:           "motivoCopia",
	archivos.FalloExiste:          "motivoExiste",
	archivos.FalloComprobacion:    "motivoComprobacion",
	archivos.FalloDocumentoGoogle: "motivoGoogle",
	archivos.FalloOrigen:          "motivoOrigen",
	archivos.FalloTamano:          "motivoTamano",
}

// moverEntreUnidades es `EnEquipo.MoverEntreUnidades`: un traslado de `archivos.Remotas`
// (copiar, comprobar y solo entonces quitar el origen), contado para la pantalla.
func moverEntreUnidades(r *archivos.Remotas, t func(string) string, idioma i18n.Locale) func(context.Context, []drive.Elemento, string, func(int, int, string)) drive.InformeMover {
	return func(ctx context.Context, els []drive.Elemento, destino string, avance func(int, int, string)) drive.InformeMover {
		ids := make([]string, len(els))
		for i, el := range els {
			ids[i] = el.ID
		}
		tr, err := r.Trasladar(ctx, ids, destino, func(a archivos.AvanceTraslado) { avance(a.Hechos, a.Total, a.Nombre) })
		if err != nil {
			slog.Warn("drive: no se pudo mover entre unidades", "error", err)
			return drive.InformeMover{Err: err}
		}
		return informeDe(tr, t, idioma)
	}
}

// informeDe cuenta un traslado para la pantalla: una línea por lo que no se movió, con su
// porqué (lo que no llegó por cancelar no se lista: lo dice el aviso).
func informeDe(tr archivos.Traslado, t func(string) string, idioma i18n.Locale) drive.InformeMover {
	inf := drive.InformeMover{Total: tr.Total, Movidos: len(tr.Movidos), Cancelado: tr.Cancelado}
	for _, n := range tr.NoMovidos {
		if n.Motivo == archivos.FalloCancelado {
			continue
		}
		if n.Err != nil {
			slog.Info("drive: no se movió", "elemento", n.Nombre, "error", n.Err)
		}
		motivo := strings.ReplaceAll(t(motivos[n.Motivo]), "{size}", i18n.FormatBytes(idioma, archivos.MaxTraslado))
		inf.NoMovidos = append(inf.NoMovidos, n.Nombre+" — "+motivo)
	}
	return inf
}

// homonimosDe es `EnEquipo.Homonimos`: los ficheros de una carpeta de la nube que subir
// con ese nombre alcanzaría, con el nombre con que se ven, y el primer nombre libre.
func homonimosDe(r *archivos.Remotas) func(context.Context, string, string) ([]drive.Elemento, string, error) {
	return func(ctx context.Context, carpeta, nombre string) ([]drive.Elemento, string, error) {
		iguales, libre, err := r.Homonimos(ctx, carpeta, nombre)
		if err != nil {
			return nil, "", err
		}
		out := make([]drive.Elemento, len(iguales))
		for i, f := range iguales {
			out[i] = drive.DeFichero(f)
		}
		return out, libre, nil
	}
}

// unidadesDe es lo que la pantalla enseña en el lateral.
func unidadesDe(r *archivos.Remotas) func() []drive.UnidadEquipo {
	return func() []drive.UnidadEquipo {
		unidades := r.Unidades()
		out := make([]drive.UnidadEquipo, 0, len(unidades))
		for _, u := range unidades {
			out = append(out, drive.UnidadEquipo{ID: u.ID, Nombre: u.Nombre})
		}
		return out
	}
}

// sincronizador sube lo cambiado en la caché, de uno en uno.
type sincronizador struct {
	r     *archivos.Remotas
	p     *drive.Pantalla
	t     func(string) string
	mu    sync.Mutex
	reloj *time.Timer
	en    bool
}

// pronto programa una subida dentro de esperaSubida (reinicia la espera si ya había una).
func (s *sincronizador) pronto() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reloj != nil {
		s.reloj.Stop()
	}
	s.reloj = time.AfterFunc(esperaSubida, s.ahora)
}

// ahora sube lo pendiente y dice cómo fue. No corre dos a la vez.
func (s *sincronizador) ahora() {
	s.mu.Lock()
	if s.en {
		s.mu.Unlock()
		s.pronto()
		return
	}
	s.en = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.en = false
		s.mu.Unlock()
	}()
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelar()
	for _, x := range s.r.Sincronizar(ctx) {
		switch {
		case x.Err != nil:
			slog.Warn("drive: no se pudo subir a la nube", "fichero", x.Nombre, "error", x.Err)
			s.p.Avisar(true, s.t("nubeFallo"), x.Nombre)
		case x.Conflicto:
			s.p.Avisar(true, s.t("nubeConflicto"), strings.ReplaceAll(s.t("nubeConflictoComo"), "{name}", x.Como))
		default:
			s.p.Avisar(false, s.t("nubeSubido"), x.Nombre)
		}
	}
}

// vigilar mira la caché y sube lo que cambie. Devuelve con qué pararlo. Sin vigilante
// (un sistema que no lo da), queda el «al volver a la ventana».
func (s *sincronizador) vigilar() func() {
	if err := os.MkdirAll(s.r.Cache(), 0o700); err != nil {
		return func() {}
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("drive: sin vigilante de la caché de la nube", "error", err)
		return func() {}
	}
	vigilarArbol(w, s.r.Cache())
	go s.atender(w)
	return func() { _ = w.Close() }
}

// atender sube lo que cambie según avisa el vigilante, hasta que se cierre. Una carpeta
// nueva se vigila también; el estado de la caché y lo oculto no se suben.
func (s *sincronizador) atender(w *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					vigilarArbol(w, ev.Name)
					continue
				}
			}
			if filepath.Base(ev.Name) == "estado.json" || strings.HasPrefix(filepath.Base(ev.Name), ".") {
				continue
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) {
				s.pronto()
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// vigilarArbol pone el vigilante sobre raiz y todas sus subcarpetas (fsnotify no baja
// solo). Lo que no se puede leer se queda sin vigilar.
func vigilarArbol(w *fsnotify.Watcher, raiz string) {
	_ = filepath.WalkDir(raiz, func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			_ = w.Add(p)
		}
		return nil
	})
}
