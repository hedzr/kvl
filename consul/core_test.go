package consul_test

import (
	"testing"
	"net/http"
	"context"
	"fmt"
	"time"
	"github.com/hedzr/kvl/store"
	"github.com/hedzr/kvl/consul"
	cu "github.com/hedzr/kvl/consul/consul_util"
)

func getConsulClient() store.KVStore {
	store := consul.New(&cu.ConsulConfig{
		"http",
		"127.0.0.1:8500",
		true,
		"", "", "", "", "",
		"",
		"30s",
	})
	return store
}

// 分布式锁的测试
func TestConsulLocker(t *testing.T) {
	store := getConsulClient()
	defer store.Close()
	s := store.(*consul.KVStoreConsul)

	lock := s.LockObject("abc", "def", "10s")
	req, _ := http.NewRequest("GET", "https://example.com/webhook", nil)

	blockFunc := s.Acquire(lock, func(cancelCtx context.Context) {
		req = req.WithContext(cancelCtx)
	}, func(holder *consul.Holder) {
		fmt.Println("requesting")
		resp, err := http.DefaultClient.Do(req)
		holder.Any = resp
		fmt.Println("got response")
		if err != nil {
			fmt.Printf("ERR-1: %v\n", err)
		}
	}, func(holder *consul.Holder) {
		fmt.Printf("on done\n")
	}, func(holder *consul.Holder) {
		time.Sleep(time.Second * 15)
		fmt.Printf("on done and unlocked. testing the k 'abc'/v: '%s'\n", s.Get("abc"))
		if resp, ok := holder.Any.(*http.Response); ok {
			fmt.Printf(" - resp: %v\n", resp)
		}
	}, func(holder *consul.Holder) {
		fmt.Println("on cancel")
		fmt.Printf("cancelled.\n")
	})

	fmt.Println("blockFunc running and blocking.")
	blockFunc()
	fmt.Println("blockFunc done.")

	if err := lock.Destroy(); err != nil {
		fmt.Printf("Error on destroying lock object: %v\n", err)
	}
}

// 带TTL的 KVPair 测试
// consul 的 TTL 没有能达到我们想要的效果。
func TestTTLKV(t *testing.T) {
	store := getConsulClient()
	defer store.Close()
	s := store.(*consul.KVStoreConsul)

	s.SetDebug(true)

	s.PutTTL("abcd", "efg", 10)
	val := s.Get("abcd")
	fmt.Printf("the value = %s\n", val)
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second * 1)
		val = s.Get("abcd")
		fmt.Printf("timeout, the value = %s\n", val)
	}
	time.Sleep(time.Second * 20)
	s.DeleteTTL("abcd")
}

func TestConsulWatch(t *testing.T) {
	st := getConsulClient()
	defer st.Close()
	s := st.(*consul.KVStoreConsul)

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
