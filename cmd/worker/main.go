package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/image"
	"github.com/rickyroynardson/image-go/internal/pubsub"
	"github.com/rickyroynardson/image-go/internal/utils"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("failed to load env: %v", err)
	}
	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		log.Fatalln("POSTGRES_URL is not set")
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		log.Fatalln("S3_BUCKET is not set")
	}
	s3CfDistribution := os.Getenv("S3_CF_DISTRIBUTION")
	if s3CfDistribution == "" {
		log.Fatalln("S3_CF_DISTRIBUTION is not set")
	}
	rabbitMqURL := os.Getenv("RABBIT_MQ_URL")
	if rabbitMqURL == "" {
		log.Fatalln("RABBIT_MQ_URL is not set")
	}

	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		log.Fatalf("failed to connect sql database: %v", err)
	}

	dbQueries := database.New(db)

	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("failed to load aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	cfg := &utils.Config{
		S3Bucket:         s3Bucket,
		S3CfDistribution: s3CfDistribution,
		S3Client:         s3Client,
	}

	conn, err := amqp.Dial(rabbitMqURL)
	if err != nil {
		log.Fatalf("failed to connect rabbitmq: %v", err)
	}
	defer conn.Close()

	err = pubsub.SubscribeJSON(conn, utils.ImageGoDirect, utils.ImageGoTask, utils.ImageGoTask, pubsub.QueueTypeDurable, image.ProcessImage(dbQueries, cfg))
	if err != nil {
		log.Fatalf("failed to subscribe json: %v", err)
	}

	log.Println("worker started...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}
