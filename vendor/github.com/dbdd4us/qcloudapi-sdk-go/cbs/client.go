package cbs

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	CbsHost = "cbs.api.qcloud.com"
	CbsPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = CbsHost
	}
	if opts.Path == "" {
		opts.Path = CbsPath
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
	region := os.Getenv("QCloudCbsAPIRegion")
	host := os.Getenv("QCloudCbsAPIHost")
	path := os.Getenv("QCloudCbsAPIPath")

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
