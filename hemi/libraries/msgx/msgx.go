// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MsgX is a trivial remote tell or call.

package msgx

import (
	"io"
)

// msg: mode(1) | comd(1) | flag(2) | nArgs(1) | size(3) | args(...arg)
// arg: nameSize(1) | valueSize(3) | name(nameSize) | value(valueSize)

// Message
type Message struct {
	Mode uint8  // 0:tell 1:call
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
func (m *Message) SetTell()     { m.Mode = 0 }
func (m *Message) IsTell() bool { return m.Mode == 0 }
func (m *Message) SetCall()     { m.Mode = 1 }
func (m *Message) IsCall() bool { return m.Mode != 0 }

func (m *Message) Get(name string) string { return m.Args[name] }
func (m *Message) Set(name string, value string) {
	if m.Args == nil {
		m.Args = make(map[string]string)
	}
	m.Args[name] = value
}

func Tell(writer io.Writer, req *Message) bool {
	req.SetTell()
	return SendMessage(writer, req)
}
func Call(readWriter io.ReadWriter, req *Message) (*Message, bool) {
	req.SetCall()
	if !SendMessage(readWriter, req) {
		return nil, false
	}
	return RecvMessage(readWriter)
}

func SendMessage(writer io.Writer, msg *Message) (ok bool) {
	nArgs := len(msg.Args)
	if nArgs > 255 {
		return false
	}
	head := make([]byte, 8)
	head[0] = msg.Mode
	head[1] = msg.Comd
	head[2], head[3] = uint8(msg.Flag>>8), uint8(msg.Flag)
	head[4] = uint8(nArgs)
	back := nArgs * 4
	size := back
	for name, value := range msg.Args {
		nameSize := len(name)
		if nameSize > 255 {
			return false
		}
		size += nameSize
		if size < 0 || size > 16777215 {
			return false
		}
		size += len(value)
		if size < 0 || size > 16777215 {
			return false
		}
	}
	head[5], head[6], head[7] = uint8(size>>16), uint8(size>>8), uint8(size)
	if _, err := writer.Write(head); err != nil {
		return false
	}
	if size == 0 {
		return true
	}
	body := make([]byte, size)
	from := 0
	for name, value := range msg.Args {
		nameSize := len(name)
		body[from] = uint8(nameSize)
		from += 1
		valueSize := len(value)
		body[from] = uint8(valueSize >> 16)
		body[from+1] = uint8(valueSize >> 8)
		body[from+2] = uint8(valueSize)
		from += 3
		fore := back + nameSize
		copy(body[back:fore], name)
		back = fore
		fore += valueSize
		copy(body[back:fore], value)
		back = fore
	}
	_, err := writer.Write(body)
	return err == nil
}
func RecvMessage(reader io.Reader) (msg *Message, ok bool) {
	msg = new(Message)
	head := make([]byte, 8)
	if _, err := io.ReadFull(reader, head); err != nil {
		return nil, false
	}
	msg.Mode = head[0]
	msg.Comd = head[1]
	msg.Flag = uint16(head[2])<<8 | uint16(head[3])
	size := int(head[5])<<16 | int(head[6])<<8 | int(head[7])
	if size == 0 {
		return msg, true
	}
	nArgs := int(head[4])
	back := nArgs * 4
	if back > size {
		return nil, false
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, false
	}
	msg.Args = make(map[string]string, nArgs)
	from := 0
	for i := 0; i < nArgs; i++ {
		nameSize := int(body[from])
		from += 1
		valueSize := int(body[from])<<16 | int(body[from+1])<<8 | int(body[from+2])
		from += 3
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
