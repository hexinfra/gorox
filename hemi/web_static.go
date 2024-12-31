// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Static handlets handle requests to local files and directories.

package hemi

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

func init() {
	RegisterHandlet("static", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(staticHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// staticHandlet handles requests to static files and directories.
type staticHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage  // current stage
	webapp *Webapp // the webapp to which the handlet belongs
	// States
	webRoot          string            // root dir for web files and directories
	aliasTo          []string          // from is an alias to to
	indexFile        string            // the file that will be used as index
	autoIndex        bool              // list files in directories if there is no index file?
	mimeTypes        map[string]string // defined mime types for file extensions
	defaultType      string            // mime type for file extensions that are not defined in mimeTypes
	useWebappWebRoot bool              // true if webRoot is same with webapp.webRoot
}

func (h *staticHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *staticHandlet) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *staticHandlet) OnConfigure() {
	// webRoot
	if v, ok := h.Find("webRoot"); ok {
		if dir, ok := v.String(); ok && dir != "" {
			h.webRoot = dir
		} else {
			UseExitln("invalid webRoot")
		}
	} else {
		UseExitln("webRoot is required for staticHandlet")
	}
	h.webRoot = strings.TrimRight(h.webRoot, "/")
	h.useWebappWebRoot = h.webRoot == h.webapp.webRoot
	if DebugLevel() >= 1 {
		if h.useWebappWebRoot {
			Printf("static=%s use webapp web root\n", h.Name())
		} else {
			Printf("static=%s NOT use webapp web root\n", h.Name())
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
	h.ConfigureString("indexFile", &h.indexFile, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, "index.html")

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
	h.ConfigureString("defaultType", &h.defaultType, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, "application/octet-stream")

	// autoIndex
	h.ConfigureBool("autoIndex", &h.autoIndex, false)
}
func (h *staticHandlet) OnPrepare() {
	if info, err := os.Stat(h.webRoot + "/" + h.indexFile); err == nil && !info.Mode().IsRegular() {
		EnvExitln("indexFile must be a regular file")
	}
}

func (h *staticHandlet) Handle(req Request, resp Response) (handled bool) {
	if req.MethodCode()&(MethodGET|MethodHEAD) == 0 {
		resp.SendMethodNotAllowed("GET, HEAD", nil)
		return true
	}

	var fullPath []byte
	var pathSize int
	if h.useWebappWebRoot {
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
	} else { // is directory, add indexFile to openPath
		if h.useWebappWebRoot {
			openPath = req.UnsafeMake(len(fullPath) + len(h.indexFile))
			copy(openPath, fullPath)
			copy(openPath[pathSize:], h.indexFile)
			fullPath = openPath
		} else { // custom web root
			openPath = fullPath
		}
	}

	fcache := h.stage.Fcache()
	entry, err := fcache.getEntry(openPath)
	if err != nil { // entry does not exist
		if DebugLevel() >= 1 {
			Println("entry MISS")
		}
		if entry, err = fcache.newEntry(string(openPath)); err != nil {
			if !os.IsNotExist(err) {
				h.webapp.Logf("open file error=%s\n", err.Error())
				resp.SendInternalServerError(nil)
			} else if isFile { // file not found
				resp.SendNotFound(h.webapp.text404)
			} else if h.autoIndex { // index file not found, but auto index is turned on, try list directory
				if dir, err := os.Open(WeakString(fullPath[:pathSize])); err == nil {
					staticListDir(dir, resp)
					dir.Close()
				} else if !os.IsNotExist(err) {
					h.webapp.Logf("open dir error=%s\n", err.Error())
					resp.SendInternalServerError(nil)
				} else { // directory not found
					resp.SendNotFound(h.webapp.text404)
				}
			} else { // not auto index
				resp.SendForbidden(nil)
			}
			return true
		}
	}
	if entry.isDir() {
		resp.SetStatus(StatusFound)
		resp.AddDirectoryRedirection()
		resp.SendBytes(nil)
		return true
	}

	if entry.isLarge() {
		defer entry.decRef()
	}

	date := entry.info.ModTime().Unix()
	size := entry.info.Size()
	etag, _ := resp.MakeETagFrom(date, size) // with ""
	const asOrigin = true
	if status, normal := req.EvalPreconditions(date, etag, asOrigin); !normal { // not modified, or precondition failed
		resp.SetStatus(status)
		if status == StatusNotModified {
			resp.AddHeaderBytes(bytesETag, etag)
		}
		resp.SendBytes(nil)
		return true
	}
	contentType := h.defaultType
	filePath := WeakString(openPath)
	if p := strings.LastIndex(filePath, "."); p >= 0 {
		ext := filePath[p+1:]
		if mimeType, ok := h.mimeTypes[ext]; ok {
			contentType = mimeType
		}
	}
	if !req.HasRanges() || (req.HasIfRange() && !req.EvalIfRange(date, etag, asOrigin)) {
		resp.AddHeaderBytes(bytesContentType, ConstBytes(contentType))
		resp.AddHeaderBytes(bytesAcceptRanges, bytesBytes)
		if DebugLevel() >= 2 { // TODO
			resp.AddHeaderBytes(bytesCacheControl, []byte("no-cache, no-store, must-revalidate"))
		} else {
			resp.AddHeaderBytes(bytesETag, etag)
			resp.SetLastModified(date)
		}
	} else if ranges := req.EvalRanges(size); ranges != nil { // ranges are satisfiable
		resp.pickRanges(ranges, contentType)
	} else { // ranges are not satisfiable
		resp.SendRangeNotSatisfiable(size, nil)
		return true
	}
	if entry.isSmall() {
		resp.sendText(entry.text)
	} else {
		resp.sendFile(entry.file, entry.info, false) // false means don't close on end. this file belongs to fcache
	}
	return true
}

func staticListDir(dir *os.File, resp Response) {
	fis, err := dir.Readdir(-1)
	if err != nil {
		resp.SendInternalServerError([]byte("Internal Server Error 5"))
		return
	}
	resp.Echo(`<table border="1">`)
	resp.Echo(`<tr><th>name</th><th>size(in bytes)</th><th>time</th></tr>`)
	for _, fi := range fis {
		name := fi.Name()
		size := strconv.FormatInt(fi.Size(), 10)
		date := fi.ModTime().String()
		line := `<tr><td><a href="` + staticEscape(name) + `">` + staticEscape(name) + `</a></td><td>` + size + `</td><td>` + date + `</td></tr>`
		resp.Echo(line)
	}
	resp.Echo("</table>")
}

func staticEscape(s string) string { return staticEscaper.Replace(s) }

var staticEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

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
	"txt":  "text/plain",
	"war":  "application/java-archive",
	"webm": "video/webm",
	"webp": "image/webp",
	"xls":  "application/vnd.ms-excel",
	"xml":  "text/xml",
	"zip":  "application/zip",
}
