package clients


import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	// "os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/hashicorp/go-retryablehttp"
)


const (
	defaultMaxRetries   = 5
	defaultBaseHostname = "hlb.%s.%s.zonehero.cloud"
	defaultBaseURL      = "https://%s/v1"
	defaultPartition    = "aws"
)


type AWSClient struct {
	httpClient  *retryablehttp.Client
	baseURL     string
	hostname    string
	apiKey      string
	partition   string
	awsConfig   aws.Config
	accountID   string
	credentials *Credentials
	debug       bool
}


func NewClient(ctx context.Context, apiKey string, awsConfig aws.Config, partition string) (*AWSClient, error) {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = defaultMaxRetries
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.CheckRetry = customRetryPolicy

	// Disable default debug logging
	retryClient.Logger = nil

	hostname := fmt.Sprintf(defaultBaseHostname, awsConfig.Region, partition)
	credentials, err := loadOrCreateCredentials(ctx, apiKey, awsConfig, hostname)
	if err != nil {
		return nil, err
	}

	if partition == "" {
		partition = defaultPartition
	}

	return &AWSClient{
		httpClient:  retryClient,
		baseURL:     fmt.Sprintf(defaultBaseURL, hostname),
		hostname:    hostname,
		apiKey:      apiKey,
		awsConfig:   awsConfig,
		accountID:   credentials.AccountID,
		credentials: credentials,
		partition:   partition,
		debug:       false,
	}, nil
}


func (c *AWSClient) sendRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	if c.debug {
		log.Printf("[DEBUG] %s %s", method, url)
	}

	XSTSGCIHeaders, err := getSCDIHeader(ctx, c.awsConfig, c.credentials, c.hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API credentials: %w", err)
	}

	var buf io.Reader
	if body != nil {
		payloadBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		buf = bytes.NewBuffer(payloadBytes)
		if c.debug {
			log.Printf("[DEBUG] Request body: %s", string(payloadBytes))
		}
	}

	req, err := retryablehttp.NewRequest(method, url, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("X-Sts-Gci-Headers", XSTSGCIHeaders)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	if c.debug && resp.Body != nil {
		// Read the response body for debug logging
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			log.Printf("[DEBUG] Response body: %s", string(bodyBytes))
			// Create a new reader with the same bytes for the actual response
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	return resp, nil
}


func customRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	// Retry only if the response status code is 429
	if resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	return false, nil
}
