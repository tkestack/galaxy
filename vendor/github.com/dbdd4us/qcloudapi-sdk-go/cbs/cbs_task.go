package cbs

import (
	"context"
	"fmt"
	"time"
)

func (client *Client) CreateCbsStorageTask(args *CreateCbsStorageArgs) ([]string, error) {
	storageIds, err := client.CreateCbsStorage(args)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*180)
	defer cancel()
	ticker := time.NewTicker(TaskCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			args := DescribeCbsStorageArgs{
				StorageIds: &storageIds,
			}
			response, err := client.DescribeCbsStorage(&args)
			if err == nil && len(response.StorageSet) == len(storageIds) {
				return storageIds, nil
			}
		}
	}
}

func (client *Client) AttachCbsStorageTask(storageId string, uInstanceId string) error {

	return WaitUntilDone(
		func() (string, error) {
			rsp, err := client.AttachCbsStorage([]string{storageId}, uInstanceId)
			if err != nil {
				return "", err
			}

			subRsp, ok := rsp.Detail[storageId]
			if ok && subRsp.Code != 0 {
				return "", fmt.Errorf("AttachCbsStorage failed, storageId:%s, uInstanceId:%s, error:%s", storageId, uInstanceId, subRsp.Msg)
			}
			return storageId, nil
		},
		func(info *StorageSet) (bool, error) {
			if info.Attached == 1 && info.UInstanceID != uInstanceId {
				return false, fmt.Errorf("storage %s is attached,but uInstanceId is %s not %s", storageId,
					info.UInstanceID, uInstanceId)
			}
			return info.Attached == 1 && info.UInstanceID == uInstanceId, nil
		},
		client,
	)
}

func (client *Client) DetachCbsStorageTask(storageId string) error {

	return WaitUntilDone(
		func() (string, error) {
			rsp, err := client.DetachCbsStorage([]string{storageId})
			if err != nil {
				return "", err
			}
			subRsp, ok := rsp.Detail[storageId]
			if ok && subRsp.Code != 0 {
				return "", fmt.Errorf("DetachCbsStorage failed, storageId:%s, error:%s", storageId, subRsp.Msg)
			}
			return storageId, nil
		},
		func(info *StorageSet) (bool, error) {
			return info.Attached == 0, nil
		},
		client,
	)
}
