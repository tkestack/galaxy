package page

import (
	"strconv"

	"github.com/emicklei/go-restful"
)

type Page struct {
	Content          interface{} `json:"content"`
	Last             bool        `json:"last"`
	TotalElements    int         `json:"totalElements"`
	TotalPages       int         `json:"totalPages"`
	First            bool        `json:"first"`
	NumberOfElements int         `json:"numberOfElements"`
	Size             int         `json:"size"`
	Number           int         `json:"number"`
}

func (Page) SwaggerDoc() map[string]string {
	return map[string]string{
		"last":             "if this is the last page",
		"totalElements":    "total number of elements",
		"totalPages":       "total number of pages",
		"first":            "if this is the first page",
		"numberOfElements": "number of elements in this page",
		"size":             "page size",
		"number":           "page index starting from 0",
	}
}

const (
	DefaultSize = 10
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Pagin(request *restful.Request, len int) (int, int, Page) {
	start, end, size := paginResult(request, len)
	page := pagin(start, end, size, len)
	return start, end, page
}

func Pagination(page, size, len int) (int, int, *Page) {
	start, end, size := paginationResult(page, size, len)
	pagination := pagin(start, end, size, len)
	return start, end, &pagination
}

func paginResult(request *restful.Request, len int) (int, int, int) {
	pageStr := request.QueryParameter("page")
	sizeStr := request.QueryParameter("size")

	return paginationResult(ParsePage(pageStr), ParseSize(sizeStr), len)
}

func paginationResult(page, size, len int) (int, int, int) {
	start := min(page*size, len)
	end := min(start+size, len)
	return start, end, size
}

func ParsePage(pageStr string) int {
	var (
		page = 0
		err  error
	)

	if pageStr != "" {
		page, err = strconv.Atoi(pageStr)
		if err != nil || page < 0 {
			page = 0
		} else if page > 99999 {
			page = 99999
		}
	}

	return page
}

func ParseSize(sizeStr string) int {
	var (
		size = DefaultSize
		err  error
	)

	if sizeStr != "" {
		size, err = strconv.Atoi(sizeStr)
		if err != nil || size <= 0 {
			size = DefaultSize
		} else if size > 9999 {
			size = 9999
		}
	}

	return size
}

func pagin(start, end, size, len int) Page {
	var page = Page{
		Last:             end >= len,
		First:            start == 0,
		TotalElements:    len,
		Size:             size,
		NumberOfElements: end - start,
		TotalPages:       (len + size - 1) / size,
		Number:           start / size,
	}
	return page
}

// PagingParams gets page,size,sort from QueryParameter, default values are 0, 10, "".
// If page < 0, page = 0. If page > 99999, page = 99999.
// If size <= 0, size = 10. If size > 9999, size = 9999.
func PagingParams(req *restful.Request) (string, int, int) {
	return req.QueryParameter("sort"), ParsePage(req.QueryParameter("page")), ParseSize(req.QueryParameter("size"))
}
