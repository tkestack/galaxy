package clb

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	CLBHost = "lb.api.qcloud.com"
	CLBPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = CLBHost
	}
	if opts.Path == "" {
		opts.Path = CLBPath
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
	region := os.Getenv("QCloudClbAPIRegion")
	host := os.Getenv("QCloudClbAPIHost")
	path := os.Getenv("QCloudClbAPIPath")

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
