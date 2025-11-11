package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

func GetAssetPath(mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	key := make([]byte, 32)
	rand.Read(key)
	name := base64.RawURLEncoding.EncodeToString(key)
	return name + ext
}

func GetObjectURL(cfDistribution, key string) string {
	return fmt.Sprintf("https://%s/%s", cfDistribution, key)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}
