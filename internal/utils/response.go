package utils

import (
	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func RespondError(c echo.Context, code int, msg string) error {
	return c.JSON(code, ErrorResponse{
		Message: msg,
	})
}

func RespondJSON(c echo.Context, code int, msg string, data any) error {
	return c.JSON(code, SuccessResponse{
		Message: msg,
		Data:    data,
	})
}
