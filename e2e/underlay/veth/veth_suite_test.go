package veth_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUnderlayVeth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Underlay-Veth Suite")
}
