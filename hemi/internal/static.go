// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Static handlers serve requests to local file system.

package internal

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"os"
	"strconv"
	"strings"
)

func init() {
	RegisterHandler("static", func(name string, stage *Stage, app *App) Handler {
		h := new(staticHandler)
		h.init(name, stage, app)
		return h
	})
}

// staticHandler
type staticHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage // current stage
	app   *App
	// States
	webRoot     string            // root dir for the web
	aliasTo     []string          // from is an alias to to
	indexFile   string            // ...
	autoIndex   bool              // ...
	enableCache bool              // ...
	mimeTypes   map[string]string // ...
	defaultType string            // ...
	useAppRoot  bool              // true if webRoot is same with app.webRoot
}

func (h *staticHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app
}

func (h *staticHandler) OnConfigure() {
	// webRoot
	if v, ok := h.Find("webRoot"); ok {
		if dir, ok := v.String(); ok && dir != "" {
			h.webRoot = dir
		} else {
			UseExitln("invalid webRoot")
		}
	} else {
		UseExitln("webRoot is required for staticHandler")
	}
	h.webRoot = strings.TrimRight(h.webRoot, "/")
	h.useAppRoot = h.webRoot == h.app.webRoot
	if Debug(1) {
		if h.useAppRoot {
			fmt.Printf("static=%s use app web root\n", h.Name())
		} else {
			fmt.Printf("static=%s NOT use app web root\n", h.Name())
		}
	}
	// aliasTo
	if v, ok := h.Find("aliasTo"); ok {
		if fromTo, ok := v.StringListN(2); ok {
			h.aliasTo = fromTo
		} else {
			UseExitln("invalid aliasTo")
		}
	} else {
		h.aliasTo = nil
	}
	// indexFile
	h.ConfigureString("indexFile", &h.indexFile, func(value string) bool { return value != "" }, "index.html")
	// autoIndex
	h.ConfigureBool("autoIndex", &h.autoIndex, false)
	// enableCache
	h.ConfigureBool("enableCache", &h.enableCache, false)
	// mimeTypes
	if v, ok := h.Find("mimeTypes"); ok {
		if mimeTypes, ok := v.StringDict(); ok {
			h.mimeTypes = make(map[string]string)
			for ext, mimeType := range staticDefaultMimeTypes {
				h.mimeTypes[ext] = mimeType
			}
			for ext, mimeType := range mimeTypes { // overwrite default
				h.mimeTypes[ext] = mimeType
			}
		} else {
			UseExitln("invalid mimeTypes")
		}
	} else {
		h.mimeTypes = staticDefaultMimeTypes
	}
	// defaultType
	h.ConfigureString("defaultType", &h.defaultType, func(value string) bool { return value != "" }, "application/octet-stream")
}
func (h *staticHandler) OnPrepare() {
	if info, err := os.Stat(h.webRoot + "/" + h.indexFile); err == nil && !info.Mode().IsRegular() {
		EnvExitln("indexFile must be a regular file")
	}
}
func (h *staticHandler) OnShutdown() {
}

func (h *staticHandler) Handle(req Request, resp Response) (next bool) {
	if req.MethodCode()&(MethodGET|MethodHEAD) == 0 {
		resp.SendMethodNotAllowed("GET, HEAD", nil)
		return
	}

	var fullPath []byte
	var pathSize int
	if h.useAppRoot {
		fullPath = req.unsafeAbsPath()
		pathSize = len(fullPath)
	} else { // custom web root
		userPath := req.UnsafePath()
		fullPath = req.UnsafeMake(len(h.webRoot) + len(userPath) + len(h.indexFile))
		pathSize = copy(fullPath, h.webRoot)
		pathSize += copy(fullPath[pathSize:], userPath)
	}
	isFile := fullPath[pathSize-1] != '/'
	var openPath []byte
	if isFile {
		openPath = fullPath[:pathSize]
	} else { // is directory
		if h.useAppRoot {
			openPath = req.UnsafeMake(len(fullPath) + len(h.indexFile))
			copy(openPath, fullPath)
			copy(openPath[pathSize:], h.indexFile)
			fullPath = openPath
		} else { // custom web root
			openPath = fullPath
		}
	}

	file, err := os.Open(risky.WeakString(openPath)) // DO NOT use risky.WeakString if file is to be stored outside of current stream lifetime!
	if err != nil {
		if !os.IsNotExist(err) {
			h.app.Logf("open file error=%s\n", err.Error())
			resp.SendInternalServerError(nil)
		} else if isFile { // file not found
			resp.SendNotFound(nil)
		} else if h.autoIndex { // index file not found, but auto index is turned on
			if file, err := os.Open(risky.WeakString(fullPath[:pathSize])); err == nil {
				h.listDir(file, resp)
				file.Close()
			} else if !os.IsNotExist(err) {
				h.app.Logf("open file error=%s\n", err.Error())
				resp.SendInternalServerError(nil)
			} else { // directory not found
				resp.SendNotFound(nil)
			}
		} else { // not auto index
			resp.SendForbidden(nil)
		}
		return
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		h.app.Logf("file stat error=%s\n", err.Error())
		resp.SendInternalServerError(nil)
		return
	}
	if info.IsDir() {
		file.Close()
		resp.SetStatus(StatusFound)
		resp.AddDirectoryRedirection()
		resp.SendBytes(nil)
		return
	}

	modTime := info.ModTime().Unix()
	etag, _ := resp.makeETagFrom(modTime, info.Size()) // with ""
	if status, pass := req.TestConditions(modTime, etag, true); pass {
		resp.SetLastModified(modTime)
		resp.SetETagBytes(etag)
		resp.SetAcceptBytesRange()
		contentType := h.defaultType
		filePath := risky.WeakString(openPath)
		if pDot := strings.LastIndex(filePath, "."); pDot >= 0 {
			ext := filePath[pDot+1:]
			if mimeType, ok := h.mimeTypes[ext]; ok {
				contentType = mimeType
			}
		}
		resp.AddContentType(contentType)
		resp.sendFile(file, info, true)
	} else { // not modified, or precondition failed
		resp.SetStatus(status)
		if status == StatusNotModified {
			resp.SetETagBytes(etag)
		}
		resp.SendBytes(nil)
	}

	return
}

func (h *staticHandler) listDir(dir *os.File, resp Response) {
	fis, err := dir.Readdir(-1)
	if err != nil {
		resp.SendInternalServerError([]byte("Internal Server Error 5"))
		return
	}
	resp.Push(`<table border="1">`)
	resp.Push(`<tr><th>name</th><th>size(in bytes)</th><th>time</th></tr>`)
	for _, fi := range fis {
		name := fi.Name()
		size := strconv.FormatInt(fi.Size(), 10)
		modTime := fi.ModTime().String()
		line := `<tr><td><a href="` + staticHTMLEscape(name) + `">` + staticHTMLEscape(name) + `</a></td><td>` + size + `</td><td>` + modTime + `</td></tr>`
		resp.Push(line)
	}
	resp.Push("</table>")
}

func staticHTMLEscape(s string) string {
	return staticHTMLEscaper.Replace(s)
}

var staticHTMLEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

var staticDefaultMimeTypes = map[string]string{
	"7z":   "application/x-7z-compressed",
	"atom": "application/atom+xml",
	"bin":  "application/octet-stream",
	"bmp":  "image/x-ms-bmp",
	"css":  "text/css",
	"deb":  "application/octet-stream",
	"dll":  "application/octet-stream",
	"doc":  "application/msword",
	"dmg":  "application/octet-stream",
	"exe":  "application/octet-stream",
	"flv":  "video/x-flv",
	"gif":  "image/gif",
	"htm":  "text/html",
	"html": "text/html",
	"ico":  "image/x-icon",
	"img":  "application/octet-stream",
	"iso":  "application/octet-stream",
	"jar":  "application/java-archive",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"js":   "application/javascript",
	"json": "application/json",
	"m4a":  "audio/x-m4a",
	"mov":  "video/quicktime",
	"mp3":  "audio/mpeg",
	"mp4":  "video/mp4",
	"mpeg": "video/mpeg",
	"mpg":  "video/mpeg",
	"pdf":  "application/pdf",
	"png":  "image/png",
	"ppt":  "application/vnd.ms-powerpoint",
	"ps":   "application/postscript",
	"rar":  "application/x-rar-compressed",
	"rss":  "application/rss+xml",
	"rtf":  "application/rtf",
	"svg":  "image/svg+xml",
	"txt":  "text/plain; charset=utf-8",
	"war":  "application/java-archive",
	"webm": "video/webm",
	"webp": "image/webp",
	"xls":  "application/vnd.ms-excel",
	"xml":  "text/xml",
	"zip":  "application/zip",
}
