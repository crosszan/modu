// mmq - CLI tool for Modu Memory & Query
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crosszan/modu/examples/mmq/cmd"
)

var (
	// Version is set during build
	Version = "dev"
	// BuildTime is set during build
	BuildTime = "unknown"
)

func main() {
	// 设置默认数据库路径
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	defaultDBPath := filepath.Join(homeDir, ".cache", "mmq", "index.db")

	// 从环境变量读取配置
	if dbPath := os.Getenv("MMQ_DB"); dbPath != "" {
		defaultDBPath = dbPath
	}

	// 设置全局配置
	cmd.DefaultDBPath = defaultDBPath
	cmd.Version = Version
	cmd.BuildTime = BuildTime

	// 执行根命令
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
