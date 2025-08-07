package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	
	"github.com/modernland/golang-live-tracking/models"
)

type S3Client struct {
	AccessKey string
	SecretKey string
	Region    string
	Bucket    string
	Endpoint  string
}

func NewS3Client(accessKey, secretKey, region, bucket, endpoint string) *S3Client {
	return &S3Client{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    region,
		Bucket:    bucket,
		Endpoint:  endpoint,
	}
}

func (s *S3Client) UploadJSON(key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", s.Endpoint, key)
	
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	// Add AWS signature headers here if needed
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload to S3: %d", resp.StatusCode)
	}

	return nil
}

func (s *S3Client) GetTrainData(key string) (*models.TrainData, error) {
	url := fmt.Sprintf("%s/%s", s.Endpoint, key)
	
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get from S3: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var trainData models.TrainData
	if err := json.Unmarshal(body, &trainData); err != nil {
		return nil, err
	}

	return &trainData, nil
}

func (s *S3Client) DeleteFile(key string) error {
	url := fmt.Sprintf("%s/%s", s.Endpoint, key)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	// Add AWS signature headers here if needed
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (s *S3Client) ListFiles(prefix string) ([]string, error) {
	url := fmt.Sprintf("%s?prefix=%s", s.Endpoint, prefix)
	
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse S3 list response and return file keys
	// This is a simplified implementation
	var files []string
	return files, nil
}