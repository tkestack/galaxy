package sts

type Response struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

type AssumeRoleArgs struct {
	RoleArn         string `qcloud_arg:"roleArn"`
	RoleSessionName string `qcloud_arg:"roleSessionName"`
	DurationSeconds *int   `qcloud_arg:"durationSeconds"`
}

type AssumeRoleResponse struct {
	Response
	Data struct {
		Credentials struct {
			SessionToken string `json:"sessionToken"`
			TmpSecretId  string `json:"tmpSecretId"`
			TmpSecretKey string `json:"tmpSecretKey"`
		} `json:"credentials"`
		ExpiredTime int    `json:"expiredTime"`
		Expiration  string `json:"expiration"`
	} `json:"data"`
}

func (client *Client) AssumeRole(args *AssumeRoleArgs) (*AssumeRoleResponse, error) {
	response := &AssumeRoleResponse{}
	err := client.Invoke("AssumeRole", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
