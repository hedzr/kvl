package store_test

import (
	"testing"
	"github.com/hedzr/kvl/store"
	"github.com/hedzr/kvl/consul/consul_util"
	"github.com/hedzr/kvl"
	"github.com/magiconair/properties/assert"
	"time"
	"github.com/hedzr/kvl/etcd"
)

func TestThisHostname(t *testing.T) {
	h := store.ThisHostname()
	t.Logf("ThisHostname = %s", h)
	ip := store.ThisHost()
	t.Logf("ThisHost = %s", ip.String())
}

func getConsulStore() store.KVStore {
	store := kvl.New(&consul_util.ConsulConfig{
		Scheme:                         "http",
		Addr:                           "127.0.0.1:8500",
		Insecure:                       true,
		CertFile:                       "",
		KeyFile:                        "",
		CACertFile:                     "",
		Username:                       "",
		Password:                       "",
		Root:                           "",
		DeregisterCriticalServiceAfter: "30s",
	})
	return store
}

func getEtcdStore() store.KVStore {
	store := kvl.New(
		&etcd.Etcdtool{
			Peers:            "127.0.0.1:2379",
			Cert:             "",
			Key:              "",
			CA:               "",
			User:             "",
			Timeout:          time.Second * 10,
			CommandTimeout:   time.Second * 5,
			Routes:           []etcd.Route{},
			PasswordFilePath: "",
			Root:             "",
		}
	)
	return store
}

func TestGetPut(t *testing.T) {
	store := getConsulStore()

	store.Put("x", "yz")
	t.Log(store.Get("x"))
	assert.Equal(t, store.Get("x"), "yz", "expect ['x'] == 'yz'.")
}
