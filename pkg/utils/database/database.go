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
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"
	"unicode"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
)

// DBConfig is the database config
type DBConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Addr     string `json:"addr,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Driver   string `json:"driver,omitempty"`
	DBName   string `json:"name,omitempty"`
	MaxConn  int    `json:"maxConn,omitempty"`
}

var (
	sqlRegexp = regexp.MustCompile(`(\$\d+)|\?`)
)

// DBRecorder keeps connection to database
type DBRecorder struct {
	*DBConfig
	conn   *gorm.DB
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ActionFunc is a func which executes all the sql in a transaction
type ActionFunc func(tx *gorm.DB) error

// NewDBRecorder creates a `DBRecorder` pointer
func NewDBRecorder(config *DBConfig) *DBRecorder {
	glog.V(4).Infof("db config: %v", config)
	if config.Protocol == "" {
		config.Protocol = "tcp"
	}
	return &DBRecorder{
		DBConfig: config,
		stopCh:   make(chan struct{}),
	}
}

// createDBIfNotExist creates database if it is not exist
func (db *DBRecorder) createDBIfNotExist() error {
	dialect := fmt.Sprintf("%s:%s@%s(%s)/mysql?parseTime=true", db.Username, db.Password, db.Protocol, db.Addr)

	glog.Infof("Login database: %s", dialect)
	conn, err := gorm.Open(db.Driver, dialect)
	if err != nil {
		return fmt.Errorf("Failed to open %s with driver %s, error(%v)", db.Addr, db.Driver, err)
	}
	defer conn.Close() // nolint: errcheck
	if err := conn.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` DEFAULT CHARACTER SET utf8 "+
		"DEFAULT COLLATE utf8_general_ci;", db.DBName)).Error; err != nil {
		return fmt.Errorf("Failed to create database %s, error(%v)", db.DBName, err)
	}
	return nil
}

// Run connects to database
func (db *DBRecorder) Run() (err error) {
	if err = db.createDBIfNotExist(); err != nil {
		return
	}
	dialect := fmt.Sprintf("%s:%s@%s(%s)/%s?parseTime=true&loc=Local", db.Username, db.Password, db.Protocol,
		db.Addr, db.DBName)

	glog.Infof("Login database: %s", dialect)
	db.conn, err = gorm.Open(db.Driver, dialect)
	if err != nil {
		err = fmt.Errorf("Failed to open %s with driver %s, error(%v)", db.Addr, db.Driver, err)
		return
	}

	db.conn.SetLogger(new(WrappedLogger))
	if glog.V(5) {
		db.conn.LogMode(true)
	}
	if db.MaxConn > 0 {
		db.conn.DB().SetMaxOpenConns(db.MaxConn)
		idleNum := db.MaxConn >> 3
		if idleNum == 0 {
			idleNum = 10
		}
		db.conn.DB().SetMaxIdleConns(idleNum)
	}

	go wait.Until(func() {
		// Reconnect to database if connection is lost
		var err error
		if err = db.conn.DB().Ping(); err != nil {
			glog.Fatalf("lost database connection reason(%v), reconnecting...", err)
		}
	}, time.Second, db.stopCh)

	return
}

// CreateTableIfNotExist creates table if it is not exist
func (db *DBRecorder) CreateTableIfNotExist(entity interface{}) error {
	if !db.conn.HasTable(entity) {
		if err := db.conn.CreateTable(entity).Error; err != nil {
			err = fmt.Errorf("Failed to create table %v, error(%v)", reflect.TypeOf(entity), err)
			return err
		}
	} else {
		if err := db.conn.AutoMigrate(entity).Error; err != nil {
			err = fmt.Errorf("Failed to auto migrate table %v, error(%v)", reflect.TypeOf(entity), err)
			return err
		}
	}
	return nil
}

// Transaction executes ActionFuncs in a transaction
func (db *DBRecorder) Transaction(ops ...ActionFunc) (err error) {
	db.wg.Add(1)
	defer db.wg.Done()
	tx := db.conn.Begin()
	if err = tx.Error; err != nil {
		glog.Errorf("Begin Tansaction error: %+v", err)
		return
	}

	defer func() {
		if err != nil {
			//FIXME rollback error
			tx.Rollback()
		}
	}()

	for _, op := range ops {
		if op == nil {
			continue
		}

		if err = op(tx); err != nil {
			return
		}
	}

	if err = tx.Commit().Error; err != nil {
		glog.Errorf("Tansaction commit error: %+v", err)
	}
	return
}

// Shutdown shut down connection to database
// nolint: errcheck
func (db *DBRecorder) Shutdown() {
	db.wg.Wait()
	if db.conn != nil {
		select {
		case <-db.stopCh:
		default:
			close(db.stopCh)
		}
		db.conn.Close()
	}
}

// GetConn gets connection
func (db *DBRecorder) GetConn() *gorm.DB {
	return db.conn
}

// WrappedLogger defines the sql log format
type WrappedLogger struct{}

// Print prints sql log
// #lizard forgives
func (l *WrappedLogger) Print(values ...interface{}) {
	if len(values) > 1 {
		level := values[0]
		source := fmt.Sprintf("\033[35m(%v)\033[0m", values[1])
		messages := []interface{}{source}

		if level == "sql" {
			// duration
			messages = append(messages, fmt.Sprintf(" \033[36;1m[%.2fms]\033[0m ",
				float64(values[2].(time.Duration).Nanoseconds()/1e4)/100.0))
			// sql
			var sql string
			var formattedValues []string

			for _, value := range values[4].([]interface{}) {
				indirectValue := reflect.Indirect(reflect.ValueOf(value))
				if indirectValue.IsValid() {
					value = indirectValue.Interface()
					if t, ok := value.(time.Time); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format(time.RFC3339)))
					} else if b, ok := value.([]byte); ok {
						if str := string(b); isPrintable(str) {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
						} else {
							formattedValues = append(formattedValues, "'<binary>'")
						}
					} else if r, ok := value.(driver.Valuer); ok {
						if value, err := r.Value(); err == nil && value != nil {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						} else {
							formattedValues = append(formattedValues, "NULL")
						}
					} else {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
					}
				} else {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
				}
			}

			var formattedValuesLength = len(formattedValues)
			for index, value := range sqlRegexp.Split(values[3].(string), -1) {
				sql += value
				if index < formattedValuesLength {
					sql += formattedValues[index]
				}
			}

			messages = append(messages, sql)
		} else {
			messages = append(messages, "\033[31;1m")
			messages = append(messages, values[2:]...)
			messages = append(messages, "\033[0m")
		}
		glog.Info(messages...)
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
