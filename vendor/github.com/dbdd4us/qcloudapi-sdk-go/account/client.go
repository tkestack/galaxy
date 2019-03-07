package account

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	AccountHost = "account.api.qcloud.com"
	AccountPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = AccountHost
	}
	if opts.Path == "" {
		opts.Path = AccountPath
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
	region := os.Getenv("QCloudAccountAPIRegion")
	host := os.Getenv("QCloudAccountAPIHost")
	path := os.Getenv("QCloudAccountAPIPath")

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
