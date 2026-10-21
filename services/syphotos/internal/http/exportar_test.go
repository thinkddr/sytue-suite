package http

import "testing"

func TestFormatoDeExportacion(t *testing.T) {
	casos := []struct{ pedido, formato, ext, tipo string }{
		{"", "jpeg", ".jpg", "image/jpeg"},
		{"jpeg", "jpeg", ".jpg", "image/jpeg"},
		{"png", "png", ".png", "image/png"},
		{" PNG ", "png", ".png", "image/png"},
		{"webp", "jpeg", ".jpg", "image/jpeg"},
	}
	for _, c := range casos {
		f, e, tp := formatoDeExportacion(c.pedido)
		if f != c.formato || e != c.ext || tp != c.tipo {
			t.Errorf("formatoDeExportacion(%q) = %q %q %q; quería %q %q %q", c.pedido, f, e, tp, c.formato, c.ext, c.tipo)
		}
	}
}
