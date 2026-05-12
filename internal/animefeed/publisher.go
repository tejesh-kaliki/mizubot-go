package animefeed

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3PublisherConfig struct {
	AccessKey string
	SecretKey string
	Region    string
	Bucket    string
	Prefix    string
}

type S3Publisher struct {
	client *s3.Client
	bucket string
	prefix string
	region string
}

func NewS3Publisher(ctx context.Context, cfg S3PublisherConfig) (*S3Publisher, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.Region == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 publisher requires access key, secret key, region, and bucket")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	return &S3Publisher{
		client: s3.NewFromConfig(awsCfg),
		bucket: cfg.Bucket,
		prefix: strings.Trim(strings.TrimSpace(cfg.Prefix), "/"),
		region: cfg.Region,
	}, nil
}

func (p *S3Publisher) PublishUserFeed(ctx context.Context, userID string, body string) (string, error) {
	key := fmt.Sprintf("%s.xml", userID)
	if p.prefix != "" {
		key = path.Join(p.prefix, key)
	}

	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       &p.bucket,
		Key:          &key,
		Body:         bytes.NewReader([]byte(body)),
		ContentType:  stringPtr("application/rss+xml"),
		CacheControl: stringPtr("public, max-age=300, stale-while-revalidate=60"),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("s3://%s/%s", p.bucket, key), nil
}

func stringPtr(v string) *string { return &v }
