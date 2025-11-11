package image

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rickyroynardson/image-go/internal/batch"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/pubsub"
	"github.com/rickyroynardson/image-go/internal/utils"
)

func ProcessImage(dbQueries *database.Queries, cfg *utils.Config) func(batch.ImageTask) pubsub.AckType {
	return func(m batch.ImageTask) pubsub.AckType {
		img, err := dbQueries.GetImageByID(context.Background(), m.ImageID)
		if err != nil {
			log.Printf("ERROR GET IMAGE: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusFailed,
			})
			return pubsub.NackDiscard
		}

		obj, err := cfg.S3Client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String(cfg.S3Bucket),
			Key:    aws.String(img.Key),
		})
		if err != nil {
			log.Printf("ERROR GET OBJECT FROM S3: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusFailed,
			})
			return pubsub.NackDiscard
		}
		defer obj.Body.Close()

		decodedImg, _, err := image.Decode(obj.Body)
		if err != nil {
			log.Printf("ERROR DECODE IMAGE: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusProcessing,
			})
			return pubsub.NackRequeue
		}

		var res bytes.Buffer
		err = jpeg.Encode(&res, decodedImg, &jpeg.Options{
			Quality: 75,
		})
		if err != nil {
			log.Printf("ERROR ENCODE IMAGE: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusProcessing,
			})
			return pubsub.NackRequeue
		}

		mediaType := "image/jpeg"
		assetPath := utils.GetAssetPath(mediaType)
		fileName := "processed/" + assetPath
		_, err = cfg.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:      aws.String(cfg.S3Bucket),
			Key:         aws.String(fileName),
			Body:        &res,
			ContentType: aws.String(mediaType),
		})
		if err != nil {
			log.Printf("ERROR UPLOAD PROCESSED IMAGE TO S3: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusProcessing,
			})
			return pubsub.NackRequeue
		}

		objectURL := utils.GetObjectURL(cfg.S3CfDistribution, fileName)
		dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
			ID:           img.ID,
			ProcessedUrl: sql.NullString{String: objectURL, Valid: true},
			Status:       database.ImageStatusCompleted,
		})

		return pubsub.Ack
	}
}
