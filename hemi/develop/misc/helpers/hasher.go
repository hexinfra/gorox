package main

import (
	"fmt"
)

func main() {
	calc([]byte("accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match te trailer transfer-encoding upgrade via x-forwarded-for"))
	//println(sum("cache-control"))
	//println(sum("last-modified"))
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
	for k := 1; k < 1148924604; k++ {
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
		name := showName(string(s[node.from:node.edge]))
		fmt.Printf("%d:{%s, %d, %d, %s},\n", good/node.hash%size, "hash"+name, node.from, node.edge, "check"+name)
	}
}

func showName(name string) string {
	s := ""
	upper := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' {
			upper = true
			continue
		}
		if upper {
			s += string(c - 0x20)
			upper = false
		} else {
			s += string(c)
		}
	}
	return s
}
