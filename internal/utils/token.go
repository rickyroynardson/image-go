package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func GenerateJWT(userID uuid.UUID, tokenSecret string) (string, error) {
	jwt := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "image-go",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(5 * time.Minute)),
		Subject:   userID.String(),
	})
	token, err := jwt.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return token, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok {
		userID, err := claims.GetSubject()
		if err != nil {
			return uuid.Nil, err
		}
		id, err := uuid.Parse(userID)
		if err != nil {
			return uuid.Nil, err
		}
		return id, nil
	}
	return uuid.Nil, errors.New("unknown claims type, cannot proceed")
}

func GenerateRefresh() (string, error) {
	key := make([]byte, 32)
	rand.Read(key)
	token := hex.EncodeToString(key)
	if token == "" {
		return "", errors.New("failed to generate refresh token")
	}
	return token, nil
}

func GetAuthorizationToken(headers http.Header) (string, error) {
	authorization := headers.Get("Authorization")
	if authorization == "" {
		return "", errors.New("missing authorization headers")
	}
	prefix := "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return "", errors.New("invalid authorization header format")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if token == "" {
		return "", errors.New("invalid token")
	}
	return token, nil
}
