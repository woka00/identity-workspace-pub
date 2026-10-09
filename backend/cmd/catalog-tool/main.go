package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"avatar-id/internal/catalogimport"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 || (os.Args[1] != "prepare-openfoodfacts" && os.Args[1] != "prepare-auchan") {
		log.Fatal("usage: catalog-tool <prepare-openfoodfacts|prepare-auchan> -input source -output catalog.jsonl.gz")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	inputPath := flags.String("input", "", "source catalog, optionally gzip compressed")
	outputPath := flags.String("output", "", "prepared JSONL output, optionally gzip compressed")
	includeMissing := flags.Bool("include-missing-nutrition", false, "include Open Food Facts products without any calorie or macro data")
	_ = flags.Parse(os.Args[2:])
	if strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*outputPath) == "" {
		flags.Usage()
		os.Exit(2)
	}

	inputFile, err := os.Open(*inputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer inputFile.Close()
	var input io.Reader = inputFile
	if strings.HasSuffix(strings.ToLower(*inputPath), ".gz") {
		gzipReader, err := gzip.NewReader(inputFile)
		if err != nil {
			log.Fatal(err)
		}
		defer gzipReader.Close()
		input = gzipReader
	}

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		log.Fatal(err)
	}
	temporaryPath := *outputPath + ".partial"
	outputFile, err := os.Create(temporaryPath)
	if err != nil {
		log.Fatal(err)
	}
	var output io.Writer = outputFile
	var gzipWriter *gzip.Writer
	if strings.HasSuffix(strings.ToLower(*outputPath), ".gz") {
		gzipWriter = gzip.NewWriter(outputFile)
		output = gzipWriter
	}
	var count int64
	var prepareErr error
	if os.Args[1] == "prepare-auchan" {
		var stats catalogimport.AuchanPrepareStats
		stats, prepareErr = catalogimport.PrepareAuchan(input, output)
		count = stats.Written
		if prepareErr == nil {
			fmt.Printf("read %d products, skipped %d invalid, merged %d duplicates\n", stats.Read, stats.InvalidSkipped, stats.DuplicatesMerged)
		}
	} else {
		count, prepareErr = catalogimport.PrepareOpenFoodFacts(input, output, *includeMissing)
	}
	if gzipWriter != nil {
		if err := gzipWriter.Close(); prepareErr == nil {
			prepareErr = err
		}
	}
	if err := outputFile.Close(); prepareErr == nil {
		prepareErr = err
	}
	if prepareErr != nil {
		_ = os.Remove(temporaryPath)
		log.Fatal(prepareErr)
	}
	if err := os.Rename(temporaryPath, *outputPath); err != nil {
		log.Fatal(err)
	}
	info, _ := os.Stat(*outputPath)
	fmt.Printf("prepared %d products (%d bytes)\n", count, info.Size())
}
