package clb

const (
	LoadBalancerTypePublicNetworkWithDailyRate = 2
	LoadBalancerTypePrivateNetwork             = 3

	LoadBalancerNameMaxLenth = 20
)

type DescribeLoadBalancersArgs struct {
	LoadBalancerIds  *[]string `qcloud_arg:"loadBalancerIds"`
	LoadBalancerType *int      `qcloud_arg:"loadBalancerType"`
	LoadBalancerName *string   `qcloud_arg:"loadBalancerName"`
	Domain           *string   `qcloud_arg:"domain"`
	LoadBalancerVips *[]string `qcloud_arg:"loadBalancerVips"`
	BackendWanIps    *[]string `qcloud_arg:"backendWanIps"`
	Offset           *int      `qcloud_arg:"offset"`
	Limit            *int      `qcloud_arg:"limit"`
	OrderBy          *string   `qcloud_arg:"orderBy"`
	OrderType        *int      `qcloud_arg:"orderType"`
	SearchKey        *string   `qcloud_arg:"searchKey"`
	ProjectId        *int      `qcloud_arg:"projectId"`
	Forward          *int      `qcloud_arg:"forward"`
	WithRs           *int      `qcloud_arg:"withRs"`
}

type DescribeLoadBalancersResponse struct {
	Response
	TotalCount      int            `json:"totalCount"`
	LoadBalancerSet []LoadBalancer `json:"loadBalancerSet"`
}

type LoadBalancer struct {
	LoadBalancerId   string   `json:"loadBalancerId"`
	UnLoadBalancerId string   `json:"unLoadBalancerId"`
	LoadBalancerName string   `json:"loadBalancerName"`
	LoadBalancerType int      `json:"loadBalancerType"`
	Domain           string   `json:"domain"`
	LoadBalancerVips []string `json:"loadBalancerVips"`
	Status           int      `json:"status"`
	Forward          int      `json:"forward"`
	CreateTime       string   `json:"createTime"`
	StatusTime       string   `json:"statusTime"`
	ProjectId        int      `json:"projectId"`
	VpcId            int      `json:"vpcId"`
	SubnetId         int      `json:"subnetId"`
}

func (client *Client) DescribeLoadBalancers(args *DescribeLoadBalancersArgs) (*DescribeLoadBalancersResponse, error) {
	response := &DescribeLoadBalancersResponse{}
	err := client.Invoke("DescribeLoadBalancers", args, response)
	if err != nil {
		return &DescribeLoadBalancersResponse{}, err
	}
	return response, nil
}

type InquiryLBPriceArgs struct {
	LoadBalancerType int `qcloud_arg:"loadBalancerType,required"`
}

type InquiryLBPriceResponse struct {
	Response
	Price int `json:"price"`
}

func (client *Client) InquiryLBPrice(args *InquiryLBPriceArgs) (*InquiryLBPriceResponse, error) {
	response := &InquiryLBPriceResponse{}
	err := client.Invoke("InquiryLBPrice", args, response)
	if err != nil {
		return &InquiryLBPriceResponse{}, err
	}
	return response, nil
}

type CreateLoadBalancerArgs struct {
	LoadBalancerType int     `qcloud_arg:"loadBalancerType,required"`
	Forward          *int    `qcloud_arg:"forward"`
	LoadBalancerName *string `qcloud_arg:"loadBalancerName"`
	DomainPrefix     *string `qcloud_arg:"domainPrefix"`
	VpcId            *string `qcloud_arg:"vpcId"`
	SubnetId         *string `qcloud_arg:"subnetId"`
	ProjectId        *int    `qcloud_arg:"projectId"`
	Number           *int    `qcloud_arg:"number"`
}

type CreateLoadBalancerResponse struct {
	Response
	UnLoadBalancerIds map[string][]string `json:"unLoadBalancerIds"`
	DealIds           []string            `json:"dealIds"`
	RequestId         int                 `json:"requestId"`
}

func (response CreateLoadBalancerResponse) Id() int {
	return response.RequestId
}

func (response CreateLoadBalancerResponse) GetUnLoadBalancerIds() (unlbIds []string) {
	for _, v := range response.UnLoadBalancerIds {
		unlbIds = append(unlbIds, v...)
	}
	return unlbIds
}

func (client *Client) CreateLoadBalancer(args *CreateLoadBalancerArgs) (*CreateLoadBalancerResponse, error) {
	response := &CreateLoadBalancerResponse{}
	err := client.Invoke("CreateLoadBalancer", args, response)
	if err != nil {
		return &CreateLoadBalancerResponse{}, err
	}
	return response, nil
}

type ModifyLoadBalancerAttributesArgs struct {
	LoadBalancerId   string  `qcloud_arg:"loadBalancerId,required"`
	LoadBalancerName *string `qcloud_arg:"loadBalancerName"`
	DomainPrefix     *string `qcloud_arg:"domainPrefix"`
}

type ModifyLoadBalancerAttributesResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response ModifyLoadBalancerAttributesResponse) Id() int {
	return response.RequestId
}

func (client *Client) ModifyLoadBalancerAttributes(args *ModifyLoadBalancerAttributesArgs) (*ModifyLoadBalancerAttributesResponse, error) {
	response := &ModifyLoadBalancerAttributesResponse{}
	err := client.Invoke("ModifyLoadBalancerAttributes", args, response)
	if err != nil {
		return &ModifyLoadBalancerAttributesResponse{}, err
	}
	return response, nil
}

type DeleteLoadBalancersArgs struct {
	LoadBalancerIds []string `qcloud_arg:"loadBalancerIds,required"`
}

type DeleteLoadBalancersResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response DeleteLoadBalancersResponse) Id() int {
	return response.RequestId
}

func (client *Client) DeleteLoadBalancers(loadBalancerIds []string) (*DeleteLoadBalancersResponse, error) {
	args := &DeleteLoadBalancersArgs{
		LoadBalancerIds: loadBalancerIds,
	}
	response := &DeleteLoadBalancersResponse{}
	err := client.Invoke("DeleteLoadBalancers", args, response)
	if err != nil {
		return &DeleteLoadBalancersResponse{}, err
	}
	return response, nil
}

type DescribeLoadBalancersTaskResultArgs struct {
	RequestId int `qcloud_arg:"requestId,required"`
}

func (response DescribeLoadBalancersTaskResultArgs) Id() int {
	return response.RequestId
}

type DescribeLoadBalancersTaskResultResponse struct {
	Response
	Data struct {
		Status int `json:"status"`
	} `json:"data"`
}

func (client *Client) DescribeLoadBalancersTaskResult(taskId int) (*DescribeLoadBalancersTaskResultResponse, error) {
	args := &DescribeLoadBalancersTaskResultArgs{
		RequestId: taskId,
	}
	response := &DescribeLoadBalancersTaskResultResponse{}
	err := client.Invoke("DescribeLoadBalancersTaskResult", args, response)
	if err != nil {
		return &DescribeLoadBalancersTaskResultResponse{}, err
	}
	return response, nil
}
