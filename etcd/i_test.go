package etcd_test

import (
	"testing"
	"time"
	"fmt"
	"golang.org/x/net/context"
	"github.com/coreos/etcd/clientv3"
	"github.com/hedzr/kvl/store"
	"github.com/hedzr/kvl/etcd"
)

func TestInterface(t *testing.T) {
	st := etcd.New(&etcd.Etcdtool{
		Peers:          "127.0.0.1:2379",
		CommandTimeout: 5 * time.Minute,
	})
	defer st.Close()

	t.Log("Connecting to localhost:2379, and got: ", st)

	// bash "etcdctl put bug nothing"
	// And:

	e := (st).(*etcd.KVStoreEtcd)
	//st.SetRoot(DEFAULT_ROOT_KEY)

	_ := st.WatchPrefix("", func(evType store.Event_EventType, key []byte, value []byte) {
		t.Logf(" - - -> [%s] %q: %q", store.Event_EventType_name[evType], key, value)
		fmt.Printf("    -> [%s] %q: %q\n", store.Event_EventType_name[evType], key, value)
	}, nil) // an go routine will run in background...

	_, err := e.Client.Put(context.TODO(), "bug", "bar ooo1")
	if err != nil {
		t.Fatal(err)
	}

	state := map[string]string{
		"ab": "111",
		"cd": "222",
	}

	st.PutYaml("state", state)
	state1 := st.GetYaml("state")
	t.Logf("state1: %v", state1)

	st.Delete("bug/project01")
	st.Put("bug/project01/state", "xxxxx first")
	//st.Put("bug/project02", "xxxxx second")
	st.Put("bug/project02/module01", "xxxxx second - module #1")
	st.DeletePrefix("bug/project01")
	st.Put("bug/project03/state", "xxxxx third")
	st.Put("bug/project01/state", "xxxxx first later")

	//st.Put("bug/project01", "etcdv3_dir_$2H#%gRe3*t")
	//st.Put("bug/project02", "etcdv3_dir_$2H#%gRe3*t")
	//st.Put("bug/project03", "etcdv3_dir_$2H#%gRe3*t")
	//st.Put("bug", "etcdv3_dir_$2H#%gRe3*t")

	requestTimeout, err := time.ParseDuration("5s")

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	resp, err := e.Client.Get(ctx, "bug", clientv3.WithPrefix())
	cancel()
	t.Logf("Get key='bug': value='%v'", resp)
	for _, ev := range resp.Kvs {
		t.Logf("    - %s : %s\n", ev.Key, ev.Value)
	}
	t.Logf("okay.")

	st.Put("bug/project01/status", "ok xxxxx1")

	r := st.Get("bug")
	//resp = r.(*clientv3.GetResponse)
	t.Logf("Got: 'bug': '%v'", r)

	fmt.Println("END of TestInterface()")
	t.Log("END of TestInterface()")

}

func getEtcdClient() store.KVStore {
	store := etcd.New(&etcd.Etcdtool{
		"127.0.0.1:2379",
		"",
		"", "", "",
		time.Second * 10,
		time.Second * 5,
		[]etcd.Route{},
		"",
		"",
	})
	return store
}

func TestEtcdWatch(t *testing.T) {
	st := getEtcdClient()
	defer st.Close()
	s := st.(*etcd.KVStoreEtcd)

	s.SetDebug(true)

	s.Put("aaaa", "ready.")

	var stopCh chan bool = make(chan bool)
	var i = 0
	var val = s.Get("aaaa")
	go func() {
		time.Sleep(time.Second * 4)
		// 发出三次PUT，以便结束blockFunc的阻塞
		s.Put("aaaa", val+"ss")
		s.Put("aaaa", val+"sstt")
		s.Put("aaaa", val+"1")
		s.Put("aaaa", val)
	}()

	blockFunc := s.Watch("aaaa", func(evType store.Event_EventType, key []byte, value []byte) {
		fmt.Printf("** [watch] %s - %q:%q\n", store.Event_EventType_name[evType], key, value)
		if i >= 3 {
			fmt.Println("watching routine will be closed.")
			stopCh <- true // 结束bolckFunc的阻塞，也结束Watch的go routine
		}
		i = i + 1
	}, stopCh)

	if blockFunc != nil {
		blockFunc()
		//stopCh <- true // no effect
	}
}
