package veth_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVeth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Veth Suite")
}
