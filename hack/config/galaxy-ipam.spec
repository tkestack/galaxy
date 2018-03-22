Name: galaxy-ipam
Version: %{version}
Release: %{commit}%{?dist}
Summary: galaxy-ipam is a kubernetes scheduler extender, it's the master server for galaxy cni network plugin

License: MIT
Requires: /bin/sh
BuildRequires: pkgconfig(systemd)
Requires: systemd-units
Source: %{name}-%{version}.tar.gz

%define __os_install_post %{nil}
%define debug_package %{nil}

%description
galaxy-ipam is a kubernetes scheduler extender, it's the master server for galaxy cni network plugin

%prep
%setup -q

%build
./hack/build-ipam.sh

%install
install -d $RPM_BUILD_ROOT/%{_bindir}
install -p -m 755 bin/galaxy-ipam $RPM_BUILD_ROOT/%{_bindir}

install -d $RPM_BUILD_ROOT/%{_unitdir}
install -p -m 644 hack/config/galaxy-ipam.service $RPM_BUILD_ROOT/%{_unitdir}/galaxy-ipam.service

install -d $RPM_BUILD_ROOT/etc/sysconfig/
install -p -m 644 hack/config/galaxy-ipam.config $RPM_BUILD_ROOT/etc/sysconfig/galaxy-ipam.config
install -p -m 644 hack/config/galaxy-ipam.json $RPM_BUILD_ROOT/etc/sysconfig/galaxy-ipam.json

%files
/%{_bindir}/galaxy-ipam
/%{_unitdir}/galaxy-ipam.service

%config(noreplace) /%{_unitdir}/galaxy-ipam.service
%config(missingok) /etc/sysconfig/galaxy-ipam.config
%config(noreplace) /etc/sysconfig/galaxy-ipam.json
