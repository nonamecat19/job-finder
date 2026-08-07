package domain

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := `Name,Email,Company,Role,LinkedIn_URL,Github_Username
Jane Doe,jane@example.com,Acme Inc,Engineering Manager,https://linkedin.com/in/janedoe,janedoe
John Smith, john@example.com , Acme Inc , Staff Engineer,https://linkedin.com/in/johnsmith,jsmith
,skip@example.com,No Name Co,Nobody,,
Only Name,,,,,
`

	contacts, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if len(contacts) != 3 {
		t.Fatalf("ParseCSV() got %d contacts, want 3", len(contacts))
	}

	if contacts[0].Name != "Jane Doe" || contacts[0].Email != "jane@example.com" || contacts[0].Company != "Acme Inc" {
		t.Errorf("row 0 = %+v, unexpected", contacts[0])
	}
	if contacts[1].Name != "John Smith" || contacts[1].Email != "john@example.com" || contacts[1].Company != "Acme Inc" {
		t.Errorf("row 1 = %+v, want trimmed fields", contacts[1])
	}
	if contacts[2].Name != "Only Name" || contacts[2].Email != "" {
		t.Errorf("row 2 = %+v, want name-only row preserved", contacts[2])
	}
}

func TestParseCSV_ColumnsCaseInsensitiveAndReordered(t *testing.T) {
	input := "COMPANY,NAME\nInitech,Peter Gibbons\n"

	contacts, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("ParseCSV() got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Name != "Peter Gibbons" || contacts[0].Company != "Initech" {
		t.Errorf("got %+v, want name/company mapped despite reordered/uppercase header", contacts[0])
	}
}

func TestParseCSV_MissingHeader(t *testing.T) {
	_, err := ParseCSV(strings.NewReader(""))
	if err == nil {
		t.Fatal("ParseCSV() error = nil, want error for empty input")
	}
}

func TestParseCSV_UnknownColumnsIgnored(t *testing.T) {
	input := "Name,Notes\nAda Lovelace,met at a conference\n"

	contacts, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(contacts) != 1 || contacts[0].Name != "Ada Lovelace" {
		t.Fatalf("got %+v, want single Ada Lovelace row", contacts)
	}
	if contacts[0].Company != "" {
		t.Errorf("Company = %q, want empty (no matching column)", contacts[0].Company)
	}
}

func TestParseCSV_ShortRowIgnoresMissingTrailingColumns(t *testing.T) {
	input := "Name,Email,Company\nBob\n"

	contacts, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Name != "Bob" || contacts[0].Email != "" || contacts[0].Company != "" {
		t.Errorf("got %+v, want Bob with empty trailing fields", contacts[0])
	}
}
