package etcd

import (
	"golang.org/x/net/context"
	"gopkg.in/yaml.v1"
	"github.com/spf13/viper"
	"github.com/hashicorp/consul/api"
	"github.com/coreos/etcd/clientv3"
	log "github.com/sirupsen/logrus"
	"fmt"
	"strings"
	"bytes"
	"unicode"
	"net"
	"time"
	"strconv"
	"math/rand"

	//pb "github.com/coreos/etcd/etcdserver/etcdserverpb"
	//mvccpb "github.com/coreos/etcd/internal/mvcc/mvccpb"
	"github.com/hedzr/kvl/kvltool"
	"github.com/hedzr/kvl/store"
)

type KVStoreEtcd struct {
	Client *clientv3.Client
	//Root       string
	e            *Etcdtool
	ctx          context.Context
	cancelFunc   context.CancelFunc
	chGlobalExit chan bool
}

const (
	DIR_TAG          = "etcdv3_dir_$2H#%gRe3*t"
	DEFAULT_ROOT_KEY = "root"

	// etcd service registry root key (under DEFAULT_ROOT_KEY):
	SERVICE_REGISTRY_ROOT = "services"
)

var (
//defaultOpOption clientv3.OpOption
)

// ------------------------------------------------

func (s *KVStoreEtcd) SetDebug(enabled bool) {
	//debug = enabled
}

func (s *KVStoreEtcd) Close() {
	if s.Client != nil {
		v("etcd client was been closing.")
		close(s.chGlobalExit)
		s.Client.Close()
		s.Client = nil
	}
}

func (s *KVStoreEtcd) IsOpen() bool {
	if s.Client != nil {
		return true
	} else {
		return false
	}
}

func (s *KVStoreEtcd) GetRootPrefix() string {
	return s.e.Root
}

func (s *KVStoreEtcd) SetRoot(keyPrefix string) {
	s.e.Root = keyPrefix
}

func (s *KVStoreEtcd) Get(key string) string {
	//ctx, cancel := contextWithCommandTimeout(s.e.CommandTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), s.e.CommandTimeout)
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}
	resp, err := s.Client.KV.Get(ctx, key)
	cancel()
	if err != nil {
		warnf("ERR: %v", err)
		return ""
	}
	for _, ev := range resp.Kvs {
		return string(ev.Value)
	}
	return ""
}

func (s *KVStoreEtcd) GetYaml(key string) interface{} {
	//ctx, cancel := contextWithCommandTimeout(s.e.CommandTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), s.e.CommandTimeout)
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}
	resp, err := s.Client.KV.Get(ctx, key)
	cancel()
	if err != nil {
		warnf("ERR: %v", err)
		return nil
	}

	//// build result
	var ret = make(map[string]interface{})
	for _, ev := range resp.Kvs {
		var v interface{}
		err := yaml.Unmarshal(ev.Value, &v)
		if err != nil {
			warnf("warn yaml unmarshal failed: %v", err)
		} else {
			if ev.Key == nil || strings.EqualFold(string(ev.Key), key) {
				return v
			}
			ret[string(ev.Key)] = v
		}
	}

	// and return the result
	return ret
}

type MyKvPair struct {
	Key1 []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// create_revision is the revision of last creation on this key.
	CreateRevision int64 `protobuf:"varint,2,opt,name=create_revision,json=createRevision,proto3" json:"create_revision,omitempty"`
	// mod_revision is the revision of last modification on this key.
	ModRevision int64 `protobuf:"varint,3,opt,name=mod_revision,json=modRevision,proto3" json:"mod_revision,omitempty"`
	// version is the version of the key. A deletion resets
	// the version to zero and any modification of the key
	// increases its version.
	Version int64 `protobuf:"varint,4,opt,name=version,proto3" json:"version,omitempty"`
	// value is the value held by the key, in bytes.
	Value1 []byte `protobuf:"bytes,5,opt,name=value,proto3" json:"value,omitempty"`
	// lease is the ID of the lease that attached to key.
	// When the attached lease expires, the key will be deleted.
	// If lease is 0, then no lease is attached to the key.
	Lease int64 `protobuf:"varint,6,opt,name=lease,proto3" json:"lease,omitempty"`
}

func (s *MyKvPair) Key() string {
	return string(s.Key1)
}

func (s *MyKvPair) Value() []byte {
	return s.Value1
}

func (s *MyKvPair) ValueString() string {
	return string(s.Value1)
}

type MyKvPairs struct {
	resp *clientv3.GetResponse
}

func (s *MyKvPairs) Count() int {
	return int(s.resp.Count) //len(s.pairs)
}

func (s *MyKvPairs) Item(index int) store.KvPair {
	keyValue := s.resp.Kvs[index]
	pp := MyKvPair{
		keyValue.Key,
		keyValue.CreateRevision,
		keyValue.ModRevision,
		keyValue.Version,
		keyValue.Value,
		keyValue.Lease,
	}
	return &pp
}

func (s *KVStoreEtcd) GetPrefix(keyPrefix string) store.KvPairs {
	//ctx, cancel := contextWithCommandTimeout(s.e.CommandTimeout)
	if len(s.e.Root) > 0 {
		if len(keyPrefix) == 0 {
			keyPrefix = s.e.Root
		} else {
			keyPrefix = fmt.Sprintf("%s/%s", s.e.Root, keyPrefix)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.e.CommandTimeout)
	resp, err := s.Client.KV.Get(ctx, keyPrefix, clientv3.WithPrefix())
	cancel()
	if err != nil {
		warnf("ERR: %v", err)
		return nil
	}
	return &MyKvPairs{resp}
}

func (s *KVStoreEtcd) buildForDirectory(key string) {
	sc := strings.Split(key, "/")
	sc = sc[0 : len(sc)-1]
	for ; len(sc) >= 1; sc = sc[0 : len(sc)-1] {
		dk := strings.Join(sc, "/")
		vf("  ** directory: %s\n", dk)
		if ! strings.EqualFold(DIR_TAG, s.Get(dk)) {
			s.Put(dk, DIR_TAG)
		}
	}
}

func (s *KVStoreEtcd) Put(key string, value string) error {
	key_old := key
	if err := s.PutLite(key, value); err != nil {
		return err
	}
	if ! strings.EqualFold(DIR_TAG, value) {
		// e3w 需要这样的隐含规则，从而形成正确的目录结构。
		// etcd 本身对此过于随意。
		s.buildForDirectory(key_old)
	}
	return nil
}

func (s *KVStoreEtcd) PutLite(key string, value string) error {
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}

	_, err := s.Client.Put(context.TODO(), key, value)
	if err != nil {
		warnf("WARN: %v", err)
		return err
	}
	return nil
}

func (s *KVStoreEtcd) PutTTL(key string, value string, seconds int64) error {
	key_old := key
	if err := s.PutTTLLite(key, value, seconds); err != nil {
		return err
	}
	if ! strings.EqualFold(DIR_TAG, value) {
		// e3w 需要这样的隐含规则，从而形成正确的目录结构。
		// etcd 本身对此过于随意。
		s.buildForDirectory(key_old)
	}
	return nil
}

func (s *KVStoreEtcd) PutTTLLite(key string, value string, seconds int64) error {
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}

	resp, err := s.Client.Grant(context.TODO(), seconds)
	if err != nil {
		warnf("WARN: %v", err)
	}

	_, err = s.Client.Put(context.TODO(), key, value, clientv3.WithLease(resp.ID))
	if err != nil {
		warnf("WARN: %v", err)
		return err
	}
	return nil
}

func (s *KVStoreEtcd) PutYaml(key string, value interface{}) error {
	key_old := key
	if err := s.PutYamlLite(key, value); err != nil {
		return err
	}

	// e3w 需要这样的隐含规则，从而形成正确的目录结构。
	// etcd 本身对此过于随意。
	s.buildForDirectory(key_old)
	return nil
}

func convert(input string) string {
	var buf bytes.Buffer
	for _, r := range input {
		if unicode.IsControl(r) {
			fmt.Fprintf(&buf, "\\u%04X", r)
		} else {
			fmt.Fprintf(&buf, "%c", r)
		}
	}
	return buf.String()
}

func (s *KVStoreEtcd) PutYamlLite(key string, value interface{}) error {
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}
	b, err := yaml.Marshal(value)
	if err != nil {
		warnf("ERR: %v", err)
	}
	str := kvltool.UnescapeUnicode(b)
	_, err = s.Client.Put(context.TODO(), key, str)
	if err != nil {
		warnf("ERR: %v", err)
		return err
	}
	return nil
}

func (s *KVStoreEtcd) Delete(key string) bool {
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}
	resp, err := s.Client.Delete(context.TODO(), key)
	if err != nil {
		warnf("ERR: %v", err)
		return false
	}
	infof("key '%s' was deleted: %v", key, resp)
	return true
}

func (s *KVStoreEtcd) DeletePrefix(keyPrefix string) bool {
	if len(s.e.Root) > 0 {
		if len(keyPrefix) == 0 {
			keyPrefix = s.e.Root
		} else {
			keyPrefix = fmt.Sprintf("%s/%s", s.e.Root, keyPrefix)
		}
	}
	resp, err := s.Client.Delete(context.TODO(), keyPrefix, clientv3.WithPrefix())
	if err != nil {
		warnf("ERR: %v", err)
		return false
	}
	infof("key prefix '%s' was deleted: %v", keyPrefix, resp)
	return true
}

func (s *KVStoreEtcd) Exists(key string) bool {
	// etcd 没有key存在性检测的支持
	return false
}

// Watch 启动一个监视线程。etcd.Watch。
// stopCh被用于阻塞 blockFunc，并接收信号以结束该线程的阻塞。
// stopCh为nil时，阻塞线程失效，而且并不需要其存在，这一特殊场景仅仅适用于etcd的Watch机制。
func (s *KVStoreEtcd) Watch(key string, fn store.WatchFunc, stopCh chan bool) func() {
	if len(s.e.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.e.Root, key)
	}
	rch := s.Client.Watch(context.Background(), key)
	go func() {
		for wresp := range rch {
			for _, ev := range wresp.Events {
				//fmt.Printf("watch event - %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
				//t := int(ev.Type)
				//et := store.Event_EventType(ev.Type)
				fn(store.Event_EventType(ev.Type), ev.Kv.Key, ev.Kv.Value)
			}
		}
		//close(rch)
	}()
	return func() {
		if stopCh != nil {
			<-stopCh
		}
	}
}

func (s *KVStoreEtcd) WatchPrefix(keyPrefix string, fn store.WatchFunc, stopCh chan bool) func() {
	if len(s.e.Root) > 0 {
		if len(keyPrefix) == 0 {
			keyPrefix = s.e.Root
		} else {
			keyPrefix = fmt.Sprintf("%s/%s", s.e.Root, keyPrefix)
		}
	}
	rch := s.Client.Watch(context.Background(), keyPrefix, clientv3.WithPrefix())
	go func() {
		//fmt.Printf("--> WatchPrefix(%s, ...) started.\n", keyPrefix)
		for wresp := range rch {
			//fmt.Printf("--> WatchPrefix(%s, ...) wakeup.\n", keyPrefix)
			for _, ev := range wresp.Events {
				//fmt.Printf("watch event - %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
				//t := int(ev.Type)
				//et := store.Event_EventType(ev.Type)
				fn(store.Event_EventType(ev.Type), ev.Kv.Key, ev.Kv.Value)
			}
		}
		//close(rch)
		//fmt.Printf("--> WatchPrefix(%s, ...) stopped.\n\n", keyPrefix)
	}()
	return func() {
		if stopCh != nil {
			log.Debugf("stopping etcd watcher routine")
			<-stopCh
			log.Debugf("stopped etcd watcher routine")
		}
		//close(rch)
	}
}

func (s *KVStoreEtcd) Register(serviceName string, id string, ipOrHost net.IP, port int, ttl int64, tags []string, meta []string, moreChecks api.AgentServiceChecks) error {
	//ip, port, err := store.LookupHostInfo()
	//if err != nil {
	//	return err
	//}

	keyBase := fmt.Sprintf("%s/%s", SERVICE_REGISTRY_ROOT, serviceName)

	//addr := fmt.Sprintf("%s:%d", ipOrHost.String(), port)
	if len(id) == 0 {
		id = fmt.Sprintf("%s-%s-%d", serviceName, strings.Replace(ipOrHost.String(), ".", "-", -1), port)
	}

	// /root/services/<serviceName>:
	//    /peers/<serviceId>:
	//        addr: <ip:port>
	//        tags:
	//        meta:
	key := fmt.Sprintf("%s/peers/%s", keyBase, id)
	if ttl == 0 {
		ttl = 8
	}

	if err := s.PutYaml(fmt.Sprintf("%s/tags", key), tags); err != nil {
		log.Errorf("ERROR: register as service failed. %v", err)
		return err
	}
	if err := s.PutYaml(fmt.Sprintf("%s/meta", key), meta); err != nil {
		log.Errorf("ERROR: register as service failed. %v", err)
		return err
	}

	key = fmt.Sprintf("%s/addr", key)
	go func() {
		// 在TTL失效阈值之内，完成心跳更新操作。etcd采用TTL机制，无需清除无效的服务注册表项。
		if err := s.PutTTL(key, fmt.Sprintf("%s:%d", ipOrHost.String(), port), ttl+1); err != nil {
			log.Errorf("ERROR: register as service failed. %v", err)
			return
		}
		var foreground = viper.GetBool("server.foreground")
		for ; s.IsOpen(); {
			if err := s.PutTTLLite(key, fmt.Sprintf("%s:%d", ipOrHost.String(), port), ttl+1); err != nil {
				log.Warningf("ERROR: etcd service heartbeat failed. %v", err)
				t := (ttl + 2) / 3
				if t == 0 {
					t = 1
				}
				time.Sleep(time.Second * time.Duration(t))
				continue
			} else {
				// 心跳信息不要记录到日志中，因此这里不使用 logger
				if foreground {
					log.Debugf("HEARTBEAT: etcd service heartbeat ok. per %d seconds.", ttl+1)
				}
				time.Sleep(time.Second * time.Duration(ttl))
			}
		}
	}()

	return nil
}

func (s *KVStoreEtcd) DeregisterAll(serviceName string) error {
	return s.deregister(serviceName, "", net.IPv4zero, 0)
}

func (s *KVStoreEtcd) Deregister(serviceName string, id string, ipOrHost net.IP, port int) error {
	return s.deregister(serviceName, id, ipOrHost, port)
}

func (s *KVStoreEtcd) deregister(serviceName string, id string, ipOrHost net.IP, port int) error {
	if s.IsOpen() {
		all := false

		//addr := fmt.Sprintf("%s:%d", ipOrHost.String(), port)
		if len(id) == 0 {
			if ipOrHost.IsUnspecified() || port == 0 {
				all = true
			} else {
				id = fmt.Sprintf("%s-%s-%d", serviceName, strings.Replace(ipOrHost.String(), ".", "-", -1), port)
			}
		} else {
			if ipOrHost.IsUnspecified() || port == 0 {
				all = true
			}
		}

		// 旧的算法比较ip和port以撤销服务注册；如果要求全部删除则无视ip和port
		// 新的算法已重新实现，比较id就好

		keyPrefix := fmt.Sprintf("%s/%s/peers", SERVICE_REGISTRY_ROOT, serviceName)
		if all {
			s.DeletePrefix(keyPrefix)
		} else {
			key := fmt.Sprintf("%s/%s", keyPrefix, id)
			s.DeletePrefix(key)
		}
		//peers := s.GetPrefix(keyPrefix)
		//for i := 0; i < peers.Count(); i++ {
		//	ip := net.ParseIP(peers.Item(i).Key())
		//	p, _ := strconv.Atoi(peers.Item(i).ValueString())
		//
		//	del := all
		//	if ! all && (ip.Equal(ipOrHost) && port == p) {
		//		del = true
		//	}
		//
		//	if del {
		//		key := fmt.Sprintf("%s/%s", keyPrefix, peers.Item(i).Key())
		//		if ! s.Delete(key) {
		//			log.Warnf("Delete etcd key '%s' failed", key)
		//		}
		//	}
		//}
	}
	return nil
}

// NameResolver etcd 服务发现框架：服务的每个实例注册自己到etcd, 且定时更新自己。
// 注册信息需要带有TTL，通常约定为 5s, 10s, 30s 等值。
func (s *KVStoreEtcd) NameResolver(serviceName string) (net.IP, uint16) {
	if s.IsOpen() {
		keyPrefix := fmt.Sprintf("%s/%s/peers", SERVICE_REGISTRY_ROOT, serviceName)
		peers := s.GetPrefix(keyPrefix)
		// 随机选择算法
		if peers.Count() > 0 {
			sel := rand.Intn(peers.Count())
			ip := net.ParseIP(peers.Item(sel).Key())
			port, err := strconv.Atoi(peers.Item(sel).ValueString())
			if err == nil {
				return ip, uint16(port)
			}
		}
	}
	return net.IPv4zero, 0
}

// NameResolverAll 返回当前的全部可用服务记录列表
func (s *KVStoreEtcd) NameResolverAll(serviceName string) (sr []*store.ServiceRecord) {
	if s.IsOpen() {
		keyPrefix := fmt.Sprintf("%s/%s/peers", SERVICE_REGISTRY_ROOT, serviceName)
		peers := s.GetPrefix(keyPrefix)
		for sel := 0; sel < peers.Count(); sel++ {
			ip := net.ParseIP(peers.Item(sel).Key())
			port, err := strconv.Atoi(peers.Item(sel).ValueString())
			if err == nil {
				// TODO build id for ipv6/ipv4
				id := fmt.Sprintf("%s-%d", ip.String(), port)
				sr = append(sr, &store.ServiceRecord{IP: ip, Port: uint16(port), ID: id})
			}
		}
	}
	return
}
