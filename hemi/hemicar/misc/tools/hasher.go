package main

import (
	"fmt"
)

func main() {
	calc([]byte("authorization content-length content-location content-range content-type cookie date host if-modified-since if-range if-unmodified-since max-forwards proxy-authorization range referer user-agent"))
	//calc([]byte("age content-disposition content-length content-location content-range content-type date etag expires last-modified location retry-after server set-cookie"))
	//calc([]byte("accept accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match keep-alive proxy-connection te trailer transfer-encoding upgrade via x-forwarded-by x-forwarded-for x-forwarded-host x-forwarded-proto"))
	//calc([]byte("accept accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language keep-alive proxy-authenticate proxy-connection trailer transfer-encoding upgrade vary via www-authenticate"))
	//println(sum("cache-control"))
	//println(sum("last-modified"))
	//println(sum("referer"))
}

type Word struct {
	hash int
	from int
	edge int
}

func sum(s string) int {
	n := 0
	for i := range len(s) {
		n += int(s[i])
	}
	return n
}

func calc(s []byte) {
	var words []Word

	hash, from := 0, 0
	for edge := range len(s) {
		b := s[edge]
		if b == ' ' {
			words = append(words, Word{hash, from, edge})
			from = edge + 1
			hash = 0
		} else {
			hash += int(b)
		}
	}
	words = append(words, Word{hash, from, len(s)})

	size := len(words)
	zero := make([]int, size)
	this := make([]int, size)
	good := 0
search:
	for k := 1; k < 3148924604; k++ {
		copy(this, zero)
		for _, word := range words {
			i := k / word.hash % size
			if this[i] == 0 {
				this[i] = word.hash
			} else {
				continue search
			}
		}
		good = k
		break
	}

	fmt.Printf("good=%d size=%d\n", good, size)
	for i := range len(words) {
		for j := range len(words) {
			word := words[j]
			if i == good/word.hash%size {
				name := showName(string(s[word.from:word.edge]))
				fmt.Printf("%d:{%s, %d, %d, %s},\n", good/word.hash%size, "hash"+name, word.from, word.edge, "check"+name)
				break
			}
		}
	}
}

func showName(name string) string {
	s := ""
	upper := true
	for i := range len(name) {
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
