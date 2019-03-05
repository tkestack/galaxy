package cni_request_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCniRequest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CniRequest Suite")
}
