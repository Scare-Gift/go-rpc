package xclient

import (
	"context"
	"go-rpc/src/client"
	"go-rpc/src/service"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	D       Discovery
	Mode    SelectMode
	opt     *service.Option
	mu      sync.Mutex
	clients map[string]*client.Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *service.Option) *XClient {
	return &XClient{D: d, Mode: mode, opt: opt, clients: make(map[string]*client.Client)}
}
func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, cli := range xc.clients {
		_ = cli.Close()
		delete(xc.clients, key)
	}
	return nil
}
func (xc *XClient) dial(rpcAddr string) (*client.Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	cli, ok := xc.clients[rpcAddr]
	if ok && !cli.IsAvailable() {
		_ = cli.Close()
		delete(xc.clients, rpcAddr)
		cli = nil
	}
	if cli == nil {
		var err error
		cli, err = client.XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = cli
	}
	return cli, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// xc will choose a proper server.
func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.D.Get(xc.Mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast invokes the named function for every server registered in discovery
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.D.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex // protect e and replyDone
	var e error
	replyDone := reply == nil // if reply is nil, don't need to set value
	ctx, cancel := context.WithCancel(ctx)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // if any call failed, cancel unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
