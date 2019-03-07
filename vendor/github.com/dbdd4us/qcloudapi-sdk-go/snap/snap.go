package snap

type Response struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

type BindAutoSnapshotPolicyArg struct {
	AspId         string   `qcloud_arg:"aspId"`
	StorageIdList []string `qcloud_arg:"storageIdList"`
}

type BindAutoSnapshotPolicyRsp struct {
	Response
}

func (client *Client) BindAutoSnapshotPolicy(aspId string, storageIdList []string) (*BindAutoSnapshotPolicyRsp, error) {
	args := &BindAutoSnapshotPolicyArg{
		AspId:         aspId,
		StorageIdList: storageIdList,
	}
	response := &BindAutoSnapshotPolicyRsp{}
	err := client.Invoke("BindAutoSnapshotPolicy", args, response)
	if err != nil {
		return &BindAutoSnapshotPolicyRsp{}, err
	}
	return response, nil
}
