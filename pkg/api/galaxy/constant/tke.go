package constant

const (
	IPPoolAnnotation = "tke.cloud.tencent.com/eni-ip-pool"
	ResourceName     = "tke.cloud.tencent.com/eni-ip"
)

func GetPool(annotations map[string]string) string {
	pool := ""
	if annotations != nil {
		pool = annotations[IPPoolAnnotation]
	}
	return pool
}
