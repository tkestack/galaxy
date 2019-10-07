package tag

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	TagHost = "tag.api.qcloud.com"
	TagPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = TagHost
	}
	if opts.Path == "" {
		opts.Path = TagPath
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
