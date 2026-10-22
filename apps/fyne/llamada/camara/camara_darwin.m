// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

#import <AVFoundation/AVFoundation.h>
#import <CoreVideo/CoreVideo.h>
#include <pthread.h>
#include <string.h>
#include "camara_darwin.h"

// El último fotograma, en I420, protegido por `mu`. El delegado lo escribe en su cola
// y Go lo lee a su ritmo.
static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static uint8_t *ultimo = NULL;
static int ultimoCap = 0, ultimoAncho = 0, ultimoAlto = 0;
static long ultimaSec = 0;

@interface SytueCam : NSObject <AVCaptureVideoDataOutputSampleBufferDelegate>
@property(strong) AVCaptureSession *sesion;
@property(strong) dispatch_queue_t cola;
@end

@implementation SytueCam
// captureOutput pasa el NV12 del sistema a I420: el plano Y tal cual y el CbCr
// entrelazado a dos planos.
- (void)captureOutput:(AVCaptureOutput *)o didOutputSampleBuffer:(CMSampleBufferRef)b fromConnection:(AVCaptureConnection *)c {
	CVImageBufferRef pb = CMSampleBufferGetImageBuffer(b);
	if (pb == NULL || CVPixelBufferGetPlaneCount(pb) < 2) return;
	CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	int w = (int)CVPixelBufferGetWidthOfPlane(pb, 0) & ~1, h = (int)CVPixelBufferGetHeightOfPlane(pb, 0) & ~1;
	const uint8_t *y = CVPixelBufferGetBaseAddressOfPlane(pb, 0);
	const uint8_t *uv = CVPixelBufferGetBaseAddressOfPlane(pb, 1);
	size_t sy = CVPixelBufferGetBytesPerRowOfPlane(pb, 0), suv = CVPixelBufferGetBytesPerRowOfPlane(pb, 1);
	int n = w * h * 3 / 2;
	pthread_mutex_lock(&mu);
	if (ultimoCap < n) {
		free(ultimo);
		ultimo = malloc(n);
		ultimoCap = ultimo ? n : 0;
	}
	if (ultimo != NULL) {
		for (int f = 0; f < h; f++) memcpy(ultimo + f * w, y + f * sy, w);
		uint8_t *u = ultimo + w * h, *v = u + (w / 2) * (h / 2);
		for (int f = 0; f < h / 2; f++) {
			const uint8_t *fila = uv + f * suv;
			for (int k = 0; k < w / 2; k++) {
				u[f * (w / 2) + k] = fila[2 * k];
				v[f * (w / 2) + k] = fila[2 * k + 1];
			}
		}
		ultimoAncho = w;
		ultimoAlto = h;
		ultimaSec++;
	}
	pthread_mutex_unlock(&mu);
	CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
}
@end

static SytueCam *cam = nil;

static NSArray<AVCaptureDevice *> *camaras(void) {
	NSArray *tipos = @[ AVCaptureDeviceTypeBuiltInWideAngleCamera ];
#if !TARGET_OS_IPHONE
	if (@available(macOS 14.0, *)) {
		tipos = @[ AVCaptureDeviceTypeBuiltInWideAngleCamera, AVCaptureDeviceTypeExternal ];
	}
#endif
	return [AVCaptureDeviceDiscoverySession discoverySessionWithDeviceTypes:tipos
	                                                              mediaType:AVMediaTypeVideo
	                                                               position:AVCaptureDevicePositionUnspecified].devices;
}

int sytueCamPermiso(void) {
	AVAuthorizationStatus s = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
	if (s == AVAuthorizationStatusAuthorized) return 1;
	if (s != AVAuthorizationStatusNotDetermined) return 0;
	// Se pregunta EN EL MOMENTO DE USARLA (ADR 0046) y se espera la respuesta: esto
	// corre fuera del hilo de la interfaz.
	dispatch_semaphore_t listo = dispatch_semaphore_create(0);
	__block BOOL si = NO;
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL ok) {
		si = ok;
		dispatch_semaphore_signal(listo);
	}];
	dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC));
	return si ? 1 : 0;
}

int sytueCamAbrir(const char *id, int ancho, int alto, int fps) {
	@autoreleasepool {
		if (!sytueCamPermiso()) return -2;
		sytueCamCerrar();
		AVCaptureDevice *d = nil;
		if (id != NULL && id[0] != 0) d = [AVCaptureDevice deviceWithUniqueID:[NSString stringWithUTF8String:id]];
		if (d == nil) d = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
		if (d == nil) return -1;
		NSError *err = nil;
		AVCaptureDeviceInput *in = [AVCaptureDeviceInput deviceInputWithDevice:d error:&err];
		if (in == nil) return -1;
		SytueCam *c = [SytueCam new];
		c.sesion = [AVCaptureSession new];
		if (![c.sesion canAddInput:in]) return -1;
		[c.sesion addInput:in];
		// El preset más cercano a lo pedido, después de la entrada. Es una petición: la
		// FaceTime HD de un MacBook da 1080p igual (medido), y por eso la llamada escala
		// antes de codificar (camara.Escalar).
		NSString *preset = AVCaptureSessionPreset640x480;
		if (ancho >= 1280) preset = AVCaptureSessionPreset1280x720;
		if ([c.sesion canSetSessionPreset:preset]) c.sesion.sessionPreset = preset;
		AVCaptureVideoDataOutput *out = [AVCaptureVideoDataOutput new];
		out.alwaysDiscardsLateVideoFrames = YES;
		NSNumber *nv12 = [NSNumber numberWithUnsignedInt:kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange];
		out.videoSettings = [NSDictionary dictionaryWithObject:nv12 forKey:(__bridge NSString *)kCVPixelBufferPixelFormatTypeKey];
		c.cola = dispatch_queue_create("eu.sytue.camara", DISPATCH_QUEUE_SERIAL);
		[out setSampleBufferDelegate:c queue:c.cola];
		if (![c.sesion canAddOutput:out]) return -1;
		[c.sesion addOutput:out];
		if (fps > 0 && [d lockForConfiguration:nil]) {
			for (AVFrameRateRange *r in d.activeFormat.videoSupportedFrameRateRanges) {
				if (r.minFrameRate <= fps && fps <= r.maxFrameRate) {
					d.activeVideoMinFrameDuration = CMTimeMake(1, fps);
					d.activeVideoMaxFrameDuration = CMTimeMake(1, fps);
					break;
				}
			}
			[d unlockForConfiguration];
		}
		[c.sesion startRunning];
		cam = c;
		return 0;
	}
}

int sytueCamLeer(uint8_t *dst, int cap, int *ancho, int *alto, long *secuencia) {
	int r = 0;
	pthread_mutex_lock(&mu);
	*ancho = ultimoAncho;
	*alto = ultimoAlto;
	int n = ultimoAncho * ultimoAlto * 3 / 2;
	if (ultimo != NULL && ultimaSec != *secuencia && n > 0 && n <= cap) {
		memcpy(dst, ultimo, n);
		*secuencia = ultimaSec;
		r = 1;
	}
	pthread_mutex_unlock(&mu);
	return r;
}

void sytueCamCerrar(void) {
	@autoreleasepool {
		if (cam != nil) {
			[cam.sesion stopRunning];
			cam = nil;
		}
		pthread_mutex_lock(&mu);
		ultimoAncho = ultimoAlto = 0;
		pthread_mutex_unlock(&mu);
	}
}

int sytueCamLista(char *dst, int cap) {
	@autoreleasepool {
		NSMutableString *s = [NSMutableString string];
		for (AVCaptureDevice *d in camaras()) [s appendFormat:@"%@\t%@\n", d.uniqueID, d.localizedName];
		const char *u = s.UTF8String;
		int n = (int)strlen(u);
		if (n >= cap) n = cap - 1;
		memcpy(dst, u, n);
		dst[n] = 0;
		return n;
	}
}
