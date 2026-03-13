package main

import (
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/log"
)

func main() {
	config.Init("configs/config.yaml")
	cfg := config.Conf
	log.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.OutputPath)
	database.InitMySQL(cfg.Database.MySQL.DSN)

	err := database.DB.AutoMigrate(&model.DocumentVector{})
	if err != nil {
		panic(err)
	}
	println("Migration successful!")
}
