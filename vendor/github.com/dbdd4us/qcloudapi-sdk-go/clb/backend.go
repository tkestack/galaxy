package clb

type RegisterInstancesWithLoadBalancerArgs struct {
	LoadBalancerId string                  `qcloud_arg:"loadBalancerId,required"`
	Backends       []RegisterInstancesOpts `qcloud_arg:"backends,required"`
}

type RegisterInstancesOpts struct {
	InstanceId string `qcloud_arg:"instanceId,required"`
	Weight     *int   `qcloud_arg:"weight"`
}

type RegisterInstancesWithLoadBalancerResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response RegisterInstancesWithLoadBalancerResponse) Id() int {
	return response.RequestId
}

func (client *Client) RegisterInstancesWithLoadBalancer(args *RegisterInstancesWithLoadBalancerArgs) (
	*RegisterInstancesWithLoadBalancerResponse,
	error,
) {
	response := &RegisterInstancesWithLoadBalancerResponse{}
	err := client.Invoke("RegisterInstancesWithLoadBalancer", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DescribeLoadBalancerBackendsArgs struct {
	LoadBalancerId string `qcloud_arg:"loadBalancerId,required"`
	Offset         int    `qcloud_arg:"offset"`
	Limit          int    `qcloud_arg:"limit"`
}

type DescribeLoadBalancerBackendsResponse struct {
	Response
	TotalCount int                    `json:"totalCount"`
	BackendSet []LoadBalancerBackends `json:"backendSet"`
}

type LoadBalancerBackends struct {
	InstanceId     string   `json:"instanceId"`
	UnInstanceId   string   `json:"unInstanceId"`
	Weight         int      `json:"weight"`
	InstanceName   string   `json:"instanceName"`
	LanIp          string   `json:"lanIp"`
	WanIpSet       []string `json:"wanIpSet"`
	InstanceStatus int      `json:"instanceStatus"`
}

func (client *Client) DescribeLoadBalancerBackends(LoadBalancerId string, Offset int, Limit int) (
	*DescribeLoadBalancerBackendsResponse,
	error,
) {
	args := &DescribeLoadBalancerBackendsArgs{
		LoadBalancerId: LoadBalancerId,
		Offset:         Offset,
		Limit:          Limit,
	}
	response := &DescribeLoadBalancerBackendsResponse{}
	err := client.Invoke("DescribeLoadBalancerBackends", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type ModifyLoadBalancerBackendsArgs struct {
	LoadBalancerId string              `qcloud_arg:"loadBalancerId,required"`
	Backends       []ModifyBackendOpts `qcloud_arg:"backends,required"`
}

type ModifyBackendOpts struct {
	InstanceId string `qcloud_arg:"instanceId,required"`
	Weight     int    `qcloud_arg:"weight,required"`
}

type ModifyLoadBalancerBackendsResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response ModifyLoadBalancerBackendsResponse) Id() int {
	return response.RequestId
}

func (client *Client) ModifyLoadBalancerBackends(args *ModifyLoadBalancerBackendsArgs) (
	*ModifyLoadBalancerBackendsResponse,
	error,
) {
	response := &ModifyLoadBalancerBackendsResponse{}
	err := client.Invoke("ModifyLoadBalancerBackends", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DeregisterInstancesFromLoadBalancerArgs struct {
	LoadBalancerId string              `qcloud_arg:"loadBalancerId,required"`
	Backends       []deRegisterBackend `qcloud_arg:"backends,required"`
}

type deRegisterBackend struct {
	InstanceId string `qcloud_arg:"instanceId"`
}

type DeregisterInstancesFromLoadBalancerResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response DeregisterInstancesFromLoadBalancerResponse) Id() int {
	return response.RequestId
}

func (client *Client) DeregisterInstancesFromLoadBalancer(LoadBalancerId string, InstanceIds []string) (
	*DeregisterInstancesFromLoadBalancerResponse,
	error,
) {

	backends := []deRegisterBackend{}
	for _, instanceId := range InstanceIds {
		backends = append(backends, deRegisterBackend{InstanceId: instanceId})
	}

	args := &DeregisterInstancesFromLoadBalancerArgs{
		LoadBalancerId: LoadBalancerId,
		Backends:       backends,
	}
	response := &DeregisterInstancesFromLoadBalancerResponse{}
	err := client.Invoke("DeregisterInstancesFromLoadBalancer", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
