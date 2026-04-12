package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	generationconfig "github.com/go-grpc-sqlc/generation/config"
)

// Client wraps an S3 client configured for Backblaze B2-compatible storage.
type Client struct {
	inner  *s3.Client
	bucket string
}

// New creates an S3 client from generation config.
func New(cfg generationconfig.Config) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKey,
			cfg.S3SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: load aws config: %w", err)
	}

	inner := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		o.UsePathStyle = true
	})

	return &Client{inner: inner, bucket: cfg.S3Bucket}, nil
}

type UploadOptions struct {
	Key         string
	Body        []byte
	ContentType string
}

func (c *Client) Upload(ctx context.Context, opts UploadOptions) error {
	if opts.ContentType == "" {
		opts.ContentType = "audio/wav"
	}
	_, err := c.inner.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(opts.Key),
		Body:        bytesReader(opts.Body),
		ContentType: aws.String(opts.ContentType),
	})
	if err != nil {
		return fmt.Errorf("s3: upload %q: %w", opts.Key, err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.inner.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete %q: %w", key, err)
	}
	return nil
}

func (c *Client) GetSignedURL(ctx context.Context, key string) (string, error) {
	presigner := s3.NewPresignClient(c.inner)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(time.Hour))
	if err != nil {
		return "", fmt.Errorf("s3: presign %q: %w", key, err)
	}
	return req.URL, nil
}
