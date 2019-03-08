package vpc

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type DescribeVpcExArgs struct {
	VpcId          *string `qcloud_arg:"vpcId"`
	VpcName        *string `qcloud_arg:"vpcName"`
	Offset         *int    `qcloud_arg:"offset"`
	Limit          *int    `qcloud_arg:"limit"`
	OrderField     *string `qcloud_arg:"orderField"`
	OrderDirection *string `qcloud_arg:"orderDirection"`
}

type DescribeVpcExResponse struct {
	Response
	TotalCount int   `json:"totalCount"`
	Data       []Vpc `json:"data"`
}

type Vpc struct {
	VpcId          string `json:"vpcId"`
	UnVpcId        string `json:"unVpcId"`
	VpcName        string `json:"vpcName"`
	CidrBlock      string `json:"cidrBlock"`
	SubnetNum      int    `json:"subnetNum"`
	RouteTableNum  int    `json:"routeTableNum"`
	VpnGwNum       int    `json:"vpnGwNum"`
	VpcPeerNum     int    `json:"vpcPeerNum"`
	VpcDeviceNum   int    `json:"vpcDeviceNum"`
	ClassicLinkNum int    `json:"classicLinkNum"`
	VpgNum         int    `json:"vpgNum"`
	NatNum         int    `json:"natNum"`
	CreateTime     string `json:"createTime"`
	IsDefault      bool   `json:"isDefault"`
}

func (client *Client) DescribeVpcEx(args *DescribeVpcExArgs) (*DescribeVpcExResponse, error) {
	response := &DescribeVpcExResponse{}
	err := client.Invoke("DescribeVpcEx", args, response)
	if err != nil {
		return &DescribeVpcExResponse{}, err
	}
	return response, nil
}
