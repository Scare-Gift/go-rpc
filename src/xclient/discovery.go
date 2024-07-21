package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}
type MultiServersDiscovery struct {
	R       *rand.Rand
	Mu      sync.RWMutex
	Servers []string
	Index   int
}

func NewMultiServerDiscovery(servers []string) *MultiServersDiscovery {
	d := &MultiServersDiscovery{
		Servers: servers,
		R:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.Index = d.R.Intn(math.MaxInt32 - 1)
	return d
}

var _ Discovery = (*MultiServersDiscovery)(nil)

// Refresh doesn't make sense for MultiServersDiscovery, so ignore it
func (d *MultiServersDiscovery) Refresh() error {
	return nil
}

// Update the servers of discovery dynamically if needed
func (d *MultiServersDiscovery) Update(servers []string) error {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	d.Servers = servers
	return nil
}

// Get a server according to mode
func (d *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	n := len(d.Servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.Servers[d.R.Intn(n)], nil
	case RoundRobinSelect:
		s := d.Servers[d.Index%n] // servers could be updated, so mode n to ensure safety
		d.Index = (d.Index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}

// GetAll returns all servers in discovery
func (d *MultiServersDiscovery) GetAll() ([]string, error) {
	d.Mu.RLock()
	defer d.Mu.RUnlock()
	// return a copy of d.servers
	servers := make([]string, len(d.Servers), len(d.Servers))
	copy(servers, d.Servers)
	return servers, nil
}
