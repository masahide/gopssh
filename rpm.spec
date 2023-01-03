%define _binaries_in_noarch_packages_terminate_build 0

Summary: parallel ssh client
Name:    gopssh
Version: %{_version}
Release: 1%{?dist}
License: MIT
Group:   Applications/System
URL:     https://github.com/masahide/gopssh

BuildRoot: %{_tmppath}/%{name}-root

%description
%{summary}

%prep

%build

%install
%{__rm} -rf %{buildroot}
%{__install} -Dp -m0755 /github/workspace/.bin/%{name} %{buildroot}/usr/local/bin/%{name}

%clean
%{__rm} -rf %{buildroot}

%post
/sbin/chkconfig --add %{name}

%files
%defattr(-,root,root)
/usr/local/bin/%{name}
