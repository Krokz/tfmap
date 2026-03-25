package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Krokz/tfmap/internal/model"
)

func (r *Reader) ReadS3(backend *model.Backend, profile string) (*model.StateSnapshot, error) {
	bucket, _ := backend.Config["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("bucket not configured in backend")
	}

	key, _ := backend.Config["key"].(string)
	if key == "" {
		key = "terraform.tfstate"
	}

	region, _ := backend.Config["region"].(string)
	if region == "" {
		region = "us-east-1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("reading s3://%s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from s3://%s/%s: %w", bucket, key, err)
	}

	var snap model.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing state from s3://%s/%s: %w", bucket, key, err)
	}

	return &snap, nil
}
