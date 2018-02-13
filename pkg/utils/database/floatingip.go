package database

type FloatingIP struct {
	IP     uint32 `gorm:"primary_key;not null"`
	Key    string `gorm:"type:varchar(255)"`
	Subnet string `gorm:"type:varchar(50)"`
}
