package sys

import (
	"net"
	"strconv"
	"syscall"
)

func getZoneID(zone string) uint32 {
	nif, err := net.InterfaceByName(zone)
	if err == nil {
		return uint32(nif.Index)
	}
	n, _ := strconv.Atoi(zone)
	return uint32(n)
}

func getZoneName(id int) string {
	nif, err := net.InterfaceByIndex(id)
	if err == nil {
		return nif.Name
	}
	return strconv.Itoa(id)
}

func ResolveSockaddr(address string) (int, syscall.Sockaddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return 0, nil, err
	}
	ip := addr.IP
	if len(ip) == 0 {
		ip = net.IPv4zero
	}
	var (
		af int
		sa syscall.Sockaddr
	)
	if len(ip) == net.IPv4len {
		sa4 := &syscall.SockaddrInet4{Port: addr.Port}
		copy(sa4.Addr[:], ip.To4())
		sa = sa4
		af = syscall.AF_INET
	} else if len(ip) == net.IPv6len {
		sa6 := &syscall.SockaddrInet6{Port: addr.Port}
		copy(sa6.Addr[:], ip.To16())
		sa6.ZoneId = getZoneID(addr.Zone)
		sa = sa6
		af = syscall.AF_INET6
	}
	return af, sa, nil
}

func ToNetAddr(sa syscall.Sockaddr) *net.TCPAddr {
	var addr net.TCPAddr
	switch rsa := sa.(type) {
	case *syscall.SockaddrInet4:
		addr = net.TCPAddr{Port: rsa.Port, IP: rsa.Addr[:]}
	case *syscall.SockaddrInet6:
		addr = net.TCPAddr{Port: rsa.Port, IP: rsa.Addr[:], Zone: getZoneName(int(rsa.ZoneId))}
	default:
		return nil
	}
	return &addr
}

func Fcntl(fd int, cmd int, arg uintptr) (int, error) {
	r, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), arg)
	if errno == 0 {
		return int(r), nil
	}
	return 0, errno
}
