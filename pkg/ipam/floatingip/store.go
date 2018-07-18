package floatingip

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/jinzhu/gorm"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
)

var (
	ErrNotUpdated = fmt.Errorf("not updated")
)

func (i *ipam) findAll() (floatingips []database.FloatingIP, err error) {
	err = i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Find(&floatingips).Error
	})
	return
}

func (i *ipam) findAvailable(limit int, fip *[]database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Limit(limit).Where("`key` = \"\"").Find(fip).Error
	})
}

func (i *ipam) findByKey(key string, fip *database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		db := tx.Table(i.TableName).Where("`key` = ?", key).Find(fip)
		if db.RecordNotFound() {
			return nil
		}
		return db.Error
	})
}

func (i *ipam) findByPrefix(prefix string, fips *[]database.FloatingIP) error {
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
		ret := tx.Table(tableName).Model(fip).Where("`key` = \"\"").UpdateColumn(`key`, fip.Key)
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	}
}

func (i *ipam) allocateOneInSubnet(key, subnet string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		//update galaxy_floatingip set `key`=? where `key` = "" AND subnet="192.168.0.0/24" limit 1
		ret := tx.Table(i.TableName).Where("`key` = \"\" AND subnet = ?", subnet).Limit(1).UpdateColumn(`key`, key)
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	})
}

func (i *ipam) create(fip *database.FloatingIP) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Create(&fip).Error
	})
}

func (i *ipam) releaseIPs(keys []string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Where("`key` IN (?)", keys).UpdateColumn(`key`, "").Error
	})
}

func (i *ipam) releaseByPrefix(prefix string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Where("substr(`key`, 1, length(?)) = ?", prefix, prefix).UpdateColumn(`key`, "").Error
	})
}

type Result struct {
	Subnet string
}

func (i *ipam) queryByKeyGroupBySubnet(key string) ([]string, error) {
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

func (i *ipam) deleteUnScoped(ips []uint32) (int, error) {
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

func (i *ipam) findKeyOfIP(ip uint32) (database.FloatingIP, error) {
	var fip database.FloatingIP
	return fip, i.store.Transaction(func(tx *gorm.DB) error {
		return tx.Table(i.TableName).Where(fmt.Sprintf("ip=%d", ip)).First(&fip).Error
	})
}

func (i *ipam) updateKey(ip uint32, key string) error {
	return i.store.Transaction(func(tx *gorm.DB) error {
		ret := tx.Table(i.TableName).Where("ip = ? and `key` = \"\"", ip).UpdateColumn(`key`, key)
		if ret.Error != nil {
			return ret.Error
		}
		if ret.RowsAffected != 1 {
			return ErrNotUpdated
		}
		return nil
	})
}
