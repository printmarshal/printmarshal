// Command pm-debug runs individual discovery methods and prints what they see.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"printmarshal/internal/discovery"
)

func main() {
	ctx := context.Background()
	fmt.Println("mDNS browse (6s)…")
	c, err := discovery.BrowseMDNS(ctx, 6*time.Second)
	fmt.Println("err:", err)
	b, _ := json.MarshalIndent(c, "", " ")
	fmt.Println(string(b))
	fmt.Println("WS-Discovery probe (3s)…")
	w, err := discovery.ProbeWSD(ctx, 3*time.Second)
	fmt.Println("err:", err)
	b, _ = json.MarshalIndent(w, "", " ")
	fmt.Println(string(b))
	os.Exit(0)
}
