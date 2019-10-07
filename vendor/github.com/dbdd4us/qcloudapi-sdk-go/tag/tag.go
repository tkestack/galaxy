package tag

type ModifyResourceTagsArgs struct {
	Resource    string `qcloud_arg:"resource,required"`
	AddTags     *[]Tag `qcloud_arg:"addTags"`
	ReplaceTags *[]Tag `qcloud_arg:"replaceTags"`
	DeleteTags  *[]Tag `qcloud_arg:"deleteTags"`
}

type Tag struct {
	TagKey   *string `qcloud_arg:"tagKey" json:"tagKey"`
	TagValue *string `qcloud_arg:"tagValue" json:"tagValue"`
}
type ModifyResourceTagsArgsResponse struct {
	Response
}

func (client *Client) ModifyResourceTags(args *ModifyResourceTagsArgs) (*ModifyResourceTagsArgsResponse, error) {
	response := &ModifyResourceTagsArgsResponse{}
	err := client.Invoke("ModifyResourceTags", args, response)
	if err != nil {
		return &ModifyResourceTagsArgsResponse{}, err
	}
	return response, nil
}

type GetResourcesByTagsArgs struct {
	CreateUin  *int        `qcloud_arg:"createUin"`
	TagFilters []TagFilter `qcloud_arg:"tagFilters,required"`
	Page       *int        `qcloud_arg:"page"`
	Rp         *int        `qcloud_arg:"rp"`
}

type TagFilter struct {
	TagKey   *string   `qcloud_arg:"tagKey"`
	TagValue *[]string `qcloud_arg:"tagValue"`
}

type GetResourcesByTagsResponse struct {
	Response
	Data struct {
		Total int        `json:"total"`
		Page  int        `json:"page"`
		Rp    int        `json:"rp"`
		Rows  []Resource `json:"rows"`
	} `json:"data"`
}

type Resource struct {
	Region        string        `json:"region"`
	ServiceType   string        `json:"resourceType"`
	ResoucePrefix string        `json:"resourcePrefix"`
	ResourceID    string        `json:"resourceId"`
	Tags          []ResourceTag `json:"tags"`
}

func (client *Client) GetResourcesByTags(args *GetResourcesByTagsArgs) (*GetResourcesByTagsResponse, error) {
	response := &GetResourcesByTagsResponse{}
	err := client.Invoke("GetResourcesByTags", args, response)
	if err != nil {
		return &GetResourcesByTagsResponse{}, err
	}
	return response, nil
}

type GetResourceTagsByResourceIdsArgs struct {
	CreateUin      *string  `qcloud_arg:"createUin"`
	Region         string   `qcloud_arg:"region,required"`
	ServiceType    string   `qcloud_arg:"serviceType,required"`
	ResourcePrefix string   `qcloud_arg:"resourcePrefix,required"`
	ResourceIds    []string `qcloud_arg:"resourceIds,required"`
	Page           *int     `qcloud_arg:"page"`
	Rp             *int     `qcloud_arg:"rp"`
}

type GetResourceTagsByResourceIdsResponse struct {
	Response
	Data struct {
		Total int           `json:"total"`
		Page  int           `json:"page"`
		Rp    int           `json:"rp"`
		Rows  []ResourceTag `json:"rows"`
	} `json:"data"`
}

type ResourceTag struct {
	TagKey     string `json:"tagKey"`
	TagValue   string `json:"tagValue"`
	ResourceId string `json:"resourceId"`
}

func (client *Client) GetResourceTagsByResourceIds(args *GetResourceTagsByResourceIdsArgs) (*GetResourceTagsByResourceIdsResponse, error) {
	response := &GetResourceTagsByResourceIdsResponse{}
	err := client.Invoke("GetResourceTagsByResourceIds", args, response)
	if err != nil {
		return &GetResourceTagsByResourceIdsResponse{}, err
	}
	return response, nil
}
