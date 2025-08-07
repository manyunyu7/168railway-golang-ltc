package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/modernland/golang-live-tracking/models"
)

type S3Client struct {
	client   *s3.S3
	bucket   string
}

func NewS3Client(accessKey, secretKey, region, bucket, endpoint string) *S3Client {
	// Create AWS session with custom endpoint
	sess := session.Must(session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true), // Important for custom S3 endpoints
	}))

	return &S3Client{
		client: s3.New(sess),
		bucket: bucket,
	}
}

func (s *S3Client) UploadJSON(key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Use AWS SDK to put object
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(jsonData),
		ContentType: aws.String("application/json"),
	}

	_, err = s.client.PutObject(input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %v", err)
	}

	fmt.Printf("DEBUG: Successfully uploaded %s to S3\n", key)
	return nil
}

func (s *S3Client) GetTrainData(key string) (*models.TrainData, error) {
	// Use AWS SDK to get object
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(input)
	if err != nil {
		return nil, fmt.Errorf("failed to get from S3: %v", err)
	}
	defer result.Body.Close()

	// Read the body
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(result.Body)
	if err != nil {
		return nil, err
	}

	var trainData models.TrainData
	if err := json.Unmarshal(buf.Bytes(), &trainData); err != nil {
		return nil, err
	}

	return &trainData, nil
}

func (s *S3Client) DeleteFile(key string) error {
	// Use AWS SDK to delete object
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(input)
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %v", err)
	}

	fmt.Printf("DEBUG: Successfully deleted %s from S3\n", key)
	return nil
}

func (s *S3Client) ListFiles(prefix string) ([]string, error) {
	// Use AWS SDK to list objects
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	result, err := s.client.ListObjectsV2(input)
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 objects: %v", err)
	}

	var files []string
	for _, obj := range result.Contents {
		files = append(files, *obj.Key)
	}

	return files, nil
}

func (s *S3Client) GetJSONData(key string) (map[string]interface{}, error) {
	// Use AWS SDK to get object
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(input)
	if err != nil {
		return nil, fmt.Errorf("failed to get from S3: %v", err)
	}
	defer result.Body.Close()

	// Read the body
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(result.Body)
	if err != nil {
		return nil, err
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &jsonData); err != nil {
		return nil, err
	}

	return jsonData, nil
}