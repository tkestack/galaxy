package cbs

import (
	"context"
	"fmt"
	"time"
)

const (
	TaskCheckInterval = time.Second * 1

	TaskSuccceed = 0
	TaskFailed   = 1
	TaskRunning  = 2

	TaskStatusUnknown = 9
)

//TODO 如果还未执行tiker已经到了呢
//TODO timeout怎么提示
func waitStorageUntilDone(ctx context.Context, storage *Storage, check Checker) error {
	ticker := time.NewTicker(TaskCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := storage.GetInfo()
			if err != nil {
				return err
			}
			ok, err := check(info)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}

type Storage struct {
	Client    *Client
	StorageId string
}

func (s Storage) GetInfo() (*StorageSet, error) {
	args := &DescribeCbsStorageArgs{
		StorageIds: &[]string{s.StorageId},
	}
	rsp, err := s.Client.DescribeCbsStorage(args)
	if err != nil {
		return nil, err
	}
	if len(rsp.StorageSet) != 1 {
		return nil, fmt.Errorf("len(rsp.StorageSet) = %d,not equal to 1", len(rsp.StorageSet))
	}
	return &rsp.StorageSet[0], nil

}

type DoFunc func() (string, error)
type Checker func(info *StorageSet) (bool, error)

func NewStorage(storageId string, client *Client) *Storage {
	return &Storage{
		StorageId: storageId,
		Client:    client,
	}
}

func WaitUntilDone(do DoFunc, check Checker, client *Client) error {
	storageId, err := do()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
	defer cancel()
	return waitStorageUntilDone(ctx, NewStorage(storageId, client), check)
}
