// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Config values.

package config

import (
	"time"
)

// Value is a value in config file.
type Value struct {
	Kind int16 // TokenXXX in values
	Data any   // bools, integers, strings, durations, lists, and dicts
}

func (v *Value) IsBool() bool     { return v.Kind == TokenBool }
func (v *Value) IsInteger() bool  { return v.Kind == TokenInteger }
func (v *Value) IsString() bool   { return v.Kind == TokenString }
func (v *Value) IsDuration() bool { return v.Kind == TokenDuration }
func (v *Value) IsList() bool     { return v.Kind == TokenList }
func (v *Value) IsDict() bool     { return v.Kind == TokenDict }

func (v *Value) Bool() (b bool, ok bool) {
	b, ok = v.Data.(bool)
	return
}
func (v *Value) Int64() (i64 int64, ok bool) {
	i64, ok = v.Data.(int64)
	return
}
func toInt[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](v *Value) (i T, ok bool) {
	i64, ok := v.Int64()
	i = T(i64)
	if ok && int64(i) != i64 {
		ok = false
	}
	return
}
func (v *Value) Uint32() (u32 uint32, ok bool) { return toInt[uint32](v) }
func (v *Value) Int32() (i32 int32, ok bool)   { return toInt[int32](v) }
func (v *Value) Int16() (i16 int16, ok bool)   { return toInt[int16](v) }
func (v *Value) Int8() (i8 int8, ok bool)      { return toInt[int8](v) }
func (v *Value) Int() (i int, ok bool)         { return toInt[int](v) }
func (v *Value) String() (s string, ok bool) {
	s, ok = v.Data.(string)
	return
}
func (v *Value) Duration() (d time.Duration, ok bool) {
	d, ok = v.Data.(time.Duration)
	return
}
func (v *Value) List() (list []Value, ok bool) {
	list, ok = v.Data.([]Value)
	return
}
func (v *Value) ListN(n int) (list []Value, ok bool) {
	list, ok = v.Data.([]Value)
	if ok && n >= 0 && len(list) != n {
		ok = false
	}
	return
}
func (v *Value) StringList() (list []string, ok bool) {
	l, ok := v.Data.([]Value)
	if !ok {
		return
	}
	for _, value := range l {
		if s, ok := value.String(); ok {
			list = append(list, s)
		}
	}
	return
}
func (v *Value) StringListN(n int) (list []string, ok bool) {
	l, ok := v.Data.([]Value)
	if !ok {
		return
	}
	if n >= 0 && len(l) != n {
		ok = false
		return
	}
	for _, value := range l {
		if s, ok := value.String(); ok {
			list = append(list, s)
		}
	}
	return
}
func (v *Value) Dict() (dict map[string]Value, ok bool) {
	dict, ok = v.Data.(map[string]Value)
	return
}
func (v *Value) StringDict() (dict map[string]string, ok bool) {
	d, ok := v.Data.(map[string]Value)
	if ok {
		dict = make(map[string]string)
		for name, value := range d {
			if s, ok := value.String(); ok {
				dict[name] = s
			}
		}
	}
	return
}
