package clb

const (
	LoadBalanceListenerProtocolHTTP  = 1
	LoadBalanceListenerProtocolTCP   = 2
	LoadBalanceListenerProtocolUDP   = 3
	LoadBalanceListenerProtocolHTTPS = 4
)

type CreateListenerOpts struct {
	LoadBalancerPort int32   `qcloud_arg:"loadBalancerPort,required"`
	InstancePort     int32   `qcloud_arg:"instancePort,required"`
	Protocol         int     `qcloud_arg:"protocol,required"`
	ListenerName     *string `qcloud_arg:"listenerName"`
	SessionExpire    *int    `qcloud_arg:"sessionExpire"`
	HealthSwitch     *int    `qcloud_arg:"healthSwitch"`
	TimeOut          *int    `qcloud_arg:"timeOut"`
	IntervalTime     *int    `qcloud_arg:"intervalTime"`
	HealthNum        *int    `qcloud_arg:"healthNum"`
	UnhealthNum      *int    `qcloud_arg:"unhealthNum"`
	HttpHash         *int    `qcloud_arg:"httpHash"`
	HttpCode         *int    `qcloud_arg:"httpCode"`
	HttpCheckPath    *string `qcloud_arg:"httpCheckPath"`
	SSLMode          *string `qcloud_arg:"SSLMode"`
	CertId           *string `qcloud_arg:"certId"`
	CertCaId         *string `qcloud_arg:"certCaId"`
	CertCaContent    *string `qcloud_arg:"certCaContent"`
	CertCaName       *string `qcloud_arg:"certCaName"`
	CertContent      *string `qcloud_arg:"certContent"`
	CertKey          *string `qcloud_arg:"certKey"`
	CertName         *string `qcloud_arg:"certName"`
}

type CreateLoadBalancerListenersArgs struct {
	LoadBalancerId string               `qcloud_arg:"loadBalancerId,required"`
	Listeners      []CreateListenerOpts `qcloud_arg:"listeners"`
}

type CreateLoadBalancerListenersResponse struct {
	Response
	RequestId   int      `json:"requestId"`
	ListenerIds []string `json:"listenerIds"`
}

func (response CreateLoadBalancerListenersResponse) Id() int {
	return response.RequestId
}

func (client *Client) CreateLoadBalancerListeners(args *CreateLoadBalancerListenersArgs) (
	*CreateLoadBalancerListenersResponse,
	error,
) {
	response := &CreateLoadBalancerListenersResponse{}
	err := client.Invoke("CreateLoadBalancerListeners", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DescribeLoadBalancerListenersArgs struct {
	LoadBalancerId   string    `qcloud_arg:"loadBalancerId,required"`
	ListenerIds      *[]string `qcloud_arg:"listenerIds"`
	Protocol         *int      `qcloud_arg:"protocol"`
	LoadBalancerPort *int32    `qcloud_arg:"loadBalancerPort"`
	Status           *int      `qcloud_arg:"status"`
}

type DescribeLoadBalancerListenersResponse struct {
	Response
	TotalCount  int        `json:"totalCount"`
	ListenerSet []Listener `json:"listenerSet"`
}

type Listener struct {
	UnListenerId     string `json:"unListenerId"`
	LoadBalancerPort int32  `json:"loadBalancerPort"`
	InstancePort     int32  `json:"instancePort"`
	Protocol         int    `json:"protocol"`
	SessionExpire    int    `json:"sessionExpire"`
	HealthSwitch     int    `json:"healthSwitch"`
	TimeOut          int    `json:"timeOut"`
	IntervalTime     int    `json:"intervalTime"`
	HealthNum        int    `json:"healthNum"`
	UnhealthNum      int    `json:"unhealthNum"`
	HttpHash         string `json:"httpHash"`
	HttpCode         int    `json:"httpCode"`
	HttpCheckPath    string `json:"httpCheckPath"`
	SSLMode          string `json:"SSLMode"`
	CertId           string `json:"certId"`
	CertCaId         string `json:"certCaId"`
	Status           int    `json:"status"`
}

func (client *Client) DescribeLoadBalancerListeners(args *DescribeLoadBalancerListenersArgs) (
	*DescribeLoadBalancerListenersResponse,
	error,
) {
	response := &DescribeLoadBalancerListenersResponse{}
	err := client.Invoke("DescribeLoadBalancerListeners", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DeleteLoadBalancerListenersArgs struct {
	LoadBalancerId string   `qcloud_arg:"loadBalancerId,required"`
	ListenerIds    []string `qcloud_arg:"listenerIds,required"`
}

type DeleteLoadBalancerListenersResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response DeleteLoadBalancerListenersResponse) Id() int {
	return response.RequestId
}

func (client *Client) DeleteLoadBalancerListeners(LoadBalancerId string, ListenerIds []string) (
	*DeleteLoadBalancerListenersResponse,
	error,
) {

	response := &DeleteLoadBalancerListenersResponse{}
	err := client.Invoke("DeleteLoadBalancerListeners", &DeleteLoadBalancerListenersArgs{
		LoadBalancerId: LoadBalancerId,
		ListenerIds:    ListenerIds,
	}, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type ModifyLoadBalancerListenerArgs struct {
	LoadBalancerId string  `qcloud_arg:"loadBalancerId,required"`
	ListenerId     string  `qcloud_arg:"listenerId,required"`
	ListenerName   *string `qcloud_arg:"listenerName"`
	SessionExpire  *int    `qcloud_arg:"sessionExpire"`
	HealthSwitch   *int    `qcloud_arg:"healthSwitch"`
	TimeOut        *int    `qcloud_arg:"timeOut"`
	IntervalTime   *int    `qcloud_arg:"intervalTime"`
	HealthNum      *int    `qcloud_arg:"healthNum"`
	UnhealthNum    *int    `qcloud_arg:"unhealthNum"`
	HttpHash       *int    `qcloud_arg:"httpHash"`
	HttpCode       *int    `qcloud_arg:"httpCode"`
	HttpCheckPath  *string `qcloud_arg:"httpCheckPath"`
	SSLMode        *string `qcloud_arg:"SSLMode"`
	CertId         *string `qcloud_arg:"certId"`
	CertCaId       *string `qcloud_arg:"certCaId"`
	CertCaContent  *string `qcloud_arg:"certCaContent"`
	CertCaName     *string `qcloud_arg:"certCaName"`
	CertContent    *string `qcloud_arg:"certContent"`
	CertKey        *string `qcloud_arg:"certKey"`
	CertName       *string `qcloud_arg:"certName"`
}

type ModifyLoadBalancerListenerResponse struct {
	Response
	RequestId int `json:"requestId"`
}

func (response ModifyLoadBalancerListenerResponse) Id() int {
	return response.RequestId
}

func (client *Client) ModifyLoadBalancerListener(args *ModifyLoadBalancerListenerArgs) (
	*ModifyLoadBalancerListenerResponse,
	error,
) {
	response := &ModifyLoadBalancerListenerResponse{}
	err := client.Invoke("ModifyLoadBalancerListener", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
