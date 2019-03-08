package sts

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	StsHost = "sts.api.qcloud.com"
	StsPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = StsHost
	}
	if opts.Path == "" {
		opts.Path = StsPath
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
	region := os.Getenv("QCloudStsAPIRegion")
	host := os.Getenv("QCloudStsAPIHost")
	path := os.Getenv("QCloudStsAPIPath")

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
