package salary

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type LevelsFyiLoader struct {
	q *sqlcgen.Queries
}

func NewLevelsFyiLoader(q *sqlcgen.Queries) *LevelsFyiLoader {
	return &LevelsFyiLoader{q: q}
}

func (l *LevelsFyiLoader) LoadCSV(ctx context.Context, csvPath string) ([]SalaryBand, error) {
	if csvPath == "" {
		return nil, nil
	}

	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("levels.fyi: open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("levels.fyi: read header: %w", err)
	}

	colIdx := columnIndex(header)

	var bands []SalaryBand
	rowCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("levels.fyi: skip malformed row", "error", err)
			continue
		}

		band, ok := rowToBand(record, colIdx)
		if !ok {
			continue
		}

		bands = append(bands, band)
		rowCount++

		if rowCount%1000 == 0 {
			slog.Info("levels.fyi: loading", "rows", rowCount)
		}
	}

	slog.Info("levels.fyi: loaded", "rows", rowCount, "bands", len(bands))

	if err := l.upsertBands(ctx, bands); err != nil {
		return bands, fmt.Errorf("levels.fyi: upsert cache: %w", err)
	}

	return bands, nil
}

type csvColumns struct {
	company    int
	title      int
	totalComp  int
	baseSalary int
	location   int
}

func columnIndex(header []string) csvColumns {
	var c csvColumns
	c.company = -1
	c.title = -1
	c.totalComp = -1
	c.baseSalary = -1
	c.location = -1

	for i, h := range header {
		hl := strings.ToLower(strings.TrimSpace(h))
		switch hl {
		case "company":
			c.company = i
		case "title":
			c.title = i
		case "totalyearlycompensation", "totalcompensation":
			c.totalComp = i
		case "basesalary":
			c.baseSalary = i
		case "location":
			c.location = i
		}
	}
	return c
}

func rowToBand(record []string, col csvColumns) (SalaryBand, bool) {
	if col.totalComp < 0 || col.totalComp >= len(record) {
		return SalaryBand{}, false
	}

	totalComp, err := strconv.ParseFloat(strings.TrimSpace(record[col.totalComp]), 64)
	if err != nil || totalComp <= 0 {
		return SalaryBand{}, false
	}

	comp := int(totalComp)

	return SalaryBand{
		Min:        comp,
		Max:        comp,
		Currency:   "USD",
		Confidence: 0.6,
		Source:     SourceLevelsFyi,
	}, true
}

func (l *LevelsFyiLoader) upsertBands(ctx context.Context, bands []SalaryBand) error {
	for _, b := range bands {
		if err := l.q.UpsertSalaryCache(ctx, sqlcgen.UpsertSalaryCacheParams{
			Bucket:     "",
			SalaryMin:  int32Ptr(int32(b.Min)),
			SalaryMax:  int32Ptr(int32(b.Max)),
			Currency:   b.Currency,
			Source:     string(b.Source),
			SampleSize: 1,
		}); err != nil {
			return err
		}
	}
	return nil
}
