package account

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type DescribeProjectArgs struct{}

type DescribeProjectResponse struct {
	Response
	Data []Project `json:"data"`
}

type Project struct {
	ProjectName string `json:"projectName"`
	ProjectId   int    `json:"projectId"`
	CreateTime  string `json:"createTime"`
	CreatorUin  int    `json:"creatorUin"`
	ProjectInfo string `json:"projectInfo"`
}

func (client *Client) DescribeProject(args *DescribeProjectArgs) (*DescribeProjectResponse, error) {
	response := &DescribeProjectResponse{}
	err := client.Invoke("DescribeProject", args, response)
	if err != nil {
		return &DescribeProjectResponse{}, err
	}
	return response, nil
}
