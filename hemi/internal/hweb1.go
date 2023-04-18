// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/1 protocol elements.

// HWEB/1 is a binary HTTP/1.1. Its design makes it easy to implement a server or client.

package internal

//////////////////////////////////////// HWEB/1 protocol elements.

// request  = headRecord *dataRecord [ tailRecord ]
// response = headRecord *dataRecord [ tailRecord ]

//   headRecord = bodySize(64) 1*nameValue
//   tailRecord = bodySize(64) 1*nameValue

//     nameValue = nameSize(8) valueSize(24) name value
//       name  = 1*OCTET
//       value = *OCTET

//   dataRecord = bodySize *OCTET

/*

stream=1 (sized output):

    --> bodySize=?     body=[:method=GET :target=/hello host=example.com:8081]

    <-- bodySize=?     body=[:status=200 content-length=12]
    <-- bodySize=12    body=[hello,world!]

stream=2 (unsized output):

    --> bodySize=?     body=[:method=POST :target=/abc?d=e host=example.com:8081 content-length=90]
    --> bodySize=90    body=[...90...]

    <-- bodySize=?     body=[:status=200 content-type=text/html;charset=utf-8]
    <-- bodySize=16376 body=[...16376...] // chunk
    <-- bodySize=123   body=[...123...] // chunk
    <-- bodySize=4567  body=[...4567...] // chunk
    <-- bodySize=0     body=[] // last chunk
    <-- bodySize=?     body=[] // trailers, MUST exist in chunked mode, MAY be empty

*/
