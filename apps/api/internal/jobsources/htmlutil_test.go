package jobsources

import (
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple text",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "text with HTML tags",
			input:    "<p>Hello <strong>World</strong></p>",
			expected: "Hello World",
		},
		{
			name:     "text with links",
			input:    "<a href='https://example.com'>Click here</a>",
			expected: "Click here",
		},
		{
			name:     "text with images",
			input:    "<img src='logo.png' alt='Logo'>",
			expected: "",
		},
		{
			name:     "text with line breaks",
			input:    "<br>Line 1<br>Line 2",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "text with paragraphs",
			input:    "<p>Paragraph 1</p><p>Paragraph 2</p>",
			expected: "Paragraph 1\nParagraph 2",
		},
		{
			name:     "text with lists",
			input:    "<ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: "Item 1\nItem 2",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "only HTML tags",
			input:    "<div><span></span></div>",
			expected: "",
		},
		{
			name:     "text with special characters",
			input:    "&amp; &lt; &gt; &quot;",
			expected: "& < > \"",
		},
		{
			name:     "nested HTML",
			input:    "<div><p><strong>Bold</strong> text</p></div>",
			expected: "Bold text",
		},
		{
			name:     "HTML with attributes",
			input:    "<a href='https://example.com' class='link' id='main'>Link text</a>",
			expected: "Link text",
		},
		{
			name:     "multiple spaces",
			input:    "Hello   World",
			expected: "Hello   World",
		},
		{
			name:     "newlines and tabs",
			input:    "Hello\n\tWorld",
			expected: "Hello\n\tWorld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTMLToText(tt.input)
			if result != tt.expected {
				t.Errorf("for input %q, expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestHTMLToTextWithUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unicode characters",
			input:    "<p>Hello 世界</p>",
			expected: "Hello 世界",
		},
		{
			name:     "emoji",
			input:    "<p>Hello 👋</p>",
			expected: "Hello 👋",
		},
		{
			name:     "accented characters",
			input:    "<p>Café résumé</p>",
			expected: "Café résumé",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTMLToText(tt.input)
			if result != tt.expected {
				t.Errorf("for input %q, expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestHTMLToTextPerformance(t *testing.T) {
	largeHTML := strings.Repeat("<p>This is a paragraph with <strong>bold</strong> text. </p>", 1000)

	result := HTMLToText(largeHTML)

	if len(result) == 0 {
		t.Error("expected non-empty result for large HTML")
	}
}

func TestHTMLToTextEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "self-closing tags",
			input:    "<hr><br/><img src='test.png'/>",
			expected: "",
		},
		{
			name:     "comments",
			input:    "<!-- This is a comment -->Visible text",
			expected: "Visible text",
		},
		{
			name:     "CDATA sections",
			input:    "<![CDATA[This is CDATA]]>Visible text",
			expected: "Visible text",
		},
		{
			name:     "mixed content",
			input:    "<div>Text <span>nested</span> more text</div>",
			expected: "Text nested more text",
		},
		{
			name:     "HTML entities",
			input:    "&amp; &lt; &gt; &quot; &#39;",
			expected: "& < > \" '",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTMLToText(tt.input)
			if result != tt.expected {
				t.Errorf("for input %q, expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestHTMLToTextPreserveWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single space",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "multiple spaces",
			input:    "Hello   World",
			expected: "Hello   World",
		},
		{
			name:     "leading/trailing spaces",
			input:    "  Hello World  ",
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTMLToText(tt.input)
			if result != tt.expected {
				t.Errorf("for input %q, expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestHTMLToTextComplexHTML(t *testing.T) {
	complexHTML := `
	<div class="job-description">
		<h2>Job Description</h2>
		<p>We are looking for a <strong>Senior Go Developer</strong> to join our team.</p>
		<h3>Requirements:</h3>
		<ul>
			<li>5+ years of Go experience</li>
			<li>Experience with distributed systems</li>
			<li>Strong knowledge of SQL and NoSQL databases</li>
		</ul>
		<h3>Benefits:</h3>
		<ul>
			<li>Competitive salary</li>
			<li>Remote work</li>
			<li>Health insurance</li>
		</ul>
		<p>Apply now at <a href="https://example.com/apply">our careers page</a>.</p>
	</div>`

	result := HTMLToText(complexHTML)

	if !strings.Contains(result, "Senior Go Developer") {
		t.Error("result should contain 'Senior Go Developer'")
	}
	if !strings.Contains(result, "5+ years of Go experience") {
		t.Error("result should contain '5+ years of Go experience'")
	}
	if !strings.Contains(result, "Competitive salary") {
		t.Error("result should contain 'Competitive salary'")
	}

	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Error("result should not contain HTML tags")
	}
}
