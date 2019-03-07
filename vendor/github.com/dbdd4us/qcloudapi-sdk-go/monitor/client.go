package monitor

import (
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/common"
)

const (
	MonitorHost = "monitor.api.qcloud.com"
	MonitorPath = "/v2/index.php"
)

type Client struct {
	*common.Client
}

func NewClient(credential common.CredentialInterface, opts common.Opts) (*Client, error) {
	if opts.Host == "" {
		opts.Host = MonitorHost
	}
	if opts.Path == "" {
		opts.Path = MonitorPath
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
	region := os.Getenv("QCloudMonitorAPIRegion")
	host := os.Getenv("QCloudMonitorAPIHost")
	path := os.Getenv("QCloudMonitorAPIPath")

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
