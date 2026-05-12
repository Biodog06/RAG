package repository

import (
	"pai-smart-go/internal/model"

	"gorm.io/gorm"
)

// StructuredDataRepository 定义了结构化数据的持久层操作接口
type StructuredDataRepository interface {
	BatchCreate(data []*model.StructuredData) error
	DeleteByFileMD5(fileMD5 string) error
	FindByFileMD5(fileMD5 string) ([]model.StructuredData, error)
}

type structuredDataRepository struct {
	db *gorm.DB
}

func NewStructuredDataRepository(db *gorm.DB) StructuredDataRepository {
	return &structuredDataRepository{db: db}
}

func (r *structuredDataRepository) BatchCreate(data []*model.StructuredData) error {
	if len(data) == 0 {
		return nil
	}
	// 针对 MySQL 的批量插入优化
	return r.db.CreateInBatches(data, 100).Error
}

func (r *structuredDataRepository) DeleteByFileMD5(fileMD5 string) error {
	return r.db.Where("file_md5 = ?", fileMD5).Delete(&model.StructuredData{}).Error
}

func (r *structuredDataRepository) FindByFileMD5(fileMD5 string) ([]model.StructuredData, error) {
	var results []model.StructuredData
	err := r.db.Where("file_md5 = ?", fileMD5).Order("row_index asc").Find(&results).Error
	return results, err
}
