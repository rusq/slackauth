package qrslack

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testqr = `data:image/png;base64,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`

func Test_decodeB64(t *testing.T) {
	type args struct {
		r io.Reader
	}
	tests := []struct {
		name    string
		args    args
		wantLen int
		wantErr bool
	}{
		{
			"decodes a valid payload",
			args{strings.NewReader(testqr)},
			4545,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeB64(tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeB64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("decodeB64() = length mismatch: %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func Test_decodeImage(t *testing.T) {
	decodedData, err := decodeB64(strings.NewReader(testqr))
	if err != nil {
		t.Fatalf("test data QR corrupt: %s", err)
	}
	type args struct {
		r io.Reader
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"successful load of QR png",
			args{bytes.NewReader(decodedData)},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeImage(tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeImage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
		})
	}
}

var testQRPNGFile = filepath.Join("fixtures", "test.png")

func Test_decodeQR(t *testing.T) {
	f, err := os.Open(testQRPNGFile)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	type args struct {
		m image.Image
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"decodes QR code",
			args{img},
			"https://app.slack.com/t/ora600/login/z-app-610187951300-9981196591425-e95b38836efcfc97428861b24e65f8b62aca253d0ed2880e06d34f74de4b40fa?src=qr_code&user_id=UHSD97ZA5&team_id=THY5HTZ8U",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeQR(tt.args.m)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeQR() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("decodeQR() = %v, want %v", got, tt.want)
			}
		})
	}
}
