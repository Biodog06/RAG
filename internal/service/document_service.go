// Package service 包含了应用的业务逻辑层。
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/storage"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"pai-smart-go/pkg/mineru"
)

// FileUploadDTO 是一个数据传输对象，用于在返回给前端时隐藏一些字段并添加额外信息。
type FileUploadDTO struct {
	model.FileUpload
	OrgTagName string `json:"orgTagName"`
}

// DownloadInfoDTO 封装了文件下载链接所需的信息。
type DownloadInfoDTO struct {
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	FileSize    int64  `json:"fileSize"`
}

// PreviewInfoDTO 封装了文件预览所需的信息。
type PreviewInfoDTO struct {
	FileName string `json:"fileName"`
	Content  string `json:"content"`
	FileSize int64  `json:"fileSize"`
}

// DocumentService 接口定义了文档管理相关的业务操作。
type DocumentService interface {
	ListAccessibleFiles(user *model.User) ([]model.FileUpload, error)
	ListUploadedFiles(userID uint) ([]FileUploadDTO, error)
	DeleteDocument(fileMD5 string, user *model.User) error
	GenerateDownloadURL(fileName string, user *model.User) (*DownloadInfoDTO, error)
	GetFilePreviewContent(fileName string, user *model.User) (*PreviewInfoDTO, error)
	GenerateBrowserPreviewURL(fileName string, user *model.User) (string, error)
}

type documentService struct {
	uploadRepo repository.UploadRepository
	userRepo   repository.UserRepository
	orgTagRepo repository.OrgTagRepository // 新增依赖
	minioCfg   config.MinIOConfig
	mineruClient *mineru.Client // 替换 tikaClient
}

// NewDocumentService 创建一个新的 DocumentService 实例。
func NewDocumentService(uploadRepo repository.UploadRepository, userRepo repository.UserRepository, orgTagRepo repository.OrgTagRepository, minioCfg config.MinIOConfig, mineruClient *mineru.Client) DocumentService {
	return &documentService{
		uploadRepo:   uploadRepo,
		userRepo:     userRepo,
		orgTagRepo:   orgTagRepo,
		minioCfg:     minioCfg,
		mineruClient: mineruClient,
	}
}

// ListAccessibleFiles 获取用户可访问的文件列表。
func (s *documentService) ListAccessibleFiles(user *model.User) ([]model.FileUpload, error) {
	orgTags := strings.Split(user.OrgTags, ",")
	return s.uploadRepo.FindAccessibleFiles(user.ID, orgTags)
}

// ListUploadedFiles 获取用户自己上传的文件列表，并附加组织标签名称。
func (s *documentService) ListUploadedFiles(userID uint) ([]FileUploadDTO, error) {
	files, err := s.uploadRepo.FindFilesByUserID(userID)
	if err != nil {
		return nil, err
	}

	dtos, err := s.mapFileUploadsToDTOs(files)
	if err != nil {
		return nil, err
	}

	return dtos, nil
}

// DeleteDocument 删除一个文档。
func (s *documentService) DeleteDocument(fileMD5 string, user *model.User) error {
	record, err := s.uploadRepo.GetFileUploadRecord(fileMD5, user.ID)
	if err != nil {
		return errors.New("文件不存在或不属于该用户")
	}

	if record.UserID != user.ID && user.Role != "ADMIN" {
		return errors.New("没有权限删除此文件")
	}

	objectName := fmt.Sprintf("merged/%s", record.FileName)
	err = storage.MinioClient.RemoveObject(context.Background(), s.minioCfg.BucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		// Log or ignore error, but proceed to delete DB record
	}

	// 从数据库删除记录
	return s.uploadRepo.DeleteFileUploadRecord(fileMD5, record.UserID)
}

// GenerateDownloadURL 生成文件的临时下载链接。
func (s *documentService) GenerateDownloadURL(fileName string, user *model.User) (*DownloadInfoDTO, error) {
	// 这是一个简化的实现，假设文件名是唯一的。生产环境需要更复杂的逻辑。
	files, err := s.ListAccessibleFiles(user)
	if err != nil {
		return nil, err
	}

	var targetFile *model.FileUpload
	for i := range files {
		if files[i].FileName == fileName {
			targetFile = &files[i]
			break
		}
	}

	if targetFile == nil {
		return nil, errors.New("文件不存在或无权访问")
	}

	// 生成预签名的 URL，有效期为1小时
	expiry := time.Hour
	// 统一使用 merged/ 路径，且由 UploadService 维护
	objectName := fmt.Sprintf("merged/%s", targetFile.FileName)
	presignedURL, err := storage.GetPresignedURL(s.minioCfg.BucketName, objectName, expiry, nil)
	if err != nil {
		return nil, err
	}

	return &DownloadInfoDTO{
		FileName:    targetFile.FileName,
		DownloadURL: presignedURL,
		FileSize:    targetFile.TotalSize,
	}, nil
}

// GenerateBrowserPreviewURL 生成支持各浏览器直接打开的预览链接（Content-Disposition: inline）
func (s *documentService) GenerateBrowserPreviewURL(fileName string, user *model.User) (string, error) {
	files, err := s.ListAccessibleFiles(user)
	if err != nil {
		return "", err
	}

	var targetFile *model.FileUpload
	for i := range files {
		if files[i].FileName == fileName {
			targetFile = &files[i]
			break
		}
	}

	if targetFile == nil {
		return "", errors.New("文件不存在或无权访问")
	}

	objectName := fmt.Sprintf("merged/%s", targetFile.FileName)
	return storage.GetBrowserViewURL(s.minioCfg.BucketName, objectName, time.Hour)
}

// GetFilePreviewContent 获取文件的纯文本预览内容。
func (s *documentService) GetFilePreviewContent(fileName string, user *model.User) (*PreviewInfoDTO, error) {
	// 权限检查逻辑与下载类似
	files, err := s.ListAccessibleFiles(user)
	if err != nil {
		return nil, err
	}

	var targetFile *model.FileUpload
	for i := range files {
		if files[i].FileName == fileName {
			targetFile = &files[i]
			break
		}
	}

	if targetFile == nil {
		return nil, errors.New("文件不存在或无权访问")
	}

	// 从 MinIO 获取文件对象
	objectName := fmt.Sprintf("merged/%s", targetFile.FileName)
	object, err := storage.MinioClient.GetObject(context.Background(), s.minioCfg.BucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()

	// 提前读入内存以便处理
	contentBytes, err := io.ReadAll(object)
	if err != nil {
		return nil, err
	}

	// 对于 .md 和 .txt 文件，直接读取原始内容，不经过 MinerU
	if strings.HasSuffix(strings.ToLower(fileName), ".md") || strings.HasSuffix(strings.ToLower(fileName), ".txt") {
		return &PreviewInfoDTO{
			FileName: targetFile.FileName,
			Content:  string(contentBytes),
			FileSize: targetFile.TotalSize,
		}, nil
	}

	// 其他文件类型将文件流发送给 MinerU 进行智能文本提取 (PDF, DOCX等)
	content, err := s.mineruClient.Parse(context.Background(), fileName, contentBytes)
	if err != nil {
		return nil, err
	}

	return &PreviewInfoDTO{
		FileName: targetFile.FileName,
		Content:  content,
		FileSize: targetFile.TotalSize,
	}, nil
}

func (s *documentService) mapFileUploadsToDTOs(files []model.FileUpload) ([]FileUploadDTO, error) {
	if len(files) == 0 {
		return []FileUploadDTO{}, nil
	}

	// To avoid N+1 queries, get all unique org tag IDs first
	tagIDs := make(map[string]struct{})
	for _, file := range files {
		if file.OrgTag != "" {
			tagIDs[file.OrgTag] = struct{}{}
		}
	}

	tagIDList := make([]string, 0, len(tagIDs))
	for id := range tagIDs {
		tagIDList = append(tagIDList, id)
	}

	tags, err := s.orgTagRepo.FindBatchByIDs(tagIDList)
	if err != nil {
		return nil, err
	}

	tagMap := make(map[string]string)
	for _, tag := range tags {
		tagMap[tag.TagID] = tag.Name
	}

	dtos := make([]FileUploadDTO, len(files))
	for i, file := range files {
		dtos[i] = FileUploadDTO{
			FileUpload: file,
			OrgTagName: tagMap[file.OrgTag], // Will be empty string if not found
		}
	}

	return dtos, nil
}
