package database

import (
	"encoding/json"

	"github.com/jinzhu/gorm"
)

const (
	TestConfig = `
{
  "database": {
    "protocol": "tcp",
    "addr": "localhost:3306",
    "username": "root",
    "password": "123456",
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
  }]
}`
)

var ForceSequential chan bool = make(chan bool, 1)

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
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("TRUNCATE floating_ips;").Error
	}); err != nil {
		return nil, err
	}
	return db, nil
}
