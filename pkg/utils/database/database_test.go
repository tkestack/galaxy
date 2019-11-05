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
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// #lizard forgives
func TestDB(t *testing.T) {
	db := dbInit(t)
	if db == nil {
		return
	}
	defer db.Shutdown()

	fip := FloatingIP{Key: fmt.Sprintf("pod1"), IP: nets.IPToInt(net.IPv4(10, 0, 0, 1))}
	if err := db.GetConn().Debug().Create(&fip).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.GetConn().Debug().Delete(&fip).Error; err != nil {
			t.Fatal(err)
		}
	}()
	var fips []FloatingIP
	if err := db.conn.Find(&fips).Error; err != nil {
		t.Fatal(err)
	}
	if len(fips) != 1 {
		t.Fatalf("%v", fips)
	}
	if fips[0].Key != fip.Key || fips[0].IP != fip.IP {
		t.Fatalf("%v %v", fips[0], fip)
	}
	// test rollback
	createOp := func(ip *FloatingIP) func(tx *gorm.DB) error {
		return func(tx *gorm.DB) error {
			err := tx.Create(&ip).Error
			return err
		}
	}
	if err := db.Transaction(
		createOp(&FloatingIP{Key: "pod2", IP: nets.IPToInt(net.IPv4(10, 0, 0, 2))}),
		createOp(&fip),
	); err == nil {
		t.Fatal(err)
	}
	if err := db.conn.Find(&fips).Error; err != nil {
		t.Fatal(err)
	}
	if len(fips) != 1 {
		t.Fatal()
	}
}

func dbInit(t *testing.T) *DBRecorder {
	db, err := NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
			return nil
		}
		t.Fatal(err)
	}
	if err := db.CreateTableIfNotExist(&FloatingIP{}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTableIfNotExist(&FloatingIP{Table: "second_fip_table"}); err != nil {
		t.Fatal(err)
	} else if !db.GetConn().HasTable("second_fip_table") {
		t.Fatal("table with name specific not create")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf("TRUNCATE %s;", DefaultFloatingipTableName)).Error
	}); err != nil {
		t.Fatal(err)
	}
	return db
}
