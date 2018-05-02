package consul_util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

const (
	DEFAULT_CONSUL_HOST      = "consul.ops.local"
	DEFAULT_CONSUL_LOCALHOST = "localhost"
	DEFAULT_CONSUL_PORT      = 8500
	DEFAULT_CONSUL_SCHEME    = "http"

	baseWait = 1 * time.Millisecond
	maxWait  = 100 * time.Millisecond

	SERVICE_CONSUL_API = "consulapi"
	SERVICE_DB         = "test-rds"
	SERVICE_MQ         = "test-mq"
	SERVICE_CACHE      = "test-redis"

	KEY_WAS_SETUP   = "ops/config/common"
	VALUE_WAS_SETUP = "---"
)

type valueEnc struct {
	Encoding string `json:"encoding,omitempty"`
	Str      string `json:"value"`
}

type kvJSON struct {
	BackupDate time.Time           `json:"date"`
	Connection map[string]string   `json:"connection_info"`
	Values     map[string]valueEnc `json:"values"`
}

type ConsulConfig struct {
	Scheme                         string `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	Addr                           string `json:"addr" yaml:"addr"`
	Insecure                       bool   `json:"insecure,omitempty" yaml:"insecure,omitempty"`
	CertFile                       string `json:"cert,omitempty" yaml:"cert,omitempty"`
	KeyFile                        string `json:"key,omitempty" yaml:"key,omitempty"`
	CACertFile                     string `json:"cacert,omitempty" yaml:"cacert,omitempty"`
	Username                       string `json:"username,omitempty" yaml:"username,omitempty"`
	Password                       string `json:"password,omitempty" yaml:"password,omitempty"`
	Root                           string `json:"root,omitempty" yaml:"root,omitempty"`
	DeregisterCriticalServiceAfter string `json:"deregister-critical-service-after,omitempty" yaml:"deregister-critical-service-after,omitempty"`
}

// GetDeregisterCriticalServiceAfter default is '30s'
//
// In Consul 0.7 and later, checks that are associated with a service
// may also contain this optional DeregisterCriticalServiceAfter field,
// which is a timeout in the same Go time format as Interval and TTL. If
// a check is in the critical state for more than this configured value,
// then its associated service (and all of its associated checks) will
// automatically be deregistered.
func (s *ConsulConfig) GetDeregisterCriticalServiceAfter() string {
	if len(s.DeregisterCriticalServiceAfter) == 0 {
		return "30s"
	}
	return s.DeregisterCriticalServiceAfter
}

type Base struct {
	FirstClient *api.Client
}

type Registrar struct {
	Base
	Clients       []*api.CatalogService
	CurrentClient *api.CatalogService
}

var (
	DefaultConsulConfig = ConsulConfig{
		Scheme:   DEFAULT_CONSUL_SCHEME,
		Addr:     "127.0.0.1:8500",
		Insecure: true,
	}
)

func GetConsulConnection(c *ConsulConfig) (client *api.Client, bkup *kvJSON, err error) {
	if c == nil {
		c = &DefaultConsulConfig
	}

	// Start with the default Consul API config
	config := api.DefaultConfig()

	// Create a TLS config to be populated with flag-defined certs if applicable
	tlsConf := &tls.Config{}

	// Set scheme and address:port
	config.Scheme = c.Scheme
	//config.Address = fmt.Sprintf("%s:%v", c.GlobalString("addr"), c.GlobalInt("port"))
	config.Address = c.Addr
	//if config.Address == "" {
	//	config.Address = c.GlobalString("consul.addr")
	//}
	log.Debugf("Connecting to %s://%s ...", config.Scheme, config.Address)

	// Populate backup metadata
	bkup = &kvJSON{
		BackupDate: time.Now(),
		Connection: map[string]string{},
	}

	// Check for insecure flag
	if c.Insecure {
		tlsConf.InsecureSkipVerify = true
		bkup.Connection["insecure"] = "true"
	}

	// Load default system root CAs
	// ignore errors since the TLS config
	// will only be applied if --cert and --key
	// are defined
	tlsConf.ClientCAs, _ = LoadSystemRootCAs()

	// If --cert and --key are defined, load them and apply the TLS config
	if len(c.CertFile) > 0 && len(c.KeyFile) > 0 {
		// Make sure scheme is HTTPS when certs are used, regardless of the flag
		config.Scheme = "https"
		bkup.Connection["cert"] = c.CertFile
		bkup.Connection["key"] = c.KeyFile

		// Load cert and key files
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			log.Fatalf("Could not load cert: %v", err)
		}
		tlsConf.Certificates = append(tlsConf.Certificates, cert)

		// If cacert is defined, add it to the cert pool
		// else just use system roots
		if len(c.CACertFile) > 0 {
			tlsConf.ClientCAs = AddCACert(c.CACertFile, tlsConf.ClientCAs)
			tlsConf.RootCAs = tlsConf.ClientCAs
			bkup.Connection["cacert"] = c.CACertFile
		}
	}

	bkup.Connection["host"] = config.Scheme + "://" + config.Address

	if config.Scheme == "https" {
		// Set Consul's transport to the TLS config
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConf,
		}
	}

	// Check for HTTP auth flags
	if len(c.Username) > 0 && len(c.Password) > 0 {
		config.HttpAuth = &api.HttpBasicAuth{
			Username: c.Username,
			Password: c.Password,
		}
		bkup.Connection["user"] = c.Username
		bkup.Connection["pass"] = c.Password
	}

	// Generate and return the API client
	client, err = api.NewClient(config)
	if err != nil {
		log.Fatalf("Error: %v", err)
		fmt.Println("[consul][connect] Failed!")
	} else {
		fmt.Println("[consul][connect] successfully")
	}
	return client, bkup, nil
}
