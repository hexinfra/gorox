// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MsgX is a trivial remote tell or call.

package msgx

import (
	"io"
)

func Tell(writer io.Writer, req *Message) bool {
	req.SetTell()
	return Send(writer, req)
}
func Call(readWriter io.ReadWriter, req *Message, maxSize int32) (*Message, bool) {
	req.SetCall()
	if !Send(readWriter, req) {
		return nil, false
	}
	return Recv(readWriter, maxSize)
}

// msg = head + body
// head = comd(8) + nArgs(8) + flag(16) + mode(1) + size(31)
// body = nArgs * argHead + nArgs * argBody
// argHead = nameSize(8) + valueSize(32)
// argBody = name(nameSize) + value(valueSize)

const maxSize = 2147483647

func Send(writer io.Writer, msg *Message) (ok bool) {
	nArgs := len(msg.Args)
	if nArgs > 255 {
		return false
	}
	size := nArgs * 5
	for name, value := range msg.Args {
		nameSize := len(name)
		if nameSize > 255 {
			return false
		}
		size += nameSize
		if size < 0 || size > maxSize {
			return false
		}
		size += len(value)
		if size < 0 || size > maxSize {
			return false
		}
	}
	buffer := make([]byte, 8+size)
	buffer[0] = msg.Comd
	buffer[1] = uint8(nArgs)
	buffer[2], buffer[3] = uint8(msg.Flag>>8), uint8(msg.Flag)
	buffer[4], buffer[5], buffer[6], buffer[7] = uint8(size>>24), uint8(size>>16), uint8(size>>8), uint8(size)
	if msg.call {
		buffer[4] |= 0b10000000
	}
	from := 8
	back := 8 + nArgs*5
	for name, value := range msg.Args {
		nameSize := len(name)
		buffer[from] = uint8(nameSize)
		valueSize := len(value)
		buffer[from+1] = uint8(valueSize >> 24)
		buffer[from+2] = uint8(valueSize >> 16)
		buffer[from+3] = uint8(valueSize >> 8)
		buffer[from+4] = uint8(valueSize)
		from += 5
		fore := back + nameSize
		copy(buffer[back:fore], name)
		back = fore
		fore += valueSize
		copy(buffer[back:fore], value)
		back = fore
	}
	_, err := writer.Write(buffer)
	return err == nil
}
func Recv(reader io.Reader, maxSize int32) (msg *Message, ok bool) {
	msg = new(Message)
	if _, err := io.ReadFull(reader, msg.head[:]); err != nil {
		return nil, false
	}
	msg.Comd = msg.head[0]
	msg.Flag = uint16(msg.head[2])<<8 | uint16(msg.head[3])
	size := int32(msg.head[4])<<24 | int32(msg.head[5])<<16 | int32(msg.head[6])<<8 | int32(msg.head[7])
	if size < 0 {
		msg.call = true
		size &= 0x7fffffff
	}
	if size > maxSize {
		return nil, false
	}
	if size == 0 {
		return msg, true
	}
	nArgs := int32(msg.head[1])
	back := nArgs * 5
	if back > size {
		return nil, false
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, false
	}
	msg.Args = make(map[string]string, nArgs)
	from := 0
	for i := int32(0); i < nArgs; i++ {
		nameSize := int32(body[from])
		valueSize := int32(body[from+1])<<24 | int32(body[from+2])<<16 | int32(body[from+3])<<8 | int32(body[from+4])
		from += 5
		fore := back + nameSize
		if fore > size {
			return nil, false
		}
		name := string(body[back:fore])
		back = fore
		fore += valueSize
		if fore > size {
			return nil, false
		}
		value := string(body[back:fore])
		back = fore
		if name != "" {
			msg.Args[name] = value
		}
	}
	if back != size {
		return nil, false
	}
	return msg, true
}

// Message
type Message struct {
	head [8]byte
	call bool
	Comd uint8  // 0-255, allow max 256 commands
	Flag uint16 // 0-65535
	Args map[string]string
}

func NewMessage(comd uint8, flag uint16, args map[string]string) *Message {
	m := new(Message)
	m.Comd = comd
	m.Flag = flag
	m.Args = args
	return m
}

func (m *Message) SetTell() { m.call = false }
func (m *Message) SetCall() { m.call = true }

func (m *Message) IsTell() bool { return !m.call }
func (m *Message) IsCall() bool { return m.call }

func (m *Message) Get(name string) string { return m.Args[name] }
func (m *Message) Set(name string, value string) {
	if m.Args == nil {
		m.Args = make(map[string]string)
	}
	m.Args[name] = value
}
