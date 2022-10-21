package system

import (
	"fmt"
	"testing"
)

func TestOpenClose(t *testing.T) {
	path := []byte("/etc/passwd\x00")
	file, err := Open0(path)
	if err != nil {
		t.Fatalf("open0 error=%s\n", err.Error())
	}
	fmt.Printf("opened file=%d\n", file)
	if err := file.Close(); err != nil {
		t.Fatalf("close error=%s\n", err.Error())
	}
}

func TestNotOpen(t *testing.T) {
	path := []byte("/etc/passwd")
	file, err := Open0(path)
	if err == nil {
		t.Fatalf("bad open0, want %v, got %v\n", nil, err)
	}
	fmt.Printf("file=%v\n", file)
}

func TestFileStat(t *testing.T) {
	path := []byte("/etc/passwd\x00")
	file, err := Open0(path)
	if err != nil {
		t.Fatalf("open error=%s\n", err.Error())
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat error=%s\n", err.Error())
	}
	fmt.Printf("info=%+v\n", info)
	fmt.Printf("valid=%v\n", info.Valid())
	fmt.Printf("isDir=%v\n", info.IsDir())
	fmt.Printf("modTime=%d\n", info.ModTime())
	if err := file.Close(); err != nil {
		t.Fatalf("close error=%s\n", err.Error())
	}
}

func TestStat(t *testing.T) {
	path := []byte("/etc/passwd\x00")
	info, err := Stat0(path)
	if err != nil {
		t.Fatalf("stat error=%s\n", err.Error())
	}
	fmt.Printf("info=%+v\n", info)
}
