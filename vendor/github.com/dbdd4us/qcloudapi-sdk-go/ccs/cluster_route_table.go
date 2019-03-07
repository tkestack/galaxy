package ccs

type CreateClusterRouteTableArgs struct {
	RouteTableName            string `qcloud_arg:"RouteTableName"`
	RouteTableCidrBlock       string `qcloud_arg:"RouteTableCidrBlock"`
	VpcId                     string `qcloud_arg:"VpcId"`
	IgnoreClusterCIDRConflict int    `qcloud_arg:"IgnoreClusterCidrConflict"`
}

type CreateClusterRouteTableResponse struct {
	Response
}

type DeleteClusterRouteTableArgs struct {
	RouteTableName string `qcloud_arg:"RouteTableName"`
}

type DeleteClusterRouteTableResponse struct {
	Response
}

type DescribeClusterRouteTableArgs struct {
}

type DescribeClusterRouteTableResponse struct {
	Response
	Data struct {
		TotalCount    int              `json:"TotalCount"`
		RouteTableSet []RouteTableInfo `json:"RouteTableSet"`
	} `json:"data"`
}

type RouteTableInfo struct {
	RouteTableName      string `json:"RouteTableName"`
	RouteTableCidrBlock string `json:"RouteTableCidrBlock"`
	VpcId               string `json:"VpcId"`
}

func (client *Client) CreateClusterRouteTable(args *CreateClusterRouteTableArgs) (*CreateClusterRouteTableResponse, error) {
	response := &CreateClusterRouteTableResponse{}
	err := client.Invoke("CreateClusterRouteTable", args, response)
	if err != nil {
		return &CreateClusterRouteTableResponse{}, err
	}
	return response, nil
}

func (client *Client) DeleteClusterRouteTable(args *DeleteClusterRouteTableArgs) (*DeleteClusterRouteTableResponse, error) {
	response := &DeleteClusterRouteTableResponse{}
	err := client.Invoke("DeleteClusterRouteTable", args, response)
	if err != nil {
		return &DeleteClusterRouteTableResponse{}, err
	}
	return response, nil
}

func (client *Client) DescribeClusterRouteTable(args *DescribeClusterRouteTableArgs) (*DescribeClusterRouteTableResponse, error) {
	response := &DescribeClusterRouteTableResponse{}
	err := client.Invoke("DescribeClusterRouteTable", args, response)
	if err != nil {
		return &DescribeClusterRouteTableResponse{}, err
	}
	return response, nil
}
