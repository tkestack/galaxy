package utils

import (
	apierr "k8s.io/client-go/1.4/pkg/api/errors"
)

func ShouldRetry(err error) bool {
	return apierr.IsConflict(err) || apierr.IsServerTimeout(err)
}
