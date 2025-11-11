package batch

import (
	"database/sql"
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

// Create godoc
// @Summary Create batch
// @Description Create a new batch with images and optional watermark
// @Tags batch
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param name formData string false "Batch name"
// @Param watermark_url formData string false "Watermark URL"
// @Param files formData file true "Image files (multiple)"
// @Param watermark formData file false "Watermark image file"
// @Success 201 {object} utils.SuccessResponse
// @Failure 400 {object} utils.ErrorResponse
// @Failure 401 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /batches [post]
func (h *BatchHandler) Create(c echo.Context) error {
	name := c.FormValue("name")
	watermark_url := c.FormValue("watermark_url")
	userID := c.Get("userID").(uuid.UUID)

	fmt.Println(name)
	fmt.Println(watermark_url)
	fmt.Println(userID)

	ch, err := h.config.RabbitMQConn.Channel()
	if err != nil {
		return utils.RespondError(c, http.StatusInternalServerError, err.Error())
	}
	defer ch.Close()

	const maxMemory = 10 << 20
	c.Request().ParseMultipartForm(int64(maxMemory))
	form, err := c.MultipartForm()
	if err != nil {
		return utils.RespondError(c, http.StatusBadRequest, err.Error())
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
			return utils.RespondError(c, http.StatusInternalServerError, err.Error())
		}
		mediaType, _, err := mime.ParseMediaType(watermark.Header.Get("Content-Type"))
		if err != nil {
			return utils.RespondError(c, http.StatusInternalServerError, err.Error())
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
			return utils.RespondError(c, http.StatusInternalServerError, err.Error())
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
		return utils.RespondError(c, http.StatusInternalServerError, err.Error())
	}

	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			fmt.Printf("error opening file: %v", err)
			continue
		}
		defer src.Close()

		mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
		if err != nil {
			fmt.Printf("error reading content-type: %v", err)
			continue
		}
		if mediaType != "image/jpeg" && mediaType != "image/png" {
			fmt.Printf("unsupported file type")
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
	}

	return utils.RespondJSON(c, http.StatusCreated, "new batch created", nil)
}
