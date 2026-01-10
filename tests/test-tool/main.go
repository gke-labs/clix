package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Hello from test-tool")
	for i, arg := range os.Args[1:] {
		fmt.Printf("Arg %d: %s\n", i, arg)
	}
}
