// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código: ver
// pantalla_linux.go.
//
//nolint:gocritic // cgo: los avisos no son de este fichero
package camara

/*
#cgo CFLAGS: -I/usr/include/pipewire-0.3 -I/usr/include/spa-0.2 -D_REENTRANT
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <fcntl.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <pipewire/pipewire.h>
#include <spa/param/video/format-utils.h>
#include <spa/param/buffers.h>

// libpipewire se abre con dlopen: un escritorio sin PipeWire (X11 puro, o una distro
// vieja) tiene que poder arrancar la app igual, y ahí se comparte por X11. Las cabeceras
// dan los tipos; __typeof__ saca de ellas el puntero de cada función que se usa.
#define PW(f) static __typeof__(f) *p_##f
PW(pw_init);
PW(pw_thread_loop_new);
PW(pw_thread_loop_get_loop);
PW(pw_thread_loop_start);
PW(pw_thread_loop_stop);
PW(pw_thread_loop_lock);
PW(pw_thread_loop_unlock);
PW(pw_thread_loop_destroy);
PW(pw_context_new);
PW(pw_context_connect_fd);
PW(pw_context_connect);
PW(pw_context_destroy);
PW(pw_core_disconnect);
PW(pw_properties_new);
PW(pw_stream_new);
PW(pw_stream_add_listener);
PW(pw_stream_connect);
PW(pw_stream_update_params);
PW(pw_stream_dequeue_buffer);
PW(pw_stream_queue_buffer);
PW(pw_stream_destroy);

static int sytuePWCargar(void) {
	static int hecho = 0;
	if (hecho) return hecho;
	void *h = dlopen("libpipewire-0.3.so.0", RTLD_NOW | RTLD_LOCAL);
	if (h == NULL) return hecho = -1;
#define CARGA(f) if ((p_##f = dlsym(h, #f)) == NULL) return hecho = -1
	CARGA(pw_init); CARGA(pw_thread_loop_new); CARGA(pw_thread_loop_get_loop);
	CARGA(pw_thread_loop_start); CARGA(pw_thread_loop_stop); CARGA(pw_thread_loop_lock);
	CARGA(pw_thread_loop_unlock); CARGA(pw_thread_loop_destroy); CARGA(pw_context_new);
	CARGA(pw_context_connect_fd); CARGA(pw_context_connect); CARGA(pw_context_destroy);
	CARGA(pw_core_disconnect); CARGA(pw_properties_new); CARGA(pw_stream_new);
	CARGA(pw_stream_add_listener); CARGA(pw_stream_connect); CARGA(pw_stream_update_params);
	CARGA(pw_stream_dequeue_buffer); CARGA(pw_stream_queue_buffer); CARGA(pw_stream_destroy);
	p_pw_init(NULL, NULL);
	return hecho = 1;
}

// Una captura: el hilo de PipeWire deja el último fotograma en `buf` (RGBA) y Go lo
// recoge con el candado del propio bucle.
typedef struct {
	struct pw_thread_loop *bucle;
	struct pw_context *ctx;
	struct pw_core *core;
	struct pw_stream *flujo;
	struct spa_hook oyente;
	struct spa_video_info_raw formato;
	uint8_t *buf;
	int ancho, alto, cap;
	unsigned long nuevo;
	int terminado;
} sytuePW;

static void alCambiarParam(void *d, uint32_t id, const struct spa_pod *param) {
	sytuePW *c = d;
	if (param == NULL || id != SPA_PARAM_Format) return;
	uint32_t tipo, subtipo;
	if (spa_format_parse(param, &tipo, &subtipo) < 0 || tipo != SPA_MEDIA_TYPE_video ||
	    subtipo != SPA_MEDIA_SUBTYPE_raw) return;
	if (spa_format_video_raw_parse(param, &c->formato) < 0) return;
	// Memoria compartida, no DMA-BUF: la imagen se copia a la CPU igualmente para
	// codificarla, y así no hace falta EGL para leerla.
	uint8_t b[256];
	struct spa_pod_builder pb = SPA_POD_BUILDER_INIT(b, sizeof(b));
	const struct spa_pod *ps[1];
	ps[0] = spa_pod_builder_add_object(&pb, SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
		SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int((1 << SPA_DATA_MemPtr) | (1 << SPA_DATA_MemFd)));
	p_pw_stream_update_params(c->flujo, ps, 1);
}

static void alEstado(void *d, enum pw_stream_state viejo, enum pw_stream_state nuevo, const char *err) {
	sytuePW *c = d;
	(void)viejo; (void)err;
	// El compositor corta (la persona pulsa «dejar de compartir» en SU aviso): se acaba.
	if (nuevo == PW_STREAM_STATE_ERROR || nuevo == PW_STREAM_STATE_UNCONNECTED) c->terminado = 1;
}

static void alProcesar(void *d) {
	sytuePW *c = d;
	struct pw_buffer *b = p_pw_stream_dequeue_buffer(c->flujo);
	if (b == NULL) return;
	struct spa_data *sd = &b->buffer->datas[0];
	int w = c->formato.size.width, h = c->formato.size.height;
	int paso = sd->chunk->stride ? sd->chunk->stride : w * 4;
	if (sd->data != NULL && w > 0 && h > 0 && (size_t)paso * h <= sd->maxsize &&
	    !(sd->chunk->flags & SPA_CHUNK_FLAG_CORRUPTED)) {
		if (w * h * 4 > c->cap) {
			free(c->buf);
			c->buf = malloc((size_t)w * h * 4);
			c->cap = c->buf ? w * h * 4 : 0;
		}
		if (c->buf != NULL) {
			int bgr = c->formato.format == SPA_VIDEO_FORMAT_BGRx || c->formato.format == SPA_VIDEO_FORMAT_BGRA;
			const uint8_t *o = (const uint8_t *)sd->data + sd->chunk->offset;
			for (int y = 0; y < h; y++) {
				const uint8_t *s = o + (size_t)y * paso;
				uint8_t *t = c->buf + (size_t)y * w * 4;
				for (int x = 0; x < w; x++, s += 4, t += 4) {
					t[0] = bgr ? s[2] : s[0]; t[1] = s[1]; t[2] = bgr ? s[0] : s[2]; t[3] = 255;
				}
			}
			c->ancho = w; c->alto = h; c->nuevo++;
		}
	}
	p_pw_stream_queue_buffer(c->flujo, b);
}

// La lista de formatos empieza por el preferido y luego los admitidos, él incluido.
#define FORMATO_PREFERIDO SPA_VIDEO_FORMAT_BGRx

static const struct pw_stream_events eventos = {
	PW_VERSION_STREAM_EVENTS,
	.state_changed = alEstado,
	.param_changed = alCambiarParam,
	.process = alProcesar,
};

// sytuePWAbrir conecta al nodo `nodo`: por el descriptor del portal si fd >= 0, o al
// demonio de la sesión si no (las pruebas). El descriptor se duplica: PipeWire se
// queda con el suyo y lo cierra al desconectar. NULL si no se pudo.
static sytuePW *sytuePWAbrir(int fd, uint32_t nodo) {
	if (sytuePWCargar() < 0) return NULL;
	sytuePW *c = calloc(1, sizeof(*c));
	c->bucle = p_pw_thread_loop_new("sytue-pantalla", NULL);
	if (c->bucle == NULL) { free(c); return NULL; }
	c->ctx = p_pw_context_new(p_pw_thread_loop_get_loop(c->bucle), NULL, 0);
	if (c->ctx == NULL) goto mal;
	p_pw_thread_loop_start(c->bucle);
	p_pw_thread_loop_lock(c->bucle);
	c->core = fd >= 0 ? p_pw_context_connect_fd(c->ctx, fcntl(fd, F_DUPFD_CLOEXEC, 3), NULL, 0)
	                  : p_pw_context_connect(c->ctx, NULL, 0);
	if (c->core == NULL) { p_pw_thread_loop_unlock(c->bucle); goto mal; }
	c->flujo = p_pw_stream_new(c->core, "Sytue Rooms",
		p_pw_properties_new(PW_KEY_MEDIA_TYPE, "Video", PW_KEY_MEDIA_CATEGORY, "Capture",
			PW_KEY_MEDIA_ROLE, "Screen", PW_KEY_NODE_NAME, "sytue-rooms-pantalla", NULL));
	if (c->flujo == NULL) { p_pw_thread_loop_unlock(c->bucle); goto mal; }
	p_pw_stream_add_listener(c->flujo, &c->oyente, &eventos, c);
	uint8_t b[1024];
	struct spa_pod_builder pb = SPA_POD_BUILDER_INIT(b, sizeof(b));
	const struct spa_pod *ps[1];
	ps[0] = spa_pod_builder_add_object(&pb, SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
		SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video),
		SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
		SPA_FORMAT_VIDEO_format, SPA_POD_CHOICE_ENUM_Id(5, FORMATO_PREFERIDO, SPA_VIDEO_FORMAT_BGRx,
			SPA_VIDEO_FORMAT_RGBx, SPA_VIDEO_FORMAT_BGRA, SPA_VIDEO_FORMAT_RGBA),
		SPA_FORMAT_VIDEO_size, SPA_POD_CHOICE_RANGE_Rectangle(&SPA_RECTANGLE(1920, 1080),
			&SPA_RECTANGLE(1, 1), &SPA_RECTANGLE(8192, 8192)),
		SPA_FORMAT_VIDEO_framerate, SPA_POD_CHOICE_RANGE_Fraction(&SPA_FRACTION(5, 1),
			&SPA_FRACTION(0, 1), &SPA_FRACTION(60, 1)));
	int r = p_pw_stream_connect(c->flujo, PW_DIRECTION_INPUT, nodo,
		PW_STREAM_FLAG_AUTOCONNECT | PW_STREAM_FLAG_MAP_BUFFERS, ps, 1);
	p_pw_thread_loop_unlock(c->bucle);
	if (r < 0) goto mal;
	return c;
mal:
	p_pw_thread_loop_stop(c->bucle);
	if (c->flujo) p_pw_stream_destroy(c->flujo);
	if (c->core) p_pw_core_disconnect(c->core);
	if (c->ctx) p_pw_context_destroy(c->ctx);
	p_pw_thread_loop_destroy(c->bucle);
	free(c);
	return NULL;
}

// sytuePWLeer copia el último fotograma a dst si es nuevo desde `*visto`.
// 1 copiado, 0 nada nuevo, -1 terminado, -3 no cabe (ancho y alto dicen cuánto hace falta).
static int sytuePWLeer(sytuePW *c, uint8_t *dst, int cap, int *ancho, int *alto, unsigned long *visto) {
	int r = 0;
	p_pw_thread_loop_lock(c->bucle);
	if (c->terminado) r = -1;
	else if (c->nuevo != *visto) {
		*ancho = c->ancho; *alto = c->alto;
		if (c->ancho * c->alto * 4 > cap) r = -3;
		else { memcpy(dst, c->buf, (size_t)c->ancho * c->alto * 4); *visto = c->nuevo; r = 1; }
	}
	p_pw_thread_loop_unlock(c->bucle);
	return r;
}

static void sytuePWCerrar(sytuePW *c) {
	p_pw_thread_loop_stop(c->bucle);
	p_pw_stream_destroy(c->flujo);
	p_pw_core_disconnect(c->core);
	p_pw_context_destroy(c->ctx);
	p_pw_thread_loop_destroy(c->bucle);
	free(c->buf);
	free(c);
}
*/
import "C"

import (
	"image"
	"sync"
	"unsafe"
)

// flujoPipeWire lee un nodo de vídeo de PipeWire: el del portal en Wayland.
type flujoPipeWire struct {
	mu     sync.Mutex
	c      *C.sytuePW
	buf    []byte
	visto  C.ulong
	sesion *sesionPortal // nil en las pruebas, que van al demonio sin portal
}

// abrirPipeWire conecta al nodo; fd < 0 va al demonio de la sesión en vez del portal.
func abrirPipeWire(fd int, nodo uint32) (*flujoPipeWire, error) {
	c := C.sytuePWAbrir(C.int(fd), C.uint32_t(nodo))
	if c == nil {
		return nil, errSinPipeWire
	}
	return &flujoPipeWire{c: c, buf: make([]byte, 1920*1080*4)}, nil
}

func (f *flujoPipeWire) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.c == nil {
		return nil, false
	}
	var w, h C.int
	switch C.sytuePWLeer(f.c, (*C.uint8_t)(unsafe.Pointer(&f.buf[0])), C.int(len(f.buf)), &w, &h, &f.visto) {
	case 1:
	case -3:
		f.buf = make([]byte, int(w)*int(h)*4) // el siguiente fotograma ya cabe
		return nil, false
	default:
		return nil, false
	}
	n := int(w) * int(h) * 4
	return DeRGBA(&image.RGBA{Pix: f.buf[:n], Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}), true
}

func (f *flujoPipeWire) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.c != nil {
		C.sytuePWCerrar(f.c)
		f.c = nil
	}
	if f.sesion != nil {
		f.sesion.Cerrar()
		f.sesion = nil
	}
}
