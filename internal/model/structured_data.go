package model

import "time"

// StructuredData 存储从 Excel/CSV 提取的结构化数据（JSON 格式）
type StructuredData struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5   string    `gorm:"type:varchar(32);index;not null" json:"fileMd5"`
	RowIndex  int       `gorm:"not null" json:"rowIndex"`
	RowData   string    `gorm:"type:json;not null" json:"rowData"` // MySQL JSON 类型
	SheetName string    `gorm:"type:varchar(100)" json:"sheetName"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (StructuredData) TableName() string {
	return "structured_data"
}
