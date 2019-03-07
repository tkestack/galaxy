package vpc

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	VpcHost = "vpc.api.qcloud.com"
	VpcPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = VpcHost
	}
	if opts.Path == "" {
		opts.Path = VpcPath
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
	region := os.Getenv("QCloudVpcAPIRegion")
	host := os.Getenv("QCloudVpcAPIHost")
	path := os.Getenv("QCloudVpcAPIPath")

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
