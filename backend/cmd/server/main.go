package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rickyroynardson/image-go/internal/auth"
	"github.com/rickyroynardson/image-go/internal/batch"
	"github.com/rickyroynardson/image-go/internal/database"
	"github.com/rickyroynardson/image-go/internal/middleware"
	"github.com/rickyroynardson/image-go/internal/utils"
)

func main() {
	e := echo.New()

	err := godotenv.Load()
	if err != nil {
		e.Logger.Fatalf("failed to load env: %v", err)
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		e.Logger.Fatal("JWT_SECRET is not set")
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		e.Logger.Fatal("S3_BUCKET is not set")
	}
	s3CfDistribution := os.Getenv("S3_CF_DISTRIBUTION")
	if s3CfDistribution == "" {
		e.Logger.Fatal("S3_CF_DISTRIBUTION is not set")
	}
	rabbitMqURL := os.Getenv("RABBIT_MQ_URL")
	if rabbitMqURL == "" {
		e.Logger.Fatal("RABBIT_MQ_URL is not set")
	}

	conn, err := amqp.Dial(rabbitMqURL)
	if err != nil {
		e.Logger.Fatalf("failed to connect rabbitmq: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		e.Logger.Fatal("failed to open channel: %v", err)
	}
	defer ch.Close()

	imageTaskQueue, err := ch.QueueDeclare("image_tasks", true, false, false, false, nil)
	if err != nil {
		e.Logger.Fatalf("failed to declare queue: %v", err)
	}

	err = ch.ExchangeDeclare(utils.ImageGoDirect, "direct", true, false, false, false, nil)
	if err != nil {
		e.Logger.Fatalf("failed to declare exchange: %v", err)
	}

	err = ch.QueueBind(imageTaskQueue.Name, utils.ImageGoDirect, utils.ImageGoDirect, false, nil)
	if err != nil {
		e.Logger.Fatalf("failed to bind queue: %v", err)
	}

	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		e.Logger.Fatalf("failed to load aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	cfg := &utils.Config{
		JwtSecret:        jwtSecret,
		S3Bucket:         s3Bucket,
		S3CfDistribution: s3CfDistribution,
		S3Client:         s3Client,
		RabbitMQConn:     conn,
	}

	db, err := sql.Open("postgres", "postgres://rickyroynardson:@localhost:5432/image_go?sslmode=disable")
	if err != nil {
		e.Logger.Fatalf("failed to connect sql database: %v", err)
	}

	dbQueries := database.New(db)

	validator := validator.New(validator.WithRequiredStructEnabled())

	authHandler := auth.NewHandler(validator, dbQueries, cfg)
	batchHandler := batch.NewHandler(validator, dbQueries, cfg)

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "image-go")
	})
	apiV1 := e.Group("/api/v1")
	apiV1.POST("/login", authHandler.Login)
	apiV1.POST("/register", authHandler.Register)
	apiV1.POST("/refresh", authHandler.Refresh)

	apiV1.Use(middleware.Authenticated(cfg))
	apiV1.POST("/batches", batchHandler.Create)
	e.Logger.Fatal(e.Start(":3000"))
}
