package database

var (
	DefaultFloatingipTableName = "galaxy_floatingip"
)

// select concat((ip>>24)%256,".",(ip>>16)%256,".",(ip>>8)%256,".",ip%256) as ip,`key` from galaxy_floatingip
type FloatingIP struct {
	Table  string `gorm:"-"`
	IP     uint32 `gorm:"primary_key;not null"`
	Key    string `gorm:"type:varchar(255)"`
	Subnet string `gorm:"type:varchar(50)"` // node subnet, not container ip's subnet
}

func (f FloatingIP) TableName() string {
	if f.Table == "" {
		return DefaultFloatingipTableName
	} else {
		return f.Table
	}
}
