package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	"github.com/Krokz/tfmap/internal/model"
)

func (r *Reader) ReadAzure(backend *model.Backend) (*model.StateSnapshot, error) {
	storageAccount, _ := backend.Config["storage_account_name"].(string)
	if storageAccount == "" {
		return nil, fmt.Errorf("storage_account_name not configured in backend")
	}

	containerName, _ := backend.Config["container_name"].(string)
	if containerName == "" {
		return nil, fmt.Errorf("container_name not configured in backend")
	}

	key, _ := backend.Config["key"].(string)
	if key == "" {
		key = "terraform.tfstate"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("Azure authentication: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccount)
	client, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure Blob client: %w", err)
	}

	resp, err := client.DownloadStream(ctx, containerName, key, nil)
	if err != nil {
		return nil, fmt.Errorf("reading %s/%s from %s: %w", containerName, key, storageAccount, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s/%s: %w", containerName, key, err)
	}

	var snap model.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing state from %s/%s/%s: %w", storageAccount, containerName, key, err)
	}

	return &snap, nil
}
