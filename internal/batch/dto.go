package batch

import (
	"time"

	"github.com/google/uuid"
	"github.com/rickyroynardson/image-go/internal/database"
)

type ImageTask struct {
	ImageID uuid.UUID `json:"image_id"`
}

type ImageResponse struct {
	ID           uuid.UUID            `json:"id"`
	BatchID      uuid.UUID            `json:"batch_id"`
	Key          string               `json:"key"`
	OriginalURL  string               `json:"original_url"`
	ProcessedURL string               `json:"processed_url"`
	Status       database.ImageStatus `json:"status"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type BatchesResponse struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	Name                 string    `json:"name"`
	WatermarkKey         string    `json:"watermark_key"`
	WatermarkURL         string    `json:"watermark_url"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	ImageCount           int       `json:"image_count"`
	ImagePendingCount    int       `json:"image_pending_count"`
	ImageProcessingCount int       `json:"image_processing_count"`
	ImageCompletedCount  int       `json:"image_completed_count"`
	ImageFailedCount     int       `json:"image_failed_count"`
}

type BatchResponse struct {
	ID           uuid.UUID       `json:"id"`
	UserID       uuid.UUID       `json:"user_id"`
	Name         string          `json:"name"`
	WatermarkKey string          `json:"watermark_key"`
	WatermarkURL string          `json:"watermark_url"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Images       []ImageResponse `json:"images"`
}
