// SPDX-License-Identifier: Apache-2.0
package validation

import devicevalidation "git.sytue.eu/sytue/one/apps/nucleo/validation"

const MaxRasterImageBytes = devicevalidation.MaxRasterImageBytes

var ErrInvalidRasterImage = devicevalidation.ErrInvalidRasterImage

func RasterImageDataURI(dataURI string) (string, error) {
	return devicevalidation.RasterImageDataURI(dataURI)
}

func RasterImageBytes(dataURI string) ([]byte, string, error) {
	return devicevalidation.RasterImageBytes(dataURI)
}
