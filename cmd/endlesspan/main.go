package main

import (
	"endlesspan"

	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() { unitchecker.Main(endlesspan.Analyzer) }
