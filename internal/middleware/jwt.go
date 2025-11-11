package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/rickyroynardson/image-go/internal/utils"
)

func Authenticated(config *utils.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, err := utils.GetAuthorizationToken(c.Request().Header)
			if err != nil {
				return utils.RespondError(c, http.StatusUnauthorized, err.Error())
			}
			userID, err := utils.ValidateJWT(token, config.JwtSecret)
			if err != nil {
				return utils.RespondError(c, http.StatusUnauthorized, err.Error())
			}
			c.Set("userID", userID)
			return next(c)
		}
	}
}
