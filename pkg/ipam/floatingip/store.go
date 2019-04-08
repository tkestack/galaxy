package floatingip

import (
	"fmt"
	"strconv"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	"github.com/golang/glog"
	"github.com/jinzhu/gorm"
)

var (
	ErrNotUpdated = fmt.Errorf("not updated")
)

func (i *dbIpam) findAll() (floatingips []database.FloatingIP, err error) {
	err = i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Find(&floatingips).Error
	})
	return
}

func (i *dbIpam) findAvailable(limit int, fip *[]database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Limit(limit).Where("`key` = \"\"").Find(fip).Error
	})
}

func (i *dbIpam) findByKey(key string, fip *database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		db := tx.Table(i.TableName).Where("`key` = ?", key).Find(fip)
		if db.RecordNotFound() {
			return nil
		}
		return db.Error
	})
}

func (i *dbIpam) findByPrefix(prefix string, fips *[]database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		db := tx.Table(i.TableName).Where("substr(`key`, 1, length(?)) = ?", prefix, prefix).Find(fips)
		if db.RecordNotFound() {
			return nil
		}
		return db.Error
	})
}

func allocateOp(fip *database.FloatingIP, tableName string) database.ActionFunc {
	return func(tx *gorm.DB) error {
		ret := tx.Table(tableName).Model(fip).Where("`key` = \"\"").UpdateColumn(`key`, fip.Key, `updated_at`, time.Now())
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	}
}

func (i *dbIpam) updateOneInSubnet(oldK, newK, subnet string, policy uint16, attr string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		// UPDATE `ip_pool` SET `key` = 'newK', `policy` = '0', `attr` = ''  WHERE (`key` = "oldK" AND subnet = '10.180.1.3/32') ORDER BY updated_at desc LIMIT 1
		ret := tx.Table(i.Name()).Where("`key` = ? AND subnet = ?", oldK, subnet).
			Order("updated_at desc").Limit(1).
			UpdateColumns(map[string]interface{}{`key`: newK, "policy": policy, "attr": attr, `updated_at`: time.Now()})
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	})
}

func (i *dbIpam) create(fip *database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Create(&fip).Error
	})
}

func (i *dbIpam) releaseIP(key string, ip uint32) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Table(i.Name()).Where("ip = ? AND `key` = ?", ip, key).
			UpdateColumns(map[string]interface{}{`key`: "", "policy": 0, "attr": "", `updated_at`: time.Now()})
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	})
}

func (i *dbIpam) releaseByPrefix(prefix string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Where("substr(`key`, 1, length(?)) = ?", prefix, prefix).
			UpdateColumns(map[string]interface{}{`key`: "", "policy": 0, "attr": "", `updated_at`: time.Now()}).Error
	})
}

type Result struct {
	Subnet string
}

func (i *dbIpam) queryByKeyGroupBySubnet(key string) ([]string, error) {
	var results []Result
	if err := i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Table(i.TableName).Select("subnet").Where("`key` = ?", key).Group("subnet").Order("subnet").Scan(&results)
		if ret.RecordNotFound() {
			return nil
		}
		return ret.Error
	}); err != nil {
		return nil, err
	}
	ret := make([]string, len(results))
	for i := range results {
		ret[i] = results[i].Subnet
	}
	return ret, nil
}

func (i *dbIpam) deleteUnScoped(ips []uint32) (int, error) {
	if glog.V(4) {
		for _, ip := range ips {
			glog.V(4).Infof("will delete unscoped ip: %v", nets.IntToIP(ip))
		}
	}
	var deleted int
	return deleted, i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Exec(fmt.Sprintf("delete from %s where ip IN (?)", i.TableName), ips)
		if ret.Error != nil {
			return ret.Error
		}
		deleted = int(ret.RowsAffected)
		return nil
	})
}

func (i *dbIpam) findByIP(ip uint32) (database.FloatingIP, error) {
	var fip database.FloatingIP
	return fip, i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Where(fmt.Sprintf("ip=%d", ip)).First(&fip).Error
	})
}

func (i *dbIpam) allocateSpecificIP(ip uint32, key string, policy uint16, attr string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Table(i.TableName).Where("ip = ? and `key` = ?", ip, "").
			UpdateColumns(map[string]interface{}{`key`: key, "policy": policy, "attr": attr, `updated_at`: time.Now()})
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	})
}

func (i *dbIpam) updatePolicy(ip uint32, key string, policy uint16, attr string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Table(i.TableName).Where("ip = ? and `key` = ?", ip, key).
			UpdateColumns(map[string]interface{}{"policy": policy, "attr": attr, `updated_at`: time.Now()})
		if ret.Error != nil {
			return ret.Error
		}
		// don't check RowsAffected != 1 as attr and policy may not be changed
		return nil
	})
}

func (i *dbIpam) updateKey(oldK, newK string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.Name()).Where("`key` = ?", oldK).
			UpdateColumns(map[string]interface{}{
				"key":        newK,
				"attr":       "",
				`updated_at`: time.Now(),
			}).Error
	})
}

func (i *dbIpam) getIPsByKeyword(tableName, keyword string) ([]database.FloatingIP, error) {
	var fips []database.FloatingIP
	err := i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(tableName).Where("`key` like ?", "%"+keyword+"%").Find(&fips).Error
	})
	return fips, err
}

func (i *dbIpam) deleteIPs(tableName string, ipToKey map[uint32]string, deleted []string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		for ip, key := range ipToKey {
			ret := tx.Table(tableName).Where("ip = ? and `key` = ?", strconv.FormatUint(uint64(ip), 10), key).UpdateColumns(map[string]interface{}{`key`: "", "policy": 0, "attr": ""})
			if ret.Error != nil {
				return ret.Error
			}
			if ret.RowsAffected == 1 {
				deleted = append(deleted, nets.IntToIP(ip).String())
			}
		}
		return nil
	})
}
