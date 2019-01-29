package database

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	"github.com/jinzhu/gorm"
)

func TestDB(t *testing.T) {
	ForceSequential <- true
	defer func() {
		<-ForceSequential
	}()
	db, err := NewTestDB()
	if err != nil {
		if strings.Contains(err.Error(), "Failed to open") {
			t.Skipf("skip testing db due to %q", err.Error())
		}
		t.Fatal(err)
	}
	defer db.Shutdown()
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
