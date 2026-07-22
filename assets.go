package iridiumlogs

import (
	"embed"
	"sync"

	"github.com/iridiumgo/iridium/core/asset"
)

const assetPackage = "iridium-logger"

//go:embed dist/*
var assets embed.FS

var registerAssetsOnce sync.Once

func registerAssets() {
	registerAssetsOnce.Do(func() {
		asset.Register(
			asset.CSS("viewer", assets, "dist/log-viewer.css").Package(assetPackage),
			asset.JS("viewer", assets, "dist/log-viewer.js").Package(assetPackage).Module(),
		)
	})
}
