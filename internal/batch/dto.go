package batch

import "github.com/google/uuid"

type ImageTask struct {
	ImageID uuid.UUID `json:"image_id"`
}
