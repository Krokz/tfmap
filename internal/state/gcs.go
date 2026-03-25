package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"

	"github.com/Krokz/tfmap/internal/model"
)

func (r *Reader) ReadGCS(backend *model.Backend) (*model.StateSnapshot, error) {
	bucket, _ := backend.Config["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("bucket not configured in backend")
	}

	// GCS backend stores state at <prefix>/default.tfstate
	prefix, _ := backend.Config["prefix"].(string)
	key := "default.tfstate"
	if prefix != "" {
		key = prefix + "/" + key
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}
	defer client.Close()

	reader, err := client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading gs://%s/%s: %w", bucket, key, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading response body from gs://%s/%s: %w", bucket, key, err)
	}

	var snap model.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing state from gs://%s/%s: %w", bucket, key, err)
	}

	return &snap, nil
}
