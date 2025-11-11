package image

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"log"

	"golang.org/x/image/draw"

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
			log.Printf("error get image, discarding message: %v", err)
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
			log.Printf("error get object, discarding message: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusFailed,
			})
			return pubsub.NackDiscard
		}
		defer obj.Body.Close()

		var watermarkImg image.Image
		if img.WatermarkKey.Valid {
			watermarkObj, err := cfg.S3Client.GetObject(context.Background(), &s3.GetObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(img.WatermarkKey.String),
			})
			if err != nil {
				log.Printf("error get watermark object, requeuing: %v", err)
				return pubsub.NackRequeue
			}
			defer watermarkObj.Body.Close()

			decodedImg, _, err := image.Decode(watermarkObj.Body)
			if err != nil {
				log.Printf("error decode watermark image, requeuing: %v", err)
				return pubsub.NackRequeue
			}
			watermarkImg = decodedImg
		}

		decodedImg, _, err := image.Decode(obj.Body)
		if err != nil {
			log.Printf("error decode image, requeuing: %v", err)
			dbQueries.UpdateImageByID(context.Background(), database.UpdateImageByIDParams{
				ID:     m.ImageID,
				Status: database.ImageStatusProcessing,
			})
			return pubsub.NackRequeue
		}

		bounds := decodedImg.Bounds()
		dst := image.NewRGBA(bounds)

		draw.Draw(dst, bounds, decodedImg, image.Point{}, draw.Src)
		baseWidth := bounds.Dx()
		baseHeight := bounds.Dy()
		if watermarkImg != nil {
			wBounds := watermarkImg.Bounds()
			wWidth := wBounds.Dx()
			wHeight := wBounds.Dy()

			targetWidth := int(float64(baseWidth) * 0.15)
			scale := float64(targetWidth) / float64(wWidth)
			targetHeight := int(float64(wHeight) * scale)

			resizedWatermark := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
			draw.BiLinear.Scale(resizedWatermark, resizedWatermark.Bounds(), watermarkImg, wBounds, draw.Over, nil)

			alphaMask := image.NewUniform(color.Alpha{128})
			padding := int(float64(baseHeight) * 0.01)
			watermarkX := baseWidth - targetWidth - padding
			watermarkY := baseHeight - targetHeight - padding
			watermarkRect := image.Rect(watermarkX, watermarkY, watermarkX+targetWidth, watermarkY+targetHeight)

			draw.DrawMask(dst, watermarkRect, resizedWatermark, image.Point{}, alphaMask, image.Point{}, draw.Over)
		}

		var res bytes.Buffer
		err = jpeg.Encode(&res, dst, &jpeg.Options{
			Quality: 50,
		})
		if err != nil {
			log.Printf("error encode image, requeuing: %v", err)
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
			log.Printf("error uploading processed image, requeuing: %v", err)
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

		log.Printf("%s processed", fileName)
		return pubsub.Ack
	}
}
