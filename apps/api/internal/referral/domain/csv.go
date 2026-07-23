package domain

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

type CSVContact struct {
	Name           string
	Email          string
	Company        string
	Role           string
	LinkedInURL    string
	GitHubUsername string
}

func ParseCSV(r io.Reader) ([]CSVContact, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	// Real-world exports often omit trailing empty columns on some rows;
	// getCol already treats an out-of-range index as "", so accept any
	// row width rather than erroring on a short/long row.
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("referral: read csv header: %w", err)
	}

	colIdx := mapCSVColumns(header)

	var contacts []CSVContact
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("referral: read csv row: %w", err)
		}

		c := CSVContact{
			Name:           getCol(row, colIdx, "name"),
			Email:          getCol(row, colIdx, "email"),
			Company:        getCol(row, colIdx, "company"),
			Role:           getCol(row, colIdx, "role"),
			LinkedInURL:    getCol(row, colIdx, "linkedin_url"),
			GitHubUsername: getCol(row, colIdx, "github_username"),
		}

		if c.Name == "" {
			continue
		}

		contacts = append(contacts, c)
	}

	return contacts, nil
}

func mapCSVColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

func getCol(row []string, colIdx map[string]int, key string) string {
	idx, ok := colIdx[key]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}
