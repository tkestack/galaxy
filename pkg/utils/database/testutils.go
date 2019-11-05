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
	"encoding/json"
)

const (
	TestConfig = `{
	"database": {
		"protocol": "tcp",
		"addr": "127.0.0.1:3306",
		"username": "root",
		"password": "root",
		"driver": "mysql",
		"name": "test",
		"maxConn": 10000
	},
	"floatingips": [{
		"routableSubnet": "10.49.27.0/24",
		"ips": ["10.49.27.205", "10.49.27.216~10.49.27.218"],
		"subnet": "10.49.27.0/24",
		"gateway": "10.49.27.1",
		"vlan": 2
	}, {
		"routableSubnet": "10.173.13.0/24",
		"ips": ["10.173.13.2", "10.173.13.10~10.173.13.13", "10.173.13.15"],
		"subnet": "10.173.13.0/24",
		"gateway": "10.173.13.1",
		"vlan": 2
	}, {
		"routableSubnet": "10.180.1.2/32",
		"ips": ["10.180.154.2~10.180.154.3"],
		"subnet": "10.180.154.0/24",
		"gateway": "10.180.154.1",
		"vlan": 3
	}, {
		"routableSubnet": "10.180.1.3/32",
		"ips": ["10.180.154.7~10.180.154.8"],
		"subnet": "10.180.154.0/24",
		"gateway": "10.180.154.1",
		"vlan": 3
	}]
}`
)

func NewTestDB() (*DBRecorder, error) {
	var config struct {
		*DBConfig `json:"database,omitempty"`
	}
	if err := json.Unmarshal([]byte(TestConfig), &config); err != nil {
		return nil, err
	}
	db := NewDBRecorder(config.DBConfig)
	if err := db.Run(); err != nil {
		return nil, err
	}
	return db, nil
}
