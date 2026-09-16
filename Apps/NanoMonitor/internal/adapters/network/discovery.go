package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

type LAN struct {
	Interface string
	Address   netip.Addr
	Prefix    netip.Prefix
}

func Select(interfaceName string, target netip.Addr) (LAN, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return LAN{}, err
	}
	var candidates []LAN
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 ||
			(interfaceName != "" && iface.Name != interfaceName) {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil || !prefix.Addr().Is4() || !prefix.Addr().IsPrivate() {
				continue
			}
			if target.IsValid() && (!prefix.Contains(target) || prefix.Addr() == target) {
				continue
			}
			candidates = append(candidates, LAN{iface.Name, prefix.Addr(), prefix.Masked()})
		}
	}
	if len(candidates) == 0 {
		return LAN{}, errors.New("같은 대역의 로컬 IPv4 네트워크가 없습니다. Mac과 Nano를 같은 공유기에 연결해 주세요")
	}
	if len(candidates) != 1 {
		var names []string
		for _, candidate := range candidates {
			names = append(names, candidate.Interface)
		}
		return LAN{}, fmt.Errorf("사용 가능한 네트워크가 여러 개입니다. -interface로 지정해 주세요: %s", strings.Join(names, ", "))
	}
	return candidates[0], nil
}

func Hosts(lan LAN) ([]netip.Addr, error) {
	if !lan.Prefix.IsValid() || !lan.Prefix.Addr().Is4() || lan.Prefix.Bits() < 22 || lan.Prefix.Bits() > 30 ||
		!lan.Prefix.Contains(lan.Address) || !lan.Prefix.Addr().IsPrivate() {
		return nil, errors.New("자동 검색은 /22부터 /30까지의 사설 IPv4 대역에서 지원합니다. 더 큰 네트워크에서는 -camera로 IP를 지정해 주세요")
	}
	var hosts []netip.Addr
	for addr := lan.Prefix.Masked().Addr().Next(); lan.Prefix.Contains(addr.Next()); addr = addr.Next() {
		if addr != lan.Address {
			hosts = append(hosts, addr)
		}
	}
	return hosts, nil
}

func Discover(ctx context.Context, lan LAN) ([]netip.Addr, error) {
	hosts, err := Hosts(lan)
	if err != nil {
		return nil, err
	}
	jobs := make(chan netip.Addr)
	var mu sync.Mutex
	var candidates []netip.Addr
	var workers sync.WaitGroup
	for range min(24, len(hosts)) {
		workers.Go(func() {
			dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IP(lan.Address.AsSlice())}, Timeout: 500 * time.Millisecond}
			for address := range jobs {
				conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(address.String(), "7001"))
				if err == nil {
					conn.Close()
					mu.Lock()
					candidates = append(candidates, address)
					mu.Unlock()
				}
			}
		})
	}
	for _, host := range hosts {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- host:
		case <-ctx.Done():
		}
	}
	close(jobs)
	workers.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Less(candidates[j]) })
	return candidates, nil
}

func ParseCamera(value string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(value)
	if err != nil || !ip.Is4() || !ip.IsPrivate() {
		return netip.Addr{}, fmt.Errorf("-camera에는 사설 IPv4 주소를 지정해 주세요: %q", value)
	}
	return ip, nil
}
