package database

import "github.com/jinzhu/gorm"

var (
	DefaultFloatingipTableName = "ip_pool"
	SecondFloatingipTableName  = "ip_pool1"
)

// select concat((ip>>24)%256,".",(ip>>16)%256,".",(ip>>8)%256,".",ip%256) as ip,`key` from ip_pool
type FloatingIP struct {
	Table  string `gorm:"-"`
	Key    string `gorm:"type:varchar(255)"`
	Subnet string `gorm:"type:varchar(50)"` // node subnet, not container ip's subnet
	Attr   string `gorm:"type:varchar(1000)"`
	IP     uint32 `gorm:"primary_key;not null"`
	Policy uint16
}

func (f FloatingIP) TableName() string {
	if f.Table == "" {
		return DefaultFloatingipTableName
	} else {
		return f.Table
	}
}

func FloatingIPsByKeyword(recorder *DBRecorder, tableName, keyword string) ([]FloatingIP, error) {
	var fips []FloatingIP
	err := recorder.Transaction(func(tx *gorm.DB) error {
		return tx.Table(tableName).Where("`key` like ?", "%"+keyword+"%").Find(&fips).Error
	})
	return fips, err
}

func FloatingIPsByIPS(recorder *DBRecorder, table string, ips []uint32) ([]FloatingIP, error) {
	var fips []FloatingIP
	if err := recorder.Transaction(func(tx *gorm.DB) error {
		return tx.Table(table).Where("ip in (?)", ips).Find(&fips).Error
	}); err != nil {
		return nil, err
	}
	return fips, nil
}

func ReleaseFloatingIPs(recorder *DBRecorder, fips, secondFips []FloatingIP) error {
	return recorder.Transaction(func(tx *gorm.DB) error {
		for i := range fips {
			ret := tx.Table(DefaultFloatingipTableName).Where("ip = ? and `key` = ?", fips[i].IP, fips[i].Key).UpdateColumns(map[string]interface{}{`key`: "", "policy": 0, "attr": ""})
			if ret.Error != nil {
				return ret.Error
			}
		}
		for i := range secondFips {
			ret := tx.Table(SecondFloatingipTableName).Where("ip = ? and `key` = ?", secondFips[i].IP, secondFips[i].Key).UpdateColumns(map[string]interface{}{`key`: "", "policy": 0, "attr": ""})
			if ret.Error != nil {
				return ret.Error
			}
		}
		return nil
	})
}
