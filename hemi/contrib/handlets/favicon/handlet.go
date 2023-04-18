// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Favicon handlets print the Gorox logo as favicon.

package favicon

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("favicon", func(name string, stage *Stage, app *App) Handlet {
		h := new(faviconHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// faviconHandlet
type faviconHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *faviconHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *faviconHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *faviconHandlet) OnConfigure() {
	// TODO
}
func (h *faviconHandlet) OnPrepare() {
	// TODO
}

func (h *faviconHandlet) Handle(req Request, resp Response) (next bool) {
	if status, pass := req.TestConditions(faviconTime, etagBytes, true); pass {
		resp.SetLastModified(faviconTime)
		resp.AddHeader("etag", etagString)
		resp.AddContentType("image/png")
		resp.SendBytes(faviconBytes)
	} else { // not modified, or precondition failed
		resp.SetStatus(status)
		if status == StatusNotModified {
			resp.AddHeader("etag", etagString)
		}
		resp.SendBytes(nil)
	}
	return
}

var (
	faviconTime = int64(1234567890)
	etagString  = `"favi-1134"`
	etagBytes   = []byte(etagString)
)

var faviconBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature

	0x00, 0x00, 0x00, 0x0D, // length = 13
	0x49, 0x48, 0x44, 0x52, // IHDR
	0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x20, 0x08, 0x06, 0x00, 0x00, 0x00, // chunk data
	0x73, 0x7A, 0x7A, 0xF4, // crc

	0x00, 0x00, 0x04, 0x35, // length = 1077
	0x49, 0x44, 0x41, 0x54, // IDAT
	0x58, 0x85, 0xC5, 0x57, 0xDD, 0x6B, 0x1C, 0x55, 0x14, 0x9F, 0xD7, 0x3C, 0xE5, 0x0F, 0x70, 0xE1, 0x8E, 0xDB, 0xC6, 0x8F,
	0x50, 0xC1, 0x6A, 0xD1, 0x28, 0x42, 0xB5, 0x28, 0x88, 0x8D, 0x5A, 0x54, 0x8A, 0x59, 0x51, 0x14, 0x8B, 0x01, 0x5F, 0x54,
	0xFA, 0x20, 0xB5, 0x14, 0x8A, 0x0F, 0xF5, 0x83, 0x90, 0x07, 0x45, 0x34, 0xCE, 0x9D, 0xDD, 0xA4, 0x4D, 0x62, 0xDD, 0xD4,
	0x26, 0xA9, 0x69, 0xEC, 0x47, 0x24, 0xA6, 0x1F, 0x86, 0x7C, 0x94, 0x68, 0x13, 0x93, 0x46, 0x4B, 0xD1, 0x46, 0x2C, 0xDD,
	0xCD, 0xDC, 0x3B, 0x33, 0xBB, 0x33, 0xBB, 0x3B, 0xB3, 0x9B, 0x9F, 0x0F, 0x59, 0x93, 0xCC, 0xDC, 0x49, 0x76, 0xF3, 0x85,
	0x07, 0xCE, 0xCB, 0x5C, 0xB8, 0xBF, 0xDF, 0xB9, 0xE7, 0xFC, 0xCE, 0x9C, 0x23, 0x49, 0x65, 0x5A, 0xB8, 0x89, 0x55, 0xCA,
	0x51, 0x56, 0x47, 0x28, 0x57, 0x08, 0xE5, 0x23, 0x84, 0xF2, 0x24, 0x51, 0x98, 0x4B, 0x14, 0xE6, 0x12, 0xCA, 0x93, 0xC5,
	0x6F, 0x8A, 0x1C, 0x35, 0xEA, 0xC2, 0x4D, 0xAC, 0xB2, 0xDC, 0x7B, 0x4B, 0x5A, 0x28, 0x66, 0x54, 0x11, 0xAA, 0xA9, 0xB2,
	0xA2, 0xD9, 0x32, 0x65, 0x28, 0xCB, 0x15, 0xCD, 0x26, 0x94, 0xAB, 0xA1, 0x98, 0x51, 0xB5, 0x76, 0xE0, 0x46, 0x54, 0x10,
	0x45, 0x6B, 0x90, 0x29, 0x73, 0xCA, 0x06, 0x16, 0xDD, 0x21, 0x8A, 0xD6, 0x10, 0x8A, 0xA3, 0x62, 0x75, 0xE0, 0x34, 0xB1,
	0x95, 0x50, 0x36, 0xBE, 0x0E, 0x60, 0x8F, 0x13, 0xCA, 0xC6, 0x43, 0xD4, 0xD8, 0x5A, 0x16, 0xB8, 0x1C, 0xE3, 0xF7, 0xCB,
	0x94, 0x27, 0x36, 0x0A, 0x7C, 0xD1, 0x79, 0x82, 0xD0, 0xC4, 0xF6, 0x92, 0x91, 0x6F, 0x0E, 0xF8, 0x22, 0x89, 0x65, 0x5F,
	0x22, 0xD4, 0x38, 0x53, 0xB1, 0x91, 0xCF, 0xBE, 0x6C, 0x3A, 0x14, 0x6D, 0x22, 0xB0, 0x26, 0x8A, 0x05, 0xB7, 0x21, 0x20,
	0x77, 0xC7, 0x18, 0x1E, 0xFB, 0x56, 0xC7, 0x53, 0x27, 0x74, 0xEC, 0x8C, 0xEB, 0xA8, 0x6E, 0xF6, 0x91, 0x50, 0x79, 0x83,
	0x37, 0xFA, 0x98, 0x51, 0xB5, 0x52, 0xB5, 0xDF, 0x13, 0x63, 0x78, 0x3C, 0xAE, 0xE3, 0xE5, 0x1E, 0x13, 0xEF, 0xF4, 0xA7,
	0xF0, 0xD1, 0x90, 0x85, 0xD8, 0x44, 0x06, 0xBD, 0x37, 0x72, 0xB8, 0x72, 0xDB, 0xC5, 0x87, 0x83, 0x16, 0x1E, 0x3D, 0xAE,
	0xE3, 0xCB, 0x5F, 0x33, 0xF8, 0x83, 0xE7, 0x51, 0x98, 0x83, 0x60, 0x09, 0xBB, 0x80, 0xFE, 0x9B, 0x0E, 0xB6, 0xB5, 0x70,
	0xC8, 0x94, 0x39, 0x1E, 0x89, 0x12, 0xAA, 0xA9, 0x6B, 0x8D, 0xF6, 0x4E, 0xCA, 0xF0, 0xF6, 0x8F, 0x69, 0x58, 0x4E, 0x00,
	0xAA, 0xCF, 0x26, 0xB5, 0xFC, 0x12, 0x65, 0x70, 0x55, 0x92, 0xA4, 0x62, 0x87, 0x0B, 0x68, 0x32, 0x0F, 0xB4, 0x72, 0x3C,
	0x73, 0xD2, 0xC0, 0x1B, 0x67, 0x4D, 0xBC, 0x7F, 0x21, 0x8D, 0x0F, 0x2E, 0x79, 0xFD, 0xC0, 0xC5, 0x34, 0x22, 0xBD, 0x26,
	0x5E, 0x3C, 0x65, 0xC2, 0x2D, 0x94, 0xC4, 0x06, 0x00, 0x1C, 0x19, 0xB2, 0x17, 0x31, 0x14, 0xCD, 0x0E, 0x37, 0xB1, 0x4A,
	0x49, 0x8E, 0xB2, 0xBA, 0x52, 0x51, 0x86, 0x55, 0x86, 0x87, 0xDA, 0x39, 0x9E, 0xEB, 0x32, 0xF0, 0xD6, 0xF9, 0x14, 0x0E,
	0x5D, 0xB6, 0xF0, 0xC5, 0x2F, 0x19, 0xEC, 0xED, 0x31, 0x31, 0xA5, 0xE5, 0x03, 0xC1, 0x9C, 0x02, 0x60, 0xB9, 0x73, 0x0B,
	0xE9, 0x28, 0xCC, 0x01, 0x35, 0xED, 0xBA, 0xBF, 0x3F, 0x44, 0x24, 0x42, 0xB9, 0xB2, 0xD6, 0xE7, 0x7F, 0xFD, 0x4C, 0x4A,
	0x00, 0xEE, 0xFB, 0xCB, 0xC1, 0x23, 0xDF, 0xE8, 0x81, 0xE4, 0x05, 0x45, 0x50, 0xAE, 0x48, 0x84, 0xF2, 0x11, 0x7F, 0x4E,
	0x77, 0xB4, 0x71, 0xD4, 0x76, 0x1A, 0x78, 0xF3, 0x5C, 0x0A, 0x07, 0x2F, 0x59, 0xF8, 0x6C, 0xCC, 0x46, 0x7C, 0x3A, 0x8B,
	0x81, 0x19, 0x07, 0xD7, 0x58, 0x1E, 0x7A, 0xB6, 0x80, 0x63, 0x93, 0x59, 0x74, 0x5E, 0xCF, 0x09, 0x04, 0x5E, 0x3B, 0x93,
	0x2A, 0x5F, 0x92, 0x94, 0x8D, 0x4A, 0x84, 0xB2, 0xE4, 0x5A, 0xA2, 0x0F, 0xAB, 0x0C, 0x5A, 0x46, 0x4C, 0x7E, 0x4D, 0x31,
	0xFA, 0xC8, 0x69, 0x13, 0xAF, 0xFE, 0xE0, 0xF5, 0x3D, 0xDD, 0x86, 0x9F, 0xC0, 0xAC, 0x44, 0x14, 0xE6, 0x2E, 0xFD, 0x58,
	0xDD, 0xC2, 0xB0, 0xAB, 0x43, 0x47, 0xE4, 0xB4, 0x89, 0xF7, 0x7E, 0x4A, 0xE3, 0x93, 0x61, 0x1B, 0xCD, 0x13, 0x19, 0xB4,
	0x4D, 0x65, 0x3D, 0xBE, 0xBB, 0xD3, 0x10, 0xC0, 0xF5, 0xEC, 0x1C, 0x64, 0xCA, 0x70, 0xDF, 0x51, 0x8E, 0x20, 0x4D, 0xF4,
	0xDE, 0xC8, 0xF9, 0x03, 0xC9, 0x0B, 0x04, 0x96, 0x93, 0xDA, 0x8E, 0x36, 0x8E, 0xDD, 0xC5, 0xB4, 0x1C, 0xB8, 0x68, 0xA1,
	0xBE, 0x4F, 0xCC, 0xFF, 0xF0, 0x2D, 0x17, 0x32, 0x65, 0x78, 0xE9, 0x7B, 0x33, 0xB0, 0x30, 0x1B, 0xAF, 0xD8, 0x01, 0x04,
	0x28, 0xF7, 0xA4, 0xE0, 0xC1, 0x56, 0x8E, 0xA7, 0x4F, 0x1A, 0x78, 0xB6, 0xCB, 0x40, 0x6D, 0x67, 0xB0, 0xEF, 0xEA, 0xD0,
	0x71, 0x64, 0xC8, 0x12, 0x00, 0x8E, 0x4D, 0x66, 0x21, 0x53, 0x86, 0x83, 0x97, 0xC4, 0x33, 0x00, 0xA8, 0x3F, 0x9F, 0x0A,
	0x48, 0x81, 0xAF, 0x08, 0xFD, 0x85, 0xB8, 0xAF, 0x58, 0x88, 0x9F, 0x8F, 0xD9, 0xE8, 0x98, 0xCE, 0xE2, 0xC2, 0xDF, 0x0E,
	0x5A, 0xA7, 0xB2, 0x50, 0xC7, 0x33, 0x02, 0xC0, 0xA1, 0xCB, 0x16, 0x64, 0xCA, 0xB0, 0xEF, 0x5C, 0x0A, 0x83, 0xFF, 0x38,
	0xC2, 0xF9, 0xCE, 0xB8, 0x20, 0xC3, 0xD1, 0x35, 0xCB, 0xF0, 0xE8, 0x64, 0x56, 0x00, 0xD8, 0xDB, 0x63, 0x2E, 0x9C, 0xC7,
	0xA7, 0xBD, 0xE7, 0x96, 0x33, 0x87, 0xB0, 0xEA, 0x53, 0x81, 0xA2, 0x51, 0x49, 0x8E, 0x1A, 0x9E, 0x46, 0xB4, 0xAD, 0x85,
	0xE3, 0xC9, 0x13, 0x3A, 0x5E, 0xE9, 0x35, 0xB1, 0x7F, 0x20, 0x8D, 0x4F, 0x47, 0x6C, 0xB4, 0xFC, 0x96, 0xC1, 0xD9, 0x3F,
	0x73, 0x18, 0xBB, 0xED, 0xE2, 0x6A, 0xD2, 0xC5, 0x77, 0xBF, 0xE7, 0xD0, 0x30, 0x6A, 0x0B, 0x04, 0x86, 0x6E, 0xB9, 0x18,
	0x98, 0x71, 0x70, 0x35, 0xE9, 0x0A, 0x45, 0x38, 0x96, 0x70, 0x83, 0x64, 0x18, 0x59, 0xB6, 0x15, 0x07, 0xF9, 0x16, 0x95,
	0xE1, 0xE1, 0xF6, 0xF9, 0x1A, 0x79, 0xBE, 0x5B, 0x54, 0xC1, 0x4A, 0x76, 0xFC, 0x5A, 0xD6, 0x7B, 0x9F, 0xC2, 0xEC, 0x85,
	0xE1, 0x95, 0x50, 0x2E, 0xFC, 0x8C, 0xB6, 0xA8, 0x0C, 0x35, 0xED, 0x1C, 0x7B, 0xBA, 0x0D, 0xD4, 0xF7, 0xA5, 0xB0, 0x7F,
	0x20, 0xED, 0xF1, 0x17, 0x4E, 0x19, 0xE8, 0x0A, 0x68, 0x44, 0xCB, 0xD9, 0xE1, 0x41, 0xCB, 0xFF, 0xFC, 0xD1, 0xB2, 0x7F,
	0xC7, 0x41, 0xA9, 0x39, 0xFC, 0xB3, 0x85, 0x7B, 0x9B, 0x19, 0x3E, 0x1E, 0xB6, 0x71, 0xD3, 0x14, 0x1B, 0xD2, 0x1C, 0x80,
	0x19, 0xB3, 0x80, 0xAE, 0xEB, 0x39, 0xBC, 0xDB, 0x9F, 0x46, 0x75, 0x33, 0x5F, 0x7A, 0x9F, 0x73, 0xC7, 0xD7, 0xC9, 0xBB,
	0x7C, 0x03, 0x09, 0x5F, 0xD7, 0x40, 0xF2, 0x9F, 0x7C, 0x6B, 0x3B, 0x0D, 0x3C, 0xD1, 0x21, 0x0E, 0x21, 0x9E, 0xE8, 0xFD,
	0x03, 0x89, 0x24, 0x49, 0x52, 0x28, 0x8E, 0xFF, 0x77, 0x24, 0x9B, 0x1F, 0x4A, 0x8D, 0x4D, 0x1D, 0x4A, 0x89, 0xC2, 0x92,
	0xA1, 0xAF, 0x12, 0x2B, 0x2F, 0x2B, 0x84, 0x26, 0xB6, 0x6F, 0x06, 0x09, 0xA2, 0xB0, 0x64, 0xC9, 0xB1, 0x7C, 0xE9, 0x4B,
	0x6C, 0xE8, 0x62, 0xA2, 0x68, 0x13, 0x25, 0x23, 0x0F, 0xAC, 0x09, 0x95, 0xAF, 0x73, 0x35, 0xE3, 0x0E, 0x51, 0xF9, 0xEA,
	0x57, 0x33, 0x0F, 0x91, 0x98, 0x51, 0x45, 0x28, 0x5F, 0xFD, 0x72, 0xAA, 0x68, 0x51, 0x41, 0x6A, 0xEB, 0xB1, 0x70, 0x13,
	0xAB, 0x24, 0x94, 0x45, 0xE6, 0xD7, 0x73, 0x36, 0x4A, 0x28, 0x9B, 0x95, 0x29, 0xCB, 0xCB, 0x94, 0xE5, 0x09, 0x65, 0xB3,
	0x44, 0x61, 0xA3, 0x44, 0xD1, 0x28, 0xA1, 0x2C, 0xB2, 0x9A, 0xF5, 0xFC, 0x5F, 0x94, 0x28, 0xA7, 0x9D, // chunk data
	0xFD, 0x4C, 0x27, 0xCF, // crc

	0x00, 0x00, 0x00, 0x00, // length = 0
	0x49, 0x45, 0x4E, 0x44, // IEND
	0xAE, 0x42, 0x60, 0x82, // crc
}
