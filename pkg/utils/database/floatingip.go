/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package database

import (
	"time"
)

/*
var (
	DefaultFloatingipTableName = "ip_pool"
	SecondFloatingipTableName  = "ip_pool1"
)
*/

// select concat((ip>>24)%256,".",(ip>>16)%256,".",(ip>>8)%256,".",ip%256) as ip,`key` from ip_pool
type FloatingIP struct {
	Table     string `gorm:"-"`
	Key       string `gorm:"type:varchar(255)"`
	Subnet    string `gorm:"type:varchar(50)"` // node subnet, not container ip's subnet
	Attr      string `gorm:"type:varchar(1000)"`
	IP        uint32 `gorm:"primary_key;not null"`
	Policy    uint16
	UpdatedAt time.Time
}

/*
func (f FloatingIP) TableName() string {
	if f.Table == "" {
		return DefaultFloatingipTableName
	} else {
		return f.Table
	}
}
*/
