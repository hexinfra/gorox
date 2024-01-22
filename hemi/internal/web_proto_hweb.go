// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB protocol elements.

// HWEB is a request/response Web protocol designed for IDC.
// HWEB is under design, the underlying transport protocol is not determined.

package internal

// WARNING: DRAFT DESIGN!

// recordHeader(64) = type(8) exchanID(24) flags(8) bodySize(24)

// initRecord = recordHeader *setting
//   setting(32) = settingCode(8) settingValue(24)

// request  = headRecord *dataRecord [ tailRecord ]
// response = headRecord *dataRecord [ tailRecord ]

//   headRecord = recordHeader 1*field
//   dataRecord = recordHeader *OCTET
//   tailRecord = recordHeader 1*field

//     field = literalField | indexedField | indexedName | indexedValue
//       literalField = 00(2) valueSize(22) nameSize(8) name value
//       indexedField = 11(2) fieldIndex(6)
//       indexedName  = 10(2) nameIndex(6) valueSize(24) value
//       indexedValue = 01(2) valueIndex(6) nameSize(8) name
//         name  = 1*OCTET
//         value = *OCTET

//   sizeRecord = recordHeader windowSize(24)

// On connection established, client sends an init record, then server sends one, too.
// Whenever a client kicks a new request, it must use the least unused exchanID starting from 1.
// An exchanID is considered as unused after a response with this exchanID was received entirely.
// If the number of concurrent exchans exceeds the limit set in init record, server can close the connection.

const ( // record types
	hwebTypeINIT = 0 // contains connection settings
	hwebTypeHEAD = 1 // contains name-value pairs for headers
	hwebTypeDATA = 2 // contains content data
	hwebTypeTAIL = 3 // contains name-value pairs for trailers
	hwebTypeSIZE = 4 // available window size for receiving content
)

const ( // record flags
	hwebFlagEndMessage = 0b00000001 // end of message, used by HEAD, DATA, and TAIL
)

const ( // setting codes
	hwebSettingMaxRecordBodySize    = 0
	hwebSettingInitialWindowSize    = 1
	hwebSettingMaxConnectionExchans = 2
	hwebSettingMaxConcurrentExchans = 3
)

var hwebSettingDefaults = [...]int32{
	hwebSettingMaxRecordBodySize:    16376, // allow: [16376-16777215]
	hwebSettingInitialWindowSize:    16376, // allow: [16376-16777215]
	hwebSettingMaxConnectionExchans: 1000,  // allow: [100-16777215]
	hwebSettingMaxConcurrentExchans: 100,   // allow: [100-16777215]. cannot be larger than maxConnectionExchans
}

/*

on connection established:

    ==> record=INIT exchanID=0 bodySize=8     body=[maxRecordBodySize=16376 initialWindowSize=16376]
    <== record=INIT exchanID=0 bodySize=16    body=[maxRecordBodySize=16376 initialWindowSize=16376 maxConnectionExchans=1000 maxConcurrentExchans=10]

exchan=1 (sized output):

    --> record=HEAD exchanID=1 bodySize=?     body=[:method=GET :target=/hello host=example.com:8081] // endMessage=1

    <-- record=HEAD exchanID=1 bodySize=?     body=[:status=200 content-length=12]
    <-- record=DATA exchanID=1 bodySize=6     body=[hello,]
    <-- record=DATA exchanID=1 bodySize=6     body=[world!] // endMessage=1

exchan=2 (vague output):

    --> record=HEAD exchanID=2 bodySize=?     body=[:method=POST :target=/abc?d=e host=example.com:8081 content-length=90]
    --> record=DATA exchanID=2 bodySize=90    body=[...90...] // endMessage=1

    <-- record=HEAD exchanID=2 bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8]
    <-- record=DATA exchanID=2 bodySize=16376 body=[...16376...]
    ==> record=SIZE exchanID=2 bodySize=3     body=123 // window update
    <-- record=DATA exchanID=2 bodySize=123   body=[...123...]
    ==> record=SIZE exchanID=2 bodySize=3     body=16376 // window update
    <-- record=DATA exchanID=2 bodySize=4567  body=[...4567...]
    <-- record=TAIL exchanID=2 bodySize=?     body=[md5-digest=12345678901234567890123456789012] // endMessage=1

*/
