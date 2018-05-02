package consul

import (
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/watch"
	"gopkg.in/yaml.v2"
	"github.com/spf13/viper"
	log "github.com/sirupsen/logrus"
	"context"
	"fmt"
	"time"
	"strings"
	"net"
	cu "github.com/hedzr/kvl/consul/consul_util"
	"github.com/hedzr/kvl/store"
	"github.com/hedzr/kvl/kvltool"
	"github.com/hedzr/kvl/set"
)

type KVStoreConsul struct {
	Client          *api.Client
	Root            string
	e               *cu.ConsulConfig
	ctx             context.Context
	cancelFunc      context.CancelFunc
	Timeout         time.Duration
	chHeartbeat     chan bool
	chHeartbeatEnd  chan bool
	hearbeatPresent bool
	ServiceID       string
}

type Holder struct {
	Any interface{}
}

const (
	DIR_TAG   = "etcdv3_dir_$2H#%gRe3*t"
	ROOT_KEY  = "root"
	CONSULAPI = "consulapi" //一个包装consul自身的服务，为用户提供对consul自身的服务发现能力
)

var (
	//defaultOpOption clientv3.OpOption
	locksForTTLKey = make(map[string][]*api.Lock)
)

// ------------------------------------------------

func NewReg() store.Registrar {
	kvs := &KVStoreConsul{}
	return kvs
}

func (s *KVStoreConsul) IsOpen() bool {
	if s.Client != nil {
		return true
	} else {
		return false
	}
}

func (s *KVStoreConsul) GetRootPrefix() string {
	return s.Root
}

func (s *KVStoreConsul) SetRoot(keyPrefix string) {
	s.Root = keyPrefix
}

func (s *KVStoreConsul) SetDebug(enabled bool) {
	//debug = enabled
}

//func (s *KVStoreConsul) SetGlobalExitCh(ch chan bool) {
//	s.chGlobalExit = ch
//}

func (s *KVStoreConsul) Get(key string) string {
	//ctx, cancel := contextWithCommandTimeout(s.e.CommandTimeout)
	//ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	kvp, _, err := s.Client.KV().Get(key, nil)
	if err != nil {
		warnf("ERR: %v", err)
		return ""
	}
	if kvp == nil {
		warnf("ERR: key '%s' not exists.", key)
		return ""
	}
	return string(kvp.Value)
}

func (s *KVStoreConsul) GetYaml(key string) interface{} {
	str := s.Get(key)

	var v interface{}
	err := yaml.Unmarshal([]byte(str), &v)
	if err != nil {
		warnf("warn yaml unmarshal failed: %v", err)
	}

	// and return the result
	return v
}

type MyKvPair struct {
	pair *api.KVPair
}

func (s *MyKvPair) Key() string {
	return s.pair.Key
}

func (s *MyKvPair) Value() []byte {
	return s.pair.Value
}

func (s *MyKvPair) ValueString() string {
	return string(s.pair.Value)
}

type MyKvPairs struct {
	pairs api.KVPairs
}

func (s *MyKvPairs) Count() int {
	if s.pairs == nil {
		return 0
	}
	return len(s.pairs)
}

func (s *MyKvPairs) Item(index int) store.KvPair {
	if s.pairs == nil {
		return nil
	}
	p := s.pairs[index]
	pp := MyKvPair{p}
	return &pp
}

func (s *KVStoreConsul) GetPrefix(keyPrefix string) store.KvPairs {
	if len(s.Root) > 0 {
		keyPrefix = fmt.Sprintf("%s/%s", s.Root, keyPrefix)
	}

	kvps, _, err := s.Client.KV().List(keyPrefix, nil)
	if err != nil {
		warnf("ERR: cannot list '%s': %v", keyPrefix, err)
		return &MyKvPairs{}
	}
	//if kvps == nil {
	//	warnf("ERR: key '%s' not exists.", keyPrefix)
	//	return MyKvPairs{}
	//}
	mkps := MyKvPairs{kvps}
	return &mkps
}

func (s *KVStoreConsul) buildForDirectory(key string) {
	// empty body here
}

// Sample: s.Lock("webhook_receiver/1", "something", "10s", )
// TTL 在 10s .. 24h 之间，
func (s *KVStoreConsul) LockObject(key string, value string, sessionTTL string) (*api.Lock) {
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	opts := &api.LockOptions{
		Key:         key,
		Value:       []byte(value),
		SessionName: key + "-session",
		SessionTTL:  sessionTTL,
		//SessionOpts: &api.SessionEntry{
		//	Checks:   []string{"check1", "check2"},
		//	Behavior: "release",
		//	TTL:      sessionTTL,
		//	Name:     key + "-session",
		//},
	}
	lock, err := s.Client.LockOpts(opts)
	if err != nil {
		warnf("CANNOT create consul lock object. %v", err)
		return nil
	}
	return lock
}

func (s *KVStoreConsul) LockObjectDestroy(lock *api.Lock) {
	if lock != nil {
		lock.Destroy()
	}
}

func (s *KVStoreConsul) Acquire(lock *api.Lock,
	withContext func(cancelCtx context.Context),
	doingSth func(*Holder),
	doneFunc func(*Holder),
	doneUnlockedFunc func(*Holder),
	cancelledFunc func(*Holder)) (BlockFunc func()) {

	stopCh := make(chan struct{})
	lockCh, err := lock.Lock(stopCh)
	if err != nil {
		warnf("WARN: lock consul lockObj failed. %v", err)
	}

	cancelCtx, cancelRequest := context.WithCancel(context.Background())

	holder := &Holder{}

	go func() {
		//http.DefaultClient.Do(req)
		doingSth(holder)
		select {
		case <-cancelCtx.Done():
			cancelledFunc(holder)
		default:
			//fmt.Println(" - unlock")
			doneFunc(holder)
			err = lock.Unlock()
			//fmt.Println(" - unlocked")
			if err != nil {
				warnf(" - lock already unlocked")
			}
		}
	}()

	return func() {
		//fmt.Println(" - waiting for lockCh unlock")
		<-lockCh // blocked here!!!

		//fmt.Println(" - lockCh unlocked")
		cancelRequest()

		doneUnlockedFunc(holder)
	}
}

func (s *KVStoreConsul) Unlock(lock *api.Lock) {
	if lock != nil {
		lock.Unlock()
		lock.Destroy()
	}
}

func (s *KVStoreConsul) Put(key string, value string) error {
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

func (s *KVStoreConsul) PutLite(key string, value string) error {
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	_, err := s.Client.KV().Put(&api.KVPair{Key: key, Value: []byte(value)}, nil)
	if err != nil {
		warnf("WARN: %v", err)
		return err
	}
	return nil
}

// TTL 在 10s .. 24h 之间，
// 在 DeleteTTL() 调用之前，即使超时key也不会被K/V Store所删除，注意这一点，这并不是etcd类似的TTL特性。
// 也就是说，除非将隐含的lock对象Unlock了，才会删除对应的key值；
// 同时，也要注意到除非隐含的lock对象被lock住，才会创建对应的key值；
func (s *KVStoreConsul) PutTTL(key string, value string, seconds int64) error {
	lock := s.LockObject(key, value, fmt.Sprintf("%ds", seconds))

	//infof("lock object = %v", lock)
	//infof("time: %s", fmt.Sprintf("%ds", seconds))

	// NOTE 长期运行，locksForTTLKey得不到清理会导致内存满，需要解决该问题
	// 用 DeleteTTL() 显示删除是一个办法
	locksForTTLKey[key] = append(locksForTTLKey[key], lock)

	//stopCh := make(chan struct{})
	//_, err := lock.Lock(stopCh)
	//if err != nil {
	//	warnf("WARN: lock consul lockObj failed. %v", err)
	//}

	return nil
}

func (s *KVStoreConsul) DeleteTTL(key string) bool {
	if a, ok := locksForTTLKey[key]; ok {
		for _, l := range a {
			l.Unlock()
			l.Destroy()
		}
	}

	return s.Delete(key)
}

func (s *KVStoreConsul) PutTTLLite(key string, value string, seconds int64) error {
	return s.PutTTL(key, value, seconds)
}

func (s *KVStoreConsul) PutYaml(key string, value interface{}) error {
	key_old := key
	if err := s.PutYamlLite(key, value); err != nil {
		return err
	}

	// e3w 需要这样的隐含规则，从而形成正确的目录结构。
	// etcd 本身对此过于随意。
	s.buildForDirectory(key_old)
	return nil
}

func (s *KVStoreConsul) PutYamlLite(key string, value interface{}) error {
	//if len(s.Root) > 0 {
	//	key = fmt.Sprintf("%s/%s", s.Root, key)
	//}

	b, err := yaml.Marshal(value)
	if err != nil {
		warnf("ERR: %v", err)
	}

	err = s.PutLite(key, kvltool.UnescapeUnicode(b))
	if err != nil {
		warnf("ERR: %v", err)
		return err
	}
	return nil
}

func (s *KVStoreConsul) Delete(key string) bool {
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	wresp, err := s.Client.KV().Delete(key, nil)
	if err != nil {
		warnf("ERR: %v", err)
		return false
	}
	infof("key '%s' was deleted: %v", key, wresp)
	return true
}

func (s *KVStoreConsul) DeletePrefix(keyPrefix string) bool {
	if len(s.Root) > 0 {
		if len(keyPrefix) == 0 {
			keyPrefix = s.Root
		} else {
			keyPrefix = fmt.Sprintf("%s/%s", s.Root, keyPrefix)
		}
	}

	wresp, err := s.Client.KV().DeleteTree(keyPrefix, nil)
	if err != nil {
		warnf("ERR: %v", err)
		return false
	}
	infof("tree from key '%s' was deleted: %v", keyPrefix, wresp)
	return true
}

func (s *KVStoreConsul) Exists(key string) bool {
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	kvp, _, err := s.Client.KV().Get(key, nil)
	if err != nil {
		return false
	}
	if kvp == nil {
		return false
	}
	return true
}

//func (s *KVStoreConsul) Watch(key string, fn store.WatchFunc) (blockFunc func()) {
//	return nil
//}

// Watch
func (s *KVStoreConsul) Watch(key string, fn store.WatchFunc, stopCh chan bool) (blockFunc func()) {
	if len(s.Root) > 0 {
		key = fmt.Sprintf("%s/%s", s.Root, key)
	}

	// https://www.consul.io/docs/agent/watches.html
	wp, err := watch.Parse(map[string]interface{}{
		"type": "key", //key, keyprefix, services, nodes, service, check, event
		"key":  key,
		//"prefix": keyPrefix,
		//"script": "",
	})
	if err != nil {
		warnf("Error: %s", err)
	}
	_, err = s.Client.Agent().NodeName()
	if err != nil {
		warnf("Error querying Consul agent: %s", err)
	}

	// errExit:
	//	0: false
	//	1: true
	//errExit := 0
	wp.Handler = func(idx uint64, data interface{}) {
		//defer wp.Stop()
		//buf, err := json.MarshalIndent(data, "", "    ")
		//if err != nil {
		//	warnf("Error encoding output: %s", err)
		//	errExit = 1
		//}
		//infof("** changes watched: %v", string(buf))
		if data == nil {
			fn(store.DELETE, []byte(key), nil)
		} else if kvp, ok := data.(*api.KVPair); ok {
			//infof("   original object: %v", kvp)
			fn(store.PUT, []byte(kvp.Key), kvp.Value)
		} else {
			infof("   original object: %v", data)
		}
	}

	//stopCh = make(chan bool)
	go func() {
		// Run the watch
		if err := wp.Run(s.e.Addr); err != nil {
			warnf("Error querying Consul agent: %s", err)
			return
		}
	}()

	return func() {
		//for wresp := range rch {
		//	for _, ev := range wresp.Events {
		//		//fmt.Printf("watch event - %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
		//		//t := int(ev.Type)
		//		//et := store.Event_EventType(ev.Type)
		//		fn(store.Event_EventType(ev.Type), ev.Kv.Key, ev.Kv.Value)
		//	}
		//}
		////close(rch)

		if stopCh != nil {
			<-stopCh
		}
		wp.Stop()
	}
}

// WatchPrefix
func (s *KVStoreConsul) WatchPrefix(keyPrefix string, fn store.WatchFunc, stopCh chan bool) func() {
	if len(s.Root) > 0 {
		if len(keyPrefix) == 0 {
			keyPrefix = s.Root
		} else {
			keyPrefix = fmt.Sprintf("%s/%s", s.Root, keyPrefix)
		}
	}

	// https://www.consul.io/docs/agent/watches.html
	wp, err := watch.Parse(map[string]interface{}{
		"type": "keyprefix", //key, keyprefix, services, nodes, service, check, event
		//"key":  key,
		"prefix": keyPrefix,
		//"script": "",
	})
	if err != nil {
		warnf("Error: %s", err)
	}
	_, err = s.Client.Agent().NodeName()
	if err != nil {
		warnf("Error querying Consul agent: %s", err)
	}

	var savedSate api.KVPairs = nil
	wp.Handler = func(idx uint64, data interface{}) {
		log.Infof("    [watch][handled] idx=%d, data: %v", idx, data)
		if data == nil {
			fn(store.DELETE, []byte(keyPrefix), nil)
		} else if kvp, ok := data.(*api.KVPair); ok {
			//infof("   original object: %v", kvp)
			fn(store.PUT, []byte(kvp.Key), kvp.Value)
		} else if kvps, ok := data.(api.KVPairs); ok {
			if savedSate == nil {
				savedSate = kvps
				log.Debug("    [watch] savedSate == nil, save as it")
				//for _, kvp := range kvps {
				//	fn(store.PUT, []byte(kvp.Key), kvp.Value)
				//}
			} else {
				len1, len2 := len(kvps), len(savedSate)
				len := len1
				if len < len2 {
					len = len2
				}
				for i := 0; i < len; i++ {
					var kvp1, kvp2 *api.KVPair = nil, nil
					if i < len1 {
						kvp1 = kvps[i]
					}
					if i < len2 {
						kvp2 = savedSate[i]
					}

					checked := false
					if kvp1 != nil && kvp2 == nil {
						// 这种情况应该很少见，有必要注意一下
						log.Warnf("    [watch] [checking] kvp: %d, %d, saved: nil.", kvp1.CreateIndex, kvp1.ModifyIndex)
						checked = true
					} else if kvp1 != nil && kvp2 != nil {
						//log.Debugf("    [watch] [checking] kvp: %d, %d, saved: %d, %d.", kvp1.CreateIndex, kvp1.ModifyIndex, kvp2.CreateIndex, kvp2.ModifyIndex)
						if kvp1.ModifyIndex != kvp1.CreateIndex && kvp1.ModifyIndex != kvp2.ModifyIndex {
							//log.Debug("    checked")
							checked = true
						}
					}
					if checked {
						//log.Debug("    => fn(...PUT)")
						fn(store.PUT, []byte(kvp1.Key), kvp1.Value)
					}
				}
				savedSate = kvps
			}
		} else {
			log.Infof("    [watch] original object: %v", data)
		}
	}

	//stopCh = make(chan bool)
	go func() {
		// Run the watch and block here
		// 当 wp.Stop() 或者相关的 client 结束时，wp.Run() 将会返回。
		log.Infof("  - Watching k/v keyPrefix: %s", keyPrefix)
		if err := wp.Run(s.e.Addr); err != nil {
			warnf("Error querying Consul agent: %s", err)
			return
		}
		log.Infof("end of watching k/v keyPrefix: %s", keyPrefix)
	}()

	return func() {
		if stopCh != nil {
			<-stopCh
		}
		wp.Stop()
	}
}

func (s *KVStoreConsul) QueryService(name string) ([]*api.CatalogService, error) {
	//metaQ := map[string]string{"Name": name}
	services, meta, err := s.Client.Catalog().Service(name, "", nil) //&api.QueryOptions{NodeMeta: metaQ})
	if err != nil {
		return nil, err
	}

	if meta.LastIndex == 0 {
		return nil, fmt.Errorf("Bad: %v", meta)
	}

	return services, nil
}

func (s *KVStoreConsul) Register(serviceName string, id string, ipOrHost net.IP, port int, ttl int64, tags []string, meta []string, moreChecks api.AgentServiceChecks) error {
	//sss, err := s.QueryService(serviceName)
	//count := 0
	//if err != nil {
	//	log.Fatalf("REGISTER ERROR::: CANNOT Query Services : %v", err)
	//	return err
	//}
	//count = len(sss)

	interval := fmt.Sprintf("%ds", ttl)
	addr := fmt.Sprintf("%s:%d", ipOrHost.String(), port)
	if len(id) == 0 {
		id = fmt.Sprintf("%s-%s-%d", serviceName, strings.Replace(ipOrHost.String(), ".", "-", -1), port)
	}

	//check service exists?
	agentService, err := cu.QueryServiceByID(id, s.Client)
	if err == nil && agentService != nil {
		//if exists, don't do register itself again.
		if strings.EqualFold(agentService.ID, id) {
			return nil
		}
	}

	checks := api.AgentServiceChecks{
		&api.AgentServiceCheck{
			// Script + Interval
			// HTTP + Interval
			// TCP + Interval
			// Time to Live (TTL)
			// Docker + Interval

			// todo TTL check and polling TTL go routine

			//Interval: interval,
			//TCP:      addr,
			TTL:   interval,
			Notes: "TTL Health Check for " + id,

			//Script            string `json:",omitempty"` // 脚本
			//DockerContainerID string `json:",omitempty"`
			//Shell             string `json:",omitempty"` // Only supported for Docker.
			//Interval          string `json:",omitempty"`
			//Timeout           string `json:",omitempty"`
			//TTL               string `json:",omitempty"`
			//HTTP              string `json:",omitempty"`
			//TCP               string `json:",omitempty"`
			//Status            string `json:",omitempty"`
			//Notes             string `json:",omitempty"`
			//TLSSkipVerify     bool   `json:",omitempty"`
			//
			// In Consul 0.7 and later, checks that are associated with a service
			// may also contain this optional DeregisterCriticalServiceAfter field,
			// which is a timeout in the same Go time format as Interval and TTL. If
			// a check is in the critical state for more than this configured value,
			// then its associated service (and all of its associated checks) will
			// automatically be deregistered.
			DeregisterCriticalServiceAfter: s.e.GetDeregisterCriticalServiceAfter(),
		},
	}
	if moreChecks != nil {
		checks = append(checks, moreChecks...)
	}

	if err := s.Client.Agent().ServiceRegister(&api.AgentServiceRegistration{
		ID:                id,
		Name:              serviceName,
		Tags:              tags,
		Port:              port,
		Address:           ipOrHost.String(),
		EnableTagOverride: true,
		Checks:            checks,
	}); err != nil {
		log.Fatalf("REGISTER ERROR::: %v", err)
		return err
	}
	if len(s.ServiceID) == 0 {
		s.ServiceID = id
		viper.Set("server.serviceId", id)
	}
	log.Infof("REGISTER OK. '%s' at '%s'", id, addr)

	// TTL 心跳 / 撤销注册
	go s.heartbeatRoutine(serviceName, id, addr, ipOrHost, port, ttl, tags, meta, moreChecks)
	return nil
}

// heartbeatRoutine 实现TTL心跳以更新服务的注册信息，确认其健康性
// 当服务停止时，自动撤销注册信息
// 注意，即使服务意外终止未能撤销注册信息，其信息也会在ttl时限内成为critical
// 状态，并随后自动被consul所删除，但是，这一特性需要 consul 0.7+以上版本才
// 能支持。
// s.chHeartbeat 策略仅允许一个app中一次注册，如果注册多个服务，则撤销注册
// 的go routine无法被全部释放
func (s *KVStoreConsul) heartbeatRoutine(serviceName string, serviceID string, addr string, ipOrHost net.IP, port int, ttl int64, tags []string, meta []string, moreChecks api.AgentServiceChecks) {
	s.hearbeatPresent = true
	shouldBeBreak := false
	ok := false
	// 或者直接在select中使用case <-time.After(time.Second * 10)
	heartbeat := time.Tick(time.Duration(ttl) * time.Second)
	checkId := "service:" + serviceID
	log.Debug("      [heartbeat] AFTER REGISTER OK, WAITING FOR APP EXIT.", s.chHeartbeat)
	for ; !shouldBeBreak; {
		select {
		case shouldBeBreak, ok = <-s.chHeartbeat:
			// 撤销注册
			if err := s.Deregister(serviceName, serviceID, ipOrHost, port); err != nil {
				log.Errorf("      [heartbeat] ERROR: %v", err)
			} else {
				log.Infof("      [heartbeat] deregister ok: '%s' (%s). [ok=%v][%v]", serviceName, addr, ok, shouldBeBreak)
			}
			//log.Info("   routine return")
		case <-heartbeat:
			vf("[heartbeat] TTL update")
			if err := s.Client.Agent().UpdateTTL(checkId, "OK", api.HealthPassing); err != nil {
				log.Warnf("      [heartbeat] TTL update failed. ERR: %v", err)
				// 尝试再次注册自己，如果是因为服务注册信息以外遭到破坏的话
				s.Register(serviceName, serviceID, ipOrHost, port, ttl, tags, meta, moreChecks)
			}
		}
	}
	s.chHeartbeatEnd <- true
	log.Infof("      [heartbeat] EXITED: stop ttl for: '%s' (%s).", serviceName, addr)
}

func (s *KVStoreConsul) Close() {
	if s.Client != nil {
		log.Info("      close heartbeat routine. ", s.chHeartbeat)
		if s.hearbeatPresent {
			s.chHeartbeat <- true
			time.Sleep(100 * time.Microsecond)
		}

		log.Info("      close watchers")
		for _, a := range locksForTTLKey {
			for _, l := range a {
				l.Unlock()
				l.Destroy()
			}
		}

		if s.hearbeatPresent {
			keep := true
			for ; keep; {
				select {
				case o, ok := <-s.chHeartbeatEnd:
					log.Infof("      heartbeat routine exited.%v,%v", o, ok)
					keep = false
				case <-time.After(time.Second * 1):
					log.Info("      -> 1s")
				}
			}
		}
		close(s.chHeartbeat)
		close(s.chHeartbeatEnd)
		s.Client = nil
	}
}

func (s *KVStoreConsul) DeregisterAll(serviceName string) error {
	return s.deregister(serviceName, "", net.IPv4zero, 0)
}

func (s *KVStoreConsul) Deregister(serviceName string, id string, ipOrHost net.IP, port int) error {
	return s.deregister(serviceName, id, ipOrHost, port)
}

type OpFunc func(c *api.Client, clientIP net.IP, clientPort int) error

func (s *KVStoreConsul) opThatClient(opFunc OpFunc) (*api.Client, error) {
	catalog := s.Client.Catalog()
	ss, meta, err := catalog.Service(CONSULAPI, "", nil)
	if err != nil {
		log.Warnf("'%s' cannot be found::: %v", CONSULAPI, err)
		if opFunc != nil {
			err = opFunc(s.Client, net.IPv4zero, 0)
		}
		return s.Client, err
	}
	vf("QueryMeta: %v", meta)

	for _, s := range ss {
		addr := s.Address
		if addr == "" {
			addr = s.ServiceAddress
		}

		clientAddr := fmt.Sprintf("%s:%d", addr, s.ServicePort)
		// todo cache this store
		store1 := New(&cu.ConsulConfig{
			Addr: clientAddr,
		})

		if store1.IsOpen() && opFunc != nil {
			ip := net.ParseIP(addr)
			if cc, ok := store1.(*KVStoreConsul); ok {
				err = opFunc(cc.Client, ip, s.ServicePort)
				return cc.Client, err
			}
		}
	}
	return nil, fmt.Errorf("no avaliable consul client found.")
}

func (s *KVStoreConsul) deregister(serviceName string, id string, ipOrHost net.IP, port int) error {
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

	// 以下，旧的算法比较ip和port以撤销服务注册；如果要求全部删除则无视ip和port
	// 新的算法待重新实现，比较id就好

	// 直接在当前client/agent中撤销注册
	if all == false {
		log.Info("   deregistering directly")
		err := deregisterServiceFast(s.Client, serviceName, all, ipOrHost, port)
		if err != nil {
			log.Warnf("deregister '%s' at this agent failed. ERR: %v", serviceName, err)
		} else {
			log.Info("   deregistering ok")
			return nil
		}
	}

	// 试图在集群中找到正确的agent并撤销注册
	// 如果集群中多个agents都有该服务的实例，并且 all = true，则依次全部撤销
	_, err := s.opThatClient(func(c *api.Client, clientIP net.IP, clientPort int) error {
		err := deregisterService(s.Client, serviceName, all, ipOrHost, port)
		if err != nil {
			log.Warnf("deregister '%s' failed. ERR: %v", serviceName, err)
			return err
		}
		return nil
	})
	return err

	//catalog := s.Client.Catalog()
	//ss, meta, err := catalog.Service(CONSULAPI, "", nil)
	//if err != nil {
	//	log.Warnf("'%s' cannot be found::: %v", CONSULAPI, err)
	//	err := deregisterService(s.Client, serviceName, false, ipOrHost, port)
	//	if err != nil {
	//		log.Warnf("deregister '%s' failed. ERR: %v", serviceName, err)
	//		//return err
	//	}
	//	return err
	//}
	//vf("QueryMeta: %v", meta)
	////fmt.Printf("#### AC: %v\n", ac)
	//
	//for _, s := range ss {
	//	vf("service '%s' found: %s, %s, %d\n", CONSULAPI, s.Address, s.ServiceAddress, s.ServicePort)
	//	addr := s.Address
	//	if addr == "" {
	//		addr = s.ServiceAddress
	//	}
	//
	//	clientAddr := fmt.Sprintf("%s:%d", addr, s.ServicePort)
	//	// todo cache this store
	//	store1 := New(&cli_common.ConsulConfig{
	//		Addr: clientAddr,
	//	})
	//
	//	if store1.IsOpen() {
	//		all := false
	//		if ipOrHost.IsUnspecified() || port == 0 {
	//			all = true
	//		}
	//		if cc, ok := store1.(*KVStoreConsul); ok {
	//			//del := all
	//			//if ! all {
	//			//	services, _ := cc.Client.Agent().Services()
	//			//	for key := range services {
	//			//		if strings.EqualFold(key, serviceName) {
	//			//			del = true
	//			//		}
	//			//	}
	//			//}
	//			//
	//			//if del {
	//				err := deregisterService(cc.Client, serviceName, all, ipOrHost, port)
	//				if err != nil {
	//					log.Warnf("deregister '%s' on '%s' failed. ERR: %v", serviceName, clientAddr, err)
	//					//return err
	//				}
	//			//}
	//		}
	//	}
	//}
	////fmt.Println("")
	//return nil
}

func deregisterServiceFast(cc *api.Client, serviceName string, all bool, ipOrHost net.IP, port int) (err error) {
	var err1 error
	cu.WaitForResult(func() (bool, error) {
		cn, err := cc.Agent().Services()
		if err != nil {
			return false, err
		}

		for id, s := range cn {
			if strings.EqualFold(s.Service, serviceName) {
				err1 = cc.Agent().ServiceDeregister(id)
				return true, err1
			}
		}

		return false, fmt.Errorf("Bad: cannot found service '%s'", serviceName)
	}, func(err error) {
		log.Fatal(fmt.Errorf("err: %v", err))
	})

	return err1
}

// DeregisterService deregister this ms via a consul agent
func deregisterService(cc *api.Client, serviceName string, all bool, ipOrHost net.IP, port int) (err error) {
	catalog := cc.Catalog()
	services, meta, err := catalog.Service(serviceName, "", nil)
	if err != nil {
		log.Fatalf("ERROR querying service '%s'::: %v", serviceName, err)
		return err
	}
	vf("QueryMeta(Catalog): %v", meta)
	if set.VerboseVVV {
		log.Print("Catalog: [")
		for _, s := range services {
			log.Debugf("  - %v", s.ServiceID)
		}
		log.Print("]")
	}

	// tag: 用于查询服务的Tags
	sssPassing, meta, err := cc.Health().Service(serviceName, "", true, &api.QueryOptions{
		AllowStale: true,
	})
	if err != nil {
		log.Fatalf("ERROR querying service '%s'::: %v", serviceName, err)
		return err
	}
	vf("QueryMeta(Health): %v", meta)

	var sPassing []*api.ServiceEntry
	if ! all {
		for _, s := range sssPassing {
			if s.Checks.AggregatedStatus() == api.HealthPassing {
				sPassing = append(sPassing, s)
			}
		}
		if set.VerboseVVV {
			log.Print("Passing: [")
			for _, s := range sPassing {
				if s.Checks.AggregatedStatus() == api.HealthPassing {
					log.Debugf("  - %v", s.Service.ID)
				}
			}
			log.Printf("].\nAll: %v", all)
		}
	}

	for _, s := range services {
		del := all

		if !cu.Contains(sPassing, s) {
			del = true
		}
		if ! all {
			if strings.EqualFold(s.ServiceAddress, ipOrHost.String()) && s.ServicePort == port {
				del = true
			}
		}

		if del {
			err := cc.Agent().ServiceDeregister(s.ServiceID)
			if err != nil {
				log.Fatalf("ERROR deregistering::: %v", err)
				return err
			}
			log.Infof("Service Deregistered: %s, %s", serviceName, s.ServiceID)
		}
	}
	return nil
}

// NameResolver 返回第一个可用服务的 SVR 记录
// return net.IPv4zero, 0    means there are no more targets to be found
func (s *KVStoreConsul) NameResolver(serviceName string) (net.IP, uint16) {
	//rc := s.Consul[s.Env]
	//log.Infof("rc = %v", rc)

	services, err := cu.QueryService(serviceName, s.Client.Catalog())
	//WaitForResult(func() (bool, error) {
	//	//
	//})

	if err == nil && services != nil {
		for _, service := range services {
			addr, err := net.LookupIP(service.ServiceAddress)
			if err == nil && len(addr) > 0 {
				//log.Debugf("   got ip array: %v", addr)
				for _, ip := range addr {
					if !ip.IsGlobalUnicast() {
						log.Warnf("   IsGlobalUnicast('%v') == false, skip to next", ip)
						continue
					}
					log.Debugf("   return ip, port: %v, %v", ip, service.ServicePort)
					return ip, uint16(service.ServicePort)
				}
			} else {
				log.Errorf("NameResolver failed: lookup ip failed: %v", err)
			}
		}
	} else {
		log.Errorf("NameResolver failed: queryService failed: %v", err)
	}
	return net.IPv4zero, 0
}

// NameResolverAll 返回当前的全部可用服务记录列表
func (s *KVStoreConsul) NameResolverAll(serviceName string) (sr []*store.ServiceRecord) {
	//rc := s.Consul[s.Env]
	//log.Infof("rc = %v", rc)

	services, err := cu.QueryService(serviceName, s.Client.Catalog())
	if err == nil && services != nil {
		for _, service := range services {
			addr, err := net.LookupIP(service.ServiceAddress)
			if err == nil && len(addr) > 0 {
				//log.Debugf("   got ip array: %v", addr)
				for _, ip := range addr {
					if !ip.IsGlobalUnicast() {
						log.Warnf("   IsGlobalUnicast('%v') == false, skip to next", ip)
						continue
					}
					//log.Debugf("   return ip, port: %v, %v", ip, service.ServicePort)
					sr = append(sr, &store.ServiceRecord{
						IP: ip, Port: uint16(service.ServicePort), ID: service.ServiceID,
					})
				}
			} else {
				log.Errorf("NameResolverA;; failed: lookup ip failed: %v", err)
			}
		}
	} else {
		log.Errorf("NameResolverAll failed: queryService failed: %v", err)
	}
	return
}
