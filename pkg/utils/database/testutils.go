package database

import (
	"encoding/json"
)

const (
	TestConfig = `
{
  "database": {
    "protocol": "tcp",
    "addr": "localhost:3306",
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

var ForceSequential = make(chan bool, 1)

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
