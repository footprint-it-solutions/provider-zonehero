package clients


import (
	"context"
	"fmt"
	// "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	// "github.com/aws/aws-sdk-go-v2/config"
	// "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	// smithyhttp "github.com/aws/smithy-go/transport/http"
	ini "gopkg.in/ini.v1"
)


const (
	credentialsDir   = ".hlb"
	credentialsFile  = "credentials"
	defaultSection   = "default"
	expiryDuration   = 15 * time.Minute
	hlbAdminUserRole = "arn:aws:iam::%s:role/hlb/hlb-admin-users-role"
)


type Credentials struct {
	APIKey         string
	XSTSGCIHeaders string
	Expiry         time.Time
	AccountID      string
}


func loadOrCreateCredentials(ctx context.Context, apiKey string, cfg aws.Config, hostname string) (*Credentials, error) {
	var credentials *Credentials
	var accountID string

	credentials, err := loadCredentials(apiKey, cfg.Region)
	if err != nil {
		return nil, err
	}
	if credentials == nil {
		// Get the AWS account ID
		stsClient := sts.NewFromConfig(cfg)
		result, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
		if err != nil {
			return nil, fmt.Errorf("error getting AWS account ID: %v", err)
		}
		accountID = *result.Account

		// Use the provided hostname for initial credentials
		headers, err := generateSTSHeaders(ctx, cfg, accountID, hostname)
		if err != nil {
			return nil, fmt.Errorf("failed to generate STS headers: %w", err)
		}
		credentials = &Credentials{
			APIKey:         apiKey,
			XSTSGCIHeaders: headers,
			Expiry:         time.Now().Add(expiryDuration),
			AccountID:      accountID,
		}
		if err := saveCredentials(credentials, cfg.Region); err != nil {
			return nil, fmt.Errorf("failed to save credentials: %w", err)
		}
	}
	return credentials, nil
}


func loadCredentials(apiKey, region string) (*Credentials, error) {
	credPath := getCredentialsPath()

	cfg, err := ini.Load(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load credentials file: %w", err)
	}

	section := cfg.Section(apiKey)
	headerKey := fmt.Sprintf("%s_x_sts_gci_headers", region)
	expiryKey := fmt.Sprintf("%s_expiry", region)
	if section == nil || section.Key("account_id").String() == "" || section.Key(headerKey).String() == "" {
		return nil, nil
	}

	expiry, _ := time.Parse(time.RFC3339, section.Key(expiryKey).String())
	return &Credentials{
		APIKey:         apiKey,
		XSTSGCIHeaders: section.Key(headerKey).String(),
		Expiry:         expiry,
		AccountID:      section.Key("account_id").String(),
	}, nil
}


func getCredentialsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, credentialsDir, credentialsFile)
}


func generateSTSHeaders(ctx context.Context, cfg aws.Config, accountID string, hostname string) (string, error) {
	assumedSTSClient, err := getSTSClient(ctx, cfg, accountID)
	if err != nil {
		return "", err
	}

	presigner := sts.NewPresignClient(assumedSTSClient)
	presignedURL, err := presigner.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(opts *sts.PresignOptions) {
		opts.ClientOptions = append(opts.ClientOptions, func(stsOptions *sts.Options) {
			// Add the x-hlb-endpoint header
			stsOptions.APIOptions = append(stsOptions.APIOptions, smithyhttp.SetHeaderValue("x-hlb-endpoint", hostname))
		})
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign request: %w", err)
	}

	// Extract query string from presigned URL
	parsedURL, err := url.Parse(presignedURL.URL)
	if err != nil {
		return "", fmt.Errorf("failed to parse presigned URL: %w", err)
	}

	headers := parsedURL.Query().Encode()

	return headers, nil
}


func getSTSClient(ctx context.Context, cfg aws.Config, accountID string) (*sts.Client, error) {
	stsClient := sts.NewFromConfig(cfg)

	// Assume the hlbAdminUserRole
	roleARN := fmt.Sprintf(hlbAdminUserRole, accountID)
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String("HLBTerraformProviderSession"),
	}

	assumeRoleOutput, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to assume role: %w", err)
	}

	assumedCredentialsProvider := credentials.NewStaticCredentialsProvider(
		*assumeRoleOutput.Credentials.AccessKeyId,
		*assumeRoleOutput.Credentials.SecretAccessKey,
		*assumeRoleOutput.Credentials.SessionToken)

	// Create a new config with the assumed role credentials
	assumedConfig, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(assumedCredentialsProvider),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create assumed role config: %w", err)
	}

	// Create a new STS client with the assumed role credentials
	assumedSTSClient := sts.NewFromConfig(assumedConfig)

	return assumedSTSClient, nil
}


func saveCredentials(creds *Credentials, region string) error {
	credPath := getCredentialsPath()
	cfg, err := ini.Load(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = ini.Empty()
		} else {
			return fmt.Errorf("failed to load credentials file: %w", err)
		}
	}

	section, err := cfg.NewSection(creds.APIKey)
	if err != nil {
		return fmt.Errorf("failed to create section in credentials file: %w", err)
	}
	headerKey := fmt.Sprintf("%s_x_sts_gci_headers", region)
	expiryKey := fmt.Sprintf("%s_expiry", region)
	section.NewKey(headerKey, creds.XSTSGCIHeaders)
	section.NewKey(expiryKey, creds.Expiry.Format(time.RFC3339))
	section.NewKey("account_id", creds.AccountID)

	if err := ensureCredentialsDir(); err != nil {
		return err
	}
	if err := cfg.SaveTo(credPath); err != nil {
		return fmt.Errorf("failed to save credentials file: %w", err)
	}

	return nil
}


func getSCDIHeader(ctx context.Context, cfg aws.Config, credentials *Credentials, hostname string) (string, error) {
	if time.Now().After(credentials.Expiry) {
		headers, err := generateSTSHeaders(ctx, cfg, credentials.AccountID, hostname)
		if err != nil {
			return "", fmt.Errorf("failed to generate STS headers: %w", err)
		}

		credentials.XSTSGCIHeaders = headers
		credentials.Expiry = time.Now().Add(expiryDuration)

		if err := saveCredentials(credentials, cfg.Region); err != nil {
			return "", fmt.Errorf("failed to save credentials: %w", err)
		}
	}
	return credentials.XSTSGCIHeaders, nil
}
