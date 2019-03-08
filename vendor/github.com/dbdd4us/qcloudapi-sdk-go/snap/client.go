package snap

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	SnapHost = "snapshot.api.qcloud.com"
	SnaoPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = SnapHost
	}
	if opts.Path == "" {
		opts.Path = SnaoPath
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
	region := os.Getenv("QCloudSnapAPIRegion")
	host := os.Getenv("QCloudSnapAPIHost")
	path := os.Getenv("QCloudSnapAPIPath")

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
