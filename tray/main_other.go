//go:build !windows

package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	fmt.Println("托盘版目前仅支持 Windows；macOS/Linux 请使用 codex-chinese 命令行版。")
	os.Exit(1)
}
