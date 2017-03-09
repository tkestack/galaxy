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

%description
kubernetes network cni plugin

%prep
%setup -q

%build
./hack/build.sh

%install
install -d $RPM_BUILD_ROOT/opt/cni/bin/
install -p -m 755 bin/disable-ipv6 $RPM_BUILD_ROOT/opt/cni/bin/disable-ipv6
install -p -m 755 bin/galaxy-sdn $RPM_BUILD_ROOT/opt/cni/bin/galaxy-sdn
install -p -m 755 bin/galaxy-k8s-vlan $RPM_BUILD_ROOT/opt/cni/bin/galaxy-k8s-vlan
install -p -m 755 bin/galaxy-bridge $RPM_BUILD_ROOT/opt/cni/bin/galaxy-bridge
install -p -m 755 bin/loopback $RPM_BUILD_ROOT/opt/cni/bin/loopback
install -p -m 755 bin/host-local $RPM_BUILD_ROOT/opt/cni/bin/host-local
install -p -m 755 tools/qcloud/network_opt $RPM_BUILD_ROOT/opt/cni/bin/network_opt

install -d $RPM_BUILD_ROOT/etc/cni/net.d/
install -p -m 644 hack/v2/galaxy.conf $RPM_BUILD_ROOT/etc/cni/net.d/galaxy.conf

install -d $RPM_BUILD_ROOT/%{_bindir}
install -p -m 755 bin/galaxy $RPM_BUILD_ROOT/%{_bindir}

install -d $RPM_BUILD_ROOT/%{_unitdir}
install -p -m 644 hack/v2/galaxy.service $RPM_BUILD_ROOT/%{_unitdir}/galaxy.service

install -d $RPM_BUILD_ROOT/etc/sysconfig/
install -p -m 644 hack/v2/galaxy-config $RPM_BUILD_ROOT/etc/sysconfig/galaxy-config

%files
/opt/cni/bin/disable-ipv6
/opt/cni/bin/galaxy-sdn
/opt/cni/bin/galaxy-k8s-vlan
/opt/cni/bin/galaxy-bridge
/opt/cni/bin/loopback
/opt/cni/bin/host-local
/opt/cni/bin/network_opt
/%{_bindir}/galaxy
/%{_unitdir}/galaxy.service

%config(noreplace) /etc/cni/net.d/galaxy.conf
%config(noreplace) /%{_unitdir}/galaxy.service
%config(missingok) /etc/sysconfig/galaxy-config

%define __debug_install_post   \
   %{_rpmconfigdir}/find-debuginfo.sh %{?_find_debuginfo_opts} "%{_builddir}/%{?buildsubdir}"\
%{nil}
