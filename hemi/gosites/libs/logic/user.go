// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package logic

import (
	. "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
)

type User struct {
	Model
}

/*
// define
type User struct {
	Model
	ID   int64
	Name string
	Pass string
	Age  int64
}
func (u *User) IsAdult() bool { return u.Age >= 18 }

var UserTable = Table("user", User{})


// insert
user := User{Name: "user1", Pass: "ffffff", Age: 80}
id, err := UserTable.Add(&user)

var users []User
id, err := UserTable.AddMany(&users)

id, err := UserTable.Insert("insert into user (name, pass, age) values (?, ?, ?)", "xx", "yy", 33)


// select
var users []User
n, err := UserTable.GetWhere(&users, "name = ?", "user1")
for _, user := range users {
	if user.IsAdult() {
		println(user.Name)
	}
	user.Name = "haha"
	user.Save()
}

var user User
n, err := UserTable.GetOne(&user, "name = ?", "user1")
if n > 0 {
	println(user.IsAdult())
	user.Name = "hehe"
	user.Save()
}
*/
