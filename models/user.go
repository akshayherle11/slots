package models

type User struct {
	ID    int64  `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Name  string `json:"name" gorm:"column:name;type:varchar(255);not null"`
	Email string `json:"email" gorm:"column:email;type:varchar(255);not null;uniqueIndex"`
}

func (User) TableName() string {
	return "users"
}
