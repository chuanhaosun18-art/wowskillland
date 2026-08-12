package main

import (
	"log"
	"os"
	"path/filepath"
)

// 全局路径配置（数据全部放 D 盘，不占 C 盘）
var (
	// DataDir 数据根目录
	DataDir string
	// ArchiveDir 上传的原始 zip 包目录
	ArchiveDir string
	// FilesDir 解压后的 skill 文件目录
	FilesDir string
	// ProofsDir 评估指标证明图片目录（按 skill id 分子目录存放）
	ProofsDir string
	// DBPath SQLite 数据库文件路径
	DBPath string
)

func init() {
	// 默认 D:\skillhub-data，可用环境变量 SKILLHUB_DATA 覆盖
	base := os.Getenv("SKILLHUB_DATA")
	if base == "" {
		base = `D:\skillhub-data`
	}
	DataDir = base
	ArchiveDir = filepath.Join(base, "archives")
	FilesDir = filepath.Join(base, "files")
	ProofsDir = filepath.Join(base, "proofs")
	DBPath = filepath.Join(base, "skillhub.db")

	for _, dir := range []string{DataDir, ArchiveDir, FilesDir, ProofsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("create data dir %s failed: %v", dir, err)
		}
	}
}
