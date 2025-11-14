# Image Go

A scalable image watermark processing service built with Go. This application provides a REST API for batch image processing with optional watermarking, using a microservices architecture with separate server and worker components.

## Features

- User authentication and authorization with JWT tokens
- Batch image upload and processing
- Optional watermark application to images
- Asynchronous image processing using RabbitMQ
- Image storage on AWS S3 with CloudFront CDN
- RESTful API with Swagger documentation
- Rate limiting and CORS support
- Type-safe database queries using SQLC
- TODO: Watermark image caching to reduce S3 API calls

## Architecture

The application consists of two main components:

- **Server**: HTTP API server built with Echo framework that handles authentication, batch creation, and image management
- **Worker**: Background worker that processes images from RabbitMQ queue, applies watermarks, and uploads processed images to S3

## Prerequisites

- Go 1.25.2 or higher
- PostgreSQL database
- RabbitMQ server
- AWS account with S3 bucket and CloudFront distribution configured
- AWS credentials configured (via IAM role, environment variables, or AWS credentials file)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/rickyroynardson/image-go.git
cd image-go
```

2. Install dependencies:
```bash
go mod download
```

3. Generate database code using SQLC:
```bash
sqlc generate
```

4. Generate Swagger documentation:
```bash
swag init -g cmd/server/main.go
```

## Configuration

Create a `.env` file in the root directory with the following variables:

```
POSTGRES_URL=postgres://user:password@localhost:5432/imagego?sslmode=disable
JWT_SECRET=your-secret-key-here
S3_BUCKET=your-s3-bucket-name
S3_CF_DISTRIBUTION=https://your-cloudfront-distribution.cloudfront.net
RABBIT_MQ_URL=amqp://user:password@localhost:5672/
```

### Environment Variables

- `POSTGRES_URL`: PostgreSQL connection string
- `JWT_SECRET`: Secret key for JWT token signing and verification
- `S3_BUCKET`: AWS S3 bucket name for storing images
- `S3_CF_DISTRIBUTION`: CloudFront distribution URL for serving images
- `RABBIT_MQ_URL`: RabbitMQ connection URL

## Database Setup

Run database migrations using Goose:

```bash
cd sql/schema
goose postgres [DB_URL] up
```

Replace `[DB_URL]` with your PostgreSQL connection string.

## Running the Application

### Start the Server

The server runs on port 3000 by default:

```bash
go run cmd/server/main.go
```

### Start the Worker

Run the worker in a separate terminal to process images:

```bash
go run cmd/worker/main.go
```

## API Documentation

Once the server is running, access the Swagger documentation at:

```
http://localhost:3000/swagger/index.html
```

## API Endpoints

### Authentication

- `POST /api/v1/register` - Register a new user
- `POST /api/v1/login` - Login and receive JWT tokens
- `POST /api/v1/refresh` - Refresh access token

### Batches (Requires Authentication)

- `GET /api/v1/batches` - Get all batches for authenticated user
- `GET /api/v1/batches/:batchID` - Get batch details by ID
- `POST /api/v1/batches` - Create a new batch with images
- `DELETE /api/v1/batches/:batchID` - Delete a batch

### Images (Requires Authentication)

- `DELETE /api/v1/images/:imageID` - Delete an image

## Project Structure

```
.
├── cmd/
│   ├── server/          # HTTP API server
│   │   ├── main.go
│   │   └── docs/        # Swagger documentation
│   └── worker/          # Background worker
│       └── main.go
├── internal/
│   ├── auth/            # Authentication handlers
│   ├── batch/           # Batch management handlers
│   ├── database/        # Generated database code (SQLC)
│   ├── image/           # Image processing service
│   ├── middleware/      # HTTP middleware (JWT auth)
│   ├── pubsub/          # RabbitMQ pub/sub utilities
│   └── utils/           # Utility functions
├── sql/
│   ├── queries/         # SQL queries for SQLC
│   └── schema/          # Database migrations
├── go.mod
├── go.sum
└── README.md
```

## Image Processing

When a batch is created with images:

1. Images are uploaded to S3 in the `raw/` directory
2. Image records are created in the database with `pending` status
3. Processing tasks are published to RabbitMQ
4. Worker consumes tasks and processes images:
   - Downloads original image from S3
   - Applies watermark if provided (scaled to 15% of image width, positioned at bottom-right with 1% padding)
   - Converts to JPEG with 50% quality
   - Uploads processed image to S3 in the `processed/` directory
   - Updates image record with processed URL and `completed` status

## Supported Image Formats

- Input: JPEG, PNG
- Output: JPEG

## Development

### Running Tests

```bash
go test ./...
```

### Code Generation

Generate database code:
```bash
sqlc generate
```

Generate Swagger docs:
```bash
swag init -g cmd/server/main.go
```