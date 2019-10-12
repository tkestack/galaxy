package helper

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/types"
	glog "k8s.io/klog"
	"github.com/google/uuid"
)

var (
	packageName  = "git.code.oa.com/tkestack/galaxy"
	CNI_IFNAME   = "eth0"
	NetNS_PREFIX = "test_"
	NetNS_PATH   = "/var/run/netns"
)

func fillDefaultArgs(root string, args *invoke.Args) *invoke.Args {
	if args.IfName == "" {
		args.IfName = CNI_IFNAME
	}
	args.Path = path.Join(root, "bin")
	return args
}

//export PATH=`pwd`/bin
//ip netns add ctn
// CNI_ARGS="IP=192.168.33.3" CNI_COMMAND="ADD" CNI_CONTAINERID=ctn1 CNI_NETNS=/var/run/netns/ctn CNI_IFNAME=eth0 CNI_PATH=`pwd`/bin galaxy-vlan < /etc/cni/net.d/10-mynet.conf
func ExecCNIWithResult(cniName string, netConfStdin []byte, args *invoke.Args) (types.Result, error) {
	root := ProjectDir()
	pluginPath := path.Join(root, "bin", cniName)
	cniArgs := fillDefaultArgs(root, args)
	glog.V(4).Infof("echo %s | %s %s", compressJson(string(netConfStdin)), strings.Join(cniArgs.AsEnv()[:6], " "), pluginPath)
	return invoke.ExecPluginWithResult(pluginPath, netConfStdin, cniArgs)
}

func compressJson(str string) string {
	for _, s := range []string{" ", "\n", " "} {
		str = strings.Replace(str, s, "", -1)
	}
	return str
}

func ExecCNI(cniName string, netConfStdin []byte, args *invoke.Args) error {
	root := ProjectDir()
	return invoke.ExecPluginWithoutResult(path.Join(root, "bin", cniName), netConfStdin, fillDefaultArgs(root, args))
}

func NewContainerId() string {
	return NetNS_PREFIX + uuid.New().String()
}

func ProjectDir() string {
	gopath := os.Getenv("GOPATH")
	if gopath != "" {
		return path.Join(gopath, "src", packageName)
	}
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	root := filepath.Dir(ex)
	if strings.HasSuffix(root, packageName) {
		return root
	}
	index := strings.LastIndex(root, packageName)
	if index == -1 {
		panic(fmt.Sprintf("current dir %s doesn't under GOPATH", root))
	}
	return root[:(index + len(packageName))]
}

func NewNetNS(containerId string) (string, error) {
	if _, err := Command("ip", "netns", "add", containerId).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to add netns %s: %v", containerId, err)
	}
	return fmt.Sprintf("/var/run/netns/%s", containerId), nil
}

func DelNetNS(containerId string) error {
	if _, err := Command("ip", "netns", "del", containerId).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to del netns %s: %v", containerId, err)
	}
	return nil
}

func Command(cmd string, args ...string) *exec.Cmd {
	DebugLog(append([]string{cmd}, args...)...)
	return exec.Command(cmd, args...)
}

func DebugLog(str ...string) {
	glog.V(4).Info(strings.Join(str, " "))
}

func SetupDummyDev(ifName, cidr string) error {
	if out, err := Command("ip", "link", "add", ifName, "type", "dummy").CombinedOutput(); err != nil {
		if !strings.HasPrefix(string(out), "RTNETLINK answers: File exists") {
			return fmt.Errorf("failed to add link %s: %v, %s", ifName, err, string(out))
		}
	}
	if out, err := Command("ip", "address", "add", cidr, "dev", ifName).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add address %s to %s: %v, %s", cidr, ifName, err, out)
	}
	if out, err := Command("ip", "link", "set", ifName, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set up %s: %v, %s", ifName, err, out)
	}
	return nil
}

func IPInfo(cidr string, vlan uint16) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`[{"ip":"%s","vlan":%d,"gateway":"%s","routable_subnet":"%s"}]`, cidr, vlan, Gateway(*ipNet), ipNet.String()), nil
}

func IPInfos(cidr string, vlan uint16, cidr2 string, vlan2 uint16) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	_, ipNet2, err := net.ParseCIDR(cidr2)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`[{"ip":"%s","vlan":%d,"gateway":"%s","routable_subnet":"%s"},{"ip":"%s","vlan":%d,"gateway":"%s","routable_subnet":"%s"}]`, cidr, vlan, Gateway(*ipNet), ipNet.String(), cidr2, vlan2, Gateway(*ipNet2), ipNet2.String()), nil
}

func Gateway(ipNet net.IPNet) net.IP {
	ip := ipNet.IP.Mask(ipNet.Mask)
	ip[len(ip)-1] = ip[len(ip)-1] + 1
	return ip
}

func CleanupIFace(name ...string) {
	for _, n := range name {
		if _, err := Command("ip", "link", "del", "dev", n).CombinedOutput(); err != nil {
			glog.Warningf("failed to del dev %s: %v", n, err)
		}
	}
}

func CleanupDummy() error {
	if _, err := Command("rmmod", "dummy").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to rmmod dummy: %v", err)
	}
	return nil
}

func CleanupNetNS() {
	filepath.Walk(NetNS_PATH, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			glog.Warning(err)
		}
		if strings.HasPrefix(info.Name(), NetNS_PREFIX) {
			DelNetNS(info.Name())
		}
		return nil
	})
}

func Ping(ip string) ([]byte, error) {
	return Command("ping", "-c", "1", ip).CombinedOutput()
}

func Curl(ip, port string) ([]byte, error) {
	return Command("curl", "--connect-timeout", "5", fmt.Sprintf("%s:%s", ip, port)).CombinedOutput()
}

func CleanupCNIData(networkName string) error {
	return os.RemoveAll(path.Join("/var/lib/cni/networks/", networkName))
}
