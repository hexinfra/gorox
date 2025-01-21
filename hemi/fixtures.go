// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Fixtures are singleton components.

package hemi

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// fixture component.
//
// Fixtures only exist in internal, and are created by stage.
// Some critical functions, like clock and resolv, are implemented as fixtures.
//
// Fixtures are singletons in stage.
type fixture interface {
	// Imports
	Component
	// Methods
	run() // runner
}

// fixture_
type fixture_ struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
}

func (f *fixture_) onCreate(compName string, stage *Stage) {
	f.MakeComp(compName)
	f.stage = stage
}

const signClock = "clock"

func init() {
	registerFixture(signClock)
}

func createClock(stage *Stage) *clockFixture {
	clock := new(clockFixture)
	clock.onCreate(stage)
	clock.setShell(clock)
	return clock
}

// clockFixture
type clockFixture struct {
	// Parent
	fixture_
	// States
	resolution time.Duration
	date       atomic.Int64 // 4, 4+4 4 4+4+4+4 4+4:4+4:4+4 = 56bit
}

func (f *clockFixture) onCreate(stage *Stage) {
	f.fixture_.onCreate(signClock, stage)
	f.resolution = 100 * time.Millisecond
	f.date.Store(0x7394804991b60000) // Sun, 06 Nov 1994 08:49:37
}
func (f *clockFixture) OnShutdown() { close(f.ShutChan) } // notifies run()

func (f *clockFixture) OnConfigure() {
}
func (f *clockFixture) OnPrepare() {
}

func (f *clockFixture) run() { // runner
	f.LoopRun(f.resolution, func(now time.Time) {
		now = now.UTC()
		weekday := now.Weekday()       // weekday: 0-6
		year, month, day := now.Date() // month: 1-12
		hour, minute, second := now.Clock()
		date := int64(0)
		date |= int64(second%10) << 60
		date |= int64(second/10) << 56
		date |= int64(minute%10) << 52
		date |= int64(minute/10) << 48
		date |= int64(hour%10) << 44
		date |= int64(hour/10) << 40
		date |= int64(year%10) << 36
		date |= int64(year/10%10) << 32
		date |= int64(year/100%10) << 28
		date |= int64(year/1000) << 24
		date |= int64(month) << 20
		date |= int64(day%10) << 16
		date |= int64(day/10) << 12
		date |= int64(weekday) << 8
		f.date.Store(date)
	})
	if DebugLevel() >= 2 {
		Println("clock done")
	}
	f.stage.DecSub() // clock
}

func (f *clockFixture) writeDate1(dst []byte) int {
	i := copy(dst, "date: ")
	i += f.writeDate(dst[i:])
	dst[i] = '\r'
	dst[i+1] = '\n'
	return i + 2
}
func (f *clockFixture) writeDate(dst []byte) int {
	date := f.date.Load()
	s := clockDayString[3*(date>>8&0xf):]
	dst[0] = s[0] // 'S'
	dst[1] = s[1] // 'u'
	dst[2] = s[2] // 'n'
	dst[3] = ','
	dst[4] = ' '
	dst[5] = byte(date>>12&0xf) + '0' // '0'
	dst[6] = byte(date>>16&0xf) + '0' // '6'
	dst[7] = ' '
	s = clockMonthString[3*(date>>20&0xf-1):]
	dst[8] = s[0]  // 'N'
	dst[9] = s[1]  // 'o'
	dst[10] = s[2] // 'v'
	dst[11] = ' '
	dst[12] = byte(date>>24&0xf) + '0' // '1'
	dst[13] = byte(date>>28&0xf) + '0' // '9'
	dst[14] = byte(date>>32&0xf) + '0' // '9'
	dst[15] = byte(date>>36&0xf) + '0' // '4'
	dst[16] = ' '
	dst[17] = byte(date>>40&0xf) + '0' // '0'
	dst[18] = byte(date>>44&0xf) + '0' // '8'
	dst[19] = ':'
	dst[20] = byte(date>>48&0xf) + '0' // '4'
	dst[21] = byte(date>>52&0xf) + '0' // '9'
	dst[22] = ':'
	dst[23] = byte(date>>56&0xf) + '0' // '3'
	dst[24] = byte(date>>60&0xf) + '0' // '7'
	dst[25] = ' '
	dst[26] = 'G'
	dst[27] = 'M'
	dst[28] = 'T'
	return clockHTTPDateSize
}

func clockWriteHTTPDate1(dst []byte, fieldName []byte, unixTime int64) int {
	i := copy(dst, fieldName)
	dst[i] = ':'
	dst[i+1] = ' '
	i += 2
	date := time.Unix(unixTime, 0)
	date = date.UTC()
	i += clockWriteHTTPDate(dst[i:], date)
	dst[i] = '\r'
	dst[i+1] = '\n'
	return i + 2
}
func clockWriteHTTPDate(dst []byte, date time.Time) int {
	if len(dst) < clockHTTPDateSize {
		BugExitln("invalid buffer for clockWriteHTTPDate")
	}
	s := clockDayString[3*date.Weekday():]
	dst[0] = s[0] // 'S'
	dst[1] = s[1] // 'u'
	dst[2] = s[2] // 'n'
	dst[3] = ','
	dst[4] = ' '
	year, month, day := date.Date() // month: 1-12
	dst[5] = byte(day/10) + '0'     // '0'
	dst[6] = byte(day%10) + '0'     // '6'
	dst[7] = ' '
	s = clockMonthString[3*(month-1):]
	dst[8] = s[0]  // 'N'
	dst[9] = s[1]  // 'o'
	dst[10] = s[2] // 'v'
	dst[11] = ' '
	dst[12] = byte(year/1000) + '0'   // '1'
	dst[13] = byte(year/100%10) + '0' // '9'
	dst[14] = byte(year/10%10) + '0'  // '9'
	dst[15] = byte(year%10) + '0'     // '4'
	dst[16] = ' '
	hour, minute, second := date.Clock()
	dst[17] = byte(hour/10) + '0' // '0'
	dst[18] = byte(hour%10) + '0' // '8'
	dst[19] = ':'
	dst[20] = byte(minute/10) + '0' // '4'
	dst[21] = byte(minute%10) + '0' // '9'
	dst[22] = ':'
	dst[23] = byte(second/10) + '0' // '3'
	dst[24] = byte(second%10) + '0' // '7'
	dst[25] = ' '
	dst[26] = 'G'
	dst[27] = 'M'
	dst[28] = 'T'
	return clockHTTPDateSize
}

func clockParseHTTPDate(date []byte) (int64, bool) {
	// format 0: Sun, 06 Nov 1994 08:49:37 GMT
	// format 1: Sunday, 06-Nov-94 08:49:37 GMT
	// format 2: Sun Nov  6 08:49:37 1994
	var format int
	fore, edge := 0, len(date)
	if n := len(date); n == clockHTTPDateSize {
		format = 0
		fore = 5 // skip 'Sun, ', stops at '0'
	} else if n >= 30 && n <= 33 {
		format = 1
		for fore < edge && date[fore] != ' ' { // skip 'Sunday, ', stops at '0'
			fore++
		}
		if edge-fore != 23 {
			return 0, false
		}
		fore++
	} else if n == clockASCTimeSize {
		format = 2
		fore = 4 // skip 'Sun ', stops at 'N'
	} else {
		return 0, false
	}
	var year, month, day, hour, minute, second int
	var b, b0, b1, b2, b3 byte
	if format != 2 {
		if b0, b1 = date[fore], date[fore+1]; b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			day = int(b0-'0')*10 + int(b1-'0')
		} else {
			return 0, false
		}
		fore += 3
		if b = date[fore-1]; (format == 0 && b != ' ') || (format == 1 && b != '-') {
			return 0, false
		}
	}
	hash := uint16(date[fore]) + uint16(date[fore+1]) + uint16(date[fore+2])
	m := clockMonthTable[clockMonthFind(hash)]
	if m.hash == hash && string(date[fore:fore+3]) == clockMonthString[m.from:m.edge] {
		month = int(m.month)
	} else {
		return 0, false
	}
	fore += 4
	if b = date[fore-1]; (format == 1 && b != '-') || (format != 1 && b != ' ') {
		return 0, false
	}
	if format == 0 {
		b0, b1, b2, b3 = date[fore], date[fore+1], date[fore+2], date[fore+3]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' && b2 >= '0' && b2 <= '9' && b3 >= '0' && b3 <= '9' {
			year = int(b0-'0')*1000 + int(b1-'0')*100 + int(b2-'0')*10 + int(b3-'0')
			fore += 5
		} else {
			return 0, false
		}
	} else if format == 1 {
		b0, b1 = date[fore], date[fore+1]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			year = int(b0-'0')*10 + int(b1-'0')
			if year < 70 {
				year += 2000
			} else {
				year += 1900
			}
			fore += 3
		} else {
			return 0, false
		}
	} else {
		b0, b1 = date[fore], date[fore+1]
		if b0 == ' ' {
			b0 = '0'
		}
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
			day = int(b0-'0')*10 + int(b1-'0')
		} else {
			return 0, false
		}
		fore += 3
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		hour = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		minute = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	b0, b1 = date[fore], date[fore+1]
	if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' {
		second = int(b0-'0')*10 + int(b1-'0')
		fore += 3
	} else {
		return 0, false
	}
	if date[fore-1] != ' ' || date[fore-4] != ':' || date[fore-7] != ':' || date[fore-10] != ' ' || hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	if format == 2 {
		b0, b1, b2, b3 = date[fore], date[fore+1], date[fore+2], date[fore+3]
		if b0 >= '0' && b0 <= '9' && b1 >= '0' && b1 <= '9' && b2 >= '0' && b2 <= '9' && b3 >= '0' && b3 <= '9' {
			year = int(b0-'0')*1000 + int(b1-'0')*100 + int(b2-'0')*10 + int(b3-'0')
		} else {
			return 0, false
		}
	} else if date[fore] != 'G' || date[fore+1] != 'M' || date[fore+2] != 'T' {
		return 0, false
	}
	leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
	if day == 29 && month == 2 {
		if !leap {
			return 0, false
		}
	} else if day > int(m.days) {
		return 0, false
	}
	days := int(m.past)
	if year > 0 {
		year--
		days += (year/4 - year/100 + year/400 + 1) // year 0000 is a leap year
		days += (year + 1) * 365
	}
	if leap && month > 2 {
		days++
	}
	days += (day - 1) // today has not past
	days -= 719528    // total days between [0000-01-01 00:00:00, 1970-01-01 00:00:00)
	return int64(days)*86400 + int64(hour*3600+minute*60+second), true
}

const ( // clock related
	clockHTTPDateSize = len("Sun, 06 Nov 1994 08:49:37 GMT")
	clockASCTimeSize  = len("Sun Nov  6 08:49:37 1994")
	clockDayString    = "SunMonTueWedThuFriSat"
	clockMonthString  = "JanFebMarAprMayJunJulAugSepOctNovDec"
)

var ( // perfect hash table for months
	clockMonthTable = [12]struct {
		hash  uint16
		from  int8
		edge  int8
		month int8
		days  int8
		past  int16
	}{
		0:  {285, 21, 24, 8, 31, 212},  // Aug
		1:  {296, 24, 27, 9, 30, 243},  // Sep
		2:  {268, 33, 36, 12, 31, 334}, // Dec
		3:  {288, 6, 9, 3, 31, 59},     // Mar
		4:  {301, 15, 18, 6, 30, 151},  // Jun
		5:  {295, 12, 15, 5, 31, 120},  // May
		6:  {307, 30, 33, 11, 30, 304}, // Nov
		7:  {299, 18, 21, 7, 31, 181},  // Jul
		8:  {294, 27, 30, 10, 31, 273}, // Oct
		9:  {291, 9, 12, 4, 30, 90},    // Apr
		10: {269, 3, 6, 2, 28, 31},     // Feb
		11: {281, 0, 3, 1, 31, 0},      // Jan
	}
	clockMonthFind = func(hash uint16) int { return (5509728 / int(hash)) % len(clockMonthTable) }
)

const signFcache = "fcache"

func init() {
	registerFixture(signFcache)
}

func createFcache(stage *Stage) *fcacheFixture {
	fcache := new(fcacheFixture)
	fcache.onCreate(stage)
	fcache.setShell(fcache)
	return fcache
}

// fcacheFixture caches file descriptors and contents.
type fcacheFixture struct {
	// Parent
	fixture_
	// States
	smallFileSize int64 // what size is considered as small file
	maxSmallFiles int32 // max number of small files. for small files, contents are cached
	maxLargeFiles int32 // max number of large files. for large files, *os.File are cached
	cacheTimeout  time.Duration
	rwMutex       sync.RWMutex // protects entries below
	entries       map[string]*fcacheEntry
}

func (f *fcacheFixture) onCreate(stage *Stage) {
	f.fixture_.onCreate(signFcache, stage)
	f.entries = make(map[string]*fcacheEntry)
}
func (f *fcacheFixture) OnShutdown() { close(f.ShutChan) } // notifies run()

func (f *fcacheFixture) OnConfigure() {
	// .smallFileSize
	f.ConfigureInt64("smallFileSize", &f.smallFileSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".smallFileSize has an invalid value")
	}, _64K1)

	// .maxSmallFiles
	f.ConfigureInt32("maxSmallFiles", &f.maxSmallFiles, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxSmallFiles has an invalid value")
	}, 1000)

	// .maxLargeFiles
	f.ConfigureInt32("maxLargeFiles", &f.maxLargeFiles, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxLargeFiles has an invalid value")
	}, 500)

	// .cacheTimeout
	f.ConfigureDuration("cacheTimeout", &f.cacheTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".cacheTimeout has an invalid value")
	}, 1*time.Second)
}
func (f *fcacheFixture) OnPrepare() {
}

func (f *fcacheFixture) run() { // runner
	f.LoopRun(time.Second, func(now time.Time) {
		f.rwMutex.Lock()
		for path, entry := range f.entries {
			if entry.last.After(now) {
				continue
			}
			if entry.isLarge() {
				entry.decRef()
			}
			delete(f.entries, path)
			if DebugLevel() >= 2 {
				Printf("fcache entry deleted: %s\n", path)
			}
		}
		f.rwMutex.Unlock()
	})
	f.rwMutex.Lock()
	f.entries = nil
	f.rwMutex.Unlock()

	if DebugLevel() >= 2 {
		Println("fcache done")
	}
	f.stage.DecSub() // fcache
}

func (f *fcacheFixture) getEntry(path []byte) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	defer f.rwMutex.RUnlock()

	if entry, ok := f.entries[WeakString(path)]; ok {
		if entry.isLarge() {
			entry.addRef()
		}
		return entry, nil
	} else {
		return nil, fcacheNotExist
	}
}

var fcacheNotExist = errors.New("entry not exist")

func (f *fcacheFixture) newEntry(path string) (*fcacheEntry, error) {
	f.rwMutex.RLock()
	if entry, ok := f.entries[path]; ok {
		if entry.isLarge() {
			entry.addRef()
		}

		f.rwMutex.RUnlock()
		return entry, nil
	}
	f.rwMutex.RUnlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	entry := new(fcacheEntry)
	if info.IsDir() {
		entry.kind = fcacheKindDir
		file.Close()
	} else if fileSize := info.Size(); fileSize <= f.smallFileSize {
		text := make([]byte, fileSize)
		if _, err := io.ReadFull(file, text); err != nil {
			file.Close()
			return nil, err
		}
		entry.kind = fcacheKindSmall
		entry.info = info
		entry.text = text
		file.Close()
	} else { // large file
		entry.kind = fcacheKindLarge
		entry.file = file
		entry.info = info
		entry.nRef.Store(1) // current caller
	}
	entry.last = time.Now().Add(f.cacheTimeout)

	f.rwMutex.Lock()
	f.entries[path] = entry
	f.rwMutex.Unlock()

	return entry, nil
}

// fcacheEntry
type fcacheEntry struct {
	kind int8         // see fcacheKindXXX
	file *os.File     // only for large file
	info os.FileInfo  // only for files, not directories
	text []byte       // content of small file
	last time.Time    // expire time
	nRef atomic.Int64 // only for large file
}

const ( // fcache entry kinds
	fcacheKindDir = iota
	fcacheKindSmall
	fcacheKindLarge
)

func (e *fcacheEntry) isDir() bool   { return e.kind == fcacheKindDir }
func (e *fcacheEntry) isLarge() bool { return e.kind == fcacheKindLarge }
func (e *fcacheEntry) isSmall() bool { return e.kind == fcacheKindSmall }

func (e *fcacheEntry) addRef() {
	e.nRef.Add(1)
}
func (e *fcacheEntry) decRef() {
	if e.nRef.Add(-1) < 0 {
		if DebugLevel() >= 2 {
			Printf("fcache large entry closed: %s\n", e.file.Name())
		}
		e.file.Close()
	}
}

const signResolv = "resolv"

func init() {
	registerFixture(signResolv)
}

func createResolv(stage *Stage) *resolvFixture {
	resolv := new(resolvFixture)
	resolv.onCreate(stage)
	resolv.setShell(resolv)
	return resolv
}

// resolvFixture resolves names.
type resolvFixture struct {
	// Parent
	fixture_
	// States
}

func (f *resolvFixture) onCreate(stage *Stage) {
	f.fixture_.onCreate(signResolv, stage)
}
func (f *resolvFixture) OnShutdown() { close(f.ShutChan) } // notifies run()

func (f *resolvFixture) OnConfigure() {
}
func (f *resolvFixture) OnPrepare() {
}

func (f *resolvFixture) run() { // runner
	f.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Println("resolv done")
	}
	f.stage.DecSub() // resolv
}

func (f *resolvFixture) Register(name string, addresses []string) bool {
	// TODO
	return false
}

func (f *resolvFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
