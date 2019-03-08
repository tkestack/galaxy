package ccs

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	CcsHost = "ccs.api.qcloud.com"
	CcsPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = CcsHost
	}
	if opts.Path == "" {
		opts.Path = CcsPath
	}

	client, err := common.NewClient(credential, opts)
	if err != nil {
		return &Client{}, err
	}
	return &Client{client}, nil
}

func NewClientFromEnv() (*Client, error) {

	secretId := os.Getenv("QCloudSecretId")
	secretKey := os.Getenv("QCloudSecretKey")
	region := os.Getenv("QCloudCcsAPIRegion")
	host := os.Getenv("QCloudCcsAPIHost")
	path := os.Getenv("QCloudCcsAPIPath")

	return NewClient(
		common.Credential{
			secretId,
			secretKey,
		},
		common.Opts{
			Region: region,
			Host:   host,
			Path:   path,
		},
	)
}
