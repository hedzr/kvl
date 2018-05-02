package store

// copy from coreos.etcd.mvcc.mvccpb
type Event_EventType int32

const (
	PUT    Event_EventType = 0
	DELETE Event_EventType = 1
)

var Event_EventType_name = map[Event_EventType]string{
	PUT:    "PUT",
	DELETE: "DELETE",
}
var Event_EventType_value = map[string]Event_EventType{
	"PUT":    PUT,
	"DELETE": DELETE,
}

type (
	WatchFunc func(evType Event_EventType, key []byte, value []byte)

	KvPair interface {
		Key() string
		Value() []byte
		ValueString() string
	}
	KvPairs interface {
		Count() int
		Item(index int) KvPair
	}

	//KvsHolder inerface{}

	KVStore interface {
		// x := kvl.New(...)
		// defer x.Close()
		Close()

		IsOpen() bool

		// SetRoot 设置访问key值时的前缀层级。
		// 例如设置为 "states"，则 Get("service1") 实际上是在访问 "states/service1"
		// default root is: "root"
		SetRoot(keyPrefix string)
		GetRootPrefix() string

		Get(key string) string
		GetPrefix(keyPrefix string) KvPairs //api.KVPair //*mvccpb.KeyValue
		//
		GetYaml(key string) interface{}
		Put(key string, value string) error
		PutLite(key string, value string) error
		// 给出一个失效时间
		PutTTL(key string, value string, ttlSeconds int64) error
		// 给出一个失效时间。和 PutTTL 的区别在于，内部实现不会再检查上级目录的有效性
		PutTTLLite(key string, value string, ttlSeconds int64) error
		// 将 value 进行yaml编码之后再放入 k/v store 中
		PutYaml(key string, value interface{}) error
		PutYamlLite(key string, value interface{}) error

		Exists(key string) bool

		Delete(key string) bool
		DeletePrefix(keyPrefix string) bool

		// 返回的func对象是一个blockFunc，调用它将阻塞调用线程，可以 stopCh <- true 来终止它。
		// 对于etcd来说，stopCh可以传入nil, blockFunc将无效。
		// 对于consul来说，stopCh不允许传入nil值，blockFunc在结束阻塞的同时也结束监视计划。
		// 对于consul来说，传入stopCh=nil，将导致调用blockFunc时立即结束监视计划！
		//
		// 如果在consul中开启一个Watch，必须在某个适当的时候调用返回的blockFunc以结束监视计划，否则一个后台的consul client以及监视线程无法被清除！
		// 但etcd使用同一个client来启动后台监视线程，因此无需特别的清理动作，在client被close时所有监视线程均会被正确清理。
		//
		// etcd的Watch机制下，没有办法取消一个已建立的Watcher，除非Close这个Client。例如：
		// ```go
		// _ = kvStore.Watch(/./)
		// kvStore.Close()  // 将会清理一切Watchers
		// ```
		//
		// stopCh将被用于go routine的同步、阻塞、结束阻塞。调用者应该创建该channel并写入true以终止阻塞线程
		Watch(key string, fn WatchFunc, stopCh chan bool) (func())
		WatchPrefix(keyPrefix string, fn WatchFunc, stopCh chan bool) (func())
	}
)
