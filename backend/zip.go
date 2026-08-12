package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractZip 解压 zip 文件到目标目录
func extractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		// 防止 zip 路径穿越
		name := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(f.Name), "./"))
		if name == "." || strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)

		// 目录条目判断：zip 规范用 "/" 结尾，但 Windows 工具（如 PowerShell
		// Compress-Archive）可能用 "\\" 结尾；Go 的 FileInfo().IsDir() 只认 "/"，
		// 否则会把空目录条目当成文件写入，导致后续同目录文件创建失败。
		if strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, "\\") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := ensureDirChain(filepath.Dir(target)); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

// ensureDirChain 逐级确保 dir 存在且为目录。若中间某级被误创建为文件
//（Windows 工具打的 zip 常见问题），先删除该文件再重建为目录。
func ensureDirChain(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		if err := os.Remove(dir); err != nil {
			return err
		}
		return os.MkdirAll(dir, 0o755)
	}
	if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(dir)
	if parent != dir {
		if err := ensureDirChain(parent); err != nil {
			return err
		}
	}
	return os.Mkdir(dir, 0o755)
}

// zipDirectory 将目录打包为 zip 文件
func zipDirectory(srcDir, zipPath string) error {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
}
