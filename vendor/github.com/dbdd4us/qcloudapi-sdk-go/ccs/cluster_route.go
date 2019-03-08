package ccs

type Response struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

type CreateClusterRouteArgs struct {
	RouteTableName       string `qcloud_arg:"RouteTableName"`
	DestinationCidrBlock string `qcloud_arg:"DestinationCidrBlock"`
	GatewayIp            string `qcloud_arg:"GatewayIp"`
}

type CreateClusterRouteResponse struct {
	Response
}

type DeleteClusterRouteArgs struct {
	RouteTableName       string `qcloud_arg:"RouteTableName"`
	DestinationCidrBlock string `qcloud_arg:"DestinationCidrBlock"`
	GatewayIp            string `qcloud_arg:"GatewayIp"`
}

type DeleteClusterRouteResponse struct {
	Response
}

type DescribeClusterRouteArgs struct {
	RouteTableName string `qcloud_arg:"RouteTableName"`
}

type DescribeClusterRouteResponse struct {
	Response
	Data struct {
		TotalCount int         `json:"TotalCount"`
		RouteSet   []RouteInfo `json:"RouteSet"`
	} `json:"data"`
}

type RouteInfo struct {
	RouteTableName       string `json:"RouteTableName"`
	DestinationCidrBlock string `json:"DestinationCidrBlock"`
	GatewayIp            string `json:"GatewayIp"`
}

func (client *Client) CreateClusterRoute(args *CreateClusterRouteArgs) (*CreateClusterRouteResponse, error) {
	response := &CreateClusterRouteResponse{}
	err := client.Invoke("CreateClusterRoute", args, response)
	if err != nil {
		return &CreateClusterRouteResponse{}, err
	}
	return response, nil
}

func (client *Client) DeleteClusterRoute(args *DeleteClusterRouteArgs) (*DeleteClusterRouteResponse, error) {
	response := &DeleteClusterRouteResponse{}
	err := client.Invoke("DeleteClusterRoute", args, response)
	if err != nil {
		return &DeleteClusterRouteResponse{}, err
	}
	return response, nil
}

func (client *Client) DescribeClusterRoute(args *DescribeClusterRouteArgs) (*DescribeClusterRouteResponse, error) {
	response := &DescribeClusterRouteResponse{}
	err := client.Invoke("DescribeClusterRoute", args, response)
	if err != nil {
		return &DescribeClusterRouteResponse{}, err
	}
	return response, nil
}
