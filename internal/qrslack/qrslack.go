// Package qrslack contains slack QR decode logic.
package qrslack

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/caiguanhao/readqr"

	"image/png"
)

const (
	maxDataSz = 1 << 16
)

var (
	ErrInvalidQR = errors.New("invalid QR code")

	errHdrLen     = errors.New("unexpected header length")
	errInvalidHdr = errors.New("invalid header")
	errNoData     = errors.New("no image data")
)

func Decode(urlImgData string) (string, error) {
	pngbytes, err := decodeB64(strings.NewReader(urlImgData))
	if err != nil {
		return "", err
	}
	img, err := decodeImage(bytes.NewReader(pngbytes))
	if err != nil {
		return "", err
	}
	return decodeQR(img)
}

func decodeB64(r io.Reader) ([]byte, error) {
	const (
		hdr    = `data:image/png;base64,`
		hdrLen = int64(len(hdr))
	)
	// read first 22 bytes
	data, err := io.ReadAll(io.LimitReader(r, hdrLen))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errHdrLen
		}
		return nil, fmt.Errorf("read header: %w", err)
	}
	if !strings.EqualFold(hdr, string(data)) {
		return nil, errInvalidHdr
	}

	b64r := base64.NewDecoder(base64.StdEncoding, io.LimitReader(r, maxDataSz))
	encoded, err := io.ReadAll(b64r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errNoData
		}
		return nil, fmt.Errorf("read data: %w", err)
	}
	return encoded, nil
}

func decodeImage(r io.Reader) (image.Image, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	if img.Bounds().Dx() != img.Bounds().Dy() {
		return nil, ErrInvalidQR
	}
	return img, nil
}

func decodeQR(m image.Image) (string, error) {
	result, err := readqr.DecodeImage(m)
	if err != nil {
		return "", err
	}
	return result, nil
}
