package utils

import (
	"github.com/labstack/echo/v4"
)

func RespondError(c echo.Context, code int, msg string) error {
	return c.JSON(code, struct {
		Message string `json:"message"`
	}{
		Message: msg,
	})
}

func RespondJSON(c echo.Context, code int, msg string, data any) error {
	return c.JSON(code, struct {
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	}{
		Message: msg,
		Data:    data,
	})
}
