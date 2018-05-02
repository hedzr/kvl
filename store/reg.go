package store

import (
	"net"
	log "github.com/sirupsen/logrus"
	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
	"github.com/hedzr/kvl/kvltool"
	"os"
	"strings"
)

const (
	DEFAULT_PORT = 1323
)

type (
	Registrar interface {
		// Register id省缺时，通过ip+port自动生成一个; 否则以id为准; 指明了ip时，可以不指定ip和port
		Register(serviceName string, id string, ipOrHost net.IP, port int, ttl int64, tags []string, meta []string, moreChecks api.AgentServiceChecks) error
		// Deregister id省缺时，通过ip+port自动生成一个; 否则以id为准; 指明了ip时，可以不指定ip和port
		Deregister(serviceName string, id string, ipOrHost net.IP, port int) error
		DeregisterAll(serviceName string) error
		NameResolver(serviceName string) (net.IP, uint16)
		NameResolverAll(serviceName string) ([]*ServiceRecord)
	}

	ServiceRecord struct {
		IP   net.IP
		Port uint16
		ID   string
	}
)

// ThisHostname 返回本机名
func ThisHostname() string {
	name, err := os.Hostname()
	if err != nil {
		log.Warnf("WARN: %v", err)
		return "host"
	}
	return name
}

// ThisHost 返回当前服务器的LAN ip，通过本机名进行反向解析
func ThisHost() (ip net.IP) {
	ip = net.IPv4zero
	name, err := os.Hostname()
	if err != nil {
		log.Warnf("WARN: %v", err)
		return
	}
	log.Infof("detected os hostname: %s", name)
	ip, _, _ = hostInfo(name, 0)
	return
}

//LookupHostInfo 依据配置文件的 server.rpc_address 尝试解释正确的rpc地址，通常是IPv4的
func LookupHostInfo() (net.IP, int, error) {
	fAddr := viper.GetString("server.rpc_address")
	fPort := viper.GetInt("server.port")
	if fPort <= 0 || fPort > 65535 {
		fPort = DEFAULT_PORT
	}

	if len(fAddr) == 0 {
		name, err := os.Hostname()
		if err != nil {
			log.Warnf("WARN: %v", err)
			return net.IPv4zero, 0, err
		}
		log.Infof("detected os hostname: %s", name)
		fAddr = name
	}
	return hostInfo(fAddr, fPort)
}

func hostInfo(ipOrHost string, port int) (net.IP, int, error) {
	// macOS 可能会得到错误的主机名
	if strings.EqualFold("bogon", ipOrHost) {
		return findExternalIP(ipOrHost, port)
	}

	ip := net.ParseIP(ipOrHost)
	var savedIP net.IP
	if ip == nil || ip.IsUnspecified() {
		addrs, err := net.LookupHost(ipOrHost)
		if err != nil {
			log.Warnf("Oops: LookupHost(): %v", err)
			return findExternalIP(ipOrHost, port)
		}
		for _, addr := range addrs {
			ip2 := net.ParseIP(addr)
			if ! ip2.IsUnspecified() {
				// 排除回环地址，广播地址
				//if ip2.IsGlobalUnicast() {
				//	continue
				//}
				// // 排除公网地址
				if isLAN(ip2) {
					return ip2, port, nil
				} else {
					savedIP = ip2
				}
			}
		}
		if savedIP != nil {
			log.Warnf("A internet ipaddress found: '%s'; but we need LAN address; keep searching with findExternalIP().", savedIP.String())
		}
		return findExternalIP(ipOrHost, port)
	} else {
		return ip, port, nil
	}
	//return net.IPv4zero, 0, fmt.Errorf("cannot lookup 'server.rpc_address' or 'server.port'. cannot register myself.")
}

func isLAN(ip net.IP) bool {
	if ipv4 := ip.To4(); ipv4 != nil {
		if ipv4[0] == 192 && ipv4[1] == 168 {
			return true
		}
		if ipv4[0] == 172 && ipv4[1] == 16 {
			return true
		}
		if ipv4[0] == 10 {
			return true
		}
	}
	//TODO 识别IPv6的LAN地址段
	return false
}

func findExternalIP(ipOrHost string, port int) (net.IP, int, error) {
	ip, err := kvltool.ExternalIP()
	if err != nil {
		log.Errorf("Oops: ExternalIP(): %v", err)
		return net.IPv4zero, 0, err
	}
	log.Infof("use ip rather than hostname: %s", ip)
	//} else {
	//	// NOTE 此分支尚未测试，由于macOS得到bogon时LookupHost() 必然失败，因此此分支应该是多余的
	//	for _, a := range addrs {
	//		fmt.Println(a)
	//	}
	//}
	return net.ParseIP(ip), port, nil
}
