package monitor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	NameSpaceQceDocker = "qce/docker"
	NameSpaceQceCvm    = "qce/cvm"

	QCloudMonitorAPITimeTemplate = "2006-01-02 15:04:05"
)

type QCloudMonitorAPITime struct {
	time.Time
}

func (qmat *QCloudMonitorAPITime) EncodeStructWithPrefix(prefix string, val reflect.Value, v *url.Values) error {
	ret := fmt.Sprintf("%d-%d-%d %d:%d:%d", qmat.Year(), qmat.Month(), qmat.Day(), qmat.Hour(), qmat.Minute(), qmat.Second())
	v.Set(strings.TrimLeft(prefix, "."), ret)
	return nil
}

func (qmat *QCloudMonitorAPITime) MarshalJSON() ([]byte, error) {

	return []byte(
		fmt.Sprintf(
			"%d-%d-%d %d:%d:%d",
			qmat.Year(),
			qmat.Month(),
			qmat.Day(),
			qmat.Hour(),
			qmat.Minute(),
			qmat.Second()),
	), nil
}

func (qmat *QCloudMonitorAPITime) UnmarshalJSON(b []byte) error {
	var tmp string
	if err := json.Unmarshal(b, &tmp); err != nil {
		return nil
	}

	t, err := time.Parse(QCloudMonitorAPITimeTemplate, tmp)
	if err != nil {
		return err
	}
	qmat.Time = t

	return nil
}

type Response struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

type GetMonitorDataArgs struct {
	Namespace  string                `qcloud_arg:"namespace"`
	MetricName string                `qcloud_arg:"metricName"`
	Dimensions []Dimension           `qcloud_arg:"dimensions"`
	Period     *int                  `qcloud_arg:"period,omitempty"`
	StartTime  *QCloudMonitorAPITime `qcloud_arg:"startTime,omitempty"`
	EndTime    *QCloudMonitorAPITime `qcloud_arg:"endTime,omitempty"`
}

type Dimension struct {
	Name  string `qcloud_arg:"name"`
	Value string `qcloud_arg:"value"`
}

type GetMonitorDataResponse struct {
	StartTime  QCloudMonitorAPITime `json:"startTime"`
	EndTime    QCloudMonitorAPITime `json:"endTime"`
	MetricName string               `json:"metricName"`
	Period     int                  `json:"period"`
	DataPoints []*float64            `json:"dataPoints"`
}

type BatchGetMonitorDataArgs struct {
	Namespace  string                `qcloud_arg:"namespace"`
	MetricName string                `qcloud_arg:"metricName"`
	Batch      []Batch               `qcloud_arg:"batch"`
	Period     *int                  `qcloud_arg:"period,omitempty"`
	StartTime  *QCloudMonitorAPITime `qcloud_arg:"startTime,omitempty"`
	EndTime    *QCloudMonitorAPITime `qcloud_arg:"endTime,omitempty"`
}

type Batch struct {
	Dimensions []Dimension `qcloud_arg:"dimensions"`
}

type BatchGetMonitorDataResponse struct {
	StartTime  QCloudMonitorAPITime `json:"startTime"`
	EndTime    QCloudMonitorAPITime `json:"endTime"`
	MetricName string               `json:"metricName"`
	Period     int                  `json:"period"`
	DataPoints map[string][]*float64 `json:"dataPoints"`
}

func (client *Client) GetMonitorData(args *GetMonitorDataArgs) (*GetMonitorDataResponse, error) {
	response := &GetMonitorDataResponse{}
	err := client.Invoke("GetMonitorData", args, response)
	if err != nil {
		return &GetMonitorDataResponse{}, err
	}
	return response, nil
}

func (client *Client) BatchGetMonitorData(args *BatchGetMonitorDataArgs) (*BatchGetMonitorDataResponse, error) {
	response := &BatchGetMonitorDataResponse{}
	err := client.Invoke("GetMonitorData", args, response)
	if err != nil {
		return &BatchGetMonitorDataResponse{}, err
	}
	return response, nil
}
