package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDB      *sql.DB
	testQueries *database.Queries
)

func setupTestDB(t *testing.T) {
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Fatalf("TEST_DATABASE_URL is not set")
	}

	var err error
	testDB, err = sql.Open("postgres", dbURL)
	require.NoError(t, err, "failed to connect to test database")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = testDB.PingContext(ctx)
	require.NoError(t, err, "failed to ping test database")

	testQueries = database.New(testDB)
	cleanupTestData(t)
}

func cleanupTestData(t *testing.T) {
	ctx := context.Background()

	_, err := testDB.ExecContext(ctx, "DELETE FROM refresh_tokens")
	require.NoError(t, err, "failed to cleanup refresh_tokens")

	_, err = testDB.ExecContext(ctx, "DELETE FROM users")
	require.NoError(t, err, "failed to cleanup users")
}

func teardownTestDB(t *testing.T) {
	if testDB != nil {
		cleanupTestData(t)
		testDB.Close()
	}
}

func createTestUser(t *testing.T, email, password string) database.User {
	hashedPassword, err := utils.HashPassword(password)
	require.NoError(t, err)

	_, err = testQueries.CreateUser(context.Background(), database.CreateUserParams{
		Email:        email,
		PasswordHash: hashedPassword,
	})
	require.NoError(t, err)

	fullUser, err := testQueries.GetUsersByEmail(context.Background(), email)
	require.NoError(t, err)

	return fullUser
}

func createTestRefreshToken(t *testing.T, token string) {
	user := createTestUser(t, "sample@mail.com", "password")
	_, err := testQueries.CreateRefreshToken(context.Background(), database.CreateRefreshTokenParams{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})
	require.NoError(t, err)
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestLogin(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	validator := validator.New(validator.WithRequiredStructEnabled())
	jwtSecret := "test-secret-key-for-integration-tests"

	tests := []struct {
		name             string
		requestBody      any
		setupData        func(*testing.T)
		expectedStatus   int
		expectedError    string
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successful login",
			requestBody: LoginRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			setupData: func(t *testing.T) {
				createTestUser(t, "test@example.com", "password123")
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var response utils.SuccessResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "login success", response.Message)
				assert.NotNil(t, response.Data)

				dataMap := response.Data.(map[string]any)
				assert.NotEmpty(t, dataMap["access_token"])
				assert.NotEmpty(t, dataMap["refresh_token"])
				assert.NotNil(t, dataMap["user"])

				userMap := dataMap["user"].(map[string]any)
				assert.Equal(t, "test@example.com", userMap["email"])
				assert.NotEmpty(t, userMap["id"])

				cookies := rec.Result().Cookies()
				var refreshCookie *http.Cookie
				for _, cookie := range cookies {
					if cookie.Name == "refresh_token" {
						refreshCookie = cookie
						break
					}
				}
				assert.NotNil(t, refreshCookie, "Refresh token cookie should be set")
				assert.NotEmpty(t, refreshCookie.Value)
				assert.True(t, refreshCookie.HttpOnly)
				assert.True(t, refreshCookie.Secure)
			},
		},
		{
			name:           "invalid request body",
			requestBody:    "invalid json",
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid request body",
		},
		{
			name: "missing email",
			requestBody: LoginRequest{
				Password: "password123",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid email format",
			requestBody: LoginRequest{
				Email:    "invalid-email",
				Password: "password123",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing password",
			requestBody: LoginRequest{
				Email: "test@example.com",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			requestBody: LoginRequest{
				Email:    "notfound@example.com",
				Password: "password123",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid email or password",
		},
		{
			name: "wrong password",
			requestBody: LoginRequest{
				Email:    "test@example.com",
				Password: "wrongpassword",
			},
			setupData: func(t *testing.T) {
				createTestUser(t, "test@example.com", "correctpassword")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid email or password",
		},
		{
			name: "multiple login attempts create multiple refresh tokens",
			requestBody: LoginRequest{
				Email:    "multilogin@example.com",
				Password: "password123",
			},
			setupData: func(t *testing.T) {
				createTestUser(t, "multilogin@example.com", "password123")
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var response1 utils.SuccessResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response1)
				assert.NoError(t, err)

				req2 := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"email":"multilogin@example.com","password":"password123"}`))
				req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec2 := httptest.NewRecorder()
				c2 := echo.New().NewContext(req2, rec2)

				handler := &AuthHandler{
					validator: validator,
					dbQueries: testQueries,
					config: &utils.Config{
						JwtSecret: jwtSecret,
					},
				}
				err = handler.Login(c2)
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec2.Code)

				var response2 utils.SuccessResponse
				err = json.Unmarshal(rec2.Body.Bytes(), &response2)
				assert.NoError(t, err)

				data1 := response1.Data.(map[string]any)
				data2 := response2.Data.(map[string]any)
				assert.NotEqual(t, data1["refresh_token"], data2["refresh_token"], "each login should create a unique refresh token")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupTestData(t)
			tt.setupData(t)

			e := echo.New()
			e.Validator = &CustomValidator{validator: validator}

			cfg := &utils.Config{
				JwtSecret: jwtSecret,
			}

			handler := &AuthHandler{
				validator: validator,
				dbQueries: testQueries,
				config:    cfg,
			}

			// Create request body
			var reqBody io.Reader
			if str, ok := tt.requestBody.(string); ok {
				reqBody = strings.NewReader(str)
			} else {
				bodyBytes, err := json.Marshal(tt.requestBody)
				require.NoError(t, err)
				reqBody = strings.NewReader(string(bodyBytes))
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/login", reqBody)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute
			err := handler.Login(c)

			// Assert
			if tt.expectedError != "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)

				var errorResponse utils.ErrorResponse
				err = json.Unmarshal(rec.Body.Bytes(), &errorResponse)
				assert.NoError(t, err)
				assert.Contains(t, errorResponse.Message, tt.expectedError)
			} else if tt.validateResponse != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)
				tt.validateResponse(t, rec)
			} else {
				// For validation errors, check status code
				if err != nil {
					he, ok := err.(*echo.HTTPError)
					if ok {
						assert.Equal(t, tt.expectedStatus, he.Code)
					} else {
						assert.Equal(t, tt.expectedStatus, rec.Code)
					}
				} else {
					assert.Equal(t, tt.expectedStatus, rec.Code)
				}
			}
		})
	}
}

func TestRegister(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	validator := validator.New(validator.WithRequiredStructEnabled())

	tests := []struct {
		name             string
		requestBody      any
		setupData        func(*testing.T)
		expectedStatus   int
		expectedError    string
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "invalid request body",
			requestBody:    "invalid json",
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid request body",
		},
		{
			name: "missing email",
			requestBody: RegisterRequest{
				Password:        "password",
				ConfirmPassword: "password",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "account already registered",
			requestBody: RegisterRequest{
				Email:           "test@mail.com",
				Password:        "password",
				ConfirmPassword: "password",
			},
			setupData: func(t *testing.T) {
				createTestUser(t, "test@mail.com", "password")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "register success",
			requestBody: RegisterRequest{
				Email:           "test@mail.com",
				Password:        "password",
				ConfirmPassword: "password",
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var res utils.SuccessResponse
				err := json.Unmarshal(rec.Body.Bytes(), &res)
				assert.NoError(t, err)
				assert.Equal(t, "register success", res.Message)
				assert.NotNil(t, res.Data)

				dataMap := res.Data.(map[string]any)
				assert.NotNil(t, dataMap["id"])
				assert.NotNil(t, dataMap["email"])
				assert.NotNil(t, dataMap["created_at"])
				assert.NotNil(t, dataMap["updated_at"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupTestData(t)
			tt.setupData(t)

			e := echo.New()
			e.Validator = &CustomValidator{validator: validator}

			handler := &AuthHandler{
				validator: validator,
				dbQueries: testQueries,
			}

			// Create request body
			var reqBody io.Reader
			if str, ok := tt.requestBody.(string); ok {
				reqBody = strings.NewReader(str)
			} else {
				bodyBytes, err := json.Marshal(tt.requestBody)
				require.NoError(t, err)
				reqBody = strings.NewReader(string(bodyBytes))
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/register", reqBody)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Register(c)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)

				var errorResponse utils.ErrorResponse
				err = json.Unmarshal(rec.Body.Bytes(), &errorResponse)
				assert.NoError(t, err)
				assert.Contains(t, errorResponse.Message, tt.expectedError)
			} else if tt.validateResponse != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)
				tt.validateResponse(t, rec)
			} else {
				if err != nil {
					he, ok := err.(*echo.HTTPError)
					if ok {
						assert.Equal(t, tt.expectedStatus, he.Code)
					} else {
						assert.Equal(t, tt.expectedStatus, rec.Code)
					}
				} else {
					assert.Equal(t, tt.expectedStatus, rec.Code)
				}
			}
		})
	}
}

func TestRefreshToken(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	validator := validator.New(validator.WithRequiredStructEnabled())
	jwtSecret := "test-secret-key-for-integration-tests"

	tests := []struct {
		name             string
		headers          http.Header
		setupData        func(t *testing.T)
		expectedStatus   int
		expectedError    string
		validateResponse func(t *testing.T, rec *httptest.ResponseRecorder)
		setCookie        bool
	}{
		{
			name:           "missing token",
			headers:        http.Header{},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "no token",
		},
		{
			name: "token from header",
			headers: http.Header{
				"Authorization": []string{"Bearer token"},
			},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid token",
		},
		{
			name:           "token from cookie",
			headers:        http.Header{},
			setupData:      func(t *testing.T) {},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid token",
			setCookie:      true,
		},
		{
			name: "valid refresh token",
			headers: http.Header{
				"Authorization": []string{"Bearer token"},
			},
			setupData: func(t *testing.T) {
				createTestRefreshToken(t, "token")
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var response utils.SuccessResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "token refreshed successfully", response.Message)
				assert.NotNil(t, response.Data)

				dataMap := response.Data.(map[string]any)
				assert.NotEmpty(t, dataMap["access_token"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupTestData(t)
			tt.setupData(t)

			e := echo.New()

			cfg := &utils.Config{
				JwtSecret: jwtSecret,
			}

			handler := &AuthHandler{
				validator: validator,
				dbQueries: testQueries,
				config:    cfg,
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
			req.Header = tt.headers
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			if tt.setCookie {
				tokenCookie := new(http.Cookie)
				tokenCookie.Name = "refresh_token"
				tokenCookie.Value = "token"
				tokenCookie.Path = "/"
				tokenCookie.Expires = time.Now().Add(5 * time.Minute)
				tokenCookie.HttpOnly = true
				tokenCookie.Secure = true
				req.AddCookie(tokenCookie)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Refresh(c)

			if tt.expectedError != "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)

				var errorResponse utils.ErrorResponse
				err = json.Unmarshal(rec.Body.Bytes(), &errorResponse)
				assert.NoError(t, err)
				assert.Contains(t, errorResponse.Message, tt.expectedError)
			} else if tt.validateResponse != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)
				tt.validateResponse(t, rec)
			} else {
				if err != nil {
					he, ok := err.(*echo.HTTPError)
					if ok {
						assert.Equal(t, tt.expectedError, he.Code)
					} else {
						assert.Equal(t, tt.expectedStatus, rec.Code)
					}
				} else {
					assert.Equal(t, tt.expectedStatus, rec.Code)
				}
			}
		})
	}
}

// CustomValidator is a wrapper for validator to implement echo.Validator interface
type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i any) error {
	return cv.validator.Struct(i)
}
