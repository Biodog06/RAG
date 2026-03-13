package main

import (
	"context"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/pipeline"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/storage"
	"pai-smart-go/pkg/tasks"
	"pai-smart-go/pkg/tika"

	"github.com/minio/minio-go/v7"
)

func main() {
	dirPtr := flag.String("dir", "", "Directory to scan and ingest")
	userPtr := flag.String("user", "admin", "Username to assign documents to")
	publicPtr := flag.Bool("public", true, "Whether documents should be public")
	forcePtr := flag.Bool("force", false, "Force re-indexing even if MD5 already exists")
	flag.Parse()

	if *dirPtr == "" {
		fmt.Println("Usage: go run rag-admin.go -dir <directory_path> [-user admin] [-public true] [-force false]")
		os.Exit(1)
	}

	// 1. Load config
	config.Init("configs/config.yaml")
	cfg := config.Conf

	// 2. Init Infrastructure
	log.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.OutputPath)
	database.InitMySQL(cfg.Database.MySQL.DSN)
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)

	if err := es.InitES(cfg.Elasticsearch); err != nil {
		log.Fatalf("ES init failed: %v", err)
	}
	storage.InitMinIO(cfg.MinIO)

	// 3. Init Repositories
	userRepo := repository.NewUserRepository(database.DB)
	uploadRepo := repository.NewUploadRepository(database.DB, database.RDB)
	docRepo := repository.NewDocumentVectorRepository(database.DB)

	// 4. Get User
	user, err := userRepo.FindByUsername(*userPtr)
	if err != nil {
		log.Fatalf("User %s not found: %v", *userPtr, err)
	}

	// 5. Init Processor
	embeddingClient := embedding.NewClient(cfg.Embedding)
	tikaClient := tika.NewClient(cfg.Tika)
	processor := pipeline.NewProcessor(
		tikaClient,
		embeddingClient,
		cfg.Elasticsearch,
		cfg.MinIO,
		cfg.Embedding,
		uploadRepo,
		docRepo,
	)

	// 6. Scan Directory
	log.Infof("[Sync] Starting sync for directory: %s", *dirPtr)
	count := 0
	err = filepath.Walk(*dirPtr, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Calculate MD5
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		h := md5.New()
		io.Copy(h, file)
		file.Close()
		fileMD5 := fmt.Sprintf("%x", h.Sum(nil))

		// Check if exists
		if !*forcePtr {
			record, err := uploadRepo.GetFileUploadRecord(fileMD5, user.ID)
			if err == nil && record.Status == 1 {
				// Also check ES (optional, but record.Status=1 usually means processed)
				log.Infof("[Sync] Skipping %s (already in database)", info.Name())
				return nil
			}
		}

		log.Infof("[Sync] Ingesting %s...", info.Name())

		// Upload to MinIO (Processor needs it there)
		file, _ = os.Open(path)
		objectName := fmt.Sprintf("merged/%s", info.Name())
		_, err = storage.MinioClient.PutObject(context.Background(), cfg.MinIO.BucketName, objectName, file, info.Size(), minio.PutObjectOptions{})
		file.Close()
		if err != nil {
			log.Errorf("[Sync] Upload to MinIO failed: %v", err)
			return nil
		}

		// Create record in DB if not exists
		record, err := uploadRepo.GetFileUploadRecord(fileMD5, user.ID)
		if err != nil {
			newRecord := &model.FileUpload{
				FileMD5:   fileMD5,
				FileName:  info.Name(),
				TotalSize: info.Size(),
				Status:    0,
				UserID:    user.ID,
				OrgTag:    user.PrimaryOrg,
				IsPublic:  *publicPtr,
			}
			uploadRepo.CreateFileUploadRecord(newRecord)
			record = newRecord
		}

		// Process
		task := tasks.FileProcessingTask{
			FileMD5:  fileMD5,
			FileName: info.Name(),
			UserID:   user.ID,
			OrgTag:   user.PrimaryOrg,
			IsPublic: *publicPtr,
		}

		if err := processor.Process(context.Background(), task); err != nil {
			log.Errorf("[Sync] Failed to process %s: %v", info.Name(), err)
		} else {
			uploadRepo.UpdateFileUploadStatus(record.ID, 1)
			log.Infof("[Sync] Successfully ingested %s", info.Name())
			count++
		}

		return nil
	})

	if err != nil {
		log.Errorf("[Sync] Walk failed: %v", err)
	}

	log.Infof("[Sync] Done. Ingested %d new documents.", count)
}
