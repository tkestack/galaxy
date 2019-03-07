package cbs

import ()

const (
	PayModePrePay           = "prePay"
	StorageTypeCloudBasic   = "cloudBasic"
	StorageTypeCloudPremium = "cloudPremium"
	StorageTypeCloudSSD     = "cloudSSD"

	DiskTypeRoot = "root"
	DiskTypeData = "data"

	StorageStatusNormal = "normal"

	RenewFlagAutoRenew               = "NOTIFY_AND_AUTO_RENEW"
	RenewFlagManulRenew              = "NOTIFY_AND_MANUAL_RENEW"
	RenewFlagManulRenewDisableNotify = "DISABLE_NOTIFY_AND_MANUAL_RENEW"
)

type Response struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

type DescribeCbsStorageArgs struct {
	StorageIds   *[]string `qcloud_arg:"storageIds"`
	UInstanceIds *[]string `qcloud_arg:"uInstanceIds"`

	DiskType *string `qcloud_arg:"diskType"`
	Portable *int    `qcloud_arg:"portable"`

	Offset *int `qcloud_arg:"offset"`
	Limit  *int `qcloud_arg:"limit"`
}

type StorageSet struct {
	StorageID       string `json:"storageId"`
	UInstanceID     string `json:"uInstanceId"`
	StorageName     string `json:"storageName"`
	ProjectID       int    `json:"projectId"`
	DiskType        string `json:"diskType"`
	StorageType     string `json:"storageType"`
	StorageStatus   string `json:"storageStatus"`
	ZoneID          int    `json:"zoneId"`
	CreateTime      string `json:"createTime"`
	StorageSize     int    `json:"storageSize"`
	SnapshotAbility int    `json:"snapshotAbility"`
	PayMode         string `json:"payMode"`
	Portable        int    `json:"portable"`
	Attached        int    `json:"attached"`
	DeadlineTime    string `json:"deadlineTime"`
	Rollbacking     int    `json:"rollbacking"`
	RollbackPercent int    `json:"rollbackPercent"`
	Zone            string `json:"zone"`
}

type DescribeCbsStorageResponse struct {
	Response
	TotalCount int          `json:"totalCount"`
	StorageSet []StorageSet `json:"storageSet"`
}

func (client *Client) DescribeCbsStorage(args *DescribeCbsStorageArgs) (*DescribeCbsStorageResponse, error) {
	response := &DescribeCbsStorageResponse{}
	err := client.Invoke("DescribeCbsStorages", args, response)
	if err != nil {
		return &DescribeCbsStorageResponse{}, err
	}
	return response, nil
}

type CreateCbsStorageArgs struct {
	StorageType string `qcloud_arg:"storageType"`
	PayMode     string `qcloud_arg:"payMode"`
	StorageSize int    `qcloud_arg:"storageSize"`

	GoodsNum int    `qcloud_arg:"goodsNum"`
	Period   int    `qcloud_arg:"period"`
	Zone     string `qcloud_arg:"zone"`
}

type CreateCbsStorageResponse struct {
	Response
	StorageIds []string `json:"storageIds"`
}

func (client *Client) CreateCbsStorage(args *CreateCbsStorageArgs) ([]string, error) {
	response := &CreateCbsStorageResponse{}
	err := client.Invoke("CreateCbsStorages", args, response)
	if err != nil {
		return []string{}, err
	}
	return response.StorageIds, nil
}

type AttachCbsStorageArgs struct {
	StorageIds  []string `qcloud_arg:"storageIds"`
	UInstanceId string   `qcloud_arg:"uInstanceId"`
}

type AttachCbsStorageResponse struct {
	Response
	Detail map[string]SubAttachDetachTask `json:"detail"`
}

type SubAttachDetachTask struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (client *Client) AttachCbsStorage(storageIds []string, uInstanceId string) (*AttachCbsStorageResponse, error) {
	args := AttachCbsStorageArgs{
		StorageIds:  storageIds,
		UInstanceId: uInstanceId,
	}
	response := &AttachCbsStorageResponse{}
	err := client.Invoke("AttachCbsStorages", args, response)
	if err != nil {
		return &AttachCbsStorageResponse{}, err
	}
	return response, nil
}

type DetachCbsStorageArgs struct {
	StorageIds []string `qcloud_arg:"storageIds"`
}

type DetachCbsStorageResponse struct {
	Response
	Detail map[string]SubAttachDetachTask `json:"detail"`
}

func (client *Client) DetachCbsStorage(storageIds []string) (*DetachCbsStorageResponse, error) {
	args := &DetachCbsStorageArgs{
		StorageIds: storageIds,
	}
	response := &DetachCbsStorageResponse{}
	err := client.Invoke("DetachCbsStorages", args, response)
	if err != nil {
		return &DetachCbsStorageResponse{}, err
	}
	return response, nil
}

type TerminateCbsStorageArgs struct {
	StorageIds []string `qcloud_arg:"storageIds"`
}

type TerminateCbsStorageResponse struct {
	Response
	DealNames []string `json:"dealNames"`
}

func (client *Client) TerminateCbsStorage(storageIds []string) (*TerminateCbsStorageResponse, error) {
	args := &TerminateCbsStorageArgs{
		StorageIds: storageIds,
	}
	response := &TerminateCbsStorageResponse{}
	err := client.Invoke("TerminateCbsStorages", args, response)
	if err != nil {
		return &TerminateCbsStorageResponse{}, err
	}
	return response, nil
}

type ModifyCbsRenewFlagArgs struct {
	StorageIds []string `qcloud_arg:"storageIds"`
	RenewFlag  string   `qcloud_arg:"renewFlag"`
}

type ModifyCbsRenewFlagResponse struct {
	Response
}

func (client *Client) ModifyCbsRenewFlag(storageIds []string, RenewFlag string) (*ModifyCbsRenewFlagResponse, error) {
	args := &ModifyCbsRenewFlagArgs{
		StorageIds: storageIds,
		RenewFlag:  RenewFlag,
	}
	response := &ModifyCbsRenewFlagResponse{}
	err := client.Invoke("ModifyCbsRenewFlag", args, response)
	if err != nil {
		return &ModifyCbsRenewFlagResponse{}, err
	}
	return response, nil
}

type ModifyCbsStorageAttributeArg struct {
	StorageId   string `qcloud_arg:"storageId"`
	StorageName string `qcloud_arg:"storageName"`
}

type ModifyCbsStorageAttribute struct {
	Response
}

func (client *Client) ModifyCbsStorageAttribute(storageId string, storageName string) (*ModifyCbsStorageAttribute, error) {
	args := &ModifyCbsStorageAttributeArg{
		StorageId:   storageId,
		StorageName: storageName,
	}
	response := &ModifyCbsStorageAttribute{}
	err := client.Invoke("ModifyCbsStorageAttributes", args, response)
	if err != nil {
		return &ModifyCbsStorageAttribute{}, err
	}
	return response, nil
}
