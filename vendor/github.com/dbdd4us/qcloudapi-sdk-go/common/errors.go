package common

const (
	NoErr         = 0
	NoErrCodeDesc = "Success"

	ErrQCloudGoClient = 9999
)

type LegacyAPIError struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CodeDesc string `json:"codeDesc"`
}

func (lae LegacyAPIError) Error() string {
	return lae.Message
}

type VersionAPIError struct {
	Response struct {
		Error apiErrorResponse `json:"Error"`
	} `json:"Response"`
}

type apiErrorResponse struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (vae VersionAPIError) Error() string {
	return vae.Response.Error.Message
}

type ClientError struct {
	Message string
}

func (ce ClientError) Error() string {
	return ce.Message
}

func makeClientError(err error) ClientError {
	return ClientError{
		Message: err.Error(),
	}
}
