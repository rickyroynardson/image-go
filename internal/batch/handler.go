package batch

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/pubsub"
	"github.com/rickyroynardson/image-go/internal/utils"
)

type BatchHandler struct {
	validator *validator.Validate
	dbQueries *database.Queries
	config    *utils.Config
}

func NewHandler(validator *validator.Validate, dbQueries *database.Queries, config *utils.Config) *BatchHandler {
	return &BatchHandler{
		validator: validator,
		dbQueries: dbQueries,
		config:    config,
	}
}

// GetAll godoc
// @Summary Get list of batches
// @Description Retrieve all batches for the authenticated user
// @Tags batches
// @Produce json
// @Security BearerAuth
// @Success 200 {object} utils.SuccessResponse{data=[]BatchesResponse}
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /batches [get]
func (h *BatchHandler) GetAll(c echo.Context) error {
	userID := c.Get("userID").(uuid.UUID)

	batches, err := h.dbQueries.GetAllUserBatches(c.Request().Context(), userID)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	batchesRes := make([]BatchesResponse, len(batches))
	for i, b := range batches {
		var watermarkKey, watermarkURL string
		if b.WatermarkKey.Valid {
			watermarkKey = b.WatermarkKey.String
		}
		if b.WatermarkUrl.Valid {
			watermarkURL = b.WatermarkUrl.String
		}
		batchesRes[i] = BatchesResponse{
			ID:                   b.ID.String(),
			UserID:               b.UserID.String(),
			Name:                 b.Name.String,
			WatermarkKey:         watermarkKey,
			WatermarkURL:         watermarkURL,
			CreatedAt:            b.CreatedAt,
			UpdatedAt:            b.UpdatedAt,
			ImageCount:           int(b.ImageCount),
			ImagePendingCount:    int(b.ImagePendingCount),
			ImageProcessingCount: int(b.ImageProcessingCount),
			ImageCompletedCount:  int(b.ImageCompletedCount),
			ImageFailedCount:     int(b.ImageFailedCount),
		}
	}

	return utils.RespondJSON(c, http.StatusOK, "batches retrieved successfully", batchesRes)
}

// GetByID godoc
// @Summary Get batch by ID
// @Description Retrieve a specific batch by its ID for the authenticated user
// @Tags batches
// @Produce json
// @Security BearerAuth
// @Param batchID path string true "Batch ID"
// @Success 200 {object} utils.SuccessResponse{data=BatchResponse}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /batches/{batchID} [get]
func (h *BatchHandler) GetByID(c echo.Context) error {
	batchID := c.Param("batchID")
	userID := c.Get("userID").(uuid.UUID)

	batchUUID, err := uuid.Parse(batchID)
	if err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid batch ID")
	}

	batch, err := h.dbQueries.GetUserBatchByID(c.Request().Context(), database.GetUserBatchByIDParams{
		ID:     batchUUID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return utils.RespondError(c, http.StatusNotFound, "batch not found")
		}
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	images, err := h.dbQueries.GetImagesByBatchID(c.Request().Context(), batch.ID)
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}
	imagesRes := make([]ImageResponse, len(images))
	for i, img := range images {
		imagesRes[i] = ImageResponse{
			ID:           img.ID,
			BatchID:      img.BatchID,
			Key:          img.Key,
			OriginalURL:  img.OriginalUrl,
			ProcessedURL: img.ProcessedUrl.String,
			Status:       img.Status,
			CreatedAt:    img.CreatedAt,
			UpdatedAt:    img.UpdatedAt,
		}
	}

	res := BatchResponse{
		ID:           batch.ID,
		UserID:       batch.UserID,
		Name:         batch.Name.String,
		WatermarkKey: batch.WatermarkKey.String,
		WatermarkURL: batch.WatermarkUrl.String,
		CreatedAt:    batch.CreatedAt,
		UpdatedAt:    batch.UpdatedAt,
		Images:       imagesRes,
	}

	return utils.RespondJSON(c, http.StatusOK, "batch retrieved successfully", res)
}

// Create godoc
// @Summary Create batch
// @Description Create a new batch with images and optional watermark
// @Tags batches
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param name formData string false "Batch name"
// @Param files formData file true "Image files (multiple)"
// @Param watermark formData file false "Watermark image file"
// @Success 201 {object} utils.SuccessResponse{data=nil}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /batches [post]
func (h *BatchHandler) Create(c echo.Context) error {
	name := c.FormValue("name")
	userID := c.Get("userID").(uuid.UUID)

	ch, err := h.config.RabbitMQConn.Channel()
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}
	defer ch.Close()

	const maxMemory = 10 << 20
	c.Request().ParseMultipartForm(int64(maxMemory))
	form, err := c.MultipartForm()
	if err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid form data")
	}
	files := form.File["files"]
	if len(files) == 0 {
		return utils.RespondError(c, http.StatusBadRequest, "no files uploaded")
	}
	watermarks := form.File["watermark"]
	if len(watermarks) > 1 {
		return utils.RespondError(c, http.StatusBadRequest, "only one watermark file allowed")
	}

	var watermarkURL string
	var watermarkKey string
	if len(watermarks) == 1 {
		watermark := watermarks[0]
		src, err := watermark.Open()
		if err != nil {
			return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
		}
		mediaType, _, err := mime.ParseMediaType(watermark.Header.Get("Content-Type"))
		if err != nil {
			return utils.RespondError(c, http.StatusBadRequest, "invalid watermark file")
		}
		if mediaType != "image/jpeg" && mediaType != "image/png" {
			return utils.RespondError(c, http.StatusBadRequest, "unsupported watermark file type")
		}
		assetPath := utils.GetAssetPath(mediaType)
		fileName := "watermark/" + assetPath
		_, err = h.config.S3Client.PutObject(c.Request().Context(), &s3.PutObjectInput{
			Bucket:      aws.String(h.config.S3Bucket),
			Key:         aws.String(fileName),
			Body:        src,
			ContentType: aws.String(mediaType),
		})
		if err != nil {
			return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
		}
		watermarkKey = fileName
		watermarkURL = utils.GetObjectURL(h.config.S3CfDistribution, fileName)
	}

	batch, err := h.dbQueries.CreateBatch(c.Request().Context(), database.CreateBatchParams{
		UserID:       userID,
		Name:         sql.NullString{String: name, Valid: true},
		WatermarkKey: sql.NullString{String: watermarkKey, Valid: true},
		WatermarkUrl: sql.NullString{String: watermarkURL, Valid: true},
	})
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}

	var imageSuccessCount int
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			fmt.Printf("error opening file: %v", err)
			continue
		}

		mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
		if err != nil {
			fmt.Printf("error reading content-type: %v", err)
			src.Close()
			continue
		}
		if mediaType != "image/jpeg" && mediaType != "image/png" {
			fmt.Printf("unsupported file type")
			src.Close()
			continue
		}

		assetPath := utils.GetAssetPath(mediaType)
		fileName := "raw/" + assetPath
		_, err = h.config.S3Client.PutObject(c.Request().Context(), &s3.PutObjectInput{
			Bucket:      aws.String(h.config.S3Bucket),
			Key:         aws.String(fileName),
			Body:        src,
			ContentType: aws.String(mediaType),
		})
		if err != nil {
			fmt.Printf("error uploading to s3: %v", err)
			src.Close()
			continue
		}

		objectURL := utils.GetObjectURL(h.config.S3CfDistribution, fileName)

		image, err := h.dbQueries.CreateImage(c.Request().Context(), database.CreateImageParams{
			BatchID:     batch.ID,
			Key:         fileName,
			OriginalUrl: objectURL,
		})
		if err != nil {
			fmt.Printf("error saving image: %s\n", assetPath)
			continue
		}

		imageTask := ImageTask{
			ImageID: image.ID,
		}
		err = pubsub.PublishJSON(ch, utils.ImageGoDirect, utils.ImageGoTask, imageTask)
		if err != nil {
			fmt.Printf("error publishing message: %v", err)
			continue
		}
		fmt.Printf("%s uploaded\n", image.OriginalUrl)
		imageSuccessCount++
	}

	if imageSuccessCount == 0 {
		h.dbQueries.HardDeleteBatchByID(c.Request().Context(), database.HardDeleteBatchByIDParams{
			ID:     batch.ID,
			UserID: userID,
		})
		return utils.RespondError(c, http.StatusBadRequest, "failed to create batch: no valid images uploaded")
	}

	return utils.RespondJSON(c, http.StatusCreated, "batch created successfully", nil)
}

// DeleteByID godoc
// @Summary Delete batch by ID
// @Description Delete a specific batch by its ID for the authenticated user
// @Tags batches
// @Produce json
// @Security BearerAuth
// @Param batchID path string true "Batch ID"
// @Success 200 {object} utils.SuccessResponse{data=nil}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /batches/{batchID} [delete]
func (h *BatchHandler) DeleteByID(c echo.Context) error {
	batchID := c.Param("batchID")
	userID := c.Get("userID").(uuid.UUID)

	batchUUID, err := uuid.Parse(batchID)
	if err != nil {
		return utils.RespondError(c, http.StatusBadRequest, "invalid batch ID")
	}

	err = h.dbQueries.DeleteBatchByID(c.Request().Context(), database.DeleteBatchByIDParams{
		ID:     batchUUID,
		UserID: userID,
	})
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, "internal server error")
	}
	return utils.RespondJSON(c, http.StatusOK, "batch deleted successfully", nil)
}
