// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package com.google.mlkit.genai.prompt.java;

import com.google.mlkit.genai.common.DownloadCallback;
import com.google.mlkit.genai.prompt.Generation;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Future;

/**
 * La envoltura de futuros de MENTIRA (el `from` de su Companion de Kotlin), solo para
 * IASistemaPrueba: devuelve el prompt al revés para comprobar que llega entero.
 */
public final class GenerativeModelFutures {
    public static final Companion Companion = new Companion();
    /** Lo que contesta checkStatus: el FeatureStatus de ML Kit. */
    public static volatile int estado = 3;
    /** El último prompt recibido. */
    public static volatile String ultimo;
    /** Las descargas pedidas. */
    public static volatile int descargas;

    private GenerativeModelFutures() {}

    public static final class Companion {
        public GenerativeModelFutures from(Generation.Modelo m) {
            if (m == null) {
                throw new IllegalArgumentException("sin modelo");
            }
            return new GenerativeModelFutures();
        }
    }

    public Future<Integer> checkStatus() {
        return CompletableFuture.completedFuture(estado);
    }

    public Future<Respuesta> generateContent(String prompt) {
        ultimo = prompt;
        if (prompt.contains("FALLA")) {
            CompletableFuture<Respuesta> f = new CompletableFuture<>();
            f.completeExceptionally(new IllegalStateException("cuota de AICore agotada"));
            return f;
        }
        return CompletableFuture.completedFuture(new Respuesta("eco: " + prompt.length()));
    }

    /** Como el de ML Kit: con un aviso al que se llama al empezar y al acabar. */
    public Future<Void> download(DownloadCallback aviso) {
        descargas++;
        aviso.onDownloadStarted(1L);
        aviso.onDownloadCompleted();
        return CompletableFuture.completedFuture(null);
    }

    /** El GenerateContentResponse. */
    public static final class Respuesta {
        private final String texto;

        Respuesta(String texto) {
            this.texto = texto;
        }

        public List<Candidato> getCandidates() {
            return Collections.singletonList(new Candidato(texto));
        }
    }

    /** Un Candidate. */
    public static final class Candidato {
        private final String texto;

        Candidato(String texto) {
            this.texto = texto;
        }

        public String getText() {
            return texto;
        }
    }
}
