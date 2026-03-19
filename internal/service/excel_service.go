package service

import (
	"encoding/json"
	"fmt"
	"io"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"

	"github.com/xuri/excelize/v2"
)

// ExcelService 定义了 Excel 处理的业务逻辑。
type ExcelService interface {
	ProcessExcel(fileReader io.Reader, fileMD5 string) error
}

type excelService struct {
	repo repository.StructuredDataRepository
}

// NewExcelService 创建一个新的 ExcelService 实例。
func NewExcelService(repo repository.StructuredDataRepository) ExcelService {
	return &excelService{repo: repo}
}

// ProcessExcel 解析 Excel 文件并将其行数据以 JSON 格式存储到 MySQL 中。
func (s *excelService) ProcessExcel(fileReader io.Reader, fileMD5 string) error {
	f, err := excelize.OpenReader(fileReader)
	if err != nil {
		return fmt.Errorf("无法通过 excelize 打开读取流: %w", err)
	}
	defer f.Close()

	// 1. 幂等性处理：首先清理掉该文件已有的结构化数据
	if err := s.repo.DeleteByFileMD5(fileMD5); err != nil {
		return fmt.Errorf("清理旧的结构化数据记录失败: %w", err)
	}

	// 2. 遍历所有 Sheet
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			// 如果某个 Sheet 读取失败，记录日志并跳过
			continue
		}

		if len(rows) < 1 {
			continue
		}

		// 3. 提取表头：假设第一行为 header
		header := rows[0]
		var structuredRows []*model.StructuredData

		// 4. 从第二行开始遍历数据
		for i := 1; i < len(rows); i++ {
			rowMap := make(map[string]interface{})
			currentRow := rows[i]

			for j, cell := range currentRow {
				key := fmt.Sprintf("column_%d", j)
				if j < len(header) && header[j] != "" {
					key = header[j]
				}
				rowMap[key] = cell
			}

			// 如果整行都是空的，则不存入
			if len(rowMap) == 0 {
				continue
			}

			rowDataJSON, _ := json.Marshal(rowMap)
			structuredRows = append(structuredRows, &model.StructuredData{
				FileMD5:   fileMD5,
				RowIndex:  i,
				RowData:   string(rowDataJSON),
				SheetName: sheet,
			})
		}

		// 5. 批量存入数据库
		if len(structuredRows) > 0 {
			if err := s.repo.BatchCreate(structuredRows); err != nil {
				return fmt.Errorf("批量保存 sheet [%s] 数据到数据库失败: %w", sheet, err)
			}
		}
	}

	return nil
}
