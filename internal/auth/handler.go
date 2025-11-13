package auth

import (
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/utils"
)

type AuthHandler struct {
	validator *validator.Validate
	dbQueries *database.Queries
	config    *utils.Config
}

func NewHandler(validator *validator.Validate, dbQueries *database.Queries, config *utils.Config) *AuthHandler {
	return &AuthHandler{
		validator: validator,
		dbQueries: dbQueries,
		config:    config,
	}
}

// Login godoc
// @Summary Login
// @Description Login with email and password
// @Tags authentication
// @Accept json
// @Produce json
// @Param login body LoginRequest true "Login Request"
// @Success 200 {object} utils.SuccessResponse{data=LoginResponse}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var body LoginRequest
	if err := c.Bind(&body); err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Struct(body); err != nil {
		return utils.RespondError(c, http.StatusBadRequest, err.Error())
	}

	user, err := h.dbQueries.GetUsersByEmail(c.Request().Context(), body.Email)
	if err != nil {
		return utils.RespondError(c, http.StatusUnauthorized, "invalid email or password")
	}

	if err := utils.ComparePassword(user.PasswordHash, body.Password); err != nil {
		return utils.RespondError(c, http.StatusUnauthorized, "invalid email or password")
	}

	token, err := utils.GenerateJWT(user.ID, h.config.JwtSecret)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	refresh, err := utils.GenerateRefresh()
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	refreshToken, err := h.dbQueries.CreateRefreshToken(c.Request().Context(), database.CreateRefreshTokenParams{
		UserID:    user.ID,
		Token:     refresh,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = refreshToken.Token
	refreshCookie.Path = "/"
	refreshCookie.Expires = refreshToken.ExpiresAt
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = true
	c.SetCookie(refreshCookie)

	res := struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         User   `json:"user"`
	}{
		AccessToken:  token,
		RefreshToken: refreshToken.Token,
		User: User{
			ID:        user.ID.String(),
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}

	return utils.RespondJSON(c, http.StatusOK, "login success", res)
}

// Register godoc
// @Summary Register
// @Description Register a new user
// @Tags authentication
// @Accept json
// @Produce json
// @Param register body RegisterRequest true "Register Request"
// @Success 201 {object} utils.SuccessResponse{data=User}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var body RegisterRequest
	if err := c.Bind(&body); err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Struct(body); err != nil {
		return utils.RespondError(c, http.StatusBadRequest, err.Error())
	}

	hashedPassword, err := utils.HashPassword(body.Password)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	user, err := h.dbQueries.CreateUser(c.Request().Context(), database.CreateUserParams{
		Email:        body.Email,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	resUser := User{
		ID:        user.ID.String(),
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	return utils.RespondJSON(c, http.StatusCreated, "register success", resUser)
}

// Refresh godoc
// @Summary Refresh access token
// @Description Refresh access token using refresh token (can be provided as cookie or Authorization header)
// @Tags authentication
// @Accept json
// @Produce json
// @Param Authorization header string false "Bearer Token (refresh token)"
// @Success 200 {object} utils.SuccessResponse{data=RefreshResponse}
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /refresh [post]
func (h *AuthHandler) Refresh(c echo.Context) error {
	var refreshToken string
	cookie, err := c.Cookie("refresh_token")
	if err != nil {
		token, err := utils.GetAuthorizationToken(c.Request().Header)
		if err != nil {
			return utils.RespondError(c, http.StatusUnauthorized, "no token")
		}
		refreshToken = token
	} else {
		refreshToken = cookie.Value
	}

	token, err := h.dbQueries.GetRefreshToken(c.Request().Context(), refreshToken)
	if err != nil {
		refreshCookie := new(http.Cookie)
		refreshCookie.Name = "refresh_token"
		refreshCookie.Value = ""
		refreshCookie.Path = "/"
		refreshCookie.MaxAge = -1
		refreshCookie.HttpOnly = true
		refreshCookie.Secure = true
		c.SetCookie(refreshCookie)

		return utils.RespondError(c, http.StatusUnauthorized, "invalid token")
	}

	accessToken, err := utils.GenerateJWT(token.UserID, h.config.JwtSecret)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	return utils.RespondJSON(c, http.StatusOK, "token refreshed successfully", struct {
		AccessToken string `json:"access_token"`
	}{
		AccessToken: accessToken,
	})
}
