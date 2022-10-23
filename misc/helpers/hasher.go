package main

import (
	"fmt"
)

func main() {
	//calc([]byte("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE PATCH LINK UNLINK QUERY"))
	//calc([]byte("accept accept-charset accept-encoding accept-language cache-control connection content-encoding content-language forwarded if-match if-none-match pragma te trailer transfer-encoding upgrade via"))
	//calc([]byte("accept-encoding accept-ranges allow cache-control connection content-encoding proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate"))
	//calc([]byte("content-length content-range content-type date etag expires last-modified location server set-cookie"))
	//calc([]byte("content-length content-type cookie expect host if-modified-since if-range if-unmodified-since range user-agent"))
	println(sum("proxy-authenticate"))
}

type Node struct {
	hash int
	from int
	edge int
}

func sum(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n += int(s[i])
	}
	return n
}

func calc(s []byte) {
	var nodes []Node

	hash, from := 0, 0
	for edge := 0; edge < len(s); edge++ {
		b := s[edge]
		if b == ' ' {
			nodes = append(nodes, Node{hash, from, edge})
			from = edge + 1
			hash = 0
		} else {
			hash += int(b)
		}
	}
	nodes = append(nodes, Node{hash, from, len(s)})

	size := len(nodes)
	zero := make([]int, size)
	this := make([]int, size)
	good := 0
search:
	for k := 1; k < 48924604; k++ {
		copy(this, zero)
		for _, node := range nodes {
			i := k / node.hash % size
			if this[i] == 0 {
				this[i] = node.hash
			} else {
				continue search
			}
		}
		good = k
		break
	}

	fmt.Printf("good=%d size=%d\n", good, size)
	for _, node := range nodes {
		fmt.Printf("%d:{%d, %d, %d, %s},\n", good/node.hash%size, node.hash, node.from, node.edge, s[node.from:node.edge])
	}
}
