package main

import (
	"context"
	"fmt"
	"time"

	"github.com/grandcat/zeroconf"
)

func main() {
	r, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		fmt.Println("resolver:", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := make(chan *zeroconf.ServiceEntry, 16)
	go func() {
		for e := range ch {
			fmt.Printf("ENTRY %s host=%s v4=%v port=%d txt=%v\n", e.Instance, e.HostName, e.AddrIPv4, e.Port, e.Text)
		}
	}()
	if err := r.Browse(ctx, "_ipp._tcp", "local.", ch); err != nil {
		fmt.Println("browse:", err)
	}
	<-ctx.Done()
	time.Sleep(200 * time.Millisecond)
	fmt.Println("done")
}
