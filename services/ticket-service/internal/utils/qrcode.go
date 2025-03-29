package utils

import (
	"encoding/base64"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCodeBase64 creates a QR code from a string and returns it as a base64 encoded PNG
func GenerateQRCodeBase64(data string) (string, error) {
	var png []byte
	png, err := qrcode.Encode(data, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	base64Image := base64.StdEncoding.EncodeToString(png)
	return base64Image, nil
}
