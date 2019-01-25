package database

var (
	DefaultFloatingipTableName = "galaxy_floatingip"
)

// select concat((ip>>24)%256,".",(ip>>16)%256,".",(ip>>8)%256,".",ip%256) as ip,`key` from galaxy_floatingip
type FloatingIP struct {
	Table  string `gorm:"-"`
	Key    string `gorm:"type:varchar(255)"`
	Subnet string `gorm:"type:varchar(50)"` // node subnet, not container ip's subnet
	Attr   string `gorm:"type:varchar(1000)"`
	IP     uint32 `gorm:"primary_key;not null"`
	Policy uint16
}

type ReleasePolicy uint16

const (
	PodDelete ReleasePolicy = iota
	AppDeleteOrScaleDown
	Never
)

func (f FloatingIP) TableName() string {
	if f.Table == "" {
		return DefaultFloatingipTableName
	} else {
		return f.Table
	}
}
