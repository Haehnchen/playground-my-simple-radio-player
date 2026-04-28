package main

import "fmt"

const appAuthor = "Daniel Espendiller"

var (
	buildVersion = "dev"
	buildDate    = "local"
)

func buildInfoText() string {
	return fmt.Sprintf("Version: %s\nBuild date: %s\nAuthor: %s", buildVersion, buildDate, appAuthor)
}
