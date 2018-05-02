package etcd

import (
	"io/ioutil"
	"net/http"
	"strings"
	"time"
	"github.com/bgentry/speakeasy"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	"golang.org/x/net/context"
	"github.com/hedzr/kvl/store"
)

func demo() {
	cli, err := clientv3.New(clientv3.Config{
		//Endpoints:   []string{"localhost:2379", "localhost:22379", "localhost:32379"},
		Endpoints:   []string{"localhost:2379", "localhost:22379", "localhost:32379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		// handle error!
	}
	defer cli.Close()
}

func New(e *Etcdtool) store.KVStore {
	c := NewClient(e)
	store := KVStoreEtcd{
		c,
		e,
		nil,
		nil,
		make(chan bool),
	}
	store.ctx, store.cancelFunc = contextWithCommandTimeout(e.CommandTimeout)
	return &store
}

func contextWithCommandTimeout(commandTimeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commandTimeout)
}

func newTransport(e *Etcdtool) *http.Transport {
	tls := transport.TLSInfo{
		CAFile:   e.CA,
		CertFile: e.Cert,
		KeyFile:  e.Key,
	}

	timeout := 30 * time.Second
	tr, err := transport.NewTransport(tls, timeout)
	if err != nil {
		warnf("WARN: %v", err)
	}

	return tr
}

func NewClient(e *Etcdtool) *clientv3.Client {
	cfg := clientv3.Config{
		DialTimeout: 5 * time.Second,
		Endpoints:   strings.Split(e.Peers, ","),
		//Transport:               newTransport(e),
		//HeaderTimeoutPerRequest: e.Timeout,
	}

	if e.User != "" {
		cfg.Username = e.User
		if e.PasswordFilePath != "" {
			passBytes, err := ioutil.ReadFile(e.PasswordFilePath)
			if err != nil {
				warnf("WARN: %v", err)
			}
			cfg.Password = strings.TrimRight(string(passBytes), "\n")
		} else {
			pwd, err := speakeasy.Ask("Password: ")
			if err != nil {
				warnf("WARN: %v", err)
			}
			cfg.Password = pwd
		}
	}

	cl, err := clientv3.New(cfg)
	if err != nil {
		warnf("WARN: %v", err)
	}

	return cl
}
