package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TelegramFileResponse represents the response from getFile
type TelegramFileResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		FileID   string `json:"file_id"`
		FileSize int    `json:"file_size"`
		FilePath string `json:"file_path"`
	} `json:"result"`
}

// ImageService is a helper service for handling images between bots
type ImageService struct {
	OrganizerBotToken string
	BaseURL           string
}

// NewImageService creates a new image service
func NewImageService(organizerBotToken string) *ImageService {
	return &ImageService{
		OrganizerBotToken: organizerBotToken,
		BaseURL:           "https://api.telegram.org/bot",
	}
}

// ConvertFileIDToURL converts a Telegram file ID to a publicly accessible URL
func (s *ImageService) ConvertFileIDToURL(ctx context.Context, fileID string) (string, error) {
	// First, get the file path using getFile method
	getFileURL := fmt.Sprintf("%s%s/getFile?file_id=%s", s.BaseURL, s.OrganizerBotToken, fileID)

	resp, err := http.Get(getFileURL)
	if err != nil {
		return "", fmt.Errorf("error getting file path: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	var fileResponse TelegramFileResponse
	if err := json.Unmarshal(body, &fileResponse); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	if !fileResponse.Ok || fileResponse.Result.FilePath == "" {
		return "", fmt.Errorf("couldn't retrieve file path for file ID: %s", fileID)
	}

	// Construct the file URL
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.OrganizerBotToken, fileResponse.Result.FilePath)

	return fileURL, nil
}
