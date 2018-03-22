package database

var (
	FloatingipTableName = FloatingIP{}.TableName()
)

type FloatingIP struct {
	IP     uint32 `gorm:"primary_key;not null"`
	Key    string `gorm:"type:varchar(255)"`
	Subnet string `gorm:"type:varchar(50)"`
}

func (FloatingIP) TableName() string {
	return "galaxy_floatingip"
}
