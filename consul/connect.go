package consul

import (
	cu "github.com/hedzr/kvl/consul/consul_util"
	"github.com/hedzr/kvl/store"
	"context"
	log "github.com/sirupsen/logrus"
	"time"
)

func New(e *cu.ConsulConfig) store.KVStore {
	cli, _, err := cu.GetConsulConnection(e)
	if err != nil {
		log.Fatalf("FATAL: CANNOT CONNECT TO CONSUL. %v", err)
		return nil
	}

	store := KVStoreConsul{
		cli,
		ROOT_KEY,
		e,
		nil,
		nil,
		5 * time.Second,
		make(chan bool),
		make(chan bool, 3),
		false,
		"",
	}

	store.ctx, store.cancelFunc = contextWithCommandTimeout(store.Timeout)
	return &store
}

func contextWithCommandTimeout(commandTimeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commandTimeout)
}
