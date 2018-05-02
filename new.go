package kvl

import (
	"github.com/hedzr/kvl/etcd"
	"github.com/hedzr/kvl/consul"
	c "github.com/hedzr/kvl/consul/consul_util"
	"github.com/hedzr/kvl/store"
)

const (
	STORE_ETCD   = iota
	STORE_CONSUL
)

func New(e interface{}) store.KVStore {
	if ee, ok := e.(*etcd.Etcdtool); ok {
		return NewETCD(ee)
	}
	if ee, ok := e.(*c.ConsulConfig); ok {
		return NewConsul(ee)
	}
	return nil
}

func NewETCD(e *etcd.Etcdtool) store.KVStore {
	return etcd.New(e)
}

func NewConsul(e *c.ConsulConfig) store.KVStore {
	return consul.New(e)
}
