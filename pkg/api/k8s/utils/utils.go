package utils

import (
	"k8s.io/apimachinery/pkg/api/errors"
)

func ShouldRetry(err error) bool {
	return errors.IsConflict(err) || errors.IsServerTimeout(err)
}
