package clb

import (
	"context"
	"time"
)

const (
	TaskCheckInterval = time.Second * 1

	TaskSuccceed = 0
	TaskFailed   = 1
	TaskRunning  = 2

	TaskStatusUnknown = 9
)

type Task struct {
	requestId int
}

func NewTask(requestId int) Task {
	return Task{requestId: requestId}
}

//TODO 如果还未执行tiker已经到了呢
//TODO timeout怎么提示
func (task Task) WaitUntilDone(ctx context.Context, client *Client) (int, error) {
	ticker := time.NewTicker(TaskCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return TaskStatusUnknown, ctx.Err()
		case <-ticker.C:
			response, err := client.DescribeLoadBalancersTaskResult(task.requestId)
			if err != nil {
				return TaskStatusUnknown, err
			}
			if response.Data.Status != TaskRunning {
				return response.Data.Status, nil
			}
		}
	}
}

type AsyncTask interface {
	Id() int
}

type CreateFunc func() (AsyncTask, error)

//TODO fix this
func WaitUntilDone(createFunc CreateFunc, client *Client) (int, error) {
	asyncTask, err := createFunc()
	if err != nil {
		return TaskFailed, err
	}

	task := NewTask(asyncTask.Id())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*180)
	defer cancel()
	return task.WaitUntilDone(ctx, client)
}
