Name: galaxy
Version: %{version}
Release: %{commit}%{?dist}
Summary: kubernetes network cni plugin

License: MIT
Requires: /bin/sh
BuildRequires: pkgconfig(systemd)
Requires: systemd-units
Requires: ebtables
Requires: iputils
Source: %{name}-%{version}.tar.gz

%define __os_install_post %{nil}
%define debug_package %{nil}

%description
kubernetes network cni plugin

%prep
%setup -q

%build
./hack/build.sh

%install
install -d $RPM_BUILD_ROOT/opt/cni/bin/
install -p -m 755 bin/disable-ipv6 $RPM_BUILD_ROOT/opt/cni/bin/disable-ipv6
install -p -m 755 bin/galaxy-veth $RPM_BUILD_ROOT/opt/cni/bin/galaxy-veth
install -p -m 755 bin/galaxy-sdn $RPM_BUILD_ROOT/opt/cni/bin/galaxy-sdn
install -p -m 755 bin/galaxy-k8s-vlan $RPM_BUILD_ROOT/opt/cni/bin/galaxy-k8s-vlan
install -p -m 755 bin/galaxy-k8s-sriov $RPM_BUILD_ROOT/opt/cni/bin/galaxy-k8s-sriov
install -p -m 755 bin/galaxy-bridge $RPM_BUILD_ROOT/opt/cni/bin/galaxy-bridge
install -p -m 755 bin/galaxy-zhiyun-ipam $RPM_BUILD_ROOT/opt/cni/bin/galaxy-zhiyun-ipam
install -p -m 755 bin/loopback $RPM_BUILD_ROOT/opt/cni/bin/loopback
install -p -m 755 bin/host-local $RPM_BUILD_ROOT/opt/cni/bin/host-local
install -p -m 755 tools/qcloud/network_opt $RPM_BUILD_ROOT/opt/cni/bin/network_opt

install -d $RPM_BUILD_ROOT/etc/cni/net.d/
install -p -m 644 hack/config/galaxy.conf $RPM_BUILD_ROOT/etc/cni/net.d/galaxy.conf

install -d $RPM_BUILD_ROOT/%{_bindir}
install -p -m 755 bin/galaxy $RPM_BUILD_ROOT/%{_bindir}

install -d $RPM_BUILD_ROOT/%{_unitdir}
install -p -m 644 hack/config/galaxy.service $RPM_BUILD_ROOT/%{_unitdir}/galaxy.service

install -d $RPM_BUILD_ROOT/etc/sysconfig/
install -p -m 644 hack/config/galaxy-config $RPM_BUILD_ROOT/etc/sysconfig/galaxy-config
install -p -m 644 hack/config/galaxy-ebtable-filter $RPM_BUILD_ROOT/etc/sysconfig/galaxy-ebtable-filter

%files
/opt/cni/bin/disable-ipv6
/opt/cni/bin/galaxy-veth
/opt/cni/bin/galaxy-sdn
/opt/cni/bin/galaxy-k8s-vlan
/opt/cni/bin/galaxy-k8s-sriov
/opt/cni/bin/galaxy-bridge
/opt/cni/bin/galaxy-zhiyun-ipam
/opt/cni/bin/loopback
/opt/cni/bin/host-local
/opt/cni/bin/network_opt
/%{_bindir}/galaxy
/%{_unitdir}/galaxy.service
%config(missingok,noreplace) /etc/sysconfig/galaxy-ebtable-filter

%config(noreplace) /etc/cni/net.d/galaxy.conf
%config(noreplace) /%{_unitdir}/galaxy.service
%config(missingok) /etc/sysconfig/galaxy-config
