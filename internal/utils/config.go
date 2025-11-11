package utils

import (
	"github.com/aws/aws-sdk-go-v2/service/s3"
	amqp "github.com/rabbitmq/amqp091-go"
)

const ImageGoDirect = "image-go_direct"
const ImageGoTask = "image_tasks"

type Config struct {
	JwtSecret        string
	S3Bucket         string
	S3CfDistribution string
	S3Client         *s3.Client
	RabbitMQConn     *amqp.Connection
}
