package kvltool

import (
	//
	"regexp"
	"strconv"
	"unicode/utf8"
	"net"
	"errors"
)

//func Open(r *forwarder.Registrar) store.KVStore {
//	switch s.Source {
//	case "etcd":
//		r.Client = etcd.New(&s.Etcd[s.Env])
//	case "consul":
//	}
//	return r.Client
//}
//
//func Close(r *forwarder.Registrar) {
//	r.Close()
//}

//var reFind = regexp.MustCompile(`^\s*[^\s\:]+\:\s*["']?.*\\u.*["']?\s*$`)
var reFind = regexp.MustCompile(`[^\s\:]+\:\s*["']?.*\\u.*["']?`)
var reFindU = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)

func expandUnicodeInYamlLine(line []byte) []byte {
	// TODO: restrict this to the quoted string value
	return reFindU.ReplaceAllFunc(line, expandUnicodeRune)
}

func expandUnicodeRune(esc []byte) []byte {
	ri, _ := strconv.ParseInt(string(esc[2:]), 16, 32)
	r := rune(ri)
	repr := make([]byte, utf8.RuneLen(r))
	utf8.EncodeRune(repr, r)
	return repr
}

// UnescapeUnicode 解码 \uxxxx 为 unicode 字符; 但是输入的 b 应该是 yaml 格式
func UnescapeUnicode(b []byte) string {
	b = reFind.ReplaceAllFunc(b, expandUnicodeInYamlLine)
	return string(b)
}

// ExternalIP try to find the internet public ip address.
// it works properly in aliyun network.
//
// TODO only available to get IPv4 address.
func ExternalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}
