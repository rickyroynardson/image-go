package image

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/utils"
)

type ImageHandler struct {
	validator *validator.Validate
	dbQueries *database.Queries
	config    *utils.Config
}

func NewHandler(validator *validator.Validate, dbQueries *database.Queries, config *utils.Config) *ImageHandler {
	return &ImageHandler{
		validator: validator,
		dbQueries: dbQueries,
		config:    config,
	}
}

// DeleteByID godoc
// @Summary Delete an image by ID
// @Description Delete an image by its ID for the authenticated user
// @Tags images
// @Param imageID path string true "Image ID"
// @Produce json
// @Security BearerAuth
// @Success 200 {object} utils.SuccessResponse{data=nil}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /images/{imageID} [delete]
func (h *ImageHandler) DeleteByID(c echo.Context) error {
	imageID := c.Param("imageID")
	userID := c.Get("userID").(uuid.UUID)

	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid image ID")
	}

	batches, err := h.dbQueries.GetAllUserBatches(c.Request().Context(), userID)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}
	batchIDs := make([]uuid.UUID, len(batches))
	for i, b := range batches {
		batchIDs[i] = b.ID
	}

	err = h.dbQueries.DeleteImageByID(c.Request().Context(), database.DeleteImageByIDParams{
		ID:      imageUUID,
		Column2: batchIDs,
	})
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}
	return utils.RespondJSON(c, http.StatusOK, "image deleted successfully", nil)
}
