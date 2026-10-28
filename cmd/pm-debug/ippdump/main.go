package main

import (
	"fmt"
	"os"

	"printmarshal/internal/driver/ipp"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	m, err := ipp.Decode(b)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=0x%04x groups=%d\n", m.StatusCode, len(m.Groups))
	for _, g := range m.Groups {
		for _, a := range g.Attributes {
			if len(a.Values) > 6 {
				fmt.Printf("%s (tag %02x) = %v ... (%d values)\n", a.Name, a.Tag, a.Values[:6], len(a.Values))
			} else {
				fmt.Printf("%s (tag %02x) = %v\n", a.Name, a.Tag, a.Values)
			}
		}
	}
}
