//go:build !windows

package main

// storeCandidatesFromRegistry 仅在 Windows 有意义（MSIX 商店包）。
func storeCandidatesFromRegistry() []string { return nil }
